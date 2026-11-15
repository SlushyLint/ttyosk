//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	evdevKeyEventType = 1
	evdevKeyA         = 30
	evdevKeyBitBytes  = 96
	evdevGrab         = 0x40044590
	evdevIOCRead      = 2
	evdevIOCType      = 'E'
	evdevGetName      = 0x06
	evdevGetBits      = 0x20
)

type physicalKeyboardCapture struct {
	files []*os.File
	done  chan struct{}
	once  sync.Once
}

func startPhysicalKeyboardCapture(events chan<- physicalKeyEvent) (*physicalKeyboardCapture, error) {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, fmt.Errorf("find keyboard input devices: %w", err)
	}

	capture := &physicalKeyboardCapture{done: make(chan struct{})}
	fail := func(err error) (*physicalKeyboardCapture, error) {
		_ = capture.Close()
		return nil, err
	}
	for _, path := range paths {
		device, err := os.Open(path)
		if err != nil {
			return fail(fmt.Errorf("open input device %s (keyboard capture needs access to /dev/input/event*): %w", path, err))
		}
		name, err := evdevDeviceName(device)
		if err != nil {
			_ = device.Close()
			return fail(fmt.Errorf("read input device name for %s: %w", path, err))
		}
		if name == "ttyosk virtual keyboard" {
			_ = device.Close()
			continue
		}
		isKeyboard, err := evdevHasKey(device, evdevKeyA)
		if err != nil {
			_ = device.Close()
			return fail(fmt.Errorf("inspect input device %s: %w", path, err))
		}
		if !isKeyboard {
			_ = device.Close()
			continue
		}
		if err := unix.IoctlSetInt(int(device.Fd()), evdevGrab, 1); err != nil {
			_ = device.Close()
			return fail(fmt.Errorf("grab keyboard %s (%s): %w", name, path, err))
		}
		capture.files = append(capture.files, device)
	}
	if len(capture.files) == 0 {
		return fail(errors.New("no physical keyboard found to grab under /dev/input/event*"))
	}
	for _, device := range capture.files {
		go capture.readEvents(device, events)
	}
	return capture, nil
}

func evdevDeviceName(device *os.File) (string, error) {
	name := make([]byte, 256)
	if err := evdevIoctl(int(device.Fd()), evdevReadRequest(evdevGetName, len(name)), name); err != nil {
		return "", err
	}
	return strings.TrimRight(string(name), "\x00"), nil
}

func evdevHasKey(device *os.File, code int) (bool, error) {
	bits := make([]byte, evdevKeyBitBytes)
	request := evdevReadRequest(evdevGetBits+evdevKeyEventType, len(bits))
	if err := evdevIoctl(int(device.Fd()), request, bits); err != nil {
		return false, err
	}
	return bits[code/8]&(1<<uint(code%8)) != 0, nil
}

func evdevReadRequest(number, size int) uintptr {
	return uintptr(evdevIOCRead<<30 | evdevIOCType<<8 | number | size<<16)
}

func evdevIoctl(fd int, request uintptr, data []byte) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(&data[0])))
	runtime.KeepAlive(data)
	if errno != 0 {
		return errno
	}
	return nil
}

func (capture *physicalKeyboardCapture) readEvents(device *os.File, events chan<- physicalKeyEvent) {
	timeSize := binary.Size(unix.Timeval{})
	eventSize := timeSize + 8
	data := make([]byte, eventSize)
	for {
		if _, err := io.ReadFull(device, data); err != nil {
			select {
			case events <- physicalKeyEvent{err: fmt.Errorf("read keyboard %s: %w", device.Name(), err)}:
			case <-capture.done:
			}
			return
		}
		eventType := binary.NativeEndian.Uint16(data[timeSize : timeSize+2])
		if eventType != evdevKeyEventType {
			continue
		}
		event := physicalKeyEvent{
			code:  binary.NativeEndian.Uint16(data[timeSize+2 : timeSize+4]),
			value: int32(binary.NativeEndian.Uint32(data[timeSize+4 : timeSize+8])),
		}
		select {
		case events <- event:
		case <-capture.done:
			return
		}
	}
}

func (capture *physicalKeyboardCapture) Close() error {
	var result error
	capture.once.Do(func() {
		close(capture.done)
		for _, device := range capture.files {
			if err := unix.IoctlSetInt(int(device.Fd()), evdevGrab, 0); err != nil {
				result = errors.Join(result, fmt.Errorf("release keyboard %s: %w", device.Name(), err))
			}
			if err := device.Close(); err != nil {
				result = errors.Join(result, fmt.Errorf("close keyboard %s: %w", device.Name(), err))
			}
		}
	})
	return result
}

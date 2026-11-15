//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	gpmSocketPath = "/dev/gpmctl"
	gpmMove       = 1
	gpmDrag       = 2
	gpmDown       = 4
	gpmUp         = 8
	gpmHard       = 256
	gpmLeftButton = 4
	gpmEventSize  = 28
)

func openGPM(hover bool) (io.ReadWriteCloser, error) {
	console, ok := currentVirtualConsole()
	if !ok {
		return nil, nil
	}

	conn, err := net.Dial("unix", gpmSocketPath)
	if err != nil {
		file, openErr := os.OpenFile(gpmSocketPath, os.O_RDWR, 0)
		if openErr == nil {
			return registerGPM(file, console, hover)
		}
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, nil
		}
		return nil, fmt.Errorf("connect to %s: %w", gpmSocketPath, err)
	}
	return registerGPM(conn, console, hover)
}

func currentVirtualConsole() (int, bool) {
	path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", os.Stdin.Fd()))
	if err != nil {
		return 0, false
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "tty") {
		return 0, false
	}
	console, err := strconv.Atoi(strings.TrimPrefix(name, "tty"))
	return console, err == nil && console > 0
}

func registerGPM(conn io.ReadWriteCloser, console int, hover bool) (io.ReadWriteCloser, error) {
	request := make([]byte, 16)
	eventMask := uint16(gpmDown | gpmUp)
	if hover {
		eventMask |= gpmMove | gpmDrag
	}
	binary.NativeEndian.PutUint16(request[0:2], eventMask)
	binary.NativeEndian.PutUint16(request[2:4], ^uint16(gpmHard))
	binary.NativeEndian.PutUint16(request[4:6], 0)
	binary.NativeEndian.PutUint16(request[6:8], ^uint16(0))
	binary.NativeEndian.PutUint32(request[8:12], uint32(os.Getpid()))
	binary.NativeEndian.PutUint32(request[12:16], uint32(console))
	if err := writeGPMRequest(conn, request); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("register with GPM: %w", err)
	}
	return conn, nil
}

func writeGPMRequest(conn io.Writer, request []byte) error {
	for len(request) > 0 {
		n, err := conn.Write(request)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		request = request[n:]
	}
	return nil
}

func readGPMEvents(conn io.Reader, events chan<- gpmEvent) {
	defer close(events)
	for {
		data := make([]byte, gpmEventSize)
		if _, err := io.ReadFull(conn, data); err != nil {
			events <- gpmEvent{err: err}
			return
		}
		event, ok := decodeGPMEvent(data)
		if !ok {
			events <- gpmEvent{err: fmt.Errorf("invalid event from GPM")}
			return
		}
		events <- event
	}
}

func decodeGPMEvent(data []byte) (gpmEvent, bool) {
	if len(data) != gpmEventSize {
		return gpmEvent{}, false
	}
	buttons := data[0]
	x := int(int16(binary.NativeEndian.Uint16(data[8:10])))
	y := int(int16(binary.NativeEndian.Uint16(data[10:12])))
	eventType := int32(binary.NativeEndian.Uint32(data[12:16]))
	return gpmEvent{
		x:            x,
		y:            y,
		pressed:      eventType&gpmDown != 0 && buttons&gpmLeftButton != 0,
		rightPressed: eventType&gpmDown != 0 && buttons&1 != 0,
		shift:        data[1]&1 != 0,
		drag:         eventType&gpmDrag != 0,
		released:     eventType&gpmUp != 0,
	}, true
}

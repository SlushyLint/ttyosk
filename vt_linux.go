//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	vtGetState = 0x5603
	vtActivate = 0x5606

	physicalKeyLeftCtrl  = 29
	physicalKeyLeftAlt   = 56
	physicalKeyRightCtrl = 97
	physicalKeyRightAlt  = 100
)

type vtState struct {
	active uint16
	signal uint16
	state  uint16
}

type vtFocus struct {
	fd       int
	target   int
	ctrlDown bool
	altDown  bool
}

func startVTOverlay(args []string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("find ttyosk executable: %w", err)
	}
	childArgs := append([]string(nil), args...)
	childArgs = append(childArgs, "-vt-service=true")
	command := exec.Command(executable, childArgs...)
	command.Stdin = nil
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("start VT overlay: %w", err)
	}
	pid := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return 0, fmt.Errorf("release VT overlay process: %w", err)
	}
	return pid, nil
}

func newVTFocus(fd int) (*vtFocus, bool, error) {
	path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return nil, false, fmt.Errorf("identify target terminal: %w", err)
	}
	target, ok := vtNumberFromPath(path)
	if !ok {
		return nil, false, nil
	}
	focus := &vtFocus{fd: fd, target: target}
	if _, err := focus.activeVT(); err != nil {
		return nil, false, fmt.Errorf("query active virtual terminal: %w", err)
	}
	return focus, true, nil
}

func vtNumberFromPath(path string) (int, bool) {
	cleanPath := filepath.Clean(path)
	if filepath.Dir(cleanPath) != "/dev" {
		return 0, false
	}
	name := strings.TrimPrefix(filepath.Base(cleanPath), "tty")
	if name == filepath.Base(cleanPath) {
		return 0, false
	}
	number, err := strconv.Atoi(name)
	if err != nil || number < 1 {
		return 0, false
	}
	return number, true
}

func (focus *vtFocus) activeVT() (int, error) {
	var state vtState
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(focus.fd),
		uintptr(vtGetState),
		uintptr(unsafe.Pointer(&state)),
	)
	runtime.KeepAlive(&state)
	if errno != 0 {
		return 0, errno
	}
	return int(state.active), nil
}

func (focus *vtFocus) isActive() (bool, error) {
	active, err := focus.activeVT()
	if err != nil {
		return false, err
	}
	return active == focus.target, nil
}

func (focus *vtFocus) resetModifiers() {
	focus.ctrlDown = false
	focus.altDown = false
}

func (focus *vtFocus) handleSwitchKey(event physicalKeyEvent) (bool, error) {
	pressed := event.value != 0
	switch event.code {
	case physicalKeyLeftCtrl, physicalKeyRightCtrl:
		focus.ctrlDown = pressed
		return true, nil
	case physicalKeyLeftAlt, physicalKeyRightAlt:
		focus.altDown = pressed
		return true, nil
	}

	target, isFunctionKey := vtFunctionKeyTarget(event.code)
	if !isFunctionKey || !focus.ctrlDown || !focus.altDown {
		return false, nil
	}
	if event.value == 1 {
		if err := unix.IoctlSetInt(focus.fd, vtActivate, target); err != nil {
			return true, fmt.Errorf("switch to tty%d: %w", target, err)
		}
	}
	return true, nil
}

func vtFunctionKeyTarget(code uint16) (int, bool) {
	if code >= 59 && code <= 68 {
		return int(code - 58), true
	}
	switch code {
	case 87:
		return 11, true
	case 88:
		return 12, true
	default:
		return 0, false
	}
}

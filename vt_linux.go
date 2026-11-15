//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func startVTOverlay(args []string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("find ttyosk executable: %w", err)
	}
	childArgs := append([]string(nil), args...)
	childArgs = append(childArgs, "-vt-child=true")
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

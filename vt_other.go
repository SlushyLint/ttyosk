//go:build !linux

package main

import "errors"

func startVTOverlay([]string) (int, error) {
	return 0, errors.New("VT mode is only supported on Linux")
}

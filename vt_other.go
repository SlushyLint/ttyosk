//go:build !linux

package main

import "errors"

type vtFocus struct{}

func startVTOverlay([]string) (int, error) {
	return 0, errors.New("VT mode is only supported on Linux")
}

func newVTFocus(int) (*vtFocus, bool, error) { return nil, false, nil }

func (*vtFocus) isActive() (bool, error) { return true, nil }

func (*vtFocus) resetModifiers() {}

func (*vtFocus) handleSwitchKey(physicalKeyEvent) (bool, error) { return false, nil }

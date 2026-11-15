//go:build !linux

package main

import (
	"errors"
	"os"
)

type physicalKeyboardCapture struct {
	files []*os.File
}

func startPhysicalKeyboardCapture(chan<- physicalKeyEvent, bool) (*physicalKeyboardCapture, error) {
	return nil, errors.New("physical keyboard capture is only supported on Linux")
}

func (*physicalKeyboardCapture) Close() error { return nil }

func (*physicalKeyboardCapture) SetGrabbed(bool) error { return nil }

//go:build !linux

package main

import "errors"

type physicalKeyboardCapture struct{}

func startPhysicalKeyboardCapture(chan<- physicalKeyEvent) (*physicalKeyboardCapture, error) {
	return nil, errors.New("physical keyboard capture is only supported on Linux")
}

func (*physicalKeyboardCapture) Close() error { return nil }

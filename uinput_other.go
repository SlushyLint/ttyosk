//go:build !linux

package main

import "errors"

func newVirtualKeyboard() (keyboardEmitter, error) {
	return nil, errors.New("keyboard.output uinput is only supported on Linux")
}

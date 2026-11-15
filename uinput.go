package main

type keyboardEmitter interface {
	EmitKey(key string, modifiers uint8) error
	TypeText(text string) error
	Close() error
}

const (
	uinputShift uint8 = 1 << iota
	uinputCtrl
	uinputAlt
)

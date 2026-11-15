//go:build linux

package main

import (
	"encoding/binary"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUinputKeyCodeMapsKeyboardKeys(t *testing.T) {
	tests := []struct {
		key       string
		code      uint16
		modifiers uint8
	}{
		{key: "q", code: 16},
		{key: "A", code: 30, modifiers: uinputShift},
		{key: "?", code: 53, modifiers: uinputShift},
		{key: "F12", code: 88},
		{key: "kp7", code: 71},
		{key: "caps", code: 58},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			code, modifiers, ok := keyCode(test.key)
			if !ok || code != test.code || modifiers != test.modifiers {
				t.Fatalf("keyCode(%q) = (%d, %d, %t), want (%d, %d, true)",
					test.key, code, modifiers, ok, test.code, test.modifiers)
			}
		})
	}
}

func TestUinputEmitsSynchronizedModifierAndKeyEvents(t *testing.T) {
	device, err := os.CreateTemp(t.TempDir(), "uinput-events")
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	keyboard := &linuxKeyboard{device: device}

	if err := keyboard.EmitKey("A", uinputCtrl); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(device.Name())
	if err != nil {
		t.Fatal(err)
	}

	timeSize := binary.Size(unix.Timeval{})
	eventSize := timeSize + 8
	want := [][3]uint16{
		{evKey, 42, 1}, {evKey, 29, 1}, {evSyn, synReport, 0},
		{evKey, 30, 1}, {evSyn, synReport, 0},
		{evKey, 30, 0}, {evKey, 29, 0}, {evKey, 42, 0}, {evSyn, synReport, 0},
	}
	if len(data) != len(want)*eventSize {
		t.Fatalf("expected %d event bytes, got %d", len(want)*eventSize, len(data))
	}
	for i, expected := range want {
		event := data[i*eventSize : (i+1)*eventSize]
		got := [3]uint16{
			binary.NativeEndian.Uint16(event[timeSize : timeSize+2]),
			binary.NativeEndian.Uint16(event[timeSize+2 : timeSize+4]),
			uint16(binary.NativeEndian.Uint32(event[timeSize+4 : timeSize+8])),
		}
		if got != expected {
			t.Errorf("event %d = %v, want %v", i, got, expected)
		}
	}

}

func TestEvdevKeyboardDetectionExcludesGamepads(t *testing.T) {
	keyboardBits := make([]byte, evdevKeyBitBytes)
	for _, code := range []int{28, 29, 30, 42, 44, 57} {
		keyboardBits[code/8] |= 1 << uint(code%8)
	}
	if !evdevKeyboardBits(keyboardBits) {
		t.Fatal("expected a standard keyboard to be detected")
	}

	gamepadBits := append([]byte(nil), keyboardBits...)
	gamepadBits[evdevBtnGamepad/8] |= 1 << uint(evdevBtnGamepad%8)
	if evdevKeyboardBits(gamepadBits) {
		t.Fatal("expected a gamepad with keyboard-like keys to be excluded")
	}
}

//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const (
	uinputSetEventBit   = 0x40045564
	uinputSetKeyBit     = 0x40045565
	uinputDeviceCreate  = 0x5501
	uinputDeviceDestroy = 0x5502

	evSyn     = 0
	evKey     = 1
	synReport = 0
	keyMax    = 0x2ff
)

type uinputUserDevice struct {
	Name       [80]byte
	BusType    uint16
	Vendor     uint16
	Product    uint16
	Version    uint16
	EffectsMax uint32
	AbsMax     [64]int32
	AbsMin     [64]int32
	AbsFuzz    [64]int32
	AbsFlat    [64]int32
}

type uinputEvent struct {
	Time  unix.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type linuxKeyboard struct {
	device  *os.File
	created bool
}

func newVirtualKeyboard() (keyboardEmitter, error) {
	device, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/uinput: %w", err)
	}
	keyboard := &linuxKeyboard{device: device}
	fail := func(err error) (keyboardEmitter, error) {
		_ = keyboard.Close()
		return nil, err
	}

	if err := unix.IoctlSetInt(int(device.Fd()), uinputSetEventBit, evSyn); err != nil {
		return fail(fmt.Errorf("enable uinput synchronization events: %w", err))
	}
	if err := unix.IoctlSetInt(int(device.Fd()), uinputSetEventBit, evKey); err != nil {
		return fail(fmt.Errorf("enable uinput keyboard events: %w", err))
	}
	for code := 1; code <= keyMax; code++ {
		if err := unix.IoctlSetInt(int(device.Fd()), uinputSetKeyBit, code); err != nil {
			return fail(fmt.Errorf("enable uinput key code %d: %w", code, err))
		}
	}

	setup := uinputUserDevice{}
	copy(setup.Name[:], "ttyosk virtual keyboard")
	setup.BusType = 0x03
	setup.Vendor = 0x1
	setup.Product = 0x1
	setup.Version = 1
	if err := binary.Write(device, binary.NativeEndian, setup); err != nil {
		return fail(fmt.Errorf("configure uinput keyboard: %w", err))
	}
	if err := unix.IoctlSetInt(int(device.Fd()), uinputDeviceCreate, 0); err != nil {
		return fail(fmt.Errorf("create uinput keyboard: %w", err))
	}
	keyboard.created = true
	return keyboard, nil
}

func (k *linuxKeyboard) EmitKey(key string, modifiers uint8) error {
	code, implicitModifiers, ok := keyCode(key)
	if !ok {
		return fmt.Errorf("unsupported key %q for uinput output", key)
	}
	modifiers |= implicitModifiers
	var pressed []uint16
	if modifiers&uinputShift != 0 {
		pressed = append(pressed, 42)
	}
	if modifiers&uinputCtrl != 0 {
		pressed = append(pressed, 29)
	}
	if modifiers&uinputAlt != 0 {
		pressed = append(pressed, 56)
	}
	for _, modifier := range pressed {
		if err := k.writeEvent(evKey, modifier, 1); err != nil {
			return err
		}
	}
	if err := k.writeEvent(evSyn, synReport, 0); err != nil {
		return err
	}
	if err := k.writeEvent(evKey, code, 1); err != nil {
		return err
	}
	if err := k.writeEvent(evSyn, synReport, 0); err != nil {
		return err
	}
	if err := k.writeEvent(evKey, code, 0); err != nil {
		return err
	}
	for index := len(pressed) - 1; index >= 0; index-- {
		if err := k.writeEvent(evKey, pressed[index], 0); err != nil {
			return err
		}
	}
	return k.writeEvent(evSyn, synReport, 0)
}

func (k *linuxKeyboard) TypeText(text string) error {
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if r == utf8.RuneError && size == 1 {
			return errors.New("invalid UTF-8 in text for uinput output")
		}
		if err := k.emitRune(r, 0); err != nil {
			return err
		}
		text = text[size:]
	}
	return nil
}

func (k *linuxKeyboard) emitRune(r rune, modifiers uint8) error {
	switch r {
	case '\r', '\n':
		return k.EmitKey("return", modifiers)
	case '\t':
		return k.EmitKey("tab", modifiers)
	case '\b', '\x7f':
		return k.EmitKey("del", modifiers)
	case 0:
		return k.EmitKey("SPACE", modifiers|uinputCtrl)
	case 27:
		return k.EmitKey("esc", modifiers)
	}
	if r > 0 && r < 27 {
		return k.EmitKey(string(r+'a'-1), modifiers|uinputCtrl)
	}
	if r >= 28 && r < 32 {
		return k.EmitKey(string(r+'@'), modifiers|uinputCtrl)
	}
	if r == 32 {
		return k.EmitKey("SPACE", modifiers)
	}
	return k.EmitKey(string(r), modifiers)
}

func (k *linuxKeyboard) writeEvent(eventType, code uint16, value int32) error {
	event := uinputEvent{
		Time:  unix.Timeval{},
		Type:  eventType,
		Code:  code,
		Value: value,
	}
	data := make([]byte, 0, binary.Size(event.Time)+8)
	if binary.Size(event.Time.Sec) == 8 {
		data = binary.NativeEndian.AppendUint64(data, uint64(event.Time.Sec))
		data = binary.NativeEndian.AppendUint64(data, uint64(event.Time.Usec))
	} else {
		data = binary.NativeEndian.AppendUint32(data, uint32(event.Time.Sec))
		data = binary.NativeEndian.AppendUint32(data, uint32(event.Time.Usec))
	}
	data = binary.NativeEndian.AppendUint16(data, event.Type)
	data = binary.NativeEndian.AppendUint16(data, event.Code)
	data = binary.NativeEndian.AppendUint32(data, uint32(event.Value))
	_, err := writeAll(k.device, data)
	return err
}

func writeAll(writer io.Writer, data []byte) (int, error) {
	total := 0
	for len(data) > 0 {
		n, err := writer.Write(data)
		total += n
		data = data[n:]
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

func (k *linuxKeyboard) Close() error {
	if k.device == nil {
		return nil
	}
	var result error
	if k.created {
		if err := unix.IoctlSetInt(int(k.device.Fd()), uinputDeviceDestroy, 0); err != nil {
			result = fmt.Errorf("destroy uinput keyboard: %w", err)
		}
		k.created = false
	}
	if err := k.device.Close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close uinput keyboard: %w", err))
	}
	k.device = nil
	return result
}

func keyCode(key string) (uint16, uint8, bool) {
	if key == "SPACE" {
		return 57, 0, true
	}
	if len(key) == 1 {
		if code, ok := usKeyCodes[strings.ToLower(key)]; ok {
			modifiers := uint8(0)
			if key != strings.ToLower(key) {
				modifiers |= uinputShift
			}
			if shifted, ok := shiftedKeyCodes[key]; ok {
				return shifted, uinputShift, true
			}
			return code, modifiers, true
		}
		if code, ok := shiftedKeyCodes[key]; ok {
			return code, uinputShift, true
		}
	}
	if code, ok := usKeyCodes[key]; ok {
		return code, 0, true
	}
	if code, ok := specialKeyCodes[key]; ok {
		return code, 0, true
	}
	return 0, 0, false
}

var usKeyCodes = map[string]uint16{
	"esc": 1, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6, "6": 7, "7": 8, "8": 9, "9": 10, "0": 11,
	"-": 12, "=": 13, "del": 14, "tab": 15,
	"q": 16, "w": 17, "e": 18, "r": 19, "t": 20, "y": 21, "u": 22, "i": 23, "o": 24, "p": 25,
	"[": 26, "]": 27, "return": 28,
	"a": 30, "s": 31, "d": 32, "f": 33, "g": 34, "h": 35, "j": 36, "k": 37, "l": 38,
	";": 39, "'": 40, "`": 41, "\\": 43,
	"z": 44, "x": 45, "c": 46, "v": 47, "b": 48, "n": 49, "m": 50,
	",": 51, ".": 52, "/": 53,
	"ins": 110, "delete": 111, "home": 102, "end": 107, "pgup": 104, "pgdn": 109,
	"left": 105, "down": 108, "up": 103, "right": 106,
	"num": 69, "numwide": 69, "caps": 58,
	"enter": 96, "F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68, "F11": 87, "F12": 88,
	"kp0": 82, "kp0wide": 82, "kp1": 79, "kp2": 80, "kp3": 81, "kp4": 75,
	"kp5": 76, "kp6": 77, "kp7": 71, "kp8": 72, "kp9": 73, "kp.": 83,
	"kp-": 74, "kp*": 55, "kp/": 98, "kp+": 78,
}

var shiftedKeyCodes = map[string]uint16{
	"!": 2, "@": 3, "#": 4, "$": 5, "%": 6, "^": 7, "&": 8, "*": 9, "(": 10, ")": 11,
	"_": 12, "+": 13, "{": 26, "}": 27, "|": 43, ":": 39, "\"": 40, "~": 41,
	"<": 51, ">": 52, "?": 53,
}

var specialKeyCodes = map[string]uint16{
	"SPACE": 57, "esc": 1, "tab": 15, "del": 14, "return": 28, "rtrn": 28,
	"enter": 96, "ins": 110, "delete": 111, "home": 102, "end": 107,
	"pgup": 104, "pgdn": 109, "num": 69, "numwide": 69, "caps": 58,
	"←": 105, "left": 105, "→": 106, "right": 106,
	"↑": 103, "up": 103, "↓": 108, "down": 108,
	"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68, "F11": 87, "F12": 88,
}

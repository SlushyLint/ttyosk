//go:build linux

package main

import (
	"encoding/binary"
	"testing"
)

func joystickTestEvent(eventType, number byte, value int16) []byte {
	data := make([]byte, 8)
	binary.NativeEndian.PutUint16(data[4:6], uint16(value))
	data[6] = eventType
	data[7] = number
	return data
}

func TestJoystickDecoderMapsAxesAndButtons(t *testing.T) {
	var decoder joystickDecoder
	tests := []struct {
		name      string
		data      []byte
		want      gamepadEvent
		wantEvent bool
	}{
		{name: "left", data: joystickTestEvent(jsEventAxis, 0, -20000), want: gamepadEvent{dx: -1}, wantEvent: true},
		{name: "held left does not repeat", data: joystickTestEvent(jsEventAxis, 0, -30000)},
		{name: "center", data: joystickTestEvent(jsEventAxis, 0, 0)},
		{name: "right after center", data: joystickTestEvent(jsEventAxis, 0, 20000), want: gamepadEvent{dx: 1}, wantEvent: true},
		{name: "hat up", data: joystickTestEvent(jsEventAxis, 7, -32767), want: gamepadEvent{dy: -1}, wantEvent: true},
		{name: "select", data: joystickTestEvent(jsEventButton, 0, 1), want: gamepadEvent{selectKey: true}, wantEvent: true},
		{name: "select release", data: joystickTestEvent(jsEventButton, 0, 0)},
		{name: "backspace", data: joystickTestEvent(jsEventButton, 1, 1), want: gamepadEvent{backspace: true}, wantEvent: true},
		{name: "dpad right", data: joystickTestEvent(jsEventButton, 15, 1), want: gamepadEvent{dx: 1}, wantEvent: true},
		{name: "initialization events ignored", data: joystickTestEvent(jsEventAxis|jsEventInit, 1, -32767)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := decoder.decode(test.data)
			if ok != test.wantEvent || got != test.want {
				t.Fatalf("decode() = (%+v, %t), want (%+v, %t)", got, ok, test.want, test.wantEvent)
			}
		})
	}
}

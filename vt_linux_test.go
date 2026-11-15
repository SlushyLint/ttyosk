//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestVTNumberFromPath(t *testing.T) {
	tests := []struct {
		path string
		want int
		ok   bool
	}{
		{path: "/dev/tty2", want: 2, ok: true},
		{path: "/dev/tty12", want: 12, ok: true},
		{path: "/dev/pts/2"},
		{path: "/dev/tty"},
		{path: "/dev/tty0"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, ok := vtNumberFromPath(test.path)
			if got != test.want || ok != test.ok {
				t.Fatalf("vtNumberFromPath(%q) = (%d, %t), want (%d, %t)", test.path, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestVTFunctionKeyTarget(t *testing.T) {
	tests := []struct {
		code uint16
		want int
		ok   bool
	}{
		{code: 59, want: 1, ok: true},
		{code: 68, want: 10, ok: true},
		{code: 87, want: 11, ok: true},
		{code: 88, want: 12, ok: true},
		{code: 35},
		{code: 69},
	}

	for _, test := range tests {
		got, ok := vtFunctionKeyTarget(test.code)
		if got != test.want || ok != test.ok {
			t.Errorf("vtFunctionKeyTarget(%d) = (%d, %t), want (%d, %t)", test.code, got, ok, test.want, test.ok)
		}
	}
}

func TestVTFocusHandlesCtrlAltFunctionKey(t *testing.T) {
	focus := vtFocus{fd: -1}
	for _, event := range []physicalKeyEvent{
		{code: physicalKeyLeftCtrl, value: 1},
		{code: physicalKeyLeftAlt, value: 1},
	} {
		handled, err := focus.handleSwitchKey(event)
		if !handled || err != nil {
			t.Fatalf("handleSwitchKey(%+v) = (%t, %v), want (true, nil)", event, handled, err)
		}
	}

	handled, err := focus.handleSwitchKey(physicalKeyEvent{code: 61, value: 1})
	if !handled || err == nil || !strings.Contains(err.Error(), "switch to tty3") {
		t.Fatalf("Ctrl+Alt+F3 = (%t, %v), want handled switch to tty3", handled, err)
	}

	focus.resetModifiers()
	handled, err = focus.handleSwitchKey(physicalKeyEvent{code: 61, value: 1})
	if handled || err != nil {
		t.Fatalf("unmodified F3 = (%t, %v), want (false, nil)", handled, err)
	}
}

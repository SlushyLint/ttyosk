package main

import (
	"bytes"
	"testing"

	"github.com/hinshun/vt10x"
)

func newTestKeyboardState() (keyboardState, *bytes.Buffer) {
	shell := &bytes.Buffer{}
	return keyboardState{shell: shell}, shell
}

func assertShellInput(t *testing.T, shell *bytes.Buffer, expected string) {
	t.Helper()

	if actual := shell.String(); actual != expected {
		t.Fatalf("expected shell input %q, got %q", expected, actual)
	}
	shell.Reset()
}

func TestNavigationAndTextInput(t *testing.T) {
	state, shell := newTestKeyboardState()

	state.applyInput([]byte{27, '[', 'C'}, 3)
	if state.x != 1 {
		t.Fatalf("expected x to move right to 1, got %d", state.x)
	}

	state.applyInput([]byte{27, '[', 'B'}, 3)
	if state.y != 1 {
		t.Fatalf("expected y to move down to 1, got %d", state.y)
	}

	state.x = 2
	state.y = 4
	state.applyInput([]byte{' '}, 1)
	assertShellInput(t, shell, " ")
}

func TestModifierModes(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x = 10
	state.y = 3
	state.insertSelectedKey()
	if state.modifier != "shift" {
		t.Fatalf("expected shift mode to activate, got %q", state.modifier)
	}

	state.x = 1
	state.y = 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "Q")

	state, shell = newTestKeyboardState()
	state.x = 1
	state.y = 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "`")

	state, shell = newTestKeyboardState()
	state.modifier = "shift"
	state.x = 1
	state.y = 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "~")
}

func TestVerticalNavigationTracksNearestKeyCenter(t *testing.T) {
	state := keyboardState{x: 4, y: 3}
	state.move(0, 1)
	if state.y != 4 {
		t.Fatalf("expected y to move to last row, got %d", state.y)
	}
	if state.x != 2 {
		t.Fatalf("expected vertical movement to select the nearest centered key at x=2, got %d", state.x)
	}
}

func TestVerticalNavigationChoosesNearestKeyCenter(t *testing.T) {
	tests := []struct {
		name string
		x    int
		want int
	}{
		{name: "left ctrl to z", x: 0, want: 0},
		{name: "left alt to c", x: 1, want: 2},
		{name: "space to v on equal-distance tie", x: 2, want: 3},
		{name: "right alt to n", x: 3, want: 5},
		{name: "right ctrl to comma", x: 4, want: 7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := keyboardState{x: test.x, y: 4, terminalCols: 80}
			state.move(0, -1)
			if state.y != 3 || state.x != test.want {
				t.Fatalf("expected nearest key at row 3, column %d; got row %d, column %d", test.want, state.y, state.x)
			}
		})
	}
}

func TestDefaultLettersAreLowercase(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x = 1
	state.y = 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "q")
}

func TestVirtualKeyboardTypesAndReturnSubmits(t *testing.T) {
	state, shell := newTestKeyboardState()
	for _, position := range [][2]int{{3, 1}, {2, 3}, {5, 2}, {9, 1}} {
		state.x, state.y = position[0], position[1]
		state.applyInput([]byte{' '}, 1)
	}
	assertShellInput(t, shell, "echo")

	state.x, state.y = 11, 2
	state.applyInput([]byte{' '}, 1)
	assertShellInput(t, shell, "\r")
}

func TestMouseClickActivatesVirtualReturn(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 11, y: 18, row: 2}}

	event := []byte("\033[<0;23;18M")
	state.applyInput(event, len(event))

	assertShellInput(t, shell, "\r")
}

func TestVirtualSpecialKeysAndModifiers(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.modifier = "shift"
	state.x, state.y = 11, 2
	state.insertSelectedKey()
	assertShellInput(t, shell, "\r")

	state.x, state.y = 0, 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b")

	state.modifier = "ctrl"
	state.x, state.y = 2, 3
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x03")

	state.modifier = ""
	state.x, state.y = 7, 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[A")
}

func TestTerminalProcessesCursorAndEraseSequences(t *testing.T) {
	terminal := vt10x.New(vt10x.WithSize(8, 2))
	_, _ = terminal.Write([]byte("hello\033[1;3H!\033[2J\033[HOK"))

	terminal.Lock()
	defer terminal.Unlock()
	if got := terminal.Cell(0, 0).Char; got != 'O' {
		t.Fatalf("expected cursor reposition and erase to render O at first cell, got %q", got)
	}
	if got := terminal.Cell(1, 0).Char; got != 'K' {
		t.Fatalf("expected K after O, got %q", got)
	}
	if got := terminal.Cell(2, 0).Char; got != ' ' {
		t.Fatalf("expected erased cell to be blank, got %q", got)
	}
}

func TestPhysicalTypingOnlyNavigatesOrActivates(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.applyInput([]byte("x"), 1)
	assertShellInput(t, shell, "")

	state.applyInput([]byte{'l'}, 1)
	state.applyInput([]byte{'\r'}, 1)
	assertShellInput(t, shell, "`")
}

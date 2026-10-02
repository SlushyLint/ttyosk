package main

import (
	"bytes"
	"strings"
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

	state.x = 1
	state.y = 4
	state.applyInput([]byte{' '}, 1)
	assertShellInput(t, shell, " ")
}

func TestModifierModes(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x = 0
	state.y = 3
	state.insertSelectedKey()
	if !state.shiftOnce {
		t.Fatal("expected one-shot shift to activate")
	}

	state.x = 1
	state.y = 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "Q")
	if state.shiftOnce {
		t.Fatal("expected shift to clear after the next character")
	}

	state, shell = newTestKeyboardState()
	state.x = 1
	state.y = 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "`")

	state, shell = newTestKeyboardState()
	state.shiftOnce = true
	state.x = 1
	state.y = 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "~")
}

func TestCapsLockTogglesLetterCase(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x, state.y = 0, 2
	state.insertSelectedKey()
	if !state.capsLock {
		t.Fatal("expected caps lock to turn on")
	}

	state.x = 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "A")
	if !state.capsLock {
		t.Fatal("expected caps lock to remain on after typing")
	}

	state.x, state.y = 2, 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "1")

	state.x, state.y = 0, 3
	state.insertSelectedKey()
	state.x, state.y = 1, 2
	state.insertSelectedKey()
	assertShellInput(t, shell, "a")
	if !state.capsLock || state.shiftOnce {
		t.Fatal("expected one-shot shift to type lowercase once while caps lock stays on")
	}

	state.x, state.y = 0, 2
	state.insertSelectedKey()
	if state.capsLock {
		t.Fatal("expected caps lock to turn off")
	}
}

func TestVerticalNavigationTracksNearestKeyCenter(t *testing.T) {
	state := keyboardState{x: 4, y: 3}
	state.move(0, 1)
	if state.y != 4 {
		t.Fatalf("expected y to move to last row, got %d", state.y)
	}
	if state.x != 1 {
		t.Fatalf("expected vertical movement to select the nearest centered key at x=1, got %d", state.x)
	}
}

func TestVerticalNavigationChoosesNearestKeyCenter(t *testing.T) {
	tests := []struct {
		name string
		x    int
		want int
	}{
		{name: "ctrl to x", x: 0, want: 2},
		{name: "space to c", x: 1, want: 3},
		{name: "meta to b", x: 2, want: 5},
		{name: "left arrow to m", x: 3, want: 7},
		{name: "down arrow to comma", x: 4, want: 8},
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
	for _, position := range [][2]int{{3, 1}, {3, 3}, {6, 2}, {9, 1}} {
		state.x, state.y = position[0], position[1]
		state.applyInput([]byte{' '}, 1)
	}
	assertShellInput(t, shell, "echo")

	state.x, state.y = 12, 2
	state.applyInput([]byte{' '}, 1)
	assertShellInput(t, shell, "\r")
}

func TestMouseClickActivatesVirtualReturn(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 12, y: 18, row: 2}}

	event := []byte("\033[<0;23;18M")
	state.applyInput(event, len(event))

	assertShellInput(t, shell, "\r")
}

func TestVirtualSpecialKeysAndModifiers(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.shiftOnce = true
	state.x, state.y = 12, 2
	state.insertSelectedKey()
	assertShellInput(t, shell, "\r")

	state.x, state.y = 0, 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b")

	state.ctrl = true
	state.x, state.y = 3, 3
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x03")

	state.ctrl = false
	state.x, state.y = 5, 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[A")
}

func TestCtrlAndMetaApplyToNextKeypressAndCanBeCombined(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x, state.y = 0, 4
	state.insertSelectedKey()
	state.x, state.y = 2, 4
	state.insertSelectedKey()
	if !state.ctrl || !state.meta {
		t.Fatal("expected Ctrl and Meta to be active together")
	}

	state.x, state.y = 1, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b\x11")
	if state.ctrl || state.meta {
		t.Fatal("expected Ctrl and Meta to clear after the next keypress")
	}

	state.x, state.y = 0, 4
	state.insertSelectedKey()
	state.x, state.y = 1, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x11")
	if state.ctrl || state.meta {
		t.Fatal("expected Ctrl to clear after the next keypress")
	}
}

func TestCtrlAndMetaCanBeToggledOffBeforeUse(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.x, state.y = 0, 4
	state.insertSelectedKey()
	state.x, state.y = 2, 4
	state.insertSelectedKey()
	state.x, state.y = 0, 4
	state.insertSelectedKey()
	if state.ctrl || !state.meta {
		t.Fatal("expected Ctrl to toggle off while Meta remains active")
	}

	state.x, state.y = 1, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1bq")
	if state.ctrl || state.meta {
		t.Fatal("expected remaining Meta modifier to clear after keypress")
	}

	state.x, state.y = 2, 4
	state.insertSelectedKey()
	state.x, state.y = 2, 4
	state.insertSelectedKey()
	if state.meta {
		t.Fatal("expected Meta to toggle off when activated a second time")
	}
}

func TestCtrlSpaceAndModifiedSpecialKeys(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.ctrl = true
	state.x, state.y = 1, 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x00")

	state.ctrl = false
	state.meta = true
	state.x, state.y = 0, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b\t")
	if state.meta {
		t.Fatal("expected Meta to clear after the next keypress")
	}

	state.ctrl = true
	state.meta = true
	state.x, state.y = 3, 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[1;7D")
	if state.ctrl || state.meta {
		t.Fatal("expected Ctrl and Meta to clear after a modified arrow key")
	}
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

func TestSelectedKeyboardCellUsesBlackOnWhiteHighlightWithoutBrackets(t *testing.T) {
	var frame strings.Builder
	writeKeyboardCell(&frame, " q  ", true, false)

	const want = "\033[30;47m q  \033[0m"
	if got := frame.String(); got != want {
		t.Fatalf("expected selected cell %q, got %q", want, got)
	}
}

func TestActiveModifierCellIsHighlighted(t *testing.T) {
	for _, cell := range []string{" ctrl ", " meta ", " shift ", " caps "} {
		var frame strings.Builder
		writeKeyboardCell(&frame, cell, false, true)

		want := "\033[37;44m" + cell + "\033[0m"
		if got := frame.String(); got != want {
			t.Fatalf("expected active modifier cell %q, got %q", want, got)
		}
	}
}

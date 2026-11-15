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

func TestDvorakLayoutChangesTypedKey(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Keyboard.Layout = "dvorak"
	state.x, state.y = 1, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "'")
}

func TestStickyShiftRemainsActiveUntilToggledOff(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Keyboard.StickyModifiers = true
	state.x, state.y = 0, 3
	state.insertSelectedKey()
	state.x, state.y = 1, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "Q")
	if !state.shiftOnce {
		t.Fatal("expected sticky Shift to remain enabled")
	}
	state.x, state.y = 0, 3
	state.insertSelectedKey()
	if state.shiftOnce {
		t.Fatal("expected toggling Shift again to disable it")
	}
}

func TestExtendedLayoutIncludesNavigationAndFunctionKeys(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Keyboard.Extended = true
	state.numLock = true
	layout := state.activeLayout()
	if len(layout) != len(key)+1 || layout[0][0] != "F1" {
		t.Fatalf("expected extended layout to include a function-key row, got %v", layout)
	}
	state.x, state.y = 17, 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[H")
}

func TestExtendedLayoutKeepsRightSectionAligned(t *testing.T) {
	state := keyboardState{config: defaultConfig()}
	state.config.Keyboard.Extended = true
	layout := state.activeLayout()
	rows := state.renderKeyboardRows(layout)

	width := 0
	for _, cell := range rows[0] {
		width += cell.width
	}
	for rowIndex, row := range rows[1:] {
		rowWidth := 0
		for _, cell := range row {
			rowWidth += cell.width
		}
		if rowWidth != width {
			t.Fatalf("expected extended rows to have uniform width %d; row %d has %d", width, rowIndex+1, rowWidth)
		}
	}

	position := func(row []renderedKeyCell, keyName string) int {
		offset := 0
		for _, cell := range row {
			if cell.keyName == keyName {
				return offset
			}
			offset += cell.width
		}
		return -1
	}
	if got, want := position(rows[1], "home"), position(rows[2], "end"); got != want {
		t.Fatalf("expected navigation column to align at %d, got %d", want, got)
	}
	if got, want := position(rows[2], "kp1"), position(rows[3], "kp4"); got != want {
		t.Fatalf("expected numpad column to align at %d, got %d", want, got)
	}
	for _, pair := range [][2]string{{"kp7", "kp8"}, {"kp4", "kp5"}, {"kp1", "kp2"}} {
		first, second := findRenderedKey(rows, pair[0]), findRenderedKey(rows, pair[1])
		if first.width != second.width {
			t.Fatalf("expected keypad columns %q and %q to have equal widths, got %+v and %+v", pair[0], pair[1], first, second)
		}
	}
	num := findRenderedKey(rows[1:2], "numwide")
	if got, want := num.width, findRenderedKey(rows[2:3], "kp1").width+findRenderedKey(rows[2:3], "kp2").width+findRenderedKey(rows[2:3], "kp3").width; got != want {
		t.Fatalf("expected Num key to span the number columns (%d), got %d", want, got)
	}
	if lastRenderedPosition(rows[1], "-") <= position(rows[1], "numwide") {
		t.Fatal("expected - to appear to the right of the Num key")
	}
	for _, row := range []struct {
		row      int
		numbers  [3]string
		operator string
	}{
		{row: 2, numbers: [3]string{"kp1", "kp2", "kp3"}, operator: "*"},
		{row: 3, numbers: [3]string{"kp4", "kp5", "kp6"}, operator: "/"},
		{row: 4, numbers: [3]string{"kp7", "kp8", "kp9"}, operator: "+"},
	} {
		if position(rows[row.row], row.operator) <= position(rows[row.row], row.numbers[2]) {
			t.Fatalf("expected operator %q to follow %v on keypad row %d", row.operator, row.numbers, row.row)
		}
		for _, number := range row.numbers {
			if position(rows[row.row], number) < 0 {
				t.Fatalf("expected %q in keypad row %d", number, row.row)
			}
		}
	}
	if position(rows[0], "F5") != position(rows[0], "F4")+findRenderedKey(rows[:1], "F4").width+1 ||
		position(rows[0], "F9") != position(rows[0], "F8")+findRenderedKey(rows[:1], "F8").width+1 {
		t.Fatal("expected function keys to have visual gaps between groups of four")
	}
	if countKey(layout, "+") != 1 || countKey(layout, "enter") != 1 {
		t.Fatal("expected exactly one plus key and one Enter key")
	}
}

func countKey(layout [][]string, keyName string) int {
	count := 0
	for _, row := range layout {
		for _, name := range row {
			if name == keyName {
				count++
			}
		}
	}
	return count
}

func findRenderedKey(rows [][]renderedKeyCell, keyName string) renderedKeyCell {
	for _, row := range rows {
		for _, cell := range row {
			if cell.keyName == keyName {
				return cell
			}
		}
	}
	return renderedKeyCell{}
}

func lastRenderedPosition(row []renderedKeyCell, keyName string) int {
	offset, found := 0, -1
	for _, cell := range row {
		if cell.keyName == keyName {
			found = offset
		}
		offset += cell.width
	}
	return found
}

func TestExtendedKeypadNumLockDisplayAndOutput(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Keyboard.Extended = true
	state.numLock = true
	layout := state.activeLayout()
	keyIndex := func(row int, keyName string) int {
		for col, name := range layout[row] {
			if name == keyName {
				return col
			}
		}
		t.Fatalf("key %q not found in row %d", keyName, row)
		return -1
	}

	if got := state.displayKeyName("kp7"); got != "7" {
		t.Fatalf("expected Num Lock keypad label 7, got %q", got)
	}
	state.x, state.y = keyIndex(4, "kp7"), 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "7")

	state.x, state.y = keyIndex(1, "numwide"), 1
	state.insertSelectedKey()
	if state.numLock {
		t.Fatal("expected Num Lock key to turn Num Lock off")
	}
	if got := state.displayKeyName("kp7"); got != "Home" {
		t.Fatalf("expected navigation label Home with Num Lock off, got %q", got)
	}
	state.x, state.y = keyIndex(4, "kp7"), 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[H")

	state.x, state.y = keyIndex(5, "kp0wide"), 5
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1b[2~")

	state.x, state.y = keyIndex(4, "+"), 4
	state.insertSelectedKey()
	assertShellInput(t, shell, "+")
	state.x, state.y = keyIndex(5, "enter"), 5
	state.insertSelectedKey()
	assertShellInput(t, shell, "\r")
}

func TestCustomLayoutAndFunctionKeyOutput(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Keyboard.Layout = "custom"
	state.config.Keyboard.CustomLayout = [][]string{{"F1", "x", "SPACE"}}
	state.x, state.y = 0, 0
	state.insertSelectedKey()
	assertShellInput(t, shell, "\x1bOP")
	state.x = 1
	state.insertSelectedKey()
	assertShellInput(t, shell, "x")
}

func TestConfiguredRightClickBackspacesAndSGRCanBeDisabled(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Mouse.RightClickAction = "backspace"
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 12, y: 18, row: 2}}
	rightClick := []byte("\033[<2;23;18M")
	state.applyInput(rightClick, len(rightClick))
	assertShellInput(t, shell, "\x7f")

	state.config.Mouse.SGR = false
	state.applyInput([]byte("\033[<0;23;18M"), len("\033[<0;23;18M"))
	assertShellInput(t, shell, "")
}

func TestSelectedTextUsesTerminalCellRange(t *testing.T) {
	state, _ := newTestKeyboardState()
	state.terminal = vt10x.New(vt10x.WithSize(8, 2))
	_, _ = state.terminal.Write([]byte("hello"))
	state.selection = textSelection{startX: 1, startY: 1, endX: 5, endY: 1}
	if got := state.selectedText(); got != "hello" {
		t.Fatalf("expected selected terminal text %q, got %q", "hello", got)
	}
}

func TestClipboardShiftDragTracksTextSelection(t *testing.T) {
	state, _ := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Clipboard.Enabled = true
	state.visibleLines = 5

	start := []byte("\033[<4;2;1M")
	state.applyInput(start, len(start))
	if !state.selection.active {
		t.Fatal("expected Shift+left press to start selection")
	}

	drag := []byte("\033[<36;5;1M")
	state.applyInput(drag, len(drag))
	if state.selection.endX != 5 || state.selection.endY != 1 {
		t.Fatalf("expected selection to extend to 5,1; got %d,%d", state.selection.endX, state.selection.endY)
	}
}

func TestClipboardPasteUsesConfiguredCommand(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.config = defaultConfig()
	state.config.Clipboard.Enabled = true
	state.config.Clipboard.PasteCommand = []string{"/bin/echo", "-n", "pasted"}
	state.pasteClipboard()
	state.reportError()
	assertShellInput(t, shell, "pasted")
}

func TestHighContrastThemeUsesConfiguredPalette(t *testing.T) {
	var frame strings.Builder
	writeKeyboardCellWithTheme(&frame, " key ", false, false, highContrastTheme())
	if got, want := frame.String(), "\033[97;40m key \033[0m"; got != want {
		t.Fatalf("expected high-contrast color sequence %q, got %q", want, got)
	}
}

func TestInputDecoderHandlesFragmentedAndConcatenatedMouseReports(t *testing.T) {
	decoder := inputDecoder{}
	if events := decoder.feed([]byte("\033[<35;23;")); len(events) != 0 {
		t.Fatalf("expected incomplete mouse report to be buffered, got %q", events)
	}
	events := decoder.feed([]byte("18M\033[<0;24;18M"))
	if len(events) != 2 || string(events[0]) != "\033[<35;23;18M" || string(events[1]) != "\033[<0;24;18M" {
		t.Fatalf("expected both complete mouse reports, got %q", events)
	}
}

func TestInputDecoderEmitsStandaloneEscape(t *testing.T) {
	decoder := inputDecoder{}
	events := decoder.feed([]byte{27})
	if len(events) != 1 || len(events[0]) != 1 || events[0][0] != 27 {
		t.Fatalf("expected standalone Escape to be emitted immediately, got %q", events)
	}
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

func TestMouseHoverHighlightsKeyAndOnlyPressActivatesIt(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 12, y: 18, row: 2}}

	motion := []byte("\033[<35;23;18M")
	state.applyInput(motion, len(motion))
	if state.x != 12 || state.y != 2 {
		t.Fatalf("expected mouse hover to select key at row 2, column 12; got row %d, column %d", state.y, state.x)
	}
	assertShellInput(t, shell, "")

	release := []byte("\033[<0;23;18m")
	state.applyInput(release, len(release))
	assertShellInput(t, shell, "")

	press := []byte("\033[<0;23;18M")
	state.applyInput(press, len(press))
	assertShellInput(t, shell, "\r")
}

func TestGPMEventSelectsAndActivatesKey(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 12, y: 18, row: 2}}

	state.handleGPMEvent(gpmEvent{x: 23, y: 18})
	if state.x != 12 || state.y != 2 {
		t.Fatalf("expected GPM pointer to select key at row 2, column 12; got row %d, column %d", state.y, state.x)
	}
	assertShellInput(t, shell, "")

	state.handleGPMEvent(gpmEvent{x: 23, y: 18, pressed: true})
	assertShellInput(t, shell, "\r")
}

func TestNonLeftMousePressStillReachesMouseReportingProgram(t *testing.T) {
	state, shell := newTestKeyboardState()
	state.terminal = vt10x.New(vt10x.WithSize(80, 18))
	_, _ = state.terminal.Write([]byte("\033[?1000h"))
	state.keyAreas = []keyHitArea{{left: 20, right: 26, x: 12, y: 18, row: 2}}

	event := []byte("\033[<2;23;18M")
	state.applyInput(event, len(event))
	assertShellInput(t, shell, string(event))
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

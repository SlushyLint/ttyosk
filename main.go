package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"golang.org/x/term"
)

var key = [][]string{
	{"esc", "`", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "-", "=", "del"},
	{"tab", "q", "w", "e", "r", "t", "y", "u", "i", "o", "p", "[", "]", "\\"},
	{"caps", "a", "s", "d", "f", "g", "h", "j", "k", "l", ";", "'", "return"},
	{"shift", "z", "x", "c", "v", "b", "n", "m", ",", ".", "/", "shift"},
	{"ctrl", "SPACE", "meta", "←", "↓", "↑", "→"},
}

var dvorakKey = [][]string{
	{"esc", "`", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "[", "]", "del"},
	{"tab", "'", ",", ".", "p", "y", "f", "g", "c", "r", "l", "/", "=", "\\"},
	{"caps", "a", "o", "e", "u", "i", "d", "h", "t", "n", "s", "-", "return"},
	{"shift", ";", "q", "j", "k", "x", "b", "m", "w", "v", "z", "shift"},
	{"ctrl", "SPACE", "meta", "←", "↓", "↑", "→"},
}

var colemakKey = [][]string{
	{"esc", "`", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "-", "=", "del"},
	{"tab", "q", "w", "f", "p", "g", "j", "l", "u", "y", ";", "[", "]", "\\"},
	{"caps", "a", "r", "s", "t", "d", "h", "n", "e", "i", "o", "'", "return"},
	{"shift", "z", "x", "c", "v", "b", "k", "m", ",", ".", "/", "shift"},
	{"ctrl", "SPACE", "meta", "←", "↓", "↑", "→"},
}

const keyboardSpacer = "\x00"

var extendedKeyRows = [][]string{
	{"ins", "home", "pgup", keyboardSpacer, "numwide", keyboardSpacer, keyboardSpacer, "-"},
	{"delete", "end", "pgdn", keyboardSpacer, "kp1", "kp2", "kp3", "*"},
	{keyboardSpacer, "↑", keyboardSpacer, keyboardSpacer, "kp4", "kp5", "kp6", "/"},
	{"←", "↓", "→", keyboardSpacer, "kp7", "kp8", "kp9", "+"},
	{keyboardSpacer, keyboardSpacer, keyboardSpacer, keyboardSpacer, "kp0wide", keyboardSpacer, "kp.", "enter"},
}

type keyboardState struct {
	x            int
	y            int
	ctrl         bool
	meta         bool
	shiftOnce    bool
	capsLock     bool
	numLock      bool
	shell        io.Writer
	ptmx         *os.File
	terminal     vt10x.Terminal
	keyAreas     []keyHitArea
	terminalCols int
	config       appConfig
	visibleLines int
	selection    textSelection
	lastError    error
	uinput       keyboardEmitter
	vtOverlay    bool
	vtStarted    bool
	vtRows       int
}

type textSelection struct {
	active bool
	startX int
	startY int
	endX   int
	endY   int
}

type keyHitArea struct {
	left  int
	right int
	x     int
	y     int
	row   int
}

type renderedKeyCell struct {
	keyName string
	col     int
	width   int
	text    string
}

type gpmEvent struct {
	x            int
	y            int
	pressed      bool
	rightPressed bool
	shift        bool
	drag         bool
	released     bool
	err          error
}

type gamepadEvent struct {
	dx        int
	dy        int
	selectKey bool
	backspace bool
	exit      bool
	err       error
}

func (s *keyboardState) move(dx, dy int) {
	layout := s.activeLayout()
	newY := s.y + dy
	if s.y < 0 || s.y >= len(layout) || newY < 0 || newY >= len(layout) {
		return
	}

	currentX := s.x
	if !validKeyAt(layout[s.y], currentX) {
		currentX = closestKeyIndex(layout[s.y], currentX)
		if currentX < 0 {
			return
		}
	}
	newX := currentX
	if dy != 0 {
		cols := s.terminalCols
		if cols <= 0 {
			cols = 80
		}
		targetCenter := s.keyboardKeyCenter(layout, s.y, currentX, cols)
		bestDistance := int(^uint(0) >> 1)
		bestIndexDistance := bestDistance
		for candidate := range layout[newY] {
			if !validKeyAt(layout[newY], candidate) {
				continue
			}
			center := s.keyboardKeyCenter(layout, newY, candidate, cols)
			distance := absInt(center - targetCenter)
			indexDistance := absInt(candidate - currentX)
			if distance < bestDistance || (distance == bestDistance && indexDistance < bestIndexDistance) {
				newX = candidate
				bestDistance = distance
				bestIndexDistance = indexDistance
			}
		}
	}
	newX += dx
	if newX < 0 || newX >= len(layout[newY]) || !validKeyAt(layout[newY], newX) {
		direction := 1
		if dx < 0 {
			direction = -1
		}
		for candidate := newX; candidate >= 0 && candidate < len(layout[newY]); candidate += direction {
			if validKeyAt(layout[newY], candidate) {
				newX = candidate
				break
			}
		}
		if newX < 0 || newX >= len(layout[newY]) || !validKeyAt(layout[newY], newX) {
			newX = closestKeyIndex(layout[newY], currentX)
		}
	}

	s.x = newX
	s.y = newY
}

func (s *keyboardState) keyboardKeyCenter(layout [][]string, row, col, cols int) int {
	if !s.config.Keyboard.Extended {
		return keyboardKeyCenterWithPadding(layout[row], col, cols, s.keyPadding())
	}
	rendered := s.renderKeyboardRows(layout)
	width := 0
	for _, cell := range rendered[row] {
		width += cell.width
	}
	startX := (cols - width) / 2
	if startX < 1 {
		startX = 1
	}
	offset := 0
	for _, cell := range rendered[row] {
		if cell.col == col {
			return startX*2 + offset*2 + cell.width
		}
		offset += cell.width
	}
	return startX*2 + offset*2
}

func keyboardKeyCenter(row []string, index, cols int) int {
	return keyboardKeyCenterWithPadding(row, index, cols, 1)
}

func keyboardKeyCenterWithPadding(row []string, index, cols, padding int) int {
	rowWidth := 0
	for _, keyName := range row {
		rowWidth += keyboardKeyWidth(keyName, padding)
	}
	startX := (cols - rowWidth) / 2
	if startX < 1 {
		startX = 1
	}
	center := startX * 2
	for i, keyName := range row {
		width := keyboardKeyWidth(keyName, padding)
		if i == index {
			return center + width
		}
		center += width * 2
	}
	return center
}

func keyboardKeyWidth(keyName string, padding int) int {
	if keyName == keyboardSpacer {
		return padding*2 + 1
	}
	return utf8.RuneCountInString(keyName) + padding*2 + 1
}

func validKeyAt(row []string, index int) bool {
	return index >= 0 && index < len(row) && row[index] != keyboardSpacer && row[index] != ""
}

func closestKeyIndex(row []string, index int) int {
	best := -1
	bestDistance := int(^uint(0) >> 1)
	for candidate := range row {
		if !validKeyAt(row, candidate) {
			continue
		}
		distance := absInt(candidate - index)
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (s *keyboardState) activeLayout() [][]string {
	rows := s.baseLayout()
	layout := make([][]string, len(rows))
	if s.config.Keyboard.Extended {
		layout = make([][]string, len(rows)+1)
		functions := make([]string, 12)
		for i := range functions {
			functions[i] = fmt.Sprintf("F%d", i+1)
		}
		layout[0] = functions
	}
	for rowIndex, row := range rows {
		layoutIndex := rowIndex
		if s.config.Keyboard.Extended {
			layoutIndex++
		}
		layout[layoutIndex] = append([]string(nil), row...)
		if s.config.Keyboard.Extended && rowIndex < len(extendedKeyRows) {
			layout[layoutIndex] = append(layout[layoutIndex], keyboardSpacer)
			layout[layoutIndex] = append(layout[layoutIndex], extendedKeyRows[rowIndex]...)
		}
	}
	if s.config.Clipboard.Enabled && len(layout) > 0 {
		layout[len(layout)-1] = append(layout[len(layout)-1], "paste")
	}
	if s.config.Keyboard.Output != "uinput" {
		for rowIndex, row := range layout {
			for col, keyName := range row {
				if keyName == keyboardSpacer || keyName == "" {
					continue
				}
				isLetter := len(keyName) == 1 && keyName[0] >= 'a' && keyName[0] <= 'z'
				if isLetter {
					if s.capsLock != s.shiftOnce {
						layout[rowIndex][col] = strings.ToUpper(keyName)
					}
				} else if s.shiftOnce && len(keyName) == 1 {
					layout[rowIndex][col] = shiftValue(keyName)
				}
			}
		}
	}
	return layout
}

func (s *keyboardState) baseLayout() [][]string {
	rows := key
	switch s.config.Keyboard.Layout {
	case "dvorak":
		rows = dvorakKey
	case "colemak":
		rows = colemakKey
	case "custom":
		rows = s.config.Keyboard.CustomLayout
	}
	return rows
}

func (s *keyboardState) currentKey() string {
	layout := s.activeLayout()
	if s.y < 0 || s.y >= len(layout) {
		return ""
	}
	if s.x < 0 || s.x >= len(layout[s.y]) {
		return ""
	}
	if !validKeyAt(layout[s.y], s.x) {
		return ""
	}
	return layout[s.y][s.x]
}

func (s *keyboardState) keyPadding() int {
	if s.config.Keyboard.Layout == "" {
		return 1
	}
	padding := s.config.Keyboard.KeyPadding
	if padding < 0 || padding > 4 {
		return 1
	}
	return padding
}

func (s *keyboardState) numLockEnabled() bool {
	return s.config.Keyboard.Layout == "" || s.numLock
}

func (s *keyboardState) displayKeyName(keyName string) string {
	if keyName == "num" || keyName == "numwide" {
		return "Num"
	}
	keypad := map[string][2]string{
		"kp7": {"7", "Home"}, "kp8": {"8", "↑"}, "kp9": {"9", "PgUp"},
		"kp4": {"4", "←"}, "kp5": {"5", "5"}, "kp6": {"6", "→"},
		"kp1": {"1", "End"}, "kp2": {"2", "↓"}, "kp3": {"3", "PgDn"},
		"kp0": {"0", "Ins"}, "kp0wide": {"0", "Ins"}, "kp.": {".", "Del"},
	}
	if names, ok := keypad[keyName]; ok {
		index := 1
		if s.numLockEnabled() {
			index = 0
		}
		return names[index]
	}
	return keyName
}

func (s *keyboardState) vimNavigationEnabled() bool {
	return s.config.Keyboard.Layout == "" || s.config.Keyboard.VimNavigation
}

func (s *keyboardState) sgrEnabled() bool {
	return s.config.Keyboard.Layout == "" || s.config.Mouse.SGR
}

func (s *keyboardState) hoverEnabled() bool {
	return s.config.Keyboard.Layout == "" || s.config.Mouse.HoverHighlight
}

func (s *keyboardState) keyTheme() colorTheme {
	if s.config.Keyboard.HighContrast {
		return highContrastTheme()
	}
	if s.config.Keyboard.Layout == "" {
		return defaultConfig().Keyboard.Colors
	}
	return s.config.Keyboard.Colors
}

func (s *keyboardState) applyInput(buf []byte, n int) {
	if s.sgrEnabled() && s.handleMouseClick(buf[:n]) {
		return
	}

	if n == 3 && buf[0] == 27 && buf[1] == '[' {
		switch buf[2] {
		case 'A':
			s.move(0, -1)
		case 'B':
			s.move(0, 1)
		case 'C':
			s.move(1, 0)
		case 'D':
			s.move(-1, 0)
		}
		return
	}

	if n != 1 {
		return
	}

	switch buf[0] {
	case 'h':
		if s.vimNavigationEnabled() {
			s.move(-1, 0)
		}
	case 'j':
		if s.vimNavigationEnabled() {
			s.move(0, 1)
		}
	case 'k':
		if s.vimNavigationEnabled() {
			s.move(0, -1)
		}
	case 'l':
		if s.vimNavigationEnabled() {
			s.move(1, 0)
		}
	case ' ', '\r', '\n':
		s.insertSelectedKey()
	}
}

func shiftValue(value string) string {
	if value == "" {
		return ""
	}

	shiftMap := map[string]string{
		"`":  "~",
		"1":  "!",
		"2":  "@",
		"3":  "#",
		"4":  "$",
		"5":  "%",
		"6":  "^",
		"7":  "&",
		"8":  "*",
		"9":  "(",
		"0":  ")",
		"-":  "_",
		"=":  "+",
		"[":  "{",
		"]":  "}",
		"\\": "|",
		";":  ":",
		"'":  "\"",
		",":  "<",
		".":  ">",
		"/":  "?",
		"q":  "Q",
		"w":  "W",
		"e":  "E",
		"r":  "R",
		"t":  "T",
		"y":  "Y",
		"u":  "U",
		"i":  "I",
		"o":  "O",
		"p":  "P",
		"a":  "A",
		"s":  "S",
		"d":  "D",
		"f":  "F",
		"g":  "G",
		"h":  "H",
		"j":  "J",
		"k":  "K",
		"l":  "L",
		"z":  "Z",
		"x":  "X",
		"c":  "C",
		"v":  "V",
		"b":  "B",
		"n":  "N",
		"m":  "M",
	}

	if shifted, ok := shiftMap[value]; ok {
		return shifted
	}
	return strings.ToUpper(value)
}

func (s *keyboardState) insertSelectedKey() {
	if s.config.Keyboard.Output == "uinput" {
		s.insertSelectedUinputKey()
		return
	}

	selected := extendedKeyIdentity(s.currentKey())
	if selected == "num" {
		s.numLock = !s.numLock
		return
	}
	if selected == "shift" {
		s.shiftOnce = !s.shiftOnce
		return
	}
	if selected == "caps" {
		s.capsLock = !s.capsLock
		s.shiftOnce = false
		return
	}
	if selected == "ctrl" {
		s.ctrl = !s.ctrl
		return
	}
	if selected == "meta" {
		s.meta = !s.meta
		return
	}

	ctrl := s.ctrl
	meta := s.meta
	if !s.config.Keyboard.StickyModifiers {
		s.shiftOnce = false
		s.ctrl = false
		s.meta = false
	}

	value := selected
	switch value {
	case "SPACE":
		value = " "
	case "del":
		value = "\x7f"
	case "return", "rtrn", "enter":
		value = "\r"
	case "esc":
		value = "\x1b"
	case "tab":
		value = "\t"
	case "paste":
		s.pasteClipboard()
		return
	case "kp7":
		value = s.keypadValue("7", "home")
	case "kp8":
		value = s.keypadValue("8", "up")
	case "kp9":
		value = s.keypadValue("9", "pgup")
	case "kp4":
		value = s.keypadValue("4", "left")
	case "kp5":
		value = "5"
	case "kp6":
		value = s.keypadValue("6", "right")
	case "kp1":
		value = s.keypadValue("1", "end")
	case "kp2":
		value = s.keypadValue("2", "down")
	case "kp3":
		value = s.keypadValue("3", "pgdn")
	case "kp0":
		value = s.keypadValue("0", "ins")
	case "kp0wide":
		value = s.keypadValue("0", "ins")
	case "kp.":
		value = s.keypadValue(".", "delete")
	case "ins":
		value = "\x1b[2~"
	case "delete":
		value = "\x1b[3~"
	case "home":
		value = "\x1b[H"
	case "end":
		value = "\x1b[F"
	case "pgup":
		value = "\x1b[5~"
	case "pgdn":
		value = "\x1b[6~"
	case "left", "right", "up", "down", "←", "→", "↑", "↓":
		s.emitToShell(s.arrowSequence(value, ctrl, meta))
		return
	}
	if sequence := functionKeySequence(value); sequence != "" {
		value = sequence
	}
	if ctrl {
		value = controlCode(value)
	}
	if meta {
		value = "\x1b" + value
	}
	s.emitToShell(value)
}

func (s *keyboardState) insertSelectedUinputKey() {
	selected := extendedKeyIdentity(s.currentKey())
	if selected == "num" {
		s.numLock = !s.numLock
		s.emitUinputKey("num", 0)
		return
	}
	if selected == "shift" {
		s.shiftOnce = !s.shiftOnce
		return
	}
	if selected == "caps" {
		s.capsLock = !s.capsLock
		s.shiftOnce = false
		s.emitUinputKey("caps", 0)
		return
	}
	if selected == "ctrl" {
		s.ctrl = !s.ctrl
		return
	}
	if selected == "meta" {
		s.meta = !s.meta
		return
	}
	if selected == "paste" {
		s.pasteClipboard()
		return
	}

	modifiers := uint8(0)
	if s.shiftOnce {
		modifiers |= uinputShift
	}
	if s.ctrl {
		modifiers |= uinputCtrl
	}
	if s.meta {
		modifiers |= uinputAlt
	}
	if !s.config.Keyboard.StickyModifiers {
		s.shiftOnce = false
		s.ctrl = false
		s.meta = false
	}
	s.emitUinputKey(s.uinputKeyAtSelection(), modifiers)
}

func (s *keyboardState) uinputKeyAtSelection() string {
	keyName := extendedKeyIdentity(s.currentKey())
	row := s.y
	if s.config.Keyboard.Extended {
		row--
	}
	rows := s.baseLayout()
	if row < 0 || row >= len(rows) || s.x < 0 {
		return keyName
	}
	if s.config.Keyboard.Extended && row < len(extendedKeyRows) && s.x > len(rows[row]) {
		sideIndex := s.x - len(rows[row]) - 1
		if sideIndex >= 0 && sideIndex < len(extendedKeyRows[row]) {
			switch extendedKeyRows[row][sideIndex] {
			case "-":
				return "kp-"
			case "*":
				return "kp*"
			case "/":
				return "kp/"
			case "+":
				return "kp+"
			}
		}
	}
	if (s.config.Keyboard.Layout == "dvorak" || s.config.Keyboard.Layout == "colemak") &&
		s.x < len(rows[row]) && s.x < len(key[row]) {
		return extendedKeyIdentity(key[row][s.x])
	}
	return keyName
}

func (s *keyboardState) emitUinputKey(key string, modifiers uint8) {
	if s.uinput == nil {
		s.lastError = fmt.Errorf("uinput keyboard is unavailable")
		return
	}
	if err := s.uinput.EmitKey(key, modifiers); err != nil {
		s.lastError = fmt.Errorf("send key with uinput keyboard: %w", err)
	}
}

func (s *keyboardState) keypadValue(number, navigation string) string {
	if s.numLockEnabled() {
		return number
	}
	switch navigation {
	case "home":
		return "\x1b[H"
	case "up":
		return s.arrowSequence("↑", s.ctrl, s.meta)
	case "pgup":
		return "\x1b[5~"
	case "left":
		return s.arrowSequence("←", s.ctrl, s.meta)
	case "right":
		return s.arrowSequence("→", s.ctrl, s.meta)
	case "end":
		return "\x1b[F"
	case "down":
		return s.arrowSequence("↓", s.ctrl, s.meta)
	case "pgdn":
		return "\x1b[6~"
	case "ins":
		return "\x1b[2~"
	case "delete":
		return "\x1b[3~"
	}
	return number
}

func functionKeySequence(value string) string {
	sequences := map[string]string{
		"F1": "\x1bOP", "F2": "\x1bOQ", "F3": "\x1bOR", "F4": "\x1bOS",
		"F5": "\x1b[15~", "F6": "\x1b[17~", "F7": "\x1b[18~", "F8": "\x1b[19~",
		"F9": "\x1b[20~", "F10": "\x1b[21~", "F11": "\x1b[23~", "F12": "\x1b[24~",
	}
	return sequences[value]
}

func controlCode(value string) string {
	if len(value) != 1 {
		return value
	}
	char := value[0]
	if char >= 'a' && char <= 'z' {
		char -= 'a' - 'A'
	}
	if char == ' ' {
		return "\x00"
	}
	if char >= '@' && char <= '_' {
		return string(char & 0x1f)
	}
	if char == '?' {
		return "\x7f"
	}
	return value
}

func (s *keyboardState) arrowSequence(keyName string, ctrl, meta bool) string {
	final := map[string]string{
		"up": "A", "↑": "A",
		"down": "B", "↓": "B",
		"right": "C", "→": "C",
		"left": "D", "←": "D",
	}[keyName]
	if final == "" {
		return ""
	}
	if ctrl {
		modifier := 5
		if meta {
			modifier = 7
		}
		return fmt.Sprintf("\x1b[1;%d%s", modifier, final)
	}
	if s.terminal != nil {
		s.terminal.Lock()
		applicationCursor := s.terminal.Mode()&vt10x.ModeAppCursor != 0
		s.terminal.Unlock()
		if applicationCursor {
			if meta {
				return "\x1b\x1bO" + final
			}
			return "\x1bO" + final
		}
	}
	sequence := "\x1b[" + final
	if meta {
		sequence = "\x1b" + sequence
	}
	return sequence
}

func (s *keyboardState) emitToShell(value string) {
	if s.config.Keyboard.Output == "uinput" {
		if s.uinput == nil {
			s.lastError = fmt.Errorf("uinput keyboard is unavailable")
			return
		}
		if err := s.uinput.TypeText(value); err != nil {
			s.lastError = fmt.Errorf("type with uinput keyboard: %w", err)
		}
		return
	}
	if s.shell == nil || value == "" {
		return
	}
	_, _ = io.WriteString(s.shell, value)
}

func (s *keyboardState) handleMouseClick(input []byte) bool {
	button, mouseX, mouseY, event, ok := parseSGRMouseEvent(input)
	if !ok {
		return false
	}

	leftPress := event == 'M' && button&32 == 0 && button&64 == 0 && button&3 == 0
	rightPress := event == 'M' && button&32 == 0 && button&64 == 0 && button&3 == 2
	isMotion := button&32 != 0
	if s.config.Clipboard.Enabled && s.handleSGRSelection(button, mouseX, mouseY, event, leftPress) {
		return true
	}
	onKeyboard := false
	if !isMotion || s.hoverEnabled() {
		onKeyboard = s.handleMouseAt(mouseX, mouseY, leftPress)
	}
	if onKeyboard && rightPress && s.config.Mouse.RightClickAction == "backspace" {
		s.emitToShell("\x7f")
		return true
	}
	if onKeyboard && (leftPress || isMotion) {
		return true
	}
	s.forwardMouseEvent(input)
	return true
}

func (s *keyboardState) handleSGRSelection(button, x, y int, event byte, leftPress bool) bool {
	if leftPress && button&4 != 0 && y > 0 && y <= s.visibleLines {
		s.selection = textSelection{active: true, startX: x, startY: y, endX: x, endY: y}
		return true
	}
	if !s.selection.active {
		return false
	}
	if button&32 != 0 && button&3 == 0 && button&64 == 0 {
		s.selection.endX, s.selection.endY = x, y
		return true
	}
	if event == 'm' && button&3 == 0 {
		s.selection.endX, s.selection.endY = x, y
		s.selection.active = false
		s.copySelection()
		return true
	}
	return false
}

func parseSGRMouseEvent(input []byte) (button, x, y int, event byte, ok bool) {
	if len(input) < 7 || string(input[:3]) != "\033[<" || (input[len(input)-1] != 'M' && input[len(input)-1] != 'm') {
		return 0, 0, 0, 0, false
	}

	fields := strings.Split(string(input[3:len(input)-1]), ";")
	if len(fields) != 3 {
		return 0, 0, 0, 0, false
	}
	values := [3]int{}
	for i, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		values[i] = value
	}
	return values[0], values[1], values[2], input[len(input)-1], true
}

func (s *keyboardState) handleMouseAt(mouseX, mouseY int, pressed bool) bool {
	for _, area := range s.keyAreas {
		if area.y == mouseY && mouseX >= area.left && mouseX <= area.right {
			s.x, s.y = area.x, area.row
			if pressed {
				s.insertSelectedKey()
			}
			return true
		}
	}
	return false
}

func (s *keyboardState) handleGPMEvent(event gpmEvent) {
	if s.config.Clipboard.Enabled {
		if event.pressed && event.shift && event.y > 0 && event.y <= s.visibleLines {
			s.selection = textSelection{active: true, startX: event.x, startY: event.y, endX: event.x, endY: event.y}
			return
		}
		if s.selection.active && event.drag {
			s.selection.endX, s.selection.endY = event.x, event.y
			return
		}
		if s.selection.active && event.released {
			s.selection.endX, s.selection.endY = event.x, event.y
			s.selection.active = false
			s.copySelection()
			return
		}
	}
	if !event.pressed && !event.rightPressed && !s.hoverEnabled() {
		return
	}
	onKeyboard := s.handleMouseAt(event.x, event.y, event.pressed)
	if onKeyboard && event.rightPressed && s.config.Mouse.RightClickAction == "backspace" {
		s.emitToShell("\x7f")
	}
}

func (s *keyboardState) forwardMouseEvent(input []byte) {
	if s.terminal == nil {
		return
	}
	s.terminal.Lock()
	mouseReporting := s.terminal.Mode()&vt10x.ModeMouseMask != 0
	s.terminal.Unlock()
	if mouseReporting {
		s.emitToShell(string(input))
	}
}

func (s *keyboardState) renderKeyboardRows(layout [][]string) [][]renderedKeyCell {
	if !s.config.Keyboard.Extended {
		rendered := make([][]renderedKeyCell, len(layout))
		for rowIndex, row := range layout {
			for col, keyName := range row {
				width := utf8.RuneCountInString(keyName) + s.keyPadding()*2
				text := strings.Repeat(" ", s.keyPadding()) + keyName + strings.Repeat(" ", s.keyPadding())
				if keyName == keyboardSpacer {
					width = s.keyPadding()*2 + 1
					text = strings.Repeat(" ", width)
				}
				rendered[rowIndex] = append(rendered[rowIndex], renderedKeyCell{
					keyName: keyName,
					col:     col,
					width:   width,
					text:    text,
				})
			}
		}
		return rendered
	}

	baseRows := s.baseLayout()
	sideRows := make([][]string, len(baseRows))
	sideStarts := make([]int, len(baseRows))
	mainWidths := make([]int, len(baseRows))
	maxMainWidth := 0
	sideColumnWidths := make([]int, 0)
	for rowIndex, baseRow := range baseRows {
		layoutRow := layout[rowIndex+1]
		for _, keyName := range layoutRow[:len(baseRow)] {
			mainWidths[rowIndex] += s.renderKeyWidth(keyName)
		}
		if mainWidths[rowIndex] > maxMainWidth {
			maxMainWidth = mainWidths[rowIndex]
		}
		sideStart := len(baseRow)
		if rowIndex < len(extendedKeyRows) {
			sideStart++
		}
		sideStarts[rowIndex] = sideStart
		if sideStart < len(layoutRow) {
			sideRows[rowIndex] = layoutRow[sideStart:]
		}
		for col, keyName := range sideRows[rowIndex] {
			span := 1
			if keyName == "numwide" {
				span = 3
			} else if keyName == "kp0wide" {
				span = 2
			}
			for offset := 0; offset < span; offset++ {
				for len(sideColumnWidths) <= col+offset {
					sideColumnWidths = append(sideColumnWidths, 0)
				}
			}
			if keyName == "numwide" {
				continue
			}
			width := s.renderKeyWidth(keyName)
			if width > sideColumnWidths[col] {
				sideColumnWidths[col] = width
			}
		}
	}
	if len(sideColumnWidths) >= 7 {
		numberColumnWidth := maxInt(sideColumnWidths[4], maxInt(sideColumnWidths[5], sideColumnWidths[6]))
		for col := 4; col <= 6; col++ {
			sideColumnWidths[col] = numberColumnWidth
		}
	}
	sideWidth := 0
	for _, width := range sideColumnWidths {
		sideWidth += width
	}
	const sectionGap = 3
	sectionWidth := maxMainWidth + sectionGap + sideWidth
	totalWidth := sectionWidth

	rendered := make([][]renderedKeyCell, len(layout))
	functionRowWidth := 0
	for col, keyName := range layout[0] {
		functionRowWidth += functionKeyWidth(keyName)
		if col == 3 || col == 7 {
			functionRowWidth++
		}
	}
	if functionRowWidth > totalWidth {
		totalWidth = functionRowWidth
	}
	functionPadding := (maxMainWidth - functionRowWidth) / 2
	if functionPadding < 0 {
		functionPadding = 0
	}
	rendered[0] = append(rendered[0], renderedKeyCell{col: -1, width: functionPadding, text: strings.Repeat(" ", functionPadding)})
	for col, keyName := range layout[0] {
		if col == 4 || col == 8 {
			rendered[0] = append(rendered[0], renderedKeyCell{col: -1, width: 1, text: " "})
		}
		width := functionKeyWidth(keyName)
		rendered[0] = append(rendered[0], renderedKeyCell{
			keyName: keyName,
			col:     col,
			width:   width,
			text:    functionKeyText(keyName, width),
		})
	}
	rendered[0] = append(rendered[0], renderedKeyCell{
		col:   -1,
		width: totalWidth - functionPadding - functionRowWidth,
		text:  strings.Repeat(" ", totalWidth-functionPadding-functionRowWidth),
	})

	for rowIndex, baseRow := range baseRows {
		layoutIndex := rowIndex + 1
		leftPadding := (maxMainWidth - mainWidths[rowIndex]) / 2
		if leftPadding > 0 {
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				col: -1, width: leftPadding, text: strings.Repeat(" ", leftPadding),
			})
		}
		for col, keyName := range layout[rowIndex+1][:len(baseRow)] {
			width := s.renderKeyWidth(keyName)
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				keyName: keyName,
				col:     col,
				width:   width,
				text:    s.renderKeyText(keyName, width),
			})
		}
		rightPadding := maxMainWidth - mainWidths[rowIndex] - leftPadding
		if rightPadding > 0 {
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				col: -1, width: rightPadding, text: strings.Repeat(" ", rightPadding),
			})
		}
		rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
			col: -1, width: sectionGap, text: strings.Repeat(" ", sectionGap),
		})
		for col := 0; col < len(sideRows[rowIndex]); col++ {
			keyName := sideRows[rowIndex][col]
			logicalCol := col
			width := sideColumnWidths[col]
			switch keyName {
			case "kp0wide":
				width += sideColumnWidths[col+1]
				col++
			case "numwide":
				width += sideColumnWidths[col+1] + sideColumnWidths[col+2]
				col += 2
			}
			text := s.renderKeyText(keyName, width)
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				keyName: keyName,
				col:     sideStarts[rowIndex] + logicalCol,
				width:   width,
				text:    text,
			})
		}
		for col := len(sideRows[rowIndex]); col < len(sideColumnWidths); col++ {
			width := sideColumnWidths[col]
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				col: -1, width: width, text: strings.Repeat(" ", width),
			})
		}
		if extra := totalWidth - sectionWidth; extra > 0 {
			rendered[layoutIndex] = append(rendered[layoutIndex], renderedKeyCell{
				col: -1, width: extra, text: strings.Repeat(" ", extra),
			})
		}
	}
	return rendered
}

func (s *keyboardState) vtOverlayFrame(cols, rows int) (string, error) {
	layout := s.activeLayout()
	keyboardStartY := rows - len(layout) + 1
	if cols <= 0 || keyboardStartY <= 1 {
		return "", fmt.Errorf("terminal is too small for VT keyboard overlay")
	}
	s.terminalCols = cols
	s.visibleLines = keyboardStartY - 1
	s.vtRows = rows

	var frame strings.Builder
	if !s.vtStarted {
		frame.WriteString("\0337")
		frame.WriteString(fmt.Sprintf("\033[1;%dr", keyboardStartY-1))
		frame.WriteString("\0338")
		s.vtStarted = true
	}
	frame.WriteString("\0337")
	s.drawKeyboardRows(&frame, cols, keyboardStartY)
	frame.WriteString("\0338")
	return frame.String(), nil
}

func (s *keyboardState) drawVTOverlay() error {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return fmt.Errorf("get terminal size for VT overlay: %w", err)
	}
	frame, err := s.vtOverlayFrame(cols, rows)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(os.Stdout, frame); err != nil {
		return fmt.Errorf("draw VT keyboard overlay: %w", err)
	}
	return nil
}

func (s *keyboardState) restoreVTOverlay() error {
	if !s.vtStarted {
		return nil
	}
	layout := s.activeLayout()
	keyboardStartY := s.vtRows - len(layout) + 1
	var frame strings.Builder
	frame.WriteString("\0337\033[r")
	for row := 0; row < len(layout); row++ {
		frame.WriteString(fmt.Sprintf("\033[%d;1H\033[2K", keyboardStartY+row))
	}
	frame.WriteString("\033[0m\0338")
	if _, err := io.WriteString(os.Stdout, frame.String()); err != nil {
		return fmt.Errorf("restore terminal after VT keyboard overlay: %w", err)
	}
	s.vtStarted = false
	return nil
}

func maxInt(first, second int) int {
	if first > second {
		return first
	}
	return second
}

func (s *keyboardState) renderKeyWidth(keyName string) int {
	padding := s.keyPadding()
	if keyName == keyboardSpacer {
		return padding*2 + 1
	}
	return utf8.RuneCountInString(s.displayKeyName(keyName)) + padding*2
}

func (s *keyboardState) renderKeyText(keyName string, width int) string {
	padding := s.keyPadding()
	if keyName == keyboardSpacer {
		return strings.Repeat(" ", width)
	}
	displayName := s.displayKeyName(keyName)
	content := strings.Repeat(" ", padding) + displayName + strings.Repeat(" ", padding)
	contentWidth := utf8.RuneCountInString(content)
	leftPadding := (width - contentWidth) / 2
	return strings.Repeat(" ", leftPadding) + content + strings.Repeat(" ", width-contentWidth-leftPadding)
}

func functionKeyWidth(string) int {
	return 4
}

func functionKeyText(keyName string, width int) string {
	leftPadding := (width - utf8.RuneCountInString(keyName)) / 2
	return strings.Repeat(" ", leftPadding) + keyName + strings.Repeat(" ", width-leftPadding-utf8.RuneCountInString(keyName))
}

func extendedKeyIdentity(keyName string) string {
	if keyName == "numwide" {
		return "num"
	}
	return keyName
}

func (s *keyboardState) draw() {
	layout := s.activeLayout()
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	s.terminalCols = cols

	visibleLines := rows - len(layout)
	if visibleLines < 1 {
		visibleLines = 1
	}
	s.visibleLines = visibleLines
	s.resizeTerminal(cols, visibleLines)
	var frame strings.Builder
	frame.WriteString("\033[?25l")
	s.drawTerminal(&frame, cols, visibleLines)

	keyboardStartY := rows - len(layout) + 1
	s.drawKeyboardRows(&frame, cols, keyboardStartY)

	if s.terminal != nil {
		s.terminal.Lock()
		cursor := s.terminal.Cursor()
		cursorVisible := s.terminal.CursorVisible()
		s.terminal.Unlock()
		if cursorVisible && cursor.Y < visibleLines {
			frame.WriteString(fmt.Sprintf("\033[?25h\033[%d;%dH", cursor.Y+1, cursor.X+1))
		}
	}
	_, _ = os.Stdout.Write([]byte(frame.String()))
}

func (s *keyboardState) drawKeyboardRows(frame *strings.Builder, cols, keyboardStartY int) {
	layout := s.activeLayout()
	s.keyAreas = nil
	theme := s.keyTheme()
	renderedRows := s.renderKeyboardRows(layout)
	for rowIndex, row := range renderedRows {
		var line strings.Builder
		lineWidth := 0
		for _, cell := range row {
			if cell.col < 0 || cell.keyName == keyboardSpacer || cell.keyName == "" {
				line.WriteString(cell.text)
				lineWidth += cell.width
				continue
			}
			selected := rowIndex == s.y && cell.col == s.x
			keyName := cell.keyName
			activeModifier := keyName == "ctrl" && s.ctrl ||
				keyName == "meta" && s.meta ||
				keyName == "shift" && s.shiftOnce ||
				keyName == "caps" && s.capsLock ||
				(keyName == "num" || keyName == "numwide") && s.numLockEnabled()
			writeKeyboardCellWithTheme(&line, cell.text, selected, activeModifier, theme)
			lineWidth += cell.width
		}
		startX := (cols - lineWidth) / 2
		if startX < 1 {
			startX = 1
		}
		keyboardY := keyboardStartY + rowIndex
		frame.WriteString(fmt.Sprintf("\033[%d;1H\033[2K", keyboardY))
		frame.WriteString(fmt.Sprintf("\033[%d;%dH", keyboardY, startX))
		cellX := startX
		for _, cell := range row {
			if cell.col >= 0 && cell.keyName != keyboardSpacer && cell.keyName != "" {
				s.keyAreas = append(s.keyAreas, keyHitArea{
					left:  cellX,
					right: cellX + cell.width - 1,
					x:     cell.col,
					y:     keyboardStartY + rowIndex,
					row:   rowIndex,
				})
			}
			cellX += cell.width
		}
		frame.WriteString(line.String())
	}
}

func writeKeyboardCell(frame *strings.Builder, cell string, selected, activeModifier bool) {
	writeKeyboardCellWithTheme(frame, cell, selected, activeModifier, colorTheme{
		KeyForeground:      -1,
		KeyBackground:      -1,
		SelectedForeground: 0,
		SelectedBackground: 7,
		ModifierForeground: 7,
		ModifierBackground: 4,
	})
}

func writeKeyboardCellWithTheme(frame *strings.Builder, cell string, selected, activeModifier bool, theme colorTheme) {
	if selected {
		frame.WriteString(colorStyle(theme.SelectedForeground, theme.SelectedBackground))
	} else if activeModifier {
		frame.WriteString(colorStyle(theme.ModifierForeground, theme.ModifierBackground))
	} else if theme.KeyForeground >= 0 || theme.KeyBackground >= 0 {
		frame.WriteString(colorStyle(theme.KeyForeground, theme.KeyBackground))
	}
	frame.WriteString(cell)
	if selected || activeModifier || theme.KeyForeground >= 0 || theme.KeyBackground >= 0 {
		frame.WriteString("\033[0m")
	}
}

func colorStyle(foreground, background int) string {
	return fmt.Sprintf("\033[%s;%sm", ansiColorCode(foreground, true), ansiColorCode(background, false))
}

func ansiColorCode(color int, foreground bool) string {
	if color < 0 {
		if foreground {
			return "39"
		}
		return "49"
	}
	if color < 8 {
		if foreground {
			return fmt.Sprint(30 + color)
		}
		return fmt.Sprint(40 + color)
	}
	if color < 16 {
		if foreground {
			return fmt.Sprint(90 + color - 8)
		}
		return fmt.Sprint(100 + color - 8)
	}
	if foreground {
		return fmt.Sprintf("38;5;%d", color)
	}
	return fmt.Sprintf("48;5;%d", color)
}

func (s *keyboardState) resizeTerminal(cols, rows int) {
	if s.terminal == nil {
		s.terminal = vt10x.New(vt10x.WithSize(cols, rows))
	}
	currentCols, currentRows := s.terminal.Size()
	if currentCols == cols && currentRows == rows {
		return
	}
	s.terminal.Resize(cols, rows)
	if s.ptmx != nil {
		_ = pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
}

func (s *keyboardState) drawTerminal(frame *strings.Builder, cols, rows int) {
	if s.terminal == nil {
		return
	}
	s.terminal.Lock()
	defer s.terminal.Unlock()

	for y := 0; y < rows; y++ {
		frame.WriteString(fmt.Sprintf("\033[%d;1H", y+1))
		style := ""
		for x := 0; x < cols; x++ {
			glyph := s.terminal.Cell(x, y)
			glyphStyle := terminalStyle(glyph)
			if s.selectionContains(x+1, y+1) {
				glyphStyle = terminalStyleSelected(glyph)
			}
			if glyphStyle != style {
				frame.WriteString(glyphStyle)
				style = glyphStyle
			}
			if glyph.Char == 0 {
				frame.WriteByte(' ')
			} else {
				frame.WriteRune(glyph.Char)
			}
		}
		frame.WriteString("\033[0m")
	}
}

func terminalStyle(glyph vt10x.Glyph) string {
	codes := []string{"0", terminalColorCode(glyph.FG, true), terminalColorCode(glyph.BG, false)}
	if glyph.Mode&1 != 0 {
		codes = append(codes, "7")
	}
	if glyph.Mode&2 != 0 {
		codes = append(codes, "4")
	}
	if glyph.Mode&4 != 0 {
		codes = append(codes, "1")
	}
	if glyph.Mode&16 != 0 {
		codes = append(codes, "3")
	}
	if glyph.Mode&32 != 0 {
		codes = append(codes, "5")
	}
	return "\033[" + strings.Join(codes, ";") + "m"
}

func terminalStyleSelected(glyph vt10x.Glyph) string {
	glyph.Mode |= 1
	return terminalStyle(glyph)
}

func terminalColorCode(color vt10x.Color, foreground bool) string {
	if color == vt10x.DefaultFG || color == vt10x.DefaultBG {
		if foreground {
			return "39"
		}
		return "49"
	}
	if color >= 16 {
		return fmt.Sprintf("%d;5;%d", map[bool]int{true: 38, false: 48}[foreground], color)
	}
	base := 30
	if !foreground {
		base = 40
	}
	if color >= 8 {
		base += 60
		color -= 8
	}
	return fmt.Sprint(base + int(color))
}

func startShell(command string, args []string, cols, rows int) (*os.File, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = os.Environ()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return ptmx, nil
}

func main() {
	config, err := loadConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
		os.Exit(2)
	}
	if config.VTMode && !config.VTService {
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			fmt.Fprintln(os.Stderr, "ttyosk: -vt=true requires a terminal")
			os.Exit(2)
		}
		pid, err := startVTOverlay(os.Args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ttyosk: VT keyboard running in background (pid %d); press controller Start to stop it\n", pid)
		return
	}
	if config.VTService && !config.VTMode {
		fmt.Fprintln(os.Stderr, "ttyosk: -vt-service=true requires -vt=true")
		os.Exit(2)
	}
	vtOverlay := config.VTMode && config.VTService
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	paneRows := rows / 2
	if paneRows < 1 {
		paneRows = 1
	}
	state := keyboardState{
		config:    config,
		numLock:   config.Keyboard.NumLock,
		vtOverlay: vtOverlay,
	}
	if !vtOverlay {
		state.terminal = vt10x.New(vt10x.WithSize(cols, paneRows))
	}

	outputChanged := make(chan struct{}, 1)
	if config.Keyboard.Output == "uinput" {
		keyboard, err := newVirtualKeyboard()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: create uinput keyboard: %v\n", err)
			os.Exit(1)
		}
		state.uinput = keyboard
		defer func() {
			if err := keyboard.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: close uinput keyboard: %v\n", err)
			}
		}()
		if !vtOverlay {
			_, _ = state.terminal.Write([]byte("uinput output active. Focus the target application to type."))
		}
	} else {
		ptmx, err := startShell(config.Shell.Command, config.Shell.Args, cols, paneRows)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: start shell: %v\n", err)
			os.Exit(1)
		}
		state.shell = ptmx
		state.ptmx = ptmx
		defer ptmx.Close()
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				_, _ = state.terminal.Write(buf[:n])
				select {
				case outputChanged <- struct{}{}:
				default:
				}
			}
		}()
	}

	var input <-chan []byte
	if !vtOverlay {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			panic(err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)
		if config.Mouse.SGR {
			fmt.Print("\033[?1000h\033[?1006h")
			if config.Mouse.HoverHighlight {
				fmt.Print("\033[?1003h")
			}
			defer fmt.Print("\033[?1003l\033[?1006l\033[?1000l")
		}
		fmt.Print("\033[2J")
		defer fmt.Print("\033[?25h\033[0m")

		inputChannel := make(chan []byte)
		input = inputChannel
		go func() {
			buf := make([]byte, 128)
			decoder := inputDecoder{}
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					for _, event := range decoder.feed(buf[:n]) {
						inputChannel <- event
					}
				}
				if err != nil {
					for _, event := range decoder.flush() {
						inputChannel <- event
					}
					return
				}
			}
		}()
	}
	var mouseEvents chan gpmEvent
	if config.Mouse.GPM && !vtOverlay {
		mouseEvents = make(chan gpmEvent, 16)
		gpmConn, err := openGPM(config.Mouse.HoverHighlight)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: GPM: %v\n", err)
			mouseEvents = nil
		} else if gpmConn != nil {
			defer gpmConn.Close()
			go readGPMEvents(gpmConn, mouseEvents)
		} else {
			mouseEvents = nil
		}
	}
	controllerEvents := make(chan gamepadEvent, 32)
	go watchGamepads(controllerEvents)

	var vtKeyboardFocus *vtFocus
	vtActive := true
	if vtOverlay {
		focus, supported, err := newVTFocus(int(os.Stdout.Fd()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
			return
		}
		if supported {
			vtKeyboardFocus = focus
			vtActive, err = vtKeyboardFocus.isActive()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: query active virtual terminal: %v\n", err)
				return
			}
		}
	}

	var physicalKeys <-chan physicalKeyEvent
	var physicalCapture *physicalKeyboardCapture
	if vtOverlay {
		keyEvents := make(chan physicalKeyEvent, 64)
		physicalCapture, err = startPhysicalKeyboardCapture(keyEvents, vtKeyboardFocus == nil || vtActive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: capture physical keyboard: %v\n", err)
			return
		}
		if vtKeyboardFocus == nil || vtActive {
			for _, device := range physicalCapture.files {
				fmt.Fprintf(os.Stderr, "ttyosk: grabbed physical keyboard %s\n", device.Name())
			}
		}
		defer func() {
			if err := physicalCapture.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: release physical keyboard: %v\n", err)
			}
		}()
		physicalKeys = keyEvents
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	var resizeTicker *time.Ticker
	var resizeTick <-chan time.Time
	if vtOverlay {
		defer func() {
			if err := state.restoreVTOverlay(); err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
			}
		}()
		time.Sleep(200 * time.Millisecond)
		if err := state.drawVTOverlay(); err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
			return
		}
		resizeTicker = time.NewTicker(time.Second)
		resizeTick = resizeTicker.C
		defer resizeTicker.Stop()
	}
	var focusTicker *time.Ticker
	var focusTick <-chan time.Time
	if vtKeyboardFocus != nil {
		focusTicker = time.NewTicker(50 * time.Millisecond)
		focusTick = focusTicker.C
		defer focusTicker.Stop()
	}

	updateVTFocus := func() bool {
		if vtKeyboardFocus == nil {
			return true
		}
		active, err := vtKeyboardFocus.isActive()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: query active virtual terminal: %v\n", err)
			return false
		}
		if active == vtActive {
			return true
		}
		if active {
			for len(physicalKeys) > 0 {
				<-physicalKeys
			}
			for len(controllerEvents) > 0 {
				<-controllerEvents
			}
		}
		if err := physicalCapture.SetGrabbed(active); err != nil {
			fmt.Fprintf(os.Stderr, "ttyosk: update physical keyboard capture: %v\n", err)
			return false
		}
		vtActive = active
		if active {
			for _, device := range physicalCapture.files {
				fmt.Fprintf(os.Stderr, "ttyosk: grabbed physical keyboard %s\n", device.Name())
			}
			if err := state.drawVTOverlay(); err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
				return false
			}
		} else {
			vtKeyboardFocus.resetModifiers()
			for _, device := range physicalCapture.files {
				fmt.Fprintf(os.Stderr, "ttyosk: released physical keyboard %s while tty is inactive\n", device.Name())
			}
		}
		return true
	}

	for {
		if !vtOverlay {
			state.draw()
		}
		select {
		case buf := <-input:
			if len(buf) == 1 && buf[0] == 27 {
				return
			}
			state.applyInput(buf, len(buf))
			state.reportError()
		case event, open := <-mouseEvents:
			if !open {
				mouseEvents = nil
				continue
			}
			if event.err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: GPM: %v\n", event.err)
				mouseEvents = nil
				continue
			}
			state.handleGPMEvent(event)
			state.reportError()
		case event := <-controllerEvents:
			if event.err != nil {
				fmt.Fprintf(os.Stderr, "ttyosk: controller: %v\n", event.err)
				continue
			}
			if !updateVTFocus() {
				return
			}
			if vtKeyboardFocus != nil && !vtActive {
				continue
			}
			if vtOverlay && event.exit {
				return
			}
			if event.dx != 0 || event.dy != 0 {
				state.move(event.dx, event.dy)
			}
			if event.selectKey {
				state.insertSelectedKey()
			}
			if event.backspace {
				state.emitToShell("\x7f")
			}
			state.reportError()
			if vtOverlay {
				if err := state.drawVTOverlay(); err != nil {
					fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
					return
				}
			}
		case event := <-physicalKeys:
			if !updateVTFocus() {
				return
			}
			if vtKeyboardFocus != nil && !vtActive {
				continue
			}
			if vtKeyboardFocus != nil {
				handled, err := vtKeyboardFocus.handleSwitchKey(event)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
				}
				if handled {
					continue
				}
			}
			if state.handlePhysicalKey(event) {
				return
			}
			state.reportError()
			if vtOverlay {
				if err := state.drawVTOverlay(); err != nil {
					fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
					return
				}
			}
		case <-focusTick:
			if !updateVTFocus() {
				return
			}
		case receivedSignal := <-signals:
			if receivedSignal != syscall.SIGWINCH {
				return
			}
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil && cols > 0 && rows > 0 {
				if vtOverlay {
					if cols != state.terminalCols || rows != state.vtRows {
						if err := state.restoreVTOverlay(); err != nil {
							fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
						}
						if err := state.drawVTOverlay(); err != nil {
							fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
							return
						}
					}
				} else {
					paneRows := rows / 2
					if paneRows < 1 {
						paneRows = 1
					}
					state.resizeTerminal(cols, paneRows)
				}
			}
		case <-resizeTick:
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil && cols > 0 && rows > 0 && (cols != state.terminalCols || rows != state.vtRows) {
				if err := state.restoreVTOverlay(); err != nil {
					fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
				}
				if err := state.drawVTOverlay(); err != nil {
					fmt.Fprintf(os.Stderr, "ttyosk: %v\n", err)
					return
				}
			}
		case <-outputChanged:
		}
	}
}

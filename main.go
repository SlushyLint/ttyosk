package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"golang.org/x/term"
)

var key = [][]string{
	{"esc", "`", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "-", "=", "del"},
	{"tab", "q", "w", "e", "r", "t", "y", "u", "i", "o", "p", "[", "]", "\\"},
	{"a", "s", "d", "f", "g", "h", "j", "k", "l", ";", "'", "return"},
	{"shift","z", "x", "c", "v", "b", "n", "m", ",", ".", "/", "shift"},
	{"ctrl", "alt", "SPACE", "alt", "ctrl", "←", "↓", "↑", "→"},
}

var shiftedKey = [][]string{
	{"esc", "~", "!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "_", "+", "del"},
	{"tab", "Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P", "{", "}", "|"},
	{"A", "S", "D", "F", "G", "H", "J", "K", "L", ":", "'", "return"},
	{"Z", "X", "C", "V", "B", "N", "M", "<", ">", "?", "shift"},
	{"ctrl", "alt", "SPACE", "alt", "ctrl", "←", "↓", "↑", "→"},
}

type keyboardState struct {
	x            int
	y            int
	modifier     string
	shell        io.Writer
	ptmx         *os.File
	terminal     vt10x.Terminal
	keyAreas     []keyHitArea
	terminalCols int
}

type keyHitArea struct {
	left  int
	right int
	x     int
	y     int
	row   int
}

func (s *keyboardState) move(dx, dy int) {
	layout := s.activeLayout()
	newY := s.y + dy
	if s.y < 0 || s.y >= len(layout) || newY < 0 || newY >= len(layout) {
		return
	}

	currentX := s.x
	if currentX < 0 {
		currentX = 0
	} else if currentX >= len(layout[s.y]) {
		currentX = len(layout[s.y]) - 1
	}
	newX := currentX
	if dy != 0 {
		cols := s.terminalCols
		if cols <= 0 {
			cols = 80
		}
		targetCenter := keyboardKeyCenter(layout[s.y], currentX, cols)
		bestDistance := int(^uint(0) >> 1)
		bestIndexDistance := bestDistance
		for candidate := range layout[newY] {
			center := keyboardKeyCenter(layout[newY], candidate, cols)
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
	if newX < 0 {
		newX = 0
	} else if newX >= len(layout[newY]) {
		newX = len(layout[newY]) - 1
	}

	s.x = newX
	s.y = newY
}

func keyboardKeyCenter(row []string, index, cols int) int {
	rowWidth := 0
	for _, keyName := range row {
		rowWidth += utf8.RuneCountInString(keyName) + 3
	}
	startX := (cols - rowWidth) / 2
	if startX < 1 {
		startX = 1
	}
	center := startX * 2
	for i, keyName := range row {
		width := utf8.RuneCountInString(keyName) + 3
		if i == index {
			return center + width
		}
		center += width * 2
	}
	return center
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (s *keyboardState) activeLayout() [][]string {
	if s.modifier == "shift" {
		return shiftedKey
	}
	return key
}

func (s *keyboardState) currentKey() string {
	layout := s.activeLayout()
	if s.y < 0 || s.y >= len(layout) {
		return ""
	}
	if s.x < 0 || s.x >= len(layout[s.y]) {
		return ""
	}
	return layout[s.y][s.x]
}

func (s *keyboardState) applyInput(buf []byte, n int) {
	if s.handleMouseClick(buf[:n]) {
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
		s.move(-1, 0)
	case 'j':
		s.move(0, 1)
	case 'k':
		s.move(0, -1)
	case 'l':
		s.move(1, 0)
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
	selected := s.currentKey()
	if selected == "shift" {
		if s.modifier == "shift" {
			s.modifier = ""
		} else {
			s.modifier = "shift"
		}
		return
	}
	if selected == "ctrl" || selected == "alt" {
		if s.modifier == selected {
			s.modifier = ""
		} else {
			s.modifier = selected
		}
		return
	}

	value := selected
	if s.modifier == "shift" && len(value) == 1 {
		value = shiftValue(value)
	}
	if s.modifier == "ctrl" && len(value) == 1 {
		upper := strings.ToUpper(value)
		if upper[0] >= '@' && upper[0] <= '_' {
			value = string(upper[0] & 0x1f)
		}
	}

	switch value {
	case "SPACE":
		s.emitToShell(" ")
	case "del":
		s.emitToShell("\x7f")
	case "rtrn":
		s.emitToShell("\r")
	case "esc":
		s.emitToShell("\x1b")
	case "tab":
		s.emitToShell("\t")
	case "left", "right", "up", "down", "←", "→", "↑", "↓":
		sequence := s.arrowSequence(value)
		if s.modifier == "alt" {
			sequence = "\x1b" + sequence
		}
		s.emitToShell(sequence)
	default:
		if value != "" {
			if s.modifier == "alt" {
				value = "\x1b" + value
			}
			s.emitToShell(value)
		}
	}
}

func (s *keyboardState) arrowSequence(keyName string) string {
	final := map[string]string{
		"up": "A", "↑": "A",
		"down": "B", "↓": "B",
		"right": "C", "→": "C",
		"left": "D", "←": "D",
	}[keyName]
	if final == "" {
		return ""
	}
	if s.terminal != nil {
		s.terminal.Lock()
		applicationCursor := s.terminal.Mode()&vt10x.ModeAppCursor != 0
		s.terminal.Unlock()
		if applicationCursor {
			return "\x1bO" + final
		}
	}
	return "\x1b[" + final
}

func (s *keyboardState) emitToShell(value string) {
	if s.shell == nil || value == "" {
		return
	}
	_, _ = io.WriteString(s.shell, value)
}

func (s *keyboardState) handleMouseClick(input []byte) bool {
	if len(input) < 7 || string(input[:3]) != "\033[<" || (input[len(input)-1] != 'M' && input[len(input)-1] != 'm') {
		return false
	}

	var button, mouseX, mouseY int
	if _, err := fmt.Sscanf(string(input[3:len(input)-1]), "%d;%d;%d", &button, &mouseX, &mouseY); err != nil {
		return true
	}

	if input[len(input)-1] == 'M' && button&3 == 0 {
		for _, area := range s.keyAreas {
			if area.y == mouseY && mouseX >= area.left && mouseX <= area.right {
				s.x, s.y = area.x, area.row
				s.insertSelectedKey()
				return true
			}
		}
	}
	if s.terminal != nil {
		s.terminal.Lock()
		mouseReporting := s.terminal.Mode()&vt10x.ModeMouseMask != 0
		s.terminal.Unlock()
		if mouseReporting {
			s.emitToShell(string(input))
		}
	}
	return true
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

	visibleLines := rows / 2
	if visibleLines < 1 {
		visibleLines = 1
	}
	s.resizeTerminal(cols, visibleLines)
	var frame strings.Builder
	frame.WriteString("\033[?25l")
	s.drawTerminal(&frame, cols, visibleLines)

	keyboardStartY := rows - len(layout) + 1

	s.keyAreas = nil
	for rowIndex, row := range layout {
		var line strings.Builder
		cells := make([]string, len(row))
		lineWidth := 0
		for col, keyName := range row {
			if rowIndex == s.y && col == s.x {
				cells[col] = fmt.Sprintf("[%s] ", keyName)
			} else {
				cells[col] = fmt.Sprintf(" %s  ", keyName)
			}
			line.WriteString(cells[col])
			lineWidth += utf8.RuneCountInString(cells[col])
		}
		startX := (cols - lineWidth) / 2
		if startX < 1 {
			startX = 1
		}
		cellX := startX
		keyboardY := keyboardStartY + rowIndex
		frame.WriteString(fmt.Sprintf("\033[%d;1H\033[2K", keyboardY))
		frame.WriteString(fmt.Sprintf("\033[%d;%dH", keyboardY, startX))
		for col, cell := range cells {
			s.keyAreas = append(s.keyAreas, keyHitArea{
				left:  cellX,
				right: cellX + utf8.RuneCountInString(cell) - 1,
				x:     col,
				y:     keyboardStartY + rowIndex,
				row:   rowIndex,
			})
			cellX += utf8.RuneCountInString(cell)
		}
		frame.WriteString(line.String())
	}

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

func startShell(cols, rows int) (*os.File, error) {
	cmd := exec.Command("zsh")
	cmd.Env = os.Environ()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return ptmx, nil
}

func main() {
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
	state := keyboardState{terminal: vt10x.New(vt10x.WithSize(cols, paneRows))}

	outputChanged := make(chan struct{}, 1)
	ptmx, err := startShell(cols, paneRows)
	if err == nil {
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

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)
	fmt.Print("\033[?1000h\033[?1006h")
	fmt.Print("\033[2J")
	defer fmt.Print("\033[?1006l\033[?1000l")
	defer fmt.Print("\033[?25h\033[0m")

	input := make(chan []byte)
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				input <- append([]byte(nil), buf[:n]...)
			}
		}
	}()
	resized := make(chan os.Signal, 1)
	signal.Notify(resized, syscall.SIGWINCH)
	defer signal.Stop(resized)

	for {
		state.draw()
		select {
		case buf := <-input:
			if len(buf) == 1 && buf[0] == 27 {
				return
			}
			state.applyInput(buf, len(buf))
		case <-resized:
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err == nil && cols > 0 && rows > 0 {
				paneRows := rows / 2
				if paneRows < 1 {
					paneRows = 1
				}
				state.resizeTerminal(cols, paneRows)
			}
		case <-outputChanged:
		}
	}
}

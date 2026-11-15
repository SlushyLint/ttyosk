package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func (s *keyboardState) pasteClipboard() {
	if !s.config.Clipboard.Enabled {
		return
	}
	command, args, err := s.pasteCommand()
	if err != nil {
		s.lastError = err
		return
	}
	output, err := exec.Command(command, args...).Output()
	if err != nil {
		s.lastError = fmt.Errorf("read clipboard: %w", err)
		return
	}
	s.emitToShell(string(output))
}

func (s *keyboardState) copySelection() {
	text := s.selectedText()
	if text == "" {
		return
	}
	if command, args, ok := s.copyCommand(); ok {
		cmd := exec.Command(command, args...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			s.lastError = fmt.Errorf("write clipboard: %w", err)
		}
		return
	}
	payload := base64.StdEncoding.EncodeToString([]byte(text))
	_, err := fmt.Fprintf(os.Stdout, "\033]52;c;%s\a", payload)
	if err != nil {
		s.lastError = fmt.Errorf("copy selection: %w", err)
	}
}

func (s *keyboardState) copyCommand() (string, []string, bool) {
	configured := s.config.Clipboard.CopyCommand
	if len(configured) > 0 {
		return configured[0], configured[1:], true
	}
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, []string{"pbcopy"})
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, []string{"wl-copy"})
		}
		candidates = append(candidates, []string{"xclip", "-selection", "clipboard"})
		candidates = append(candidates, []string{"xsel", "--clipboard", "--input"})
	}
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate[0]); err == nil {
			return candidate[0], candidate[1:], true
		}
	}
	return "", nil, false
}

func (s *keyboardState) pasteCommand() (string, []string, error) {
	configured := s.config.Clipboard.PasteCommand
	if len(configured) > 0 {
		return configured[0], configured[1:], nil
	}

	candidates := make([][]string, 0, 3)
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, []string{"pbpaste"})
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, []string{"wl-paste", "--no-newline"})
		}
		candidates = append(candidates, []string{"xclip", "-selection", "clipboard", "-o"})
		candidates = append(candidates, []string{"xsel", "--clipboard", "--output"})
	}
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate[0]); err == nil {
			return candidate[0], candidate[1:], nil
		}
	}
	return "", nil, errors.New("no clipboard reader found; set clipboard.paste_command in the config")
}

func (s *keyboardState) selectedText() string {
	if s.terminal == nil || !s.selection.active && s.selection.startX == 0 {
		return ""
	}
	cols, rows := s.terminal.Size()
	startX, startY := s.selection.startX, s.selection.startY
	endX, endY := s.selection.endX, s.selection.endY
	if startY > endY || startY == endY && startX > endX {
		startX, endX = endX, startX
		startY, endY = endY, startY
	}
	startX = clampInt(startX, 1, cols)
	endX = clampInt(endX, 1, cols)
	startY = clampInt(startY, 1, rows)
	endY = clampInt(endY, 1, rows)

	s.terminal.Lock()
	defer s.terminal.Unlock()
	var lines []string
	for y := startY; y <= endY; y++ {
		fromX, toX := 1, cols
		if y == startY {
			fromX = startX
		}
		if y == endY {
			toX = endX
		}
		var line strings.Builder
		for x := fromX; x <= toX; x++ {
			glyph := s.terminal.Cell(x-1, y-1)
			if glyph.Char == 0 {
				line.WriteByte(' ')
			} else {
				line.WriteRune(glyph.Char)
			}
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	return strings.Join(lines, "\n")
}

func (s *keyboardState) selectionContains(x, y int) bool {
	if s.selection.startX == 0 || s.selection.startY == 0 {
		return false
	}
	startX, startY := s.selection.startX, s.selection.startY
	endX, endY := s.selection.endX, s.selection.endY
	if startY > endY || startY == endY && startX > endX {
		startX, endX = endX, startX
		startY, endY = endY, startY
	}
	if y < startY || y > endY {
		return false
	}
	if y == startY && x < startX {
		return false
	}
	if y == endY && x > endX {
		return false
	}
	return true
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (s *keyboardState) reportError() {
	if s.lastError == nil {
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, "ttyosk:", s.lastError)
	s.lastError = nil
}

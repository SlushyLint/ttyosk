package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type appConfig struct {
	Shell     shellConfig     `json:"shell"`
	Keyboard  keyboardConfig  `json:"keyboard"`
	Mouse     mouseConfig     `json:"mouse"`
	Clipboard clipboardConfig `json:"clipboard"`
}

type shellConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type keyboardConfig struct {
	Output          string     `json:"output"`
	Layout          string     `json:"layout"`
	CustomLayout    [][]string `json:"custom_layout"`
	Extended        bool       `json:"extended"`
	NumLock         bool       `json:"num_lock"`
	StickyModifiers bool       `json:"sticky_modifiers"`
	KeyPadding      int        `json:"key_padding"`
	VimNavigation   bool       `json:"vim_navigation"`
	HighContrast    bool       `json:"high_contrast"`
	Colors          colorTheme `json:"colors"`
}

type mouseConfig struct {
	SGR              bool   `json:"sgr"`
	GPM              bool   `json:"gpm"`
	HoverHighlight   bool   `json:"hover_highlight"`
	RightClickAction string `json:"right_click_action"`
}

type clipboardConfig struct {
	Enabled      bool     `json:"enabled"`
	PasteCommand []string `json:"paste_command"`
	CopyCommand  []string `json:"copy_command"`
}

type colorTheme struct {
	KeyForeground      int `json:"key_foreground"`
	KeyBackground      int `json:"key_background"`
	SelectedForeground int `json:"selected_foreground"`
	SelectedBackground int `json:"selected_background"`
	ModifierForeground int `json:"modifier_foreground"`
	ModifierBackground int `json:"modifier_background"`
}

func defaultConfig() appConfig {
	return appConfig{
		Shell: shellConfig{Command: "zsh"},
		Keyboard: keyboardConfig{
			Output:        "shell",
			Layout:        "qwerty",
			NumLock:       true,
			KeyPadding:    1,
			VimNavigation: true,
			Colors: colorTheme{
				KeyForeground:      -1,
				KeyBackground:      -1,
				SelectedForeground: 0,
				SelectedBackground: 7,
				ModifierForeground: 7,
				ModifierBackground: 4,
			},
		},
		Mouse: mouseConfig{
			SGR:              true,
			GPM:              true,
			HoverHighlight:   true,
			RightClickAction: "none",
		},
	}
}

func defaultConfigPath() (string, error) {
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return filepath.Join(configHome, "ttyosk", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(home, ".config", "ttyosk", "config.json"), nil
}

func loadConfig(args []string) (appConfig, error) {
	flags := flag.NewFlagSet("ttyosk", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "path to JSON configuration file")
	if err := flags.Parse(args); err != nil {
		return appConfig{}, err
	}
	if flags.NArg() != 0 {
		return appConfig{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	config := defaultConfig()
	path := *configPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return appConfig{}, err
		}
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && *configPath == "" {
		return config, nil
	}
	if err != nil {
		return appConfig{}, fmt.Errorf("read config %q: %w", path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return appConfig{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return appConfig{}, fmt.Errorf("parse config %q: multiple JSON values", path)
		}
		return appConfig{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := config.validate(); err != nil {
		return appConfig{}, fmt.Errorf("invalid config %q: %w", path, err)
	}
	return config, nil
}

func (c appConfig) validate() error {
	if c.Keyboard.Output != "uinput" && strings.TrimSpace(c.Shell.Command) == "" {
		return errors.New("shell.command must not be empty")
	}
	if c.Keyboard.Output != "shell" && c.Keyboard.Output != "uinput" {
		return errors.New("keyboard.output must be shell or uinput")
	}
	switch c.Keyboard.Layout {
	case "qwerty", "dvorak", "colemak":
	case "custom":
		if len(c.Keyboard.CustomLayout) == 0 {
			return errors.New("keyboard.custom_layout is required when layout is custom")
		}
		for rowIndex, row := range c.Keyboard.CustomLayout {
			if len(row) == 0 {
				return fmt.Errorf("keyboard.custom_layout row %d is empty", rowIndex)
			}
			for _, key := range row {
				if !validCustomKey(key) {
					return fmt.Errorf("unsupported key %q in keyboard.custom_layout", key)
				}
			}
		}
	default:
		return fmt.Errorf("keyboard.layout must be qwerty, dvorak, colemak, or custom")
	}
	if c.Keyboard.KeyPadding < 0 || c.Keyboard.KeyPadding > 4 {
		return errors.New("keyboard.key_padding must be between 0 and 4")
	}
	if c.Mouse.RightClickAction != "none" && c.Mouse.RightClickAction != "backspace" {
		return errors.New("mouse.right_click_action must be none or backspace")
	}
	for _, command := range [][]string{c.Clipboard.PasteCommand, c.Clipboard.CopyCommand} {
		if len(command) > 0 && strings.TrimSpace(command[0]) == "" {
			return errors.New("clipboard command names must not be empty")
		}
	}
	for _, color := range []int{
		c.Keyboard.Colors.KeyForeground,
		c.Keyboard.Colors.KeyBackground,
		c.Keyboard.Colors.SelectedForeground,
		c.Keyboard.Colors.SelectedBackground,
		c.Keyboard.Colors.ModifierForeground,
		c.Keyboard.Colors.ModifierBackground,
	} {
		if color < -1 || color > 255 {
			return errors.New("keyboard.colors values must be between -1 and 255")
		}
	}
	return nil
}

func validCustomKey(key string) bool {
	switch key {
	case "esc", "del", "tab", "caps", "return", "rtrn", "shift", "ctrl", "meta", "SPACE",
		"enter", "ins", "delete", "home", "end", "pgup", "pgdn", "paste",
		"num", "kp0", "kp0wide", "kp1", "kp2", "kp3", "kp4", "kp5", "kp6", "kp7", "kp8", "kp9", "kp.",
		"←", "→", "↑", "↓", "left", "right", "up", "down":
		return true
	}
	if strings.HasPrefix(key, "F") {
		number, err := strconv.Atoi(strings.TrimPrefix(key, "F"))
		if err == nil && number >= 1 && number <= 12 {
			return true
		}
	}
	return utf8.RuneCountInString(key) == 1
}

func highContrastTheme() colorTheme {
	return colorTheme{
		KeyForeground:      15,
		KeyBackground:      0,
		SelectedForeground: 0,
		SelectedBackground: 15,
		ModifierForeground: 15,
		ModifierBackground: 4,
	}
}

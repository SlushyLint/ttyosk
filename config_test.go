package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigUsesDefaultsWhenDefaultFileIsMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	config, err := loadConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Shell.Command != "zsh" || config.Keyboard.Layout != "qwerty" {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	if !config.Mouse.SGR || !config.Mouse.GPM || !config.Mouse.HoverHighlight {
		t.Fatalf("expected mouse reporting enabled by default, got %+v", config.Mouse)
	}
}

func TestLoadConfigMergesPartialJSONWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"keyboard":{"layout":"dvorak"},"mouse":{"sgr":false}}`), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Keyboard.Layout != "dvorak" || config.Keyboard.KeyPadding != 1 {
		t.Fatalf("expected partial config to retain defaults, got %+v", config.Keyboard)
	}
	if config.Keyboard.Output != "shell" {
		t.Fatalf("expected shell output by default, got %q", config.Keyboard.Output)
	}
	if config.Mouse.SGR || !config.Mouse.GPM {
		t.Fatalf("expected only SGR to be disabled, got %+v", config.Mouse)
	}
}

func TestLoadConfigRejectsUnknownFieldsAndInvalidValues(t *testing.T) {
	for _, contents := range []string{
		`{"unknown":true}`,
		`{"keyboard":{"layout":"unknown"}}`,
		`{"keyboard":{"output":"unknown"}}`,
		`{"mouse":{"right_click_action":"execute"}}`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig([]string{"--config", path}); err == nil {
			t.Fatalf("expected config %s to be rejected", contents)
		}
	}
}

func TestLoadConfigEnablesUinputOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"keyboard":{"output":"uinput"},"shell":{"command":""}}`), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	if config.Keyboard.Output != "uinput" {
		t.Fatalf("expected uinput output, got %q", config.Keyboard.Output)
	}
}

func TestLoadConfigRequiresExplicitFileToExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, err := loadConfig([]string{"--config", path}); err == nil {
		t.Fatal("expected missing explicit config file to return an error")
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	if _, err := loadConfig([]string{"--config", "config.example.json"}); err != nil {
		t.Fatalf("expected example config to be valid: %v", err)
	}
}

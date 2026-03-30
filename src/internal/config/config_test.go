package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.General.RefreshInterval != 5 {
		t.Errorf("RefreshInterval = %d, want 5", d.General.RefreshInterval)
	}
	if d.General.CheckForUpdates != true {
		t.Error("CheckForUpdates should default to true")
	}
	if d.General.IgnoreSSHHostKeys {
		t.Error("IgnoreSSHHostKeys should default to false")
	}
	if d.Colors.Primary != "#7D56F4" {
		t.Errorf("Primary = %s, want #7D56F4", d.Colors.Primary)
	}
	if d.Colors.Muted != "#657B83" {
		t.Errorf("Muted = %s, want #657B83", d.Colors.Muted)
	}
	if d.Keybindings["quit"] != "q,ctrl+c" {
		t.Errorf("quit binding = %s, want q,ctrl+c", d.Keybindings["quit"])
	}
	if d.Keybindings["config"] != "ctrl+k" {
		t.Errorf("config binding = %s, want ctrl+k", d.Keybindings["config"])
	}
	if d.Keybindings["assign_fip"] != "ctrl+u" {
		t.Errorf("assign_fip binding = %s, want ctrl+u", d.Keybindings["assign_fip"])
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Defaults()
	cfg.General.RefreshInterval = 10
	cfg.Colors.Primary = "#FF0000"
	cfg.Keybindings["quit"] = "ctrl+q"

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.General.RefreshInterval != 10 {
		t.Errorf("RefreshInterval = %d, want 10", loaded.General.RefreshInterval)
	}
	if loaded.Colors.Primary != "#FF0000" {
		t.Errorf("Primary = %s, want #FF0000", loaded.Colors.Primary)
	}
	if loaded.Keybindings["quit"] != "ctrl+q" {
		t.Errorf("quit = %s, want ctrl+q", loaded.Keybindings["quit"])
	}
	// Unmodified defaults should still be present
	if loaded.Keybindings["help"] != "?" {
		t.Errorf("help = %s, want ?", loaded.Keybindings["help"])
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if cfg.General.RefreshInterval != 5 {
		t.Errorf("RefreshInterval = %d, want default 5", cfg.General.RefreshInterval)
	}
}

func TestMergeCLIFlags(t *testing.T) {
	cfg := Defaults()

	refresh := 10 * time.Second
	plain := true
	flags := CLIFlags{
		RefreshInterval: &refresh,
		PlainMode:       &plain,
	}

	merged := Merge(cfg, flags)
	if merged.General.RefreshInterval != 10 {
		t.Errorf("RefreshInterval = %d, want 10", merged.General.RefreshInterval)
	}
	if !merged.General.PlainMode {
		t.Error("PlainMode should be true from CLI flag")
	}
	// Unset flags should keep file values
	if !merged.General.CheckForUpdates {
		t.Error("CheckForUpdates should remain true (not overridden)")
	}
}

func TestMergeWithDefaults(t *testing.T) {
	// Simulate a partial config file (only general section set)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("general:\n  refresh_interval: 15\n"), 0o644)

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.General.RefreshInterval != 15 {
		t.Errorf("RefreshInterval = %d, want 15", loaded.General.RefreshInterval)
	}
	// Bool defaults should be preserved when absent from file
	if !loaded.General.CheckForUpdates {
		t.Error("CheckForUpdates should default to true when absent from config file")
	}
	// Colors should be filled from defaults
	if loaded.Colors.Primary != "#7D56F4" {
		t.Errorf("Primary = %s, want default #7D56F4", loaded.Colors.Primary)
	}
	// Keybindings should be filled from defaults
	if loaded.Keybindings["quit"] != "q,ctrl+c" {
		t.Errorf("quit = %s, want default q,ctrl+c", loaded.Keybindings["quit"])
	}
}

func TestBoolExplicitFalse(t *testing.T) {
	// When a bool is explicitly set to false in the file, it should stay false
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("general:\n  check_for_updates: false\n"), 0o644)

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.General.CheckForUpdates {
		t.Error("CheckForUpdates should be false when explicitly set to false")
	}
}

func TestLoadIgnoreSSHHostKeysExplicitTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("general:\n  ignore_ssh_host_keys: true\n"), 0o644)

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.General.IgnoreSSHHostKeys {
		t.Error("IgnoreSSHHostKeys should be true when explicitly set")
	}
}

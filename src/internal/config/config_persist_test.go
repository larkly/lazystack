package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadAuditAndSavedFilters(t *testing.T) {
	for _, enabled := range []string{"true", "false"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		data := "audit:\n  enabled: " + enabled + "\n" +
			"saved_filters:\n  - name: web\n    pattern: \"name:web-*\"\n  - name: errors\n    pattern: \"status:ERROR\"\n"
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(path)
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		if got := cfg.Audit.Enabled; got != (enabled == "true") {
			t.Errorf("Audit.Enabled = %v, want %s", got, enabled)
		}
		want := []SavedFilter{{Name: "web", Pattern: "name:web-*"}, {Name: "errors", Pattern: "status:ERROR"}}
		if !reflect.DeepEqual(cfg.SavedFilters, want) {
			t.Errorf("SavedFilters = %#v, want %#v", cfg.SavedFilters, want)
		}
	}
}

func TestLoadRejectsReservedKeybindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "keybindings:\n  attach: \"i, ctrl+a\"\n  detach: \"x, ctrl+b\"\n  refresh: F5\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	d := DefaultKeybindings()
	if cfg.Keybindings["attach"] != d["attach"] {
		t.Errorf("attach = %q, want default %q", cfg.Keybindings["attach"], d["attach"])
	}
	if cfg.Keybindings["detach"] != d["detach"] {
		t.Errorf("detach = %q, want default %q", cfg.Keybindings["detach"], d["detach"])
	}
	if cfg.Keybindings["refresh"] != "F5" {
		t.Errorf("refresh = %q, want F5 (unaffected)", cfg.Keybindings["refresh"])
	}
	if len(cfg.Warnings) != 2 {
		t.Fatalf("Warnings = %q, want one per rejected binding", cfg.Warnings)
	}
	for _, w := range cfg.Warnings {
		if !strings.Contains(w, "reserved") {
			t.Errorf("warning %q does not explain the rejection", w)
		}
	}
}

// A v0.11.0 config holds the whole keybinding map as it was then, including
// defaults that have since changed. Those stale defaults must give way to the
// current ones silently, and saving must make the migration stick.
func TestLoadMigratesLegacyDefaultKeybindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "keybindings:\n  attach: ctrl+a\n  column_pick: ctrl+shift+c\n  refresh: F5\n  quit: \"q,ctrl+c\"\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	d := DefaultKeybindings()
	for _, name := range []string{"attach", "column_pick"} {
		if cfg.Keybindings[name] != d[name] {
			t.Errorf("%s = %q, want current default %q", name, cfg.Keybindings[name], d[name])
		}
	}
	if cfg.Keybindings["refresh"] != "F5" {
		t.Errorf("refresh = %q, want F5 (user choice kept)", cfg.Keybindings["refresh"])
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %q, want none for a migrated legacy default", cfg.Warnings)
	}

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"ctrl+a", "ctrl+shift+c"} {
		if strings.Contains(string(saved), legacy) {
			t.Errorf("saved config still contains legacy %q:\n%s", legacy, saved)
		}
	}
	again, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("second LoadFrom: %v", err)
	}
	if !reflect.DeepEqual(again.Keybindings, cfg.Keybindings) || len(again.Warnings) != 0 {
		t.Errorf("reload after save: keybindings %v warnings %q, want %v and none", again.Keybindings, again.Warnings, cfg.Keybindings)
	}
}

// Only the exact legacy default is migrated; any other value is the user's.
func TestLoadKeepsNonLegacyKeybindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "keybindings:\n  column_pick: \"ctrl+shift+c, K\"\n  attach: I\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got := cfg.Keybindings["column_pick"]; got != "ctrl+shift+c, K" {
		t.Errorf("column_pick = %q, want the user's value", got)
	}
	if got := cfg.Keybindings["attach"]; got != "I" {
		t.Errorf("attach = %q, want the user's value", got)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %q, want none", cfg.Warnings)
	}
}

func TestLoadSaveLoadPreservesAuditAndSavedFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Defaults()
	cfg.Audit.Enabled = true
	cfg.SavedFilters = []SavedFilter{{Name: "prod", Pattern: "name:prod"}}
	cfg.General.RefreshInterval = 17
	cfg.Colors.Primary = "#123456"
	cfg.Columns = []ColumnConfig{{Key: "name"}, {Key: "status", Hidden: true}}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if err := loaded.SaveTo(path); err != nil {
		t.Fatalf("second SaveTo: %v", err)
	}
	again, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("second LoadFrom: %v", err)
	}
	if !reflect.DeepEqual(again, cfg) {
		t.Errorf("load-save-load mismatch:\n got  %#v\n want %#v", again, cfg)
	}
}

func TestSaveToTightensExistingPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("general: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestSaveToNewFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	cfg := Defaults()
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
}

func TestSaveToSurfacesChmodErrorAndKeepsContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := []byte("general:\n  refresh_interval: 9\n")
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := chmodConfigFile
	defer func() { chmodConfigFile = prev }()
	boom := errors.New("chmod denied")
	chmodConfigFile = func(*os.File, os.FileMode) error { return boom }

	cfg := Defaults()
	if err := cfg.SaveTo(path); !errors.Is(err, boom) {
		t.Fatalf("SaveTo error = %v, want %v", err, boom)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(orig) {
		t.Errorf("config content changed after failed chmod: %q", got)
	}
}

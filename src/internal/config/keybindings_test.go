package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/larkly/lazystack/internal/shared"
)

// formActionDefaults are the bindings that replaced hard-coded key literals
// in the forms, the LB and image views, the error modal and the small
// modals. Their defaults are the keys those literals used.
var formActionDefaults = map[string]string{
	"submit":                "ctrl+s",
	"health_monitor":        "ctrl+h",
	"drain_member":          "w",
	"select_all":            "x",
	"download":              "ctrl+g",
	"background":            "b",
	"toggle_details":        "d,D",
	"metadata_add":          "a",
	"metadata_edit":         "e",
	"metadata_delete":       "d",
	"save_key":              "s",
	"jump_start":            "g",
	"jump_end":              "G",
	"audit_filter_action":   "a",
	"audit_filter_resource": "r",
	"audit_filter_date":     "d",
	"audit_filter_clear":    "c",
	"copy_value":            "c",
	"reveal":                "r",
}

func TestFormActionKeybindingDefaults(t *testing.T) {
	d := DefaultKeybindings()
	for name, want := range formActionDefaults {
		if got, ok := d[name]; !ok || got != want {
			t.Errorf("default %q = %q (present %v), want %q", name, got, ok, want)
		}
	}
}

// Applying the default map must reproduce shared.Keys exactly, so the
// config defaults and the compiled-in bindings cannot drift apart.
func TestDefaultKeybindingsReproduceSharedKeys(t *testing.T) {
	prev := shared.Keys
	defer func() { shared.Keys = prev }()

	ApplyKeybindings(DefaultKeybindings())
	before := reflect.ValueOf(prev)
	after := reflect.ValueOf(shared.Keys)
	for i := 0; i < before.NumField(); i++ {
		name := before.Type().Field(i).Name
		want := before.Field(i).Interface().(key.Binding).Keys()
		got := after.Field(i).Interface().(key.Binding).Keys()
		if !slices.Equal(got, want) {
			t.Errorf("%s: config default keys %q, shared.Keys has %q", name, got, want)
		}
	}
}

func TestApplyKeybindings_FormActionsRebindable(t *testing.T) {
	prev := shared.Keys
	defer func() { shared.Keys = prev }()

	ApplyKeybindings(map[string]string{
		"submit":         "f2",
		"health_monitor": "M",
		"toggle_details": "i",
	})
	cases := []struct {
		name    string
		binding key.Binding
		key     string
		desc    string
	}{
		{"submit", shared.Keys.Submit, "f2", "submit"},
		{"health_monitor", shared.Keys.HealthMonitor, "M", "health monitor"},
		{"toggle_details", shared.Keys.ToggleDetails, "i", "details"},
	}
	for _, c := range cases {
		h := c.binding.Help()
		if h.Key != c.key || h.Desc != c.desc {
			t.Errorf("%s help = %q/%q, want %q/%q", c.name, h.Key, h.Desc, c.key, c.desc)
		}
	}
}

func TestLoadRejectsReservedKeysForFormActions(t *testing.T) {
	names := make([]string, 0, len(formActionDefaults))
	for name := range formActionDefaults {
		names = append(names, name)
	}
	sort.Strings(names)

	var yaml strings.Builder
	yaml.WriteString("keybindings:\n")
	for i, name := range names {
		reserved := "ctrl+a"
		if i%2 == 1 {
			reserved = "f3, ctrl+b"
		}
		fmt.Fprintf(&yaml, "  %s: %q\n", name, reserved)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	for _, name := range names {
		if got := cfg.Keybindings[name]; got != formActionDefaults[name] {
			t.Errorf("%s = %q, want default %q", name, got, formActionDefaults[name])
		}
	}
	if len(cfg.Warnings) != len(names) {
		t.Fatalf("Warnings = %d, want one per rejected binding (%d)", len(cfg.Warnings), len(names))
	}
	for _, w := range cfg.Warnings {
		if !strings.Contains(w, "reserved") {
			t.Errorf("warning %q does not explain the rejection", w)
		}
	}
}

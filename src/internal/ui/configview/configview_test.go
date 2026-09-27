package configview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func selectItem(t *testing.T, m *Model, k string) {
	t.Helper()
	for i := 0; i < m.totalItems(); i++ {
		m.cursor = i
		if m.currentItem().key == k {
			return
		}
	}
	t.Fatalf("item %q missing", k)
}

func TestKeybindingCaptureRejectsReservedAndSupportsCancelAndSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defaults := config.Defaults()
	t.Cleanup(func() { config.ApplyAll(defaults) })
	m := New(nil)
	m.Open()
	selectItem(t, &m, "help")
	original := m.cfg.Keybindings["help"]
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.keyCapture {
		t.Fatal("enter did not start capture")
	}
	for _, r := range []rune{'a', 'b'} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl}))
		if !m.keyCapture || !strings.Contains(m.errMsg, "reserved key") || m.cfg.Keybindings["help"] != original || cmd != nil {
			t.Fatal("reserved key changed binding or ended capture")
		}
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.keyCapture || !m.Visible || m.errMsg != "" {
		t.Fatal("cancel should close capture only")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"}))
	if m.keyCapture || m.errMsg != "" || m.cfg.Keybindings["help"] != "z" || cmd == nil {
		t.Fatalf("capture failed: %q", m.errMsg)
	}
	if _, ok := cmd().(shared.ConfigChangedMsg); !ok {
		t.Fatal("missing config changed message")
	}
	loaded, err := config.Load()
	if err != nil || loaded.Keybindings["help"] != "z" {
		t.Fatalf("saved binding not persisted: %v", err)
	}
}

func TestKeybindingOrderCoversEveryDefault(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range keybindingOrder() {
		listed[name] = true
	}
	for name := range config.DefaultKeybindings() {
		if !listed[name] {
			t.Errorf("keybinding %q is not editable in the config view", name)
		}
	}
}

func TestKeybindingCaptureAcceptsAssignFIPDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults := config.Defaults()
	t.Cleanup(func() { config.ApplyAll(defaults) })
	m := New(nil)
	m.Open()
	selectItem(t, &m, "assign_fip")
	m.cfg.Keybindings["assign_fip"] = "F7"
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	if m.keyCapture || m.errMsg != "" || m.cfg.Keybindings["assign_fip"] != "ctrl+u" {
		t.Fatalf("restoring the ctrl+u default was rejected: %q", m.errMsg)
	}
}

func TestNumericAndColorValidation(t *testing.T) {
	m := New(nil)
	for _, tc := range []struct{ key, invalid, valid string }{{"refresh_interval", "0", "12"}, {"idle_timeout", "-1", "0"}} {
		selectItem(t, &m, tc.key)
		item := m.currentItem()
		before := item.get()
		for _, invalid := range []string{tc.invalid, "abc"} {
			if item.set(invalid) == nil || item.get() != before {
				t.Fatalf("%s accepted %q", tc.key, invalid)
			}
		}
		if err := item.set(tc.valid); err != nil || item.get() != tc.valid {
			t.Fatalf("%s rejected valid value", tc.key)
		}
	}
	for _, item := range m.buildColorItems() {
		before := item.get()
		for _, bad := range []string{"red", "#abc", "#12345g", "123456", "#1234567"} {
			if item.set(bad) == nil || item.get() != before {
				t.Fatalf("%s accepted %q", item.label, bad)
			}
		}
		if item.set("#aB12Ef") != nil || item.get() != "#aB12Ef" {
			t.Fatal("valid hex rejected")
		}
	}
	for _, item := range m.buildKeybindingItems() {
		for _, bad := range []string{"ctrl+a", "ctrl+b", "q,ctrl+b"} {
			before := item.get()
			if item.set(bad) == nil || item.get() != before {
				t.Fatal("setter bypasses reserved validation")
			}
		}
	}
}

func TestEditingErrorsNavigationAndRender(t *testing.T) {
	m := New(nil)
	m.Open()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m.textInput.SetValue("not a number")
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.editing || cmd != nil || !strings.Contains(m.Render(), "positive integer") {
		t.Fatal("invalid edit must remain open with rendered error")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.editing || m.errMsg != "" || !m.Visible {
		t.Fatal("edit cancel")
	}
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	}
	if m.cursor != m.totalItems()-1 || m.scroll == 0 {
		t.Fatal("page down not clamped/scrolled")
	}
	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	}
	if m.cursor != 0 {
		t.Fatal("page up not clamped")
	}
	if !strings.Contains(m.Render(), "Configuration") {
		t.Fatal("missing title")
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.Visible {
		t.Fatal("escape did not close")
	}
}

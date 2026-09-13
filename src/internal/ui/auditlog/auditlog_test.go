package auditlog

import (
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
	"time"
)

func TestFiltersNavigationAndBack(t *testing.T) {
	entries := []audit.Entry{{Action: "create", ResourceType: "server", ResourceName: "web", Result: "success", Timestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}, {Action: "delete", ResourceType: "volume", ResourceName: "disk", Result: "error", Timestamp: time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)}}
	for _, tc := range []struct {
		key   rune
		query string
		name  string
	}{{'a', "CRE", "web"}, {'r', "vol", "disk"}, {'d', "2025", "disk"}} {
		m := New()
		m.SetSize(100, 8)
		m.SetEntries(entries)
		if m.Init() == nil || !strings.Contains(m.Hints(), "esc") {
			t.Fatal("init/hints")
		}
		for i := 0; i < 3; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		if m.cursor != 1 || m.scroll != 1 {
			t.Fatal("scroll")
		}
		for i := 0; i < 3; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		}
		if m.cursor != 0 || m.scroll != 0 {
			t.Fatal("upper bound")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tc.key, Text: string(tc.key)})
		for _, r := range tc.query {
			m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if len(m.filtered) != 1 || m.filtered[0].ResourceName != tc.name || m.cursor != 0 || m.scroll != 0 {
			t.Fatalf("filtered=%+v", m.filtered)
		}
		if !strings.Contains(m.View(), tc.name) {
			t.Fatal(m.View())
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.filterMode != FilterNone || len(m.filtered) != 1 {
			t.Fatal("confirm filter")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
		if len(m.filtered) != 2 {
			t.Fatal("clear filter")
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd == nil || cmd().(shared.ViewChangeMsg).View != "serverlist" {
			t.Fatal("back")
		}
	}
}
func TestFilterEditingErrorAndEmpty(t *testing.T) {
	m := New()
	m.SetSize(100, 20)
	m.SetEntries([]audit.Entry{{Action: "create", ResourceName: "web"}})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !strings.Contains(m.View(), "No entries") {
		t.Fatal(m.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if len(m.filtered) != 1 {
		t.Fatal("backspace")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if m.filterInput != "" || len(m.filtered) != 1 {
		t.Fatal("clear input")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.filterMode != FilterNone {
		t.Fatal("cancel filter")
	}
	m.SetError("read denied")
	if !strings.Contains(m.View(), "read denied") || m.loading {
		t.Fatal("error state")
	}
}

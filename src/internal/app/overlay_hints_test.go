package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// statusLine returns the last rendered line with styling removed.
func statusLine(t *testing.T, m Model) string {
	t.Helper()
	lines := strings.Split(m.viewContent(), "\n")
	if len(lines) != m.height {
		t.Errorf("screen has %d lines, want %d", len(lines), m.height)
	}
	return strings.TrimSpace(ansi.Strip(lines[len(lines)-1]))
}

// An open form or picker used to leave the underlying list's hints in the
// status bar (or no status bar at all); it must show the overlay's own keys.
func TestOpenOverlayShowsItsHintsInStatusBar(t *testing.T) {
	activators := overlayActivators()
	activators["confirm"] = func(m *Model) { m.activeModal = modalConfirm }
	activators["errModal"] = func(m *Model) { m.activeModal = modalError }
	for name, activate := range activators {
		t.Run(name, func(t *testing.T) {
			m := initTestModel()
			next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
			m = next.(Model)
			m.view = viewServerList
			m.statusBar.Hint = m.serverList.Hints()
			activate(&m)
			o, ok := m.topOverlay()
			if !ok {
				t.Fatal("no overlay active")
			}
			if got, want := statusLine(t, m), o.Hints(); got != want {
				t.Errorf("status bar = %q, want %q", got, want)
			}
		})
	}
}

// Closing the overlay brings the view's own hints back.
func TestClosedOverlayRestoresViewHints(t *testing.T) {
	m := initTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = next.(Model)
	m.view = viewServerList
	m.statusBar.Hint = "list hints"
	if got := statusLine(t, m); got != "list hints" {
		t.Errorf("status bar = %q, want the list hints", got)
	}
}

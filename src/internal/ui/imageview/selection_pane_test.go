package imageview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Switching panes to look at an image's details must not throw away a bulk
// selection; only Esc clears it.
func TestPaneSwitchKeepsSelection(t *testing.T) {
	m := loadedModel(namedImages("alpha", "beta", "gamma")...)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if m.SelectionCount() != 2 {
		t.Fatalf("setup: %d images selected, want 2", m.SelectionCount())
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.SelectionCount() != 2 {
		t.Fatalf("Tab left %d images selected, want 2", m.SelectionCount())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.SelectionCount() != 2 || m.focus != FocusSelector {
		t.Fatalf("Shift+Tab left %d images selected, focus %v; want 2 on the selector", m.SelectionCount(), m.focus)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.SelectionCount() != 0 {
		t.Fatalf("Esc left %d images selected, want 0", m.SelectionCount())
	}
}

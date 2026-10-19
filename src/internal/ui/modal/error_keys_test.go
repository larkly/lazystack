package modal

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// The details toggle follows its binding, and the hints name the bound key.
func TestToggleDetailsFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.ToggleDetails
	t.Cleanup(func() { shared.Keys.ToggleDetails = prev })
	shared.Keys.ToggleDetails = key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "details"))

	m := ErrorModel{}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.ShowDetails {
		t.Fatal("d still toggles details after rebinding")
	}
	if !strings.Contains(m.Hints(), "i details") {
		t.Fatalf("hints %q do not name the bound key", m.Hints())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if !m.ShowDetails {
		t.Fatal("rebound key did not toggle details")
	}
	if !strings.Contains(m.Hints(), "i hide details") {
		t.Fatalf("hints %q do not name the bound key", m.Hints())
	}
}

// The default binding keeps accepting D, as the old case-insensitive check did.
func TestToggleDetailsDefaultAcceptsUppercase(t *testing.T) {
	m := ErrorModel{}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', ShiftedCode: 'D', Mod: tea.ModShift, Text: "D"})
	if !m.ShowDetails {
		t.Fatal("D did not toggle details")
	}
}

package app

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/lbview"
)

// The LB members pane's select-all key follows its binding.
func TestLBSelectAllFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.SelectAll
	t.Cleanup(func() { shared.Keys.SelectAll = prev })
	shared.Keys.SelectAll = key.NewBinding(key.WithKeys("z"))

	m, _ := lbMemberFixture(t, &memberFixture{}, "success")
	for i := 0; i < 4 && m.lbView.FocusedPane() != lbview.FocusMembers; i++ {
		res, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = res.(Model)
	}
	if m.lbView.FocusedPane() != lbview.FocusMembers {
		t.Fatal("could not focus the members pane")
	}

	res, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = res.(Model)
	if n := m.lbView.SelectedMemberCount(); n != 0 {
		t.Fatalf("x still selects all after rebinding: %d selected", n)
	}
	res, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = res.(Model)
	if n := m.lbView.SelectedMemberCount(); n != 3 {
		t.Fatalf("rebound select-all selected %d members, want 3", n)
	}
}

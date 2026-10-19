package auditlog

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func TestFilterKeysFollowKeybindings(t *testing.T) {
	prev := shared.Keys.AuditFilterAction
	t.Cleanup(func() { shared.Keys.AuditFilterAction = prev })
	shared.Keys.AuditFilterAction = key.NewBinding(key.WithKeys("A"))

	m := New()
	m.SetSize(100, 8)
	m.SetEntries(nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.filterMode != FilterNone {
		t.Fatalf("a still starts an action filter after rebinding: mode=%v", m.filterMode)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', ShiftedCode: 'A', Mod: tea.ModShift, Text: "A"})
	if m.filterMode != FilterAction {
		t.Fatalf("rebound key: mode=%v, want FilterAction", m.filterMode)
	}
}

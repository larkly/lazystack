package lbmembercreate

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// The server picker's filter opens with the filter binding, not a literal "/".
func TestServerPickerFilterFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.Filter
	t.Cleanup(func() { shared.Keys.Filter = prev })
	shared.Keys.Filter = key.NewBinding(key.WithKeys("ctrl+f"))

	m := New(nil, nil, "pool-1", "pool-1", "10.0.0.10", nil)
	m.openServerPicker()
	m, _ = m.handleServerPickerKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.serverFiltering {
		t.Fatal("/ still opens the filter after rebinding")
	}
	m, _ = m.handleServerPickerKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if !m.serverFiltering {
		t.Fatal("rebound filter key did not open the filter")
	}
	m, _ = m.handleServerPickerKey(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m, _ = m.handleServerPickerKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m, _ = m.handleServerPickerKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.serverFilter != "x" {
		t.Fatalf("filter = %q, want x", m.serverFilter)
	}
}

package lbcreate

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// The subnet picker's filter opens with the filter binding, not a literal "/".
func TestSubnetPickerFilterFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.Filter
	t.Cleanup(func() { shared.Keys.Filter = prev })
	shared.Keys.Filter = key.NewBinding(key.WithKeys("ctrl+f"))

	m := New(nil, nil)
	m.pickerOpen = true
	m, _ = m.handlePickerKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.subnetFiltering {
		t.Fatal("/ still opens the filter after rebinding")
	}
	m, _ = m.handlePickerKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if !m.subnetFiltering {
		t.Fatal("rebound filter key did not open the filter")
	}
	m, _ = m.handlePickerKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.subnetFilter != "/" {
		t.Fatalf("filter = %q, want / typed as text", m.subnetFilter)
	}
	m, _ = m.handlePickerKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.subnetFilter != "" {
		t.Fatalf("backspace left filter %q", m.subnetFilter)
	}
}

package routercreate

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

var (
	keyF2    = tea.KeyPressMsg{Code: tea.KeyF2}
	keyCtrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
)

// A rebound submit key submits from both the text field and the other
// fields, and the old ctrl+s no longer does.
func TestSubmitFollowsKeybinding(t *testing.T) {
	prev := shared.Keys.Submit
	t.Cleanup(func() { shared.Keys.Submit = prev })
	shared.Keys.Submit = key.NewBinding(key.WithKeys("f2"))

	for _, field := range []int{fieldName, fieldExtNet} {
		m := New(nil)
		m, _ = m.Update(extNetsLoadedMsg{})
		m.focusField = field
		m.updateFocus()

		m, _ = m.Update(keyCtrlS)
		if m.err != "" {
			t.Fatalf("field %d: ctrl+s still submits after rebinding (err=%q)", field, m.err)
		}
		m, _ = m.Update(keyF2)
		if m.err != "Router name is required" {
			t.Fatalf("field %d: f2 did not submit (err=%q)", field, m.err)
		}
	}
}

// A picker with a type-to-filter input navigates with a rebound non-text
// key, while the letters in the binding still reach the filter.
func TestPickerNavigationFollowsKeybinding(t *testing.T) {
	prevUp, prevDown := shared.Keys.Up, shared.Keys.Down
	t.Cleanup(func() { shared.Keys.Up, shared.Keys.Down = prevUp, prevDown })
	shared.Keys.Up = key.NewBinding(key.WithKeys("ctrl+p", "k"))
	shared.Keys.Down = key.NewBinding(key.WithKeys("ctrl+n", "j"))

	m := openExtNetPicker(t)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if m.pickerCursor != 1 {
		t.Fatalf("ctrl+n: cursor = %d, want 1", m.pickerCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if m.pickerCursor != 0 {
		t.Fatalf("ctrl+p: cursor = %d, want 0", m.pickerCursor)
	}
	m = typeRune(m, 'j')
	if got := m.pickerFilter.Value(); got != "j" {
		t.Fatalf("filter = %q, want j", got)
	}
}

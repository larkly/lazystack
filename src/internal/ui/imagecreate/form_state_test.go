package imagecreate

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The form opens with the Name field focused and accepting input.
func TestNewFocusesNameField(t *testing.T) {
	m := New(nil)
	_ = m.Init()
	if m.focusField != fieldName || !m.nameInput.Focused() {
		t.Fatalf("focusField = %d, name focused = %v; want the Name field focused", m.focusField, m.nameInput.Focused())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.nameInput.Value() != "x" {
		t.Fatalf("typing went to field %d, name = %q", m.focusField, m.nameInput.Value())
	}
}

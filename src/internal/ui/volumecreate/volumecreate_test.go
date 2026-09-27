package volumecreate

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/volume"
)

func openPicker(t *testing.T, field int) Model {
	t.Helper()
	m := New(nil)
	m, _ = m.Update(volumeTypesLoadedMsg{types: []volume.VolumeType{
		{ID: "11111111-aaaa", Name: "ssd"},
		{ID: "22222222-bbbb", Name: "jk-fast"},
		{ID: "33333333-cccc", Name: "jk-slow"},
	}})
	m.focusField = field
	m.updateFocus()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.pickerOpen || m.pickerField != field {
		t.Fatalf("picker for field %d did not open", field)
	}
	return m
}

func TestPickerFiltersAcceptJAndK(t *testing.T) {
	for _, field := range []int{fieldType, fieldAZ} {
		m := openPicker(t, field)
		for _, r := range "jk" {
			m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if got := m.pickerFilter.Value(); got != "jk" {
			t.Fatalf("field %d: filter = %q, want jk", field, got)
		}
		if m.pickerCursor != 0 {
			t.Fatalf("field %d: cursor moved to %d while typing", field, m.pickerCursor)
		}
	}

	// Arrows navigate the filtered type list and Enter selects from it.
	m := openPicker(t, fieldType)
	for _, r := range "jk" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pickerOpen || m.selectedType != 2 {
		t.Fatalf("enter: open=%v selected=%d, want jk-slow (2)", m.pickerOpen, m.selectedType)
	}

	m = openPicker(t, fieldAZ)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickerOpen || m.selectedAZ != -1 {
		t.Fatalf("esc: open=%v selected=%d", m.pickerOpen, m.selectedAZ)
	}
}

func TestShortVolumeTypeIDsDoNotPanic(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(volumeTypesLoadedMsg{types: []volume.VolumeType{{ID: "", Name: "a"}, {ID: "t", Name: "b"}, {ID: "類型類型類型類型類型", Name: "c"}}})
	m.focusField = fieldType
	m.updateFocus()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.pickerOpen || !utf8.ValidString(m.View()) {
		t.Fatal("type picker did not render")
	}
}

package routercreate

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
)

func typeRune(m Model, r rune) Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return m
}

func openExtNetPicker(t *testing.T) Model {
	t.Helper()
	m := New(nil)
	m, _ = m.Update(extNetsLoadedMsg{nets: []network.Network{
		{ID: "11111111-aaaa", Name: "public"},
		{ID: "22222222-bbbb", Name: "jk-net"},
		{ID: "33333333-cccc", Name: "jk-backup"},
	}})
	m.focusField = fieldExtNet
	m.updateFocus()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.pickerOpen {
		t.Fatal("picker did not open")
	}
	return m
}

func TestExtNetPickerFilterAcceptsJAndK(t *testing.T) {
	m := openExtNetPicker(t)
	m = typeRune(m, 'j')
	m = typeRune(m, 'k')
	if got := m.pickerFilter.Value(); got != "jk" {
		t.Fatalf("filter = %q, want jk", got)
	}
	if m.pickerCursor != 0 {
		t.Fatalf("cursor moved to %d while typing", m.pickerCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.pickerCursor != 1 {
		t.Fatalf("down arrow: cursor = %d", m.pickerCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pickerOpen || m.selectedExtNet != 2 {
		t.Fatalf("enter: open=%v selected=%d, want jk-backup (2)", m.pickerOpen, m.selectedExtNet)
	}

	m = openExtNetPicker(t)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickerOpen || m.selectedExtNet != -1 {
		t.Fatalf("esc: open=%v selected=%d", m.pickerOpen, m.selectedExtNet)
	}
}

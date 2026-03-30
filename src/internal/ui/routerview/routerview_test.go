package routerview

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestShiftTabMovesFocusBackward(t *testing.T) {
	m := New(nil, 5*time.Second)
	if m.focus != FocusSelector {
		t.Fatalf("initial focus = %v, want FocusSelector", m.focus)
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = updated
	if m.focus != focusInfo {
		t.Fatalf("focus after tab = %v, want focusInfo", m.focus)
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	m = updated
	if m.focus != FocusSelector {
		t.Fatalf("focus after shift+tab = %v, want FocusSelector", m.focus)
	}
}

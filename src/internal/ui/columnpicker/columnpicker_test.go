package columnpicker

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestReorderToggleAndApply(t *testing.T) {
	m := New([]PickerColumn{{Title: "Name", Key: "name"}, {Title: "ID", Key: "id", Hidden: true}})
	m.SetSize(80, 24)
	for _, code := range []rune{tea.KeyUp, tea.KeyLeft, tea.KeyRight, tea.KeyRight} {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	if m.cursor != 1 || m.columns[0].Key != "id" {
		t.Fatal("reorder or bounds")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.columns[1].Hidden || !strings.Contains(ansi.Strip(m.View()), "[ ] Name") {
		t.Fatal(m.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Active || cmd == nil {
		t.Fatal("apply failed")
	}
	got := cmd().(ColumnsChosenMsg)
	if len(got.Columns) != 2 || got.Columns[0].Key != "name" || !got.Columns[0].Hidden {
		t.Fatalf("columns=%+v", got.Columns)
	}
}
func TestEmptyAndCancel(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, code := range []rune{tea.KeyDown, tea.KeyUp, tea.KeyLeft, tea.KeyRight, tea.KeySpace} {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	if !strings.Contains(m.View(), "Columns") {
		t.Fatal(m.View())
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(cmd().(ColumnsChosenMsg).Columns) != 0 {
		t.Fatal("nonempty result")
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Active {
		t.Fatal("still active")
	}
	if _, ok := cmd().(ColumnsCancelledMsg); !ok {
		t.Fatal("wrong message")
	}
}

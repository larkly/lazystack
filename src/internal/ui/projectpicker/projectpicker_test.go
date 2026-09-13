package projectpicker

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestCurrentSelectionAndSwitch(t *testing.T) {
	projects := []shared.ProjectInfo{{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}, {ID: "c", Name: "Gamma"}}
	m := New(projects, "b")
	m.SetSize(80, 10)
	if m.cursor != 1 || !strings.Contains(ansi.Strip(m.View()), "* Beta") {
		t.Fatal(m.View())
	}
	closed, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if closed.Active || cmd != nil {
		t.Fatal("current project should close without switching")
	}
	for i := 0; i < 4; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != 2 || m.scroll != 1 || !strings.Contains(m.View(), "Gamma") {
		t.Fatalf("cursor=%d scroll=%d", m.cursor, m.scroll)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Active || cmd == nil {
		t.Fatal("switch failed")
	}
	if got := cmd().(shared.ProjectSelectedMsg); got.ProjectID != "c" || got.ProjectName != "Gamma" {
		t.Fatalf("message=%+v", got)
	}
}
func TestNavigationEmptyAndCancel(t *testing.T) {
	for _, projects := range [][]shared.ProjectInfo{nil, {{ID: "a", Name: "Alpha"}, {ID: "b", Name: "Beta"}}} {
		m := New(projects, "missing")
		m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 9})
		for i := 0; i < 3; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		for i := 0; i < 3; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		}
		if m.cursor != 0 {
			t.Fatal("upper boundary")
		}
		_ = m.View()
		if len(projects) == 0 {
			var cmd tea.Cmd
			m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil || !m.Active {
				t.Fatal("empty selection")
			}
		}
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.Active || cmd != nil {
			t.Fatal("cancel failed")
		}
	}
}

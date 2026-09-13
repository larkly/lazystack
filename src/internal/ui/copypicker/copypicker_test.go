package copypicker

import (
	tea "charm.land/bubbletea/v2"
	"reflect"
	"strings"
	"testing"
)

func TestBuilderSkipsEmptyAndLabelsMultipleValues(t *testing.T) {
	b := Builder{}
	b.Add("Empty", "").Add("Name", "web").AddEach("None", nil).AddEach("IP", []string{"", "one"}).AddEach("DNS", []string{"a", "", "b"})
	want := []Entry{{"Name", "web"}, {"IP", "one"}, {"DNS (a)", "a"}, {"DNS (b)", "b"}}
	if !reflect.DeepEqual(b.Entries(), want) {
		t.Fatalf("entries = %#v", b.Entries())
	}
}
func TestNavigationSelectionAndQuickPick(t *testing.T) {
	for _, quick := range []bool{false, true} {
		t.Run(map[bool]string{false: "enter", true: "quick"}[quick], func(t *testing.T) {
			m := New("Copy server", []Entry{{"ID", "id"}, {"Name", "web"}})
			if m.Init() != nil {
				t.Fatal("unexpected init command")
			}
			m.SetSize(100, 30)
			for _, code := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyDown} {
				m, _ = m.Update(tea.KeyPressMsg{Code: code})
			}
			if m.cursor != 1 {
				t.Fatalf("cursor = %d", m.cursor)
			}
			if !strings.Contains(m.View(), "web") {
				t.Fatal(m.View())
			}
			key := tea.KeyPressMsg{Code: tea.KeyEnter}
			if quick {
				key = tea.KeyPressMsg{Code: '2', Text: "2"}
			}
			m, cmd := m.Update(key)
			if m.Active || cmd == nil {
				t.Fatal("selection did not close and emit")
			}
			if got := cmd().(ChosenMsg); got != (ChosenMsg{"Name", "web"}) {
				t.Fatalf("chosen = %+v", got)
			}
		})
	}
}
func TestEmptyInvalidAndCancel(t *testing.T) {
	m := New("Copy", nil)
	for _, code := range []rune{tea.KeyEnter, tea.KeyDown, tea.KeyUp, '9'} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg{Code: code})
		if cmd != nil || !m.Active {
			t.Fatal("empty selection changed state")
		}
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	_ = m.View()
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.Active || cmd == nil {
		t.Fatal("cancel failed")
	}
	if _, ok := cmd().(CancelledMsg); !ok {
		t.Fatal("wrong cancel message")
	}
}
func TestDisplayHelpers(t *testing.T) {
	for _, tc := range []struct {
		s    string
		n    int
		want string
	}{{"abc", 0, "abc"}, {"abc", 1, "…"}, {"abcd", 3, "ab…"}, {"abc", 3, "abc"}} {
		if got := truncate(tc.s, tc.n); got != tc.want {
			t.Errorf("truncate=%q want %q", got, tc.want)
		}
	}
	if padRight("a", 3) != "a  " || padRight("long", 2) != "long" {
		t.Fatal("padding")
	}
	m := New("Copy", []Entry{{"Name", strings.Repeat("x", 80)}})
	l, v := m.columnWidths()
	if l != 4 || v != 60 {
		t.Fatalf("widths=%d,%d", l, v)
	}
}

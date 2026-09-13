package consolelog

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestLoadScrollRefreshAndBack(t *testing.T) {
	m := New(nil, "server", "web")
	m.SetSize(20, 5)
	if m.Init() == nil || !m.loading {
		t.Fatal("initial state")
	}
	m, _ = m.Update(consoleLoadedMsg{"zero\none\ntwo\nthree\nfour"})
	if m.loading || m.scroll != 3 || !strings.Contains(m.View(), "four") {
		t.Fatal("load bottom")
	}
	for _, tc := range []struct {
		code rune
		want int
	}{{tea.KeyDown, 3}, {tea.KeyUp, 2}, {tea.KeyPgUp, 0}, {tea.KeyPgUp, 0}, {tea.KeyPgDown, 2}, {tea.KeyPgDown, 3}, {'g', 0}, {'G', 3}} {
		m, _ = m.Update(tea.KeyPressMsg{Code: tc.code})
		if m.scroll != tc.want {
			t.Fatalf("key=%d scroll=%d want=%d", tc.code, m.scroll, tc.want)
		}
	}
	if m.ForceRefresh() == nil || !m.loading {
		t.Fatal("refresh")
	}
	m, _ = m.Update(consoleErrMsg{errors.New("denied")})
	if m.loading || !strings.Contains(m.View(), "denied") {
		t.Fatal("error state")
	}
	m, _ = m.Update(consoleLoadedMsg{"recovered"})
	if m.err != "" || m.scroll != 0 || !strings.Contains(m.View(), "recovered") {
		t.Fatal("recovery")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil || cmd().(shared.ViewChangeMsg).View != "serverdetail" {
		t.Fatal("back")
	}
	if !strings.Contains(m.Hints(), "refresh") {
		t.Fatal("hints")
	}
}
func TestEmptyOutputShowsEmptyState(t *testing.T) {
	m := New(nil, "server", "web")
	m.SetSize(80, 24)
	m, _ = m.Update(consoleLoadedMsg{""})
	if !strings.Contains(m.View(), "No console output available") {
		t.Fatal(m.View())
	}
}
func TestNarrowViewDoesNotPanic(t *testing.T) {
	for _, width := range []int{0, 1, 2, 3} {
		m := New(nil, "server", "web")
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 5})
		m, _ = m.Update(consoleLoadedMsg{"output"})
		_ = m.View()
	}
}

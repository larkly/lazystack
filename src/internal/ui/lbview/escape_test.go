package lbview

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
)

func TestEscapeClearsSearchThenMemberSelection(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(lbsLoadedMsg{lbs: []loadbalancer.LoadBalancer{{ID: "lb", Name: "alpha"}, {ID: "lb2", Name: "beta"}}})
	m.selectedMembers["mem-1"] = true
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.FilterValue() != "a" {
		t.Fatalf("setup: filter = %q", m.FilterValue())
	}

	if !m.Escape() || m.FilterValue() != "" || len(m.selectedMembers) != 1 {
		t.Fatalf("first escape: filter=%q members=%d, want filter cleared and members kept", m.FilterValue(), len(m.selectedMembers))
	}
	if !m.Escape() || len(m.selectedMembers) != 0 {
		t.Fatalf("second escape: members=%d, want cleared", len(m.selectedMembers))
	}
	if m.Escape() {
		t.Fatal("third escape reported handled with nothing left to clear")
	}
}

func TestHintsNeverSayEscGoesBack(t *testing.T) {
	m := New(nil, time.Second)
	for pane := FocusPane(0); pane < focusPaneCount; pane++ {
		m.focus = pane
		if h := m.Hints(); strings.Contains(h, "esc back") {
			t.Errorf("pane %d hints %q say esc goes back", pane, h)
		}
	}
}

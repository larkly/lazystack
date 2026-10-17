package fippicker

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestSelectExistingOrAllocate(t *testing.T) {
	for _, allocate := range []bool{false, true} {
		m := New(nil, "server", "web")
		m.SetSize(80, 15)
		if m.Init() == nil || !strings.Contains(m.View(), "Loading") {
			t.Fatal("initial state")
		}
		m, _ = m.Update(fipsLoadedMsg{[]network.FloatingIP{{ID: "fip", FloatingIP: "192.0.2.1"}}})
		if !strings.Contains(m.View(), "192.0.2.1") || !strings.Contains(m.View(), "Allocate new") {
			t.Fatal(m.View())
		}
		if allocate {
			for i := 0; i < 3; i++ {
				m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			if m.cursor != 1 {
				t.Fatal("lower bound")
			}
		} else {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			if m.cursor != 0 {
				t.Fatal("upper bound")
			}
		}
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if !m.submitting || cmd == nil || !strings.Contains(m.View(), "Finding server addresses") {
			t.Fatal("not submitting")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.Active {
			t.Fatal("busy modal closed")
		}
		action := "Assigned"
		if allocate {
			m, cmd = m.Update(allocateDoneMsg{fipAddr: "192.0.2.1", serverName: "web"})
			action = "Allocated & assigned"
		} else {
			m, cmd = m.Update(associateDoneMsg{fipAddr: "192.0.2.1", serverName: "web"})
		}
		if m.Active || m.submitting || cmd == nil {
			t.Fatal("completion state")
		}
		got := cmd().(shared.ResourceActionMsg)
		if got.Action != action || got.Name != "192.0.2.1 → web" {
			t.Fatalf("message=%+v", got)
		}
	}
}
func TestEmptyListWaitsForEnterAndErrorsCanClose(t *testing.T) {
	m := New(nil, "server", "web")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	m, cmd := m.Update(fipsLoadedMsg{})
	if m.loading || m.submitting || cmd != nil || !strings.Contains(m.View(), "+ Allocate new floating IP") {
		t.Fatalf("empty list should offer allocation and wait:\n%s", m.View())
	}
	for _, msg := range []tea.Msg{fetchErrMsg{err: errors.New("denied")}, associateErrMsg{err: errors.New("denied")}, allocateErrMsg{err: errors.New("denied")}} {
		n := New(nil, "server", "web")
		n.loading = false
		n.submitting = false
		n, _ = n.Update(msg)
		if !strings.Contains(n.View(), "denied") {
			t.Fatal(n.View())
		}
		n, cmd = n.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if n.Active || cmd != nil {
			t.Fatal("error cancel")
		}
	}
}

// Esc closes the modal while it is still loading, but not mid-submit.
func TestEscClosesWhileLoading(t *testing.T) {
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	m := New(nil, "srv-1", "srv")
	m, _ = m.Update(esc)
	if m.Active {
		t.Fatal("esc while loading did not close the modal")
	}

	m = New(nil, "srv-1", "srv")
	m.submitting = true
	m, _ = m.Update(esc)
	if !m.Active {
		t.Fatal("esc while submitting closed the modal")
	}
}

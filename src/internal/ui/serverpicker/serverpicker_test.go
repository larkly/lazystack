package serverpicker

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"strings"
	"testing"
)

func TestNavigationFilterSelection(t *testing.T) {
	m := New(nil, "vol-id", "disk")
	m.SetSize(80, 15)
	if m.Init() == nil || !strings.Contains(m.View(), "Loading") {
		t.Fatal("missing initial loading")
	}
	items := []compute.Server{{ID: "id-one", Name: "Alpha"}, {ID: "id-two", Name: "Beta"}, {ID: "id-three", Name: "Gamma"}, {ID: "id-four", Name: "Delta"}}
	m, _ = m.Update(serversLoadedMsg{servers: items})
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != 3 || m.scrollOff != 1 {
		t.Fatalf("cursor=%d scroll=%d", m.cursor, m.scrollOff)
	}
	for i := 0; i < 6; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.cursor != 0 || m.scrollOff != 0 {
		t.Fatal("upper boundary")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'B', Text: "B"})
	if len(m.filtered) != 1 || m.filtered[0].Name != "Beta" || m.cursor != 0 || m.scrollOff != 0 {
		t.Fatalf("filter=%q items=%+v", m.filter, m.filtered)
	}
	if !strings.Contains(m.View(), "Beta") {
		t.Fatal(m.View())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !strings.Contains(m.View(), "No servers found") {
		t.Fatal(m.View())
	}
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.submitting {
		t.Fatal("empty selection submitted")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.submitting || !strings.Contains(m.View(), "Attaching") {
		t.Fatal("selection did not submit")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.Active {
		t.Fatal("busy modal closed")
	}
	m, _ = m.Update(attachErrMsg{errors.New("attach denied")})
	if m.submitting || !strings.Contains(m.View(), "attach denied") {
		t.Fatal("attach error not shown")
	}
	m, cmd = m.Update(attachDoneMsg{serverName: "web", volumeName: "disk"})
	if m.Active || cmd == nil {
		t.Fatal("completion did not close")
	}
	got := cmd().(shared.ResourceActionMsg)
	if got.Action != "Attached" || got.Name != "disk → web" {
		t.Fatalf("message=%+v", got)
	}
}
func TestFetchErrorEmptyAndCancel(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := New(nil, "vol-id", "disk")
		m, _ = m.Update(tea.WindowSizeMsg{Width: 55, Height: 20})
		if fail {
			m, _ = m.Update(fetchErrMsg{errors.New("fetch denied")})
			if !strings.Contains(m.View(), "fetch denied") {
				t.Fatal(m.View())
			}
		} else {
			m, _ = m.Update(serversLoadedMsg{})
			if !strings.Contains(m.View(), "No servers found") {
				t.Fatal(m.View())
			}
		}
		if m.loading {
			t.Fatal("still loading")
		}
		m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.Active || cmd != nil {
			t.Fatal("cancel failed")
		}
	}
}

func TestFilterAcceptsJK(t *testing.T) {
	m := New(nil, "vol-id", "disk")
	m.SetSize(80, 20)
	items := []compute.Server{{ID: "1", Name: "alpha"}, {ID: "2", Name: "jack"}, {ID: "3", Name: "kube-a"}, {ID: "4", Name: "kube-b"}}
	m, _ = m.Update(serversLoadedMsg{servers: items})
	for _, r := range "jack" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.filter != "jack" || len(m.filtered) != 1 || m.filtered[0].Name != "jack" {
		t.Fatalf("filter=%q filtered=%+v", m.filter, m.filtered)
	}
	for range "jack" {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, r := range "kube" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.filter != "kube" || len(m.filtered) != 2 {
		t.Fatalf("filter=%q filtered=%+v", m.filter, m.filtered)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("down arrow cursor=%d", m.cursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.submitting {
		t.Fatal("enter did not submit the visible filtered row")
	}
	if m.filtered[m.cursor].Name != "kube-b" {
		t.Fatalf("selected %q", m.filtered[m.cursor].Name)
	}

	m2 := New(nil, "vol-id", "disk")
	m2, _ = m2.Update(serversLoadedMsg{servers: items})
	m2, _ = m2.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m2, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m2.Active || cmd != nil {
		t.Fatal("escape did not cancel cleanly")
	}
}

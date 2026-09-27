package serveradminact

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(m Model, keys ...tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(k)
	}
	return m, cmd
}

var (
	enter = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	down  = tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	esc   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
)

func text(s string) []tea.KeyPressMsg {
	var out []tea.KeyPressMsg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return out
}

func requestFrom(t *testing.T, cmd tea.Cmd) ActionRequestMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no request emitted")
	}
	req, ok := cmd().(ActionRequestMsg)
	if !ok {
		t.Fatal("command did not produce an ActionRequestMsg")
	}
	return req
}

func TestHostActionsPromptForHostAndEmitRequest(t *testing.T) {
	for _, tc := range []struct {
		downs  int
		action string
	}{{1, "Live Migrate"}, {2, "Evacuate"}} {
		t.Run(tc.action, func(t *testing.T) {
			m := New("srv-id", "srv")
			for range tc.downs {
				m, _ = m.Update(down)
			}
			m, _ = m.Update(enter)
			if m.promptStage != "host" || !strings.Contains(m.View(), "Target Host") {
				t.Fatalf("host prompt not shown (stage %q)", m.promptStage)
			}
			// An empty host is rejected in place.
			m, cmd := m.Update(enter)
			if cmd != nil || !strings.Contains(m.View(), "Host name is required") {
				t.Fatal("empty host accepted")
			}
			m, _ = press(m, text("compute-2")...)
			m, cmd = m.Update(enter)
			req := requestFrom(t, cmd)
			if req != (ActionRequestMsg{Action: tc.action, ServerID: "srv-id", ServerName: "srv", Arg: "compute-2"}) {
				t.Fatalf("request=%+v", req)
			}
			if m.Active {
				t.Error("modal stays open after submitting")
			}
		})
	}
}

func TestForceDeleteRequiresConfirmation(t *testing.T) {
	m := New("srv-id", "srv")
	m, _ = press(m, down, down, down)
	m, cmd := m.Update(enter)
	if cmd != nil || m.promptStage != "confirm" || !strings.Contains(m.View(), "Force delete srv?") {
		t.Fatalf("confirmation not shown (stage %q)", m.promptStage)
	}
	m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	if cmd != nil || m.promptStage != "" || !m.Active {
		t.Fatal("deny did not return to the action list")
	}
	m, _ = m.Update(enter)
	m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if req := requestFrom(t, cmd); req.Action != "Force Delete" || req.ServerID != "srv-id" {
		t.Fatalf("request=%+v", req)
	}
}

func TestResetStatePicksState(t *testing.T) {
	m := New("srv-id", "srv")
	m, _ = press(m, down, down, down, down)
	m, _ = m.Update(enter)
	if m.promptStage != "state" || !strings.Contains(m.View(), "New State") {
		t.Fatalf("state picker not shown (stage %q)", m.promptStage)
	}
	m, _ = m.Update(down)
	_, cmd := m.Update(enter)
	if req := requestFrom(t, cmd); req.Action != "Reset State" || req.Arg != "error" {
		t.Fatalf("request=%+v", req)
	}
}

func TestColdMigrateSubmitsOnce(t *testing.T) {
	m := New("srv-id", "srv")
	m, cmd := m.Update(enter)
	if req := requestFrom(t, cmd); req.Action != "Migrate" || req.Arg != "" {
		t.Fatalf("request=%+v", req)
	}
	if m.Active {
		t.Fatal("modal still active; a second enter could submit again")
	}
}

func TestEscapeBacksOutOfPromptThenCloses(t *testing.T) {
	m := New("srv-id", "srv")
	m, _ = press(m, down, enter)
	m, _ = m.Update(esc)
	if m.promptStage != "" || !m.Active {
		t.Fatal("esc should leave the prompt first")
	}
	m, _ = m.Update(esc)
	if m.Active {
		t.Fatal("esc should close the modal")
	}
}

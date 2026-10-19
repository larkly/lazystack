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
	yes   = tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"})
	no    = tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"})
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
			if cmd != nil || m.promptStage != "confirm" {
				t.Fatalf("host accepted without confirmation (stage %q)", m.promptStage)
			}
			m, cmd = m.Update(yes)
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
	m, cmd := m.Update(enter)
	if cmd != nil || m.promptStage != "confirm" {
		t.Fatalf("state applied without confirmation (stage %q)", m.promptStage)
	}
	_, cmd = m.Update(yes)
	if req := requestFrom(t, cmd); req.Action != "Reset State" || req.Arg != "error" {
		t.Fatalf("request=%+v", req)
	}
}

func TestColdMigrateSubmitsOnce(t *testing.T) {
	m := New("srv-id", "srv")
	m, _ = m.Update(enter)
	m, cmd := m.Update(yes)
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

// Every disruptive action waits for an explicit y and names the server and
// its target in the prompt; n and esc step back without a request.
func TestDisruptiveActionsConfirmWithTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keys   []tea.KeyPressMsg
		prompt string
		back   string // stage that n/esc return to
		action string
		arg    string
	}{
		{"cold migrate", []tea.KeyPressMsg{enter}, "Cold migrate srv?", "", "Migrate", ""},
		{"live migrate", append(append([]tea.KeyPressMsg{down, enter}, text("cmp-7")...), enter),
			"Live migrate srv to host cmp-7?", "host", "Live Migrate", "cmp-7"},
		{"evacuate", append(append([]tea.KeyPressMsg{down, down, enter}, text("cmp-8")...), enter),
			"Evacuate srv to host cmp-8?", "host", "Evacuate", "cmp-8"},
		{"force delete", []tea.KeyPressMsg{down, down, down, enter}, "Force delete srv?", "", "Force Delete", ""},
		{"reset state", []tea.KeyPressMsg{down, down, down, down, enter, down, enter},
			"Reset state of srv to error?", "state", "Reset State", "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, cmd := press(New("srv-id", "srv"), tc.keys...)
			if cmd != nil {
				if _, ok := cmd().(ActionRequestMsg); ok {
					t.Fatal("request sent before confirmation")
				}
			}
			if m.promptStage != "confirm" || !strings.Contains(m.View(), tc.prompt) {
				t.Fatalf("stage %q, want prompt %q:\n%s", m.promptStage, tc.prompt, m.View())
			}
			if !strings.Contains(m.Hints(), "y confirm") {
				t.Fatalf("hints=%q", m.Hints())
			}
			for _, k := range []tea.KeyPressMsg{no, esc} {
				var c tea.Cmd
				m, c = m.Update(k)
				if c != nil {
					if _, ok := c().(ActionRequestMsg); ok {
						t.Fatalf("%s sent a request", k.String())
					}
				}
				if !m.Active || m.promptStage != tc.back {
					t.Fatalf("%s: active=%v stage=%q want %q", k.String(), m.Active, m.promptStage, tc.back)
				}
				if tc.back == "host" && m.hostInput.Value() != tc.arg {
					t.Fatalf("host input lost: %q", m.hostInput.Value())
				}
				m, _ = m.Update(enter)
				if m.promptStage != "confirm" {
					t.Fatalf("enter did not reopen the prompt (stage %q)", m.promptStage)
				}
			}
			m, cmd = m.Update(yes)
			req := requestFrom(t, cmd)
			if req != (ActionRequestMsg{Action: tc.action, ServerID: "srv-id", ServerName: "srv", Arg: tc.arg}) {
				t.Fatalf("request=%+v", req)
			}
			if m.Active {
				t.Fatal("modal stays open after confirming")
			}
		})
	}
}

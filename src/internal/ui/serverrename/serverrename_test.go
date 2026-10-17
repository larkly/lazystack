package serverrename

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func clearName(m Model) Model {
	m.nameInput.SetValue("")
	return m
}

func TestNewPrefillsCurrentName(t *testing.T) {
	m := New(nil, "srv-1", "web")
	if !m.Active || m.nameInput.Value() != "web" || !m.nameInput.Focused() {
		t.Errorf("Active=%v name=%q focused=%v, want active, prefilled and focused", m.Active, m.nameInput.Value(), m.nameInput.Focused())
	}
}

func TestTypingEditsName(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, _ = press(m, tea.Key{Code: '2', Text: "2"})
	if m.nameInput.Value() != "web2" {
		t.Errorf("name = %q, want web2", m.nameInput.Value())
	}
	m, _ = press(m, tea.Key{Code: tea.KeyBackspace})
	m, _ = press(m, tea.Key{Code: tea.KeyBackspace})
	if m.nameInput.Value() != "we" {
		t.Errorf("name after two backspaces = %q, want we", m.nameInput.Value())
	}
}

func TestEnterWithEmptyNameShowsErrorAndNoCommand(t *testing.T) {
	m := clearName(New(nil, "srv-1", "web"))
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd != nil || m.submitting {
		t.Errorf("cmd=%v submitting=%v, want neither", cmd != nil, m.submitting)
	}
	if m.err != "Name cannot be empty" || !strings.Contains(m.View(), "Name cannot be empty") {
		t.Errorf("err=%q, want it set and rendered", m.err)
	}
}

func TestWhitespaceOnlyNameIsRejected(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m.nameInput.SetValue("   ")
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd != nil || m.err == "" {
		t.Errorf("cmd=%v err=%q, want validation error", cmd != nil, m.err)
	}
}

func TestUnchangedNameClosesWithoutRequest(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if m.Active || cmd != nil || m.submitting {
		t.Errorf("Active=%v cmd=%v submitting=%v, want closed with no request", m.Active, cmd != nil, m.submitting)
	}
}

func TestChangedNameStartsRequest(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, _ = press(m, tea.Key{Code: '2', Text: "2"})
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd == nil || !m.submitting || m.err != "" {
		t.Fatalf("cmd=%v submitting=%v err=%q, want request started", cmd != nil, m.submitting, m.err)
	}
	if !strings.Contains(m.View(), "Renaming") {
		t.Error("view should show the renaming state")
	}
	// Further keys are ignored while the request is in flight.
	m, c := press(m, tea.Key{Code: tea.KeyEscape})
	if !m.Active || c != nil {
		t.Error("esc must be ignored while submitting")
	}
}

func TestEscCancels(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if m.Active || cmd != nil {
		t.Errorf("Active=%v cmd=%v, want closed with no command", m.Active, cmd != nil)
	}
}

func TestSuccessReportsNewName(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, cmd := m.Update(renameSuccessMsg{newName: "web2"})
	if m.Active || cmd == nil {
		t.Fatalf("Active=%v cmd=%v, want closed with result", m.Active, cmd != nil)
	}
	got, ok := cmd().(shared.ServerActionMsg)
	if !ok || got.Action != "Renamed" || got.Name != "web2" {
		t.Errorf("result = %#v, want Renamed web2", got)
	}
}

func TestErrorShownAndModalStaysOpen(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, _ = m.Update(renameErrMsg{err: errors.New("boom")})
	if !m.Active || m.err == "" || !strings.Contains(m.View(), m.err) {
		t.Errorf("Active=%v err=%q, want open modal showing the error", m.Active, m.err)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := New(nil, "srv-1", "web")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	testutil.AssertRenderSafe(t, m.View(), 40)
	m.err = "Name cannot be empty"
	testutil.AssertRenderSafe(t, m.View(), 40)
}

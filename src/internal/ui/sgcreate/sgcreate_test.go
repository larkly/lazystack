package sgcreate

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

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, tea.Key{Code: r, Text: string(r)})
	}
	return m
}

var ctrlS = tea.Key{Code: 's', Mod: tea.ModCtrl}

func TestNewStartsActiveOnNameField(t *testing.T) {
	m := New(nil)
	if !m.Active || m.mode != ModeCreate {
		t.Errorf("Active=%v mode=%v, want active create mode", m.Active, m.mode)
	}
	if m.focusField != fieldName || !m.nameInput.Focused() || m.descInput.Focused() {
		t.Errorf("focus=%d name=%v desc=%v, want name field focused", m.focusField, m.nameInput.Focused(), m.descInput.Focused())
	}
}

func TestRenameAndCloneArePrefilled(t *testing.T) {
	r := NewRename(nil, "sg-1", "web", "front")
	if r.mode != ModeRename || r.sgID != "sg-1" || r.nameInput.Value() != "web" || r.descInput.Value() != "front" {
		t.Errorf("rename model wrong: mode=%v id=%q name=%q desc=%q", r.mode, r.sgID, r.nameInput.Value(), r.descInput.Value())
	}
	if !strings.Contains(r.View(), "Rename Security Group") {
		t.Error("rename view should be titled accordingly")
	}

	c := NewClone(nil, "sg-2", "web", "front")
	if c.mode != ModeClone || c.srcSGID != "sg-2" || c.nameInput.Value() != "Copy of web" || c.descInput.Value() != "front" {
		t.Errorf("clone model wrong: mode=%v src=%q name=%q desc=%q", c.mode, c.srcSGID, c.nameInput.Value(), c.descInput.Value())
	}
	if !strings.Contains(c.View(), "Clone Security Group") {
		t.Error("clone view should be titled accordingly")
	}
}

func TestTabMovesFocusBetweenInputsAndButtons(t *testing.T) {
	m := New(nil)
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	if m.focusField != fieldDesc || !m.descInput.Focused() || m.nameInput.Focused() {
		t.Fatalf("after tab: focus=%d name=%v desc=%v, want description focused", m.focusField, m.nameInput.Focused(), m.descInput.Focused())
	}
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	if m.focusField != fieldCancel {
		t.Fatalf("focus = %d, want fieldCancel", m.focusField)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	if m.focusField != fieldName || !m.nameInput.Focused() {
		t.Errorf("tab should wrap to the name field, got focus=%d", m.focusField)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focusField != fieldCancel {
		t.Errorf("shift+tab from name: focus=%d, want fieldCancel", m.focusField)
	}
}

func TestUpDownAndLeftRightOnButtons(t *testing.T) {
	m := New(nil)
	m.focusField = fieldSubmit
	m.updateFocus()
	m, _ = press(m, tea.Key{Code: tea.KeyRight})
	if m.focusField != fieldCancel {
		t.Errorf("right from submit: focus=%d, want fieldCancel", m.focusField)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyLeft})
	if m.focusField != fieldSubmit {
		t.Errorf("left from cancel: focus=%d, want fieldSubmit", m.focusField)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyUp})
	if m.focusField != fieldDesc {
		t.Errorf("up from submit: focus=%d, want fieldDesc", m.focusField)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	if m.focusField != fieldCancel {
		t.Errorf("down from submit: focus=%d, want fieldCancel", m.focusField)
	}
}

func TestSubmitWithEmptyNameShowsErrorAndNoCommand(t *testing.T) {
	m := New(nil)
	m, cmd := press(m, ctrlS)
	if cmd != nil {
		t.Error("empty name must not produce a command")
	}
	if m.err != "Security group name is required" || m.submitting {
		t.Errorf("err=%q submitting=%v, want validation error and no submit", m.err, m.submitting)
	}
	if !strings.Contains(m.View(), "Security group name is required") {
		t.Error("view should render the validation error")
	}
}

func TestEnterOnSubmitButtonValidatesToo(t *testing.T) {
	m := New(nil)
	m.focusField = fieldSubmit
	m.updateFocus()
	m, cmd := press(m, tea.Key{Code: tea.KeyEnter})
	if cmd != nil || m.err == "" {
		t.Errorf("cmd=%v err=%q, want validation error without command", cmd != nil, m.err)
	}
}

func TestSubmitWithNameStartsRequestInEveryMode(t *testing.T) {
	for name, m := range map[string]Model{
		"create": typeText(New(nil), "web"),
		"rename": NewRename(nil, "sg-1", "web", ""),
		"clone":  NewClone(nil, "sg-1", "web", ""),
	} {
		m, cmd := press(m, ctrlS)
		if cmd == nil || !m.submitting || m.err != "" {
			t.Errorf("%s: cmd=%v submitting=%v err=%q, want request started", name, cmd != nil, m.submitting, m.err)
		}
	}
}

func TestEscCancelsFromEveryFieldKind(t *testing.T) {
	for _, f := range []int{fieldName, fieldDesc, fieldSubmit, fieldCancel} {
		m := New(nil)
		m.focusField = f
		m.updateFocus()
		m, _ = press(m, tea.Key{Code: tea.KeyEscape})
		if m.Active {
			t.Errorf("esc on field %d should close the modal", f)
		}
	}
}

func TestEnterOnCancelButtonCloses(t *testing.T) {
	m := New(nil)
	m.focusField = fieldCancel
	m.updateFocus()
	m, _ = press(m, tea.Key{Code: tea.KeyEnter})
	if m.Active {
		t.Error("enter on Cancel should close the modal")
	}
}

func TestKeysIgnoredWhileSubmitting(t *testing.T) {
	m := typeText(New(nil), "web")
	m, _ = press(m, ctrlS)
	m, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if !m.Active || cmd != nil {
		t.Error("esc must be ignored while a request is in flight")
	}
}

func TestErrorShownFriendlyAndModalStaysOpen(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(sgCreateErrMsg{err: errors.New("boom")})
	if !m.Active || m.err == "" || !strings.Contains(m.View(), m.err) {
		t.Errorf("Active=%v err=%q, want open modal showing the error", m.Active, m.err)
	}
}

func TestSuccessReportsActionPerMode(t *testing.T) {
	cases := map[string]struct {
		m      Model
		action string
	}{
		"create": {typeText(New(nil), "web"), "Created security group"},
		"rename": {NewRename(nil, "sg-1", "web", ""), "Renamed security group"},
		"clone":  {NewClone(nil, "sg-1", "web", ""), "Cloned security group"},
	}
	for name, c := range cases {
		m, cmd := c.m.Update(sgCreatedMsg{})
		if m.Active || cmd == nil {
			t.Errorf("%s: Active=%v cmd=%v, want closed with result", name, m.Active, cmd != nil)
			continue
		}
		got, ok := cmd().(shared.ResourceActionMsg)
		if !ok || got.Action != c.action {
			t.Errorf("%s: result=%#v, want action %q", name, got, c.action)
		}
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	testutil.AssertRenderSafe(t, m.View(), 40)
	m.err = "Security group name is required"
	testutil.AssertRenderSafe(t, m.View(), 40)
}

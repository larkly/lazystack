package networkcreate

import (
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

func TestNewStartsActiveOnNameField(t *testing.T) {
	m := New(nil)
	if !m.Active {
		t.Error("modal should start active")
	}
	if m.focusField != fieldName {
		t.Errorf("focusField = %d, want fieldName", m.focusField)
	}
	if !m.nameInput.Focused() {
		t.Error("name input should be focused initially")
	}
	if m.adminState != 0 {
		t.Errorf("adminState = %d, want 0 (Up)", m.adminState)
	}
}

func TestTabCyclesFieldsAndFocusFollows(t *testing.T) {
	m := New(nil)
	want := []int{fieldAdminState, fieldSubmit, fieldCancel, fieldName}
	for i, w := range want {
		m, _ = press(m, tea.Key{Code: tea.KeyTab})
		if m.focusField != w {
			t.Fatalf("tab %d: focusField = %d, want %d", i+1, m.focusField, w)
		}
	}
	if !m.nameInput.Focused() {
		t.Error("name input should regain focus after wrapping around")
	}

	m, _ = press(m, tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focusField != fieldCancel {
		t.Errorf("shift+tab from name: focusField = %d, want fieldCancel", m.focusField)
	}
	if m.nameInput.Focused() {
		t.Error("name input should be blurred when focus leaves it")
	}
}

func TestArrowKeysCycleAdminState(t *testing.T) {
	m := New(nil)
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	m, _ = press(m, tea.Key{Code: tea.KeyRight})
	if m.adminState != 1 {
		t.Fatalf("adminState after right = %d, want 1 (Down)", m.adminState)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyRight})
	if m.adminState != 0 {
		t.Fatalf("adminState after second right = %d, want wrap to 0", m.adminState)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyLeft})
	if m.adminState != 1 {
		t.Fatalf("adminState after left = %d, want wrap to 1", m.adminState)
	}
}

func TestSubmitWithEmptyNameShowsErrorAndNoCommand(t *testing.T) {
	m := New(nil)
	m, cmd := press(m, tea.Key{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Error("empty name must not produce a command")
	}
	if m.err != "Network name is required" {
		t.Errorf("err = %q, want %q", m.err, "Network name is required")
	}
	if m.submitting {
		t.Error("must not enter submitting state")
	}
	if !strings.Contains(m.View(), "Network name is required") {
		t.Error("view should render the validation error")
	}
}

func TestSubmitWithWhitespaceOnlyNameIsRejected(t *testing.T) {
	m := typeText(New(nil), "   ")
	m, cmd := press(m, tea.Key{Code: 's', Mod: tea.ModCtrl})
	if cmd != nil || m.err == "" {
		t.Errorf("whitespace name: cmd=%v err=%q, want validation error", cmd != nil, m.err)
	}
}

func TestSubmitWithNameStartsRequest(t *testing.T) {
	m := typeText(New(nil), "mynet")
	if m.nameInput.Value() != "mynet" {
		t.Fatalf("name = %q, want mynet", m.nameInput.Value())
	}
	m, cmd := press(m, tea.Key{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("valid name should produce a command")
	}
	if !m.submitting || m.err != "" {
		t.Errorf("submitting=%v err=%q, want submitting with no error", m.submitting, m.err)
	}
	if !strings.Contains(m.View(), "Creating") {
		t.Error("view should show the creating state")
	}
}

func TestKeysIgnoredWhileSubmitting(t *testing.T) {
	m := typeText(New(nil), "mynet")
	m, _ = press(m, tea.Key{Code: 's', Mod: tea.ModCtrl})
	m, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if !m.Active || cmd != nil {
		t.Error("esc must be ignored while a request is in flight")
	}
}

func TestEscCancels(t *testing.T) {
	m := New(nil)
	m, _ = press(m, tea.Key{Code: tea.KeyEscape})
	if m.Active {
		t.Error("esc on the name field should close the modal")
	}

	m = New(nil)
	m, _ = press(m, tea.Key{Code: tea.KeyTab})
	m, _ = press(m, tea.Key{Code: tea.KeyEscape})
	if m.Active {
		t.Error("esc on a non-text field should close the modal")
	}
}

func TestErrorMessageIsShownAndModalStaysOpen(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(netCreateErrMsg{err: errString("boom")})
	if !m.Active || m.err == "" {
		t.Errorf("Active=%v err=%q, want open modal with a friendly error", m.Active, m.err)
	}
	if !strings.Contains(m.View(), m.err) {
		t.Error("view should render the error")
	}
}

func TestCreatedMessageClosesAndReportsName(t *testing.T) {
	m := typeText(New(nil), "mynet")
	m, cmd := m.Update(netCreatedMsg{})
	if m.Active {
		t.Error("modal should close after creation")
	}
	if cmd == nil {
		t.Fatal("expected a result command")
	}
	got, ok := cmd().(shared.ResourceActionMsg)
	if !ok || got.Name != "mynet" || got.Action != "Created network" {
		t.Errorf("result = %#v, want Created network / mynet", got)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	testutil.AssertRenderSafe(t, m.View(), 40)
	m.err = "Network name is required"
	testutil.AssertRenderSafe(t, m.View(), 40)
}

type errString string

func (e errString) Error() string { return string(e) }

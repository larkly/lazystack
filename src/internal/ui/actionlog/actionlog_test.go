package actionlog

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func sampleActions(n int) []compute.Action {
	out := make([]compute.Action, n)
	for i := range out {
		out[i] = compute.Action{
			Action:    "reboot",
			RequestID: "req-" + strings.Repeat("x", 30),
			StartTime: time.Now().Add(-time.Duration(i+1) * time.Hour),
		}
	}
	if n > 0 {
		out[0].Action = "stop"
	}
	return out
}

func loaded(n int) Model {
	m := New(nil, "srv-1", "web")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 15})
	m, _ = m.Update(actionsLoadedMsg{actions: sampleActions(n)})
	return m
}

func TestNewIsLoadingAndShowsTitle(t *testing.T) {
	m := New(nil, "srv-1", "web")
	if !m.loading || m.scroll != 0 {
		t.Errorf("loading=%v scroll=%d, want loading at top", m.loading, m.scroll)
	}
	if !strings.Contains(m.View(), "Action History: web") {
		t.Error("view should be titled with the server name")
	}
}

func TestLoadedActionsRenderRows(t *testing.T) {
	m := loaded(3)
	if m.loading {
		t.Fatal("loaded message should end loading")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Action", "Request ID", "stop", "reboot", "h ago", "req-xxxxxxxxxxxxxxxx…"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestEmptyActionsSayNoneRecorded(t *testing.T) {
	m := loaded(0)
	if !strings.Contains(m.View(), "No actions recorded.") {
		t.Error("empty list should say so")
	}
}

func TestScrollingIsClampedAtBothEnds(t *testing.T) {
	m := loaded(30) // viewHeight = 15-5 = 10, max scroll 20
	m, _ = press(m, tea.Key{Code: tea.KeyUp})
	if m.scroll != 0 {
		t.Errorf("up at top: scroll=%d, want 0", m.scroll)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	if m.scroll != 1 {
		t.Errorf("down: scroll=%d, want 1", m.scroll)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	if m.scroll != 11 {
		t.Errorf("page down: scroll=%d, want 11", m.scroll)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	if m.scroll != 20 {
		t.Errorf("page down past end: scroll=%d, want clamped to 20", m.scroll)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyDown})
	if m.scroll != 20 {
		t.Errorf("down at end: scroll=%d, want 20", m.scroll)
	}
	m, _ = press(m, tea.Key{Code: tea.KeyPgUp})
	m, _ = press(m, tea.Key{Code: tea.KeyPgUp})
	m, _ = press(m, tea.Key{Code: tea.KeyPgUp})
	if m.scroll != 0 {
		t.Errorf("page up past top: scroll=%d, want 0", m.scroll)
	}
}

func TestScrolledViewShowsOnlyVisibleWindow(t *testing.T) {
	m := loaded(30)
	for i := range m.actions {
		m.actions[i].Action = "act-" + string(rune('a'+i%26)) + string(rune('A'+i/26))
	}
	first := ansi.Strip(m.View())
	if !strings.Contains(first, "act-aA") || strings.Contains(first, "act-kA") {
		t.Errorf("unscrolled view should show rows 0-9 only:\n%s", first)
	}
	m.scroll = 20
	later := ansi.Strip(m.View())
	if strings.Contains(later, "act-aA") || !strings.Contains(later, "act-uA") {
		t.Errorf("scrolled view should show rows 20-29:\n%s", later)
	}
}

func TestEscReturnsToServerDetail(t *testing.T) {
	m := loaded(3)
	_, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc should produce a view change")
	}
	got, ok := cmd().(shared.ViewChangeMsg)
	if !ok || got.View != "serverdetail" {
		t.Errorf("result = %#v, want ViewChangeMsg to serverdetail", got)
	}
}

func TestErrorShowsFriendlyLineAndKeepsRows(t *testing.T) {
	m := loaded(3)
	m, _ = m.Update(actionsErrMsg{err: errors.New("boom")})
	if m.loading || m.err == nil {
		t.Fatalf("loading=%v err=%v, want error recorded", m.loading, m.err)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, shared.ParseError(m.err).FriendlyMessage) {
		t.Errorf("view missing friendly error line:\n%s", v)
	}
	if !strings.Contains(v, "reboot") {
		t.Errorf("rows should stay visible after an error:\n%s", v)
	}
}

func TestSuccessfulReloadClearsError(t *testing.T) {
	m := loaded(3)
	m, _ = m.Update(actionsErrMsg{err: errors.New("boom")})
	m, _ = m.Update(actionsLoadedMsg{actions: sampleActions(2)})
	if m.err != nil {
		t.Errorf("err = %v, want cleared after a successful load", m.err)
	}
}

func TestForceRefreshSetsLoading(t *testing.T) {
	m := loaded(3)
	cmd := m.ForceRefresh()
	if !m.loading || cmd == nil {
		t.Errorf("loading=%v cmd=%v, want reload started", m.loading, cmd != nil)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := loaded(5)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	m, _ = m.Update(actionsErrMsg{err: errors.New("boom")})
	v := m.View()
	// Table rows are fixed-width columns and are not clamped to the
	// terminal width, so only validity is checked for the whole view.
	testutil.AssertRenderSafe(t, v, 0)
	// The error line is width-aware and must fit in 40 cells.
	line := shared.LoadErrorLine(m.err, 40)
	if !strings.Contains(v, line) {
		t.Error("view should contain the width-clamped error line")
	}
	testutil.AssertRenderSafe(t, line, 40)
}

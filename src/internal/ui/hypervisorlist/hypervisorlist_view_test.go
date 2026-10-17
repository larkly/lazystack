package hypervisorlist

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func hosts(n int) []compute.Hypervisor {
	out := make([]compute.Hypervisor, n)
	for i := range out {
		out[i] = compute.Hypervisor{
			Name:         fmt.Sprintf("compute-%02d", i),
			Status:       "enabled",
			VCPUs:        64,
			VCPUsUsed:    8,
			MemoryMB:     262144,
			MemoryMBUsed: 65536,
			RunningVMs:   i,
		}
	}
	return out
}

func loaded(n, height int) Model {
	m := New(nil)
	m.SetSize(100, height)
	m, _ = m.Update(hypervisorsLoadedMsg{items: hosts(n)})
	return m
}

func TestNewIsLoading(t *testing.T) {
	m := New(nil)
	if !m.loading || m.cursor != 0 || m.scroll != 0 {
		t.Errorf("loading=%v cursor=%d scroll=%d, want loading at top", m.loading, m.cursor, m.scroll)
	}
	if !strings.Contains(m.View(), "Hypervisors") {
		t.Error("view should carry the title while loading")
	}
}

func TestLoadedItemsRenderRowsWithUsage(t *testing.T) {
	m := loaded(3, 20)
	if m.loading {
		t.Fatal("loaded message should end loading")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Hostname", "Running VMs", "compute-00", "compute-02", "8/64", "64/256"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "▶ compute-00") {
		t.Errorf("cursor marker should be on the first row:\n%s", v)
	}
}

func TestEmptyListSaysNoHypervisors(t *testing.T) {
	m := loaded(0, 20)
	if !strings.Contains(m.View(), "No hypervisors found.") {
		t.Error("empty list should say so")
	}
}

func TestCursorMovesAndClamps(t *testing.T) {
	m := loaded(3, 20)
	m, _ = press(m, tea.Key{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("up at top: cursor=%d, want 0", m.cursor)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.Key{Code: tea.KeyDown})
	}
	if m.cursor != 2 {
		t.Errorf("down past end: cursor=%d, want clamped to 2", m.cursor)
	}
	if !strings.Contains(ansi.Strip(m.View()), "▶ compute-02") {
		t.Error("cursor marker should follow the cursor")
	}
}

func TestPagingScrollsAndClamps(t *testing.T) {
	m := loaded(30, 14) // 10 visible rows
	m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	if m.cursor != 10 || m.scroll != 1 {
		t.Errorf("page down: cursor=%d scroll=%d, want 10 and 1", m.cursor, m.scroll)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	}
	if m.cursor != 29 {
		t.Errorf("page down past end: cursor=%d, want clamped to 29", m.cursor)
	}
	if m.cursor < m.scroll || m.cursor >= m.scroll+m.visibleRows() {
		t.Errorf("cursor %d off screen: scroll=%d rows=%d", m.cursor, m.scroll, m.visibleRows())
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.Key{Code: tea.KeyPgUp})
	}
	if m.cursor != 0 || m.scroll != 0 {
		t.Errorf("page up past top: cursor=%d scroll=%d, want 0 and 0", m.cursor, m.scroll)
	}
}

func TestEscReturnsToServerList(t *testing.T) {
	m := loaded(3, 20)
	_, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc should produce a view change")
	}
	got, ok := cmd().(shared.ViewChangeMsg)
	if !ok || got.View != "serverlist" {
		t.Errorf("result = %#v, want ViewChangeMsg to serverlist", got)
	}
}

func TestErrorShowsFriendlyLineAndKeepsRows(t *testing.T) {
	m := loaded(3, 20)
	m, _ = m.Update(hypervisorsErrMsg{err: errors.New("boom")})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, shared.ParseError(m.err).FriendlyMessage) {
		t.Errorf("view missing friendly error line:\n%s", v)
	}
	if !strings.Contains(v, "compute-00") {
		t.Errorf("rows should stay visible after an error:\n%s", v)
	}
}

func TestSuccessfulReloadClearsError(t *testing.T) {
	m := loaded(3, 20)
	m, _ = m.Update(hypervisorsErrMsg{err: errors.New("boom")})
	m, _ = m.Update(hypervisorsLoadedMsg{items: hosts(2)})
	if m.err != nil {
		t.Errorf("err = %v, want cleared after a successful load", m.err)
	}
}

func TestDisabledHostsStillRender(t *testing.T) {
	m := New(nil)
	m.SetSize(100, 20)
	items := hosts(2)
	items[1].Status = "disabled"
	m, _ = m.Update(hypervisorsLoadedMsg{items: items})
	if !strings.Contains(ansi.Strip(m.View()), "disabled") {
		t.Error("disabled status should be shown")
	}
}

func TestForceRefreshSetsLoading(t *testing.T) {
	m := loaded(3, 20)
	cmd := m.ForceRefresh()
	if !m.loading || cmd == nil {
		t.Errorf("loading=%v cmd=%v, want reload started", m.loading, cmd != nil)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := loaded(5, 20)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	m, _ = m.Update(hypervisorsErrMsg{err: errors.New("boom")})
	v := m.View()
	testutil.AssertRenderSafe(t, v, 0)
	// Data rows and the error line are clamped to the width. The column
	// header row is not (it is 80 cells wide); current behaviour, noted here
	// rather than asserted.
	rows := 0
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(ansi.Strip(line), "compute-") {
			rows++
			testutil.AssertRenderSafe(t, line, 40)
		}
	}
	if rows == 0 {
		t.Error("expected data rows in the narrow view")
	}
	testutil.AssertRenderSafe(t, shared.LoadErrorLine(m.err, 40), 40)
}

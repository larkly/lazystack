package servicecatalog

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/testutil"
)

func press(m Model, k tea.Key) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg(k))
}

func entries(n int) []compute.ServiceEntry {
	out := make([]compute.ServiceEntry, n)
	for i := range out {
		out[i] = compute.ServiceEntry{
			Name:      fmt.Sprintf("service-%02d", i),
			Type:      "compute",
			Available: i%2 == 0,
			Endpoints: []compute.EndpointInfo{{URL: fmt.Sprintf("https://cloud.example.com/svc%d", i)}},
		}
	}
	return out
}

func loaded(n, height int) Model {
	m := New(nil, gophercloud.EndpointOpts{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: height})
	m, _ = m.Update(catalogLoadedMsg{entries: entries(n)})
	return m
}

func TestNewShowsLoadingPlaceholder(t *testing.T) {
	m := New(nil, gophercloud.EndpointOpts{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if !m.loading || !strings.Contains(m.View(), "Loading service catalog") {
		t.Errorf("loading=%v, want loading placeholder in view", m.loading)
	}
}

func TestLoadedEntriesRenderRowsAndAvailableCount(t *testing.T) {
	m := loaded(4, 30)
	if m.loading {
		t.Fatal("loaded message should end loading")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Service Catalog", "Public Endpoint", "service-00", "service-03", "https://cloud.example.com/svc0", "ready", "2/4 available"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "https://cloud.example.com/svc1") {
		t.Error("unavailable services must not list their endpoint")
	}
}

func TestCursorMovesAndClamps(t *testing.T) {
	m := loaded(3, 30)
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
	m, _ = press(m, tea.Key{Code: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("up: cursor=%d, want 1", m.cursor)
	}
}

func TestPagingClampsAtBothEnds(t *testing.T) {
	m := loaded(30, 16) // 10 rows per page in Update
	m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	if m.cursor != 10 {
		t.Errorf("page down: cursor=%d, want 10", m.cursor)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.Key{Code: tea.KeyPgDown})
	}
	if m.cursor != 29 {
		t.Errorf("page down past end: cursor=%d, want 29", m.cursor)
	}
	for i := 0; i < 5; i++ {
		m, _ = press(m, tea.Key{Code: tea.KeyPgUp})
	}
	if m.cursor != 0 {
		t.Errorf("page up past top: cursor=%d, want 0", m.cursor)
	}
}

func TestEscReturnsToServerList(t *testing.T) {
	m := loaded(3, 30)
	_, cmd := press(m, tea.Key{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc should produce a view change")
	}
	got, ok := cmd().(shared.ViewChangeMsg)
	if !ok || got.View != "serverlist" {
		t.Errorf("result = %#v, want ViewChangeMsg to serverlist", got)
	}
}

func TestEmptyCatalogRendersZeroCount(t *testing.T) {
	m := loaded(0, 30)
	if !strings.Contains(ansi.Strip(m.View()), "0/0 available") {
		t.Error("empty catalog should render a 0/0 footer")
	}
}

// The package has no error message: a failed catalog fetch is folded into the
// loaded message by compute.FetchServiceCatalog, so err is only ever set by
// tests. This documents that a set error renders the friendly line and keeps
// the rows.
func TestErrorLineKeepsRows(t *testing.T) {
	m := loaded(3, 30)
	m.err = errors.New("boom")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, shared.ParseError(m.err).FriendlyMessage) {
		t.Errorf("view missing friendly error line:\n%s", v)
	}
	if !strings.Contains(v, "service-00") {
		t.Errorf("rows should stay visible with an error:\n%s", v)
	}
}

func TestViewAtNarrowWidth(t *testing.T) {
	m := loaded(5, 30)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	// Row columns are fixed-width (about 100 cells); lipgloss.Place does not
	// clip them. Check validity, and that the width-aware pieces fit.
	v := m.View()
	testutil.AssertRenderSafe(t, v, 0)
	m.err = errors.New("boom")
	line := shared.LoadErrorLine(m.err, 40)
	testutil.AssertRenderSafe(t, line, 40)
	if !strings.Contains(m.View(), line) {
		t.Error("view should contain the width-clamped error line")
	}
}

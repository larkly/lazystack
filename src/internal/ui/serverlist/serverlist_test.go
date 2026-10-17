package serverlist

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

func TestDescendingSortKeepsTiesStable(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.filtered = []compute.Server{
		{ID: "s-1", Name: "a", Status: "SHUTOFF"},
		{ID: "s-2", Name: "b", Status: "ACTIVE"},
		{ID: "s-3", Name: "c", Status: "SHUTOFF"},
		{ID: "s-4", Name: "d", Status: "SHUTOFF"},
	}
	m.sortCol = -1
	for i := 0; m.visibleColKey(i) != ""; i++ {
		if m.visibleColKey(i) == "status" {
			m.sortCol = i
		}
	}
	if m.sortCol < 0 {
		t.Fatal("status column not visible by default")
	}
	m.sortAsc = false
	m.sortServers()

	want := []string{"s-1", "s-3", "s-4", "s-2"}
	for i, s := range m.filtered {
		if s.ID != want[i] {
			var got []string
			for _, g := range m.filtered {
				got = append(got, g.ID)
			}
			t.Fatalf("descending by status = %v, want %v (ties must keep input order)", got, want)
		}
	}
}

func TestEscClearsFilterAndExitsFiltering(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '/'}))
	m = updated
	if !m.filtering {
		t.Fatalf("expected filtering mode to be enabled")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	m = updated
	if got := m.filter.Value(); got != "a" {
		t.Fatalf("filter value = %q, want %q", got, "a")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = updated

	if m.filtering {
		t.Fatalf("expected filtering mode to be disabled after esc")
	}
	if got := m.filter.Value(); got != "" {
		t.Fatalf("filter value = %q, want empty after esc", got)
	}
	if len(m.filtered) != len(m.servers) {
		t.Fatalf("filtered len = %d, want %d after clear", len(m.filtered), len(m.servers))
	}
}

func TestEscClearsSelectionAsAdvertised(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()
	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	}
	if m.SelectionCount() != 2 || !strings.Contains(m.Hints(), "esc clear") {
		t.Fatalf("setup: selection=%d hints=%q", m.SelectionCount(), m.Hints())
	}
	m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.SelectionCount() != 0 || cmd != nil {
		t.Fatalf("esc: selection=%d cmd=%v, want cleared with no command", m.SelectionCount(), cmd != nil)
	}
}

func TestRenderServerRow_IPv6SuffixTruncation(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.columns = []Column{
		{Title: "IPv6", MinWidth: 15, Flex: 0, Priority: 0, Key: "ipv6"},
	}
	m.columns = ComputeWidths(m.columns, 40, nil)

	longIPv6 := "2001:db8:85a3:0000:0000:8a2e:0370:7334"
	s := compute.Server{ID: "s-1", Name: "n", IPv6: []string{longIPv6}}

	row := m.renderServerRow(s, false)

	if !strings.Contains(row, "…") {
		t.Errorf("expected ellipsis in row, got %q", row)
	}
	suffix := longIPv6[len(longIPv6)-5:]
	if !strings.Contains(row, suffix) {
		t.Errorf("expected suffix %q preserved in row, got %q", suffix, row)
	}
	prefix := longIPv6[:5]
	if strings.Contains(row, prefix) {
		t.Errorf("expected prefix %q to be truncated out, but found it in %q", prefix, row)
	}
}

func TestRenderServerRow_OtherColumnsPrefixTruncation(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.columns = []Column{
		{Title: "Name", MinWidth: 10, Flex: 0, Priority: 0, Key: "name"},
	}
	m.columns = ComputeWidths(m.columns, 30, nil)

	longName := "very-long-server-name-here"
	s := compute.Server{ID: "s-1", Name: longName}

	row := m.renderServerRow(s, false)

	if !strings.Contains(row, "…") {
		t.Errorf("expected ellipsis in row, got %q", row)
	}
	if !strings.Contains(row, longName[:5]) {
		t.Errorf("expected prefix preserved for non-ipv6 column, got %q", row)
	}
}

// Background ticks must not stack list fetches, and a slow older response
// must never overwrite the result of a newer fetch.
func TestTickRefreshesDoNotOverlapOrRegress(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.loading = false

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch")
	}
	staleSeq := m.refresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	// A manual refresh supersedes the slow tick fetch.
	m.ForceRefresh()
	newSeq := m.refresh.Seq()
	m, _ = m.Update(serversLoadedMsg{seq: newSeq, servers: []compute.Server{{ID: "new", Name: "new"}}})
	m, _ = m.Update(serversLoadedMsg{seq: staleSeq, servers: []compute.Server{{ID: "old", Name: "old"}}})
	m, _ = m.Update(serversErrMsg{seq: staleSeq, err: errors.New("stale failure")})
	if len(m.servers) != 1 || m.servers[0].ID != "new" || m.err != nil {
		t.Fatalf("stale response applied: servers=%v err=%v", m.servers, m.err)
	}
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}

func fiftyServers() []compute.Server {
	servers := make([]compute.Server, 50)
	for i := range servers {
		id := fmt.Sprintf("s-%02d", i)
		servers[i] = compute.Server{ID: id, Name: "srv-" + id}
	}
	return servers
}

// An auto-refresh (and the image-name lookup that follows it) must keep the
// cursor row on screen instead of snapping the viewport back to the top.
func TestRefreshKeepsCursorRowVisible(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.width = 120
	for m.height = 1; m.tableHeight() < 10; m.height++ {
	}
	m.loading = false
	m.servers = fiftyServers()
	m.applyFilter()
	for m.cursor < 30 {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	assertVisible := func(stage string) {
		t.Helper()
		if m.cursor != 30 {
			t.Fatalf("%s: cursor = %d, want 30", stage, m.cursor)
		}
		if h := m.tableHeight(); m.cursor < m.scrollOff || m.cursor >= m.scrollOff+h {
			t.Fatalf("%s: cursor %d outside viewport [%d, %d)", stage, m.cursor, m.scrollOff, m.scrollOff+h)
		}
	}
	assertVisible("setup")

	m, _ = m.Update(serversLoadedMsg{seq: m.refresh.Seq(), servers: fiftyServers()})
	assertVisible("after servers loaded")

	m, _ = m.Update(imageNamesMsg{"img": "ubuntu"})
	assertVisible("after image names")
}

// commitFilter types query into the filter and confirms it with enter, so the
// filter stays applied after filter input mode ends.
func commitFilter(t *testing.T, m Model, query string) Model {
	t.Helper()
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/'}))
	for _, r := range query {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.filtering || m.filter.Value() != query {
		t.Fatalf("setup: filtering=%v filter=%q, want committed %q", m.filtering, m.filter.Value(), query)
	}
	return m
}

func TestEscClearsCommittedFilter(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()
	m = commitFilter(t, m, "alp")
	if len(m.filtered) != 1 {
		t.Fatalf("setup: filtered len = %d, want 1", len(m.filtered))
	}

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if got := m.filter.Value(); got != "" {
		t.Fatalf("filter = %q after esc, want cleared", got)
	}
	if len(m.filtered) != len(m.servers) {
		t.Fatalf("filtered len = %d after esc, want %d", len(m.filtered), len(m.servers))
	}
}

// With both a committed filter and a selection, esc clears the filter first
// and the selection on the next press.
func TestEscClearsFilterBeforeSelection(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "beta"},
	}
	m.applyFilter()
	m = commitFilter(t, m, "alp")
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	if m.SelectionCount() != 1 {
		t.Fatalf("setup: selection = %d, want 1", m.SelectionCount())
	}

	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.filter.Value() != "" || m.SelectionCount() != 1 {
		t.Fatalf("first esc: filter=%q selection=%d, want filter cleared and selection kept", m.filter.Value(), m.SelectionCount())
	}
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if m.SelectionCount() != 0 {
		t.Fatalf("second esc: selection = %d, want cleared", m.SelectionCount())
	}
}

// The empty-state hint must name the create key (c is clone).
func TestEmptyStateHintNamesCreateKey(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(80, 24)
	m.loading = false
	got := m.View()
	want := "Press [" + shared.Keys.Create.Help().Key + "] to create one."
	if !strings.Contains(got, want) {
		t.Fatalf("empty view = %q, want it to contain %q", got, want)
	}
}

// Under a filter, the advertised bulk count must match the servers a bulk
// action would actually target.
func TestSelectionCountMatchesFilteredTargets(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-2", Name: "alpine"},
		{ID: "s-3", Name: "beta"},
	}
	m.applyFilter()
	m.SelectIDs([]string{"s-1", "s-3"})
	m = commitFilter(t, m, "alp")

	if got := len(m.SelectedServers()); got != 1 {
		t.Fatalf("SelectedServers len = %d, want 1 (beta is filtered out)", got)
	}
	if got := m.SelectionCount(); got != 1 {
		t.Fatalf("SelectionCount = %d, want 1", got)
	}
	if h := m.Hints(); !strings.Contains(h, "(1 selected)") {
		t.Fatalf("hints = %q, want the listed count (1 selected)", h)
	}
}

// When every selected server is hidden by the filter, nothing listed is
// selected: actions fall back to the cursor server, as the hints show.
func TestHiddenSelectionFallsBackToCursor(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.servers = []compute.Server{
		{ID: "s-1", Name: "alpha"},
		{ID: "s-3", Name: "beta"},
	}
	m.applyFilter()
	m.SelectIDs([]string{"s-3"})
	m = commitFilter(t, m, "alp")

	if got := m.SelectionCount(); got != 0 {
		t.Fatalf("SelectionCount = %d, want 0", got)
	}
	if got := m.SelectedServers(); len(got) != 1 || got[0].ID != "s-1" {
		t.Fatalf("SelectedServers = %v, want the cursor server s-1", got)
	}
	if h := m.Hints(); strings.Contains(h, "selected") {
		t.Fatalf("hints = %q, want no selection count", h)
	}
}

// The view gets the full terminal height; the root model prepends a tab bar
// and keeps the last line for the status bar, so a full list must render in
// at most height-2 lines or its bottom row is clipped.
func TestFullListFitsBetweenTabBarAndStatusBar(t *testing.T) {
	const height = 24
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, height)
	m, _ = m.Update(serversLoadedMsg{seq: m.refresh.Seq(), servers: fiftyServers()})
	lines := strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n")
	if len(lines) > height-2 {
		t.Fatalf("full list rendered %d lines, want at most %d", len(lines), height-2)
	}
}

// A failed refresh must not wipe the table: the last good rows stay and the
// error shows as one friendly line, which the next good load clears.
func TestRefreshErrorKeepsRowsAndShowsFriendlyError(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(serversLoadedMsg{seq: m.refresh.Seq(), servers: []compute.Server{{ID: "s-1", Name: "web-01", Status: "ACTIVE"}}})

	seq := m.refresh.Start()
	apiErr := gophercloud.ErrUnexpectedResponseCode{Actual: 503}
	m, _ = m.Update(serversErrMsg{seq: seq, err: apiErr})
	view := m.View()
	friendly := shared.ParseError(apiErr).FriendlyMessage
	if !strings.Contains(view, "web-01") {
		t.Fatalf("failed refresh wiped the rows:\n%s", view)
	}
	if !strings.Contains(view, friendly) {
		t.Fatalf("view lacks the friendly error %q:\n%s", friendly, view)
	}

	seq = m.refresh.Start()
	m, _ = m.Update(serversLoadedMsg{seq: seq, servers: []compute.Server{{ID: "s-1", Name: "web-01", Status: "ACTIVE"}}})
	if strings.Contains(m.View(), friendly) {
		t.Fatalf("successful load did not clear the error:\n%s", m.View())
	}
}

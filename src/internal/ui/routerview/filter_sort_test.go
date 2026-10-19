package routerview

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/larkly/lazystack/internal/network"
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}) }

var (
	enterKey = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	escKey   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	downKey  = tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
)

func filterRouters() []network.Router {
	return []network.Router{
		{ID: "r1", Name: "edge", Status: "ACTIVE", ExternalGatewayIPv6: "2001:db8::1", Routes: []network.Route{{}, {}}},
		{ID: "r2", Name: "core", Status: "ERROR"},
		{ID: "r3", Name: "branch", Status: "ACTIVE", Routes: []network.Route{{}, {}, {}}},
	}
}

func loadedRouters(t *testing.T) Model {
	t.Helper()
	m := New(nil, time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(routersLoadedMsg{seq: m.refresh.Seq(), routers: filterRouters()})
	return m
}

func typeFilter(m Model, q string) Model {
	m, _ = m.Update(runeKey('/'))
	for _, r := range q {
		m, _ = m.Update(runeKey(r))
	}
	m, _ = m.Update(enterKey)
	return m
}

func routerNames(m Model) string {
	var out []string
	for _, r := range m.routers {
		out = append(out, r.Name)
	}
	return strings.Join(out, ",")
}

func TestRoutersSortByNameByDefault(t *testing.T) {
	if got := routerNames(loadedRouters(t)); got != "branch,core,edge" {
		t.Fatalf("routers = %s, want branch,core,edge", got)
	}
}

func TestSortCyclesStatusThenRouteCount(t *testing.T) {
	m := loadedRouters(t)
	m, cmd := m.Update(runeKey('s'))
	if cmd == nil || !m.order.Highlight {
		t.Fatal("s did not highlight the new sort")
	}
	if got := routerNames(m); got != "branch,edge,core" {
		t.Fatalf("by status = %s, want ACTIVE ones before ERROR", got)
	}
	m, _ = m.Update(runeKey('s'))
	if got := routerNames(m); got != "core,edge,branch" {
		t.Fatalf("by routes = %s, want core,edge,branch", got)
	}
	m, _ = m.Update(runeKey('S'))
	if got := routerNames(m); got != "branch,edge,core" {
		t.Fatalf("by routes reversed = %s", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "by routes ▼") {
		t.Fatalf("selector title does not show the sort:\n%s", v)
	}
}

func TestSortKeepsCursorOnSameRouter(t *testing.T) {
	m := loadedRouters(t)
	m, _ = m.Update(downKey) // core
	m, _ = m.Update(runeKey('s'))
	if got := m.SelectedRouterName(); got != "core" {
		t.Fatalf("cursor on %q after sort, want core", got)
	}
}

func TestFilterNarrowsRoutersAndFollowsSelection(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "2001:DB8")
	if m.IsFiltering() || m.FilterValue() != "2001:DB8" || routerNames(m) != "edge" {
		t.Fatalf("filter: active=%v value=%q routers=%s", m.IsFiltering(), m.FilterValue(), routerNames(m))
	}
	if m.SelectedRouterID() != "r1" || m.lastDetailID != "r1" || !m.detailLoading {
		t.Fatalf("selection %q detail %q loading=%v; want r1 being fetched", m.SelectedRouterID(), m.lastDetailID, m.detailLoading)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "/2001:DB8") {
		t.Fatalf("selector title does not show the filter:\n%s", v)
	}
}

func TestFilterPersistsAcrossRefresh(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "active")
	m, _ = m.Update(downKey)
	want := m.SelectedRouterID()
	routers := append(filterRouters(), network.Router{ID: "r4", Name: "access", Status: "ACTIVE"})
	m, _ = m.Update(routersLoadedMsg{seq: m.refresh.Start(), routers: routers})
	if m.FilterValue() != "active" || routerNames(m) != "access,branch,edge" {
		t.Fatalf("after refresh: filter=%q routers=%s", m.FilterValue(), routerNames(m))
	}
	if m.SelectedRouterID() != want {
		t.Fatalf("cursor on %s after refresh, want %s", m.SelectedRouterID(), want)
	}
}

func TestEscClearsRouterFilter(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "core")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.routers) != 3 {
		t.Fatalf("esc: filter=%q routers=%s", m.FilterValue(), routerNames(m))
	}
}

func TestFilterWithNoMatchesClearsDetail(t *testing.T) {
	m := loadedRouters(t)
	m = typeFilter(m, "zzz")
	if m.SelectedRouterID() != "" || m.lastDetailID != "" || m.interfaces != nil {
		t.Fatalf("no match still selects a router: %q", m.SelectedRouterID())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No routers match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

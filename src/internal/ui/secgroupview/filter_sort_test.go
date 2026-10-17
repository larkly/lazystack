package secgroupview

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

func filterGroups() []network.SecurityGroup {
	return []network.SecurityGroup{
		{ID: "g1", Name: "web", Description: "public http", Rules: rulesFixture("r1", "r2")},
		{ID: "g2", Name: "default", Rules: rulesFixture("r3")},
		{ID: "g3", Name: "db", Description: "postgres from web", Rules: rulesFixture("r4", "r5", "r6")},
	}
}

func loadedGroups(t *testing.T) Model {
	t.Helper()
	m := New(nil, time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(sgLoadedMsg{seq: m.refresh.Seq(), groups: filterGroups()})
	return m
}

func typeFilter(m Model, q string) (Model, tea.Cmd) {
	m, _ = m.Update(runeKey('/'))
	for _, r := range q {
		m, _ = m.Update(runeKey(r))
	}
	return m.Update(enterKey)
}

func groupNames(m Model) []string {
	var out []string
	for _, g := range m.groups {
		out = append(out, g.Name)
	}
	return out
}

func TestGroupsSortByNameByDefault(t *testing.T) {
	m := loadedGroups(t)
	if got := strings.Join(groupNames(m), ","); got != "db,default,web" {
		t.Fatalf("groups = %s, want db,default,web", got)
	}
}

func TestSortCyclesToRuleCountAndReverses(t *testing.T) {
	m := loadedGroups(t)
	m, cmd := m.Update(runeKey('s'))
	if cmd == nil || !m.order.Highlight {
		t.Fatal("s did not highlight the new sort")
	}
	if got := strings.Join(groupNames(m), ","); got != "default,web,db" {
		t.Fatalf("by rules = %s, want default,web,db", got)
	}
	m, _ = m.Update(runeKey('S'))
	if got := strings.Join(groupNames(m), ","); got != "db,web,default" {
		t.Fatalf("by rules reversed = %s, want db,web,default", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "by rules ▼") {
		t.Fatalf("selector title does not show the sort:\n%s", v)
	}
}

func TestSortKeepsCursorOnSameGroup(t *testing.T) {
	m := loadedGroups(t)
	m, _ = m.Update(downKey) // default
	m, _ = m.Update(runeKey('s'))
	if got := m.SelectedGroupName(); got != "default" {
		t.Fatalf("cursor on %q after sort, want default", got)
	}
}

func TestFilterNarrowsGroupsAndFetchesNewSelection(t *testing.T) {
	m := loadedGroups(t)
	if m.SelectedGroupName() != "db" {
		t.Fatalf("setup: cursor on %q", m.SelectedGroupName())
	}
	m, _ = typeFilter(m, "HTTP")
	if m.IsFiltering() || m.FilterValue() != "HTTP" {
		t.Fatalf("filter not committed: active=%v value=%q", m.IsFiltering(), m.FilterValue())
	}
	if got := strings.Join(groupNames(m), ","); got != "web" {
		t.Fatalf("filtered groups = %s, want web (matches description)", got)
	}
	if m.SelectedGroupName() != "web" || m.lastDetailSGID != "g1" || !m.detailLoading {
		t.Fatalf("selection = %q detail=%q loading=%v; want web with its detail being fetched", m.SelectedGroupName(), m.lastDetailSGID, m.detailLoading)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "/HTTP") {
		t.Fatalf("selector title does not show the filter:\n%s", v)
	}
}

func TestFilterPersistsAcrossRefresh(t *testing.T) {
	m := loadedGroups(t)
	m, _ = typeFilter(m, "d")
	m, _ = m.Update(downKey)
	want := m.SelectedGroupID()
	groups := append(filterGroups(), network.SecurityGroup{ID: "g4", Name: "admin"})
	m, _ = m.Update(sgLoadedMsg{seq: m.refresh.Start(), groups: groups})
	if m.FilterValue() != "d" || len(m.groups) != 3 {
		t.Fatalf("after refresh: filter=%q groups=%v", m.FilterValue(), groupNames(m))
	}
	if m.SelectedGroupID() != want {
		t.Fatalf("cursor on %s after refresh, want %s", m.SelectedGroupID(), want)
	}
}

func TestEscClearsGroupFilter(t *testing.T) {
	m := loadedGroups(t)
	m, _ = typeFilter(m, "web")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.groups) != 3 {
		t.Fatalf("esc: filter=%q groups=%v", m.FilterValue(), groupNames(m))
	}
}

func TestFilterWithNoMatchesClearsDetail(t *testing.T) {
	m := loadedGroups(t)
	m, _ = typeFilter(m, "zzz")
	if m.SelectedGroupID() != "" || m.servers != nil || m.ports != nil {
		t.Fatalf("no match still shows a group's detail: id=%q", m.SelectedGroupID())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No security groups match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

// Jumping to a group from server detail must land on it even when the
// committed filter hides it: the filter gives way.
func TestScrollToNamesClearsFilterThatHidesTarget(t *testing.T) {
	m := loadedGroups(t)
	m, _ = typeFilter(m, "web")
	m.ScrollToNames([]string{"default"})
	if m.FilterValue() != "" || m.SelectedGroupName() != "default" {
		t.Fatalf("filter=%q selected=%q, want filter cleared and default selected", m.FilterValue(), m.SelectedGroupName())
	}
}

package networkview

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

func filterNets() []network.Network {
	return []network.Network{
		{ID: "n1", Name: "public", Status: "ACTIVE", SubnetIDs: []string{"s1", "s2"}},
		{ID: "n2", Name: "app-net", Status: "DOWN", SubnetIDs: []string{"s3"}},
		{ID: "n3", Name: "db-net", Status: "ACTIVE", SubnetIDs: []string{"s4", "s5", "s6"}},
	}
}

func loadedNets(t *testing.T) Model {
	t.Helper()
	m := New(nil, time.Second)
	m.SetSize(120, 40)
	m, _ = m.Update(networksLoadedMsg{seq: m.refresh.Seq(), networks: filterNets()})
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

func netNames(m Model) string {
	var out []string
	for _, n := range m.networks {
		out = append(out, n.Name)
	}
	return strings.Join(out, ",")
}

func TestNetworksSortByNameByDefault(t *testing.T) {
	if got := netNames(loadedNets(t)); got != "app-net,db-net,public" {
		t.Fatalf("networks = %s, want app-net,db-net,public", got)
	}
}

func TestSortCyclesStatusThenSubnetCount(t *testing.T) {
	m := loadedNets(t)
	m, cmd := m.Update(runeKey('s'))
	if cmd == nil || !m.order.Highlight {
		t.Fatal("s did not highlight the new sort")
	}
	if got := netNames(m); got != "db-net,public,app-net" {
		t.Fatalf("by status = %s, want ACTIVE ones (name order kept) before DOWN", got)
	}
	m, _ = m.Update(runeKey('s'))
	if got := netNames(m); got != "app-net,public,db-net" {
		t.Fatalf("by subnets = %s, want app-net,public,db-net", got)
	}
	m, _ = m.Update(runeKey('S'))
	if got := netNames(m); got != "db-net,public,app-net" {
		t.Fatalf("by subnets reversed = %s", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "by subnets ▼") {
		t.Fatalf("selector title does not show the sort:\n%s", v)
	}
}

func TestSortKeepsCursorOnSameNetwork(t *testing.T) {
	m := loadedNets(t)
	m, _ = m.Update(downKey) // db-net
	m, _ = m.Update(runeKey('s'))
	if got := m.SelectedNetworkName(); got != "db-net" {
		t.Fatalf("cursor on %q after sort, want db-net", got)
	}
}

func TestFilterNarrowsNetworksAndFollowsSelection(t *testing.T) {
	m := loadedNets(t)
	m = typeFilter(m, "PUB")
	if m.IsFiltering() || m.FilterValue() != "PUB" || netNames(m) != "public" {
		t.Fatalf("filter: active=%v value=%q networks=%s", m.IsFiltering(), m.FilterValue(), netNames(m))
	}
	if m.SelectedNetworkID() != "n1" || m.lastDetailNetID != "n1" || !m.detailLoading {
		t.Fatalf("selection %q detail %q loading=%v; want n1 being fetched", m.SelectedNetworkID(), m.lastDetailNetID, m.detailLoading)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "/PUB") {
		t.Fatalf("selector title does not show the filter:\n%s", v)
	}
}

func TestFilterPersistsAcrossRefresh(t *testing.T) {
	m := loadedNets(t)
	m = typeFilter(m, "net")
	m, _ = m.Update(downKey)
	want := m.SelectedNetworkID()
	nets := append(filterNets(), network.Network{ID: "n4", Name: "mgmt-net"})
	m, _ = m.Update(networksLoadedMsg{seq: m.refresh.Start(), networks: nets})
	if m.FilterValue() != "net" || netNames(m) != "app-net,db-net,mgmt-net" {
		t.Fatalf("after refresh: filter=%q networks=%s", m.FilterValue(), netNames(m))
	}
	if m.SelectedNetworkID() != want {
		t.Fatalf("cursor on %s after refresh, want %s", m.SelectedNetworkID(), want)
	}
}

func TestEscClearsNetworkFilter(t *testing.T) {
	m := loadedNets(t)
	m = typeFilter(m, "db")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.networks) != 3 {
		t.Fatalf("esc: filter=%q networks=%s", m.FilterValue(), netNames(m))
	}
}

func TestFilterWithNoMatchesClearsDetail(t *testing.T) {
	m := loadedNets(t)
	m = typeFilter(m, "zzz")
	if m.SelectedNetworkID() != "" || m.lastDetailNetID != "" {
		t.Fatalf("no match still selects a network: %q", m.SelectedNetworkID())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No networks match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

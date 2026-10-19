package floatingiplist

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

func filterFIPs() []network.FloatingIP {
	return []network.FloatingIP{
		{ID: "f1", FloatingIP: "2001:db8::10", FixedIP: "fd00::5", Status: "ACTIVE", PortID: "port-a"},
		{ID: "f2", FloatingIP: "2001:db8::20", Status: "DOWN"},
		{ID: "f3", FloatingIP: "2001:db8::30", FixedIP: "fd00::6", Status: "ACTIVE", PortID: "port-b"},
	}
}

func typeFilter(m Model, q string) Model {
	m, _ = m.Update(runeKey('/'))
	for _, r := range q {
		m, _ = m.Update(runeKey(r))
	}
	m, _ = m.Update(enterKey)
	return m
}

func TestFilterNarrowsListAndPersistsAcrossRefresh(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(fipsLoadedMsg{seq: m.refresh.Seq(), fips: filterFIPs()})
	m = typeFilter(m, "active")
	if m.IsFiltering() || m.FilterValue() != "active" || len(m.fips) != 2 {
		t.Fatalf("filter: active=%v value=%q listed=%d, want committed with 2 rows", m.IsFiltering(), m.FilterValue(), len(m.fips))
	}
	m, _ = m.Update(downKey)
	want := m.SelectedFIP().ID
	fips := filterFIPs()
	fips[0], fips[2] = fips[2], fips[0]
	m, _ = m.Update(fipsLoadedMsg{seq: m.refresh.Seq(), fips: fips})
	if len(m.fips) != 2 || m.SelectedFIP().ID != want {
		t.Fatalf("after refresh: listed=%d cursor=%s, want 2 rows and cursor on %s", len(m.fips), m.SelectedFIP().ID, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), "filter: active") {
		t.Fatal("committed filter not shown")
	}
}

func TestFilterMatchesFixedIP(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(fipsLoadedMsg{seq: m.refresh.Seq(), fips: filterFIPs()})
	m = typeFilter(m, "fd00::6")
	if len(m.fips) != 1 || m.fips[0].ID != "f3" {
		t.Fatalf("listed %+v, want only f3", m.fips)
	}
}

func TestEscClearsCommittedFilter(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(fipsLoadedMsg{seq: m.refresh.Seq(), fips: filterFIPs()})
	m = typeFilter(m, "down")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.fips) != 3 {
		t.Fatalf("esc: filter=%q listed=%d, want cleared and 3", m.FilterValue(), len(m.fips))
	}
}

func TestFilterWithNoMatchesSaysSo(t *testing.T) {
	m := New(nil, time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(fipsLoadedMsg{seq: m.refresh.Seq(), fips: filterFIPs()})
	m = typeFilter(m, "zzz")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No floating IPs match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

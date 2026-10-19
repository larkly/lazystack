package dnslist

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}) }

var (
	enterKey = tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	escKey   = tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	downKey  = tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
)

func filterZones() []zones.Zone {
	return []zones.Zone{
		{ID: "z1", Name: "example.com.", Type: "PRIMARY"},
		{ID: "z2", Name: "corp.example.", Type: "SECONDARY"},
		{ID: "z3", Name: "app.example.com.", Type: "PRIMARY"},
	}
}

func loadedZones(t *testing.T) Model {
	t.Helper()
	m := New(nil)
	m.SetSize(120, 30)
	m, _ = m.Update(zonesLoadedMsg{seq: m.refresh.Seq(), zones: filterZones()})
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

func zoneNames(m Model) string {
	var out []string
	for _, z := range m.zones {
		out = append(out, z.Name)
	}
	return strings.Join(out, ",")
}

func TestZonesSortByNameByDefault(t *testing.T) {
	if got := zoneNames(loadedZones(t)); got != "app.example.com.,corp.example.,example.com." {
		t.Fatalf("zones = %s", got)
	}
}

func TestSortCyclesToTypeAndReverses(t *testing.T) {
	m := loadedZones(t)
	m, cmd := m.Update(runeKey('s'))
	if cmd == nil || !m.order.Highlight {
		t.Fatal("s did not highlight the new sort")
	}
	if got := zoneNames(m); got != "app.example.com.,example.com.,corp.example." {
		t.Fatalf("by type = %s, want PRIMARY zones before SECONDARY", got)
	}
	m, _ = m.Update(runeKey('S'))
	if got := zoneNames(m); got != "corp.example.,app.example.com.,example.com." {
		t.Fatalf("by type reversed = %s", got)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Type ▼") {
		t.Fatalf("header does not show the sort:\n%s", v)
	}
}

func TestSortKeepsSelectedZone(t *testing.T) {
	m := loadedZones(t)
	m, _ = m.Update(downKey) // corp.example.
	m, _ = m.Update(runeKey('s'))
	if m.selectedZone == nil || m.selectedZone.ID != "z2" || m.zones[m.cursor].ID != "z2" {
		t.Fatalf("selection moved off corp.example. after sort: %+v", m.selectedZone)
	}
}

func TestFilterNarrowsZonesAndFetchesNewSelection(t *testing.T) {
	m := loadedZones(t)
	m = typeFilter(m, "CORP")
	if m.IsFiltering() || m.FilterValue() != "CORP" || zoneNames(m) != "corp.example." {
		t.Fatalf("filter: active=%v value=%q zones=%s", m.IsFiltering(), m.FilterValue(), zoneNames(m))
	}
	if m.selectedZone == nil || m.selectedZone.ID != "z2" || !m.loading {
		t.Fatalf("selection %+v loading=%v; want z2 with its records loading", m.selectedZone, m.loading)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "filter: CORP") {
		t.Fatalf("committed filter not shown:\n%s", v)
	}
}

func TestFilterPersistsAcrossRefresh(t *testing.T) {
	m := loadedZones(t)
	m = typeFilter(m, "example.com")
	m, _ = m.Update(downKey)
	want := m.selectedZone.ID
	zs := append(filterZones(), zones.Zone{ID: "z4", Name: "www.example.com."})
	m, _ = m.Update(zonesLoadedMsg{seq: m.refresh.Start(), zones: zs})
	if m.FilterValue() != "example.com" || zoneNames(m) != "app.example.com.,example.com.,www.example.com." {
		t.Fatalf("after refresh: filter=%q zones=%s", m.FilterValue(), zoneNames(m))
	}
	if m.selectedZone == nil || m.selectedZone.ID != want {
		t.Fatalf("selection %+v after refresh, want %s", m.selectedZone, want)
	}
}

func TestEscClearsZoneFilter(t *testing.T) {
	m := loadedZones(t)
	m = typeFilter(m, "corp")
	m, _ = m.Update(escKey)
	if m.FilterValue() != "" || len(m.zones) != 3 {
		t.Fatalf("esc: filter=%q zones=%s", m.FilterValue(), zoneNames(m))
	}
}

func TestFilterWithNoMatchesClearsSelection(t *testing.T) {
	m := loadedZones(t)
	m = typeFilter(m, "zzz")
	if m.selectedZone != nil || m.recordsets != nil {
		t.Fatalf("no match still selects a zone: %+v", m.selectedZone)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No DNS zones match") {
		t.Fatalf("view does not say nothing matches:\n%s", v)
	}
}

func TestFilterLineKeepsZoneRowsInView(t *testing.T) {
	m := New(nil)
	m.SetSize(100, 16)
	var zs []zones.Zone
	for i := 0; i < 30; i++ {
		zs = append(zs, zones.Zone{ID: string(rune('a' + i)), Name: "zone"})
	}
	m, _ = m.Update(zonesLoadedMsg{zones: zs})
	before := m.visibleRows()
	m = typeFilter(m, "zone")
	if m.visibleRows() != before-1 {
		t.Fatalf("visible rows %d with the filter line, want %d", m.visibleRows(), before-1)
	}
}

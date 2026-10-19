package dnslist

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/larkly/lazystack/internal/shared"
)

// loadedOn returns a model whose zone list has loaded and whose cursor sits
// on the zone at index sel, with that zone's records loaded.
func loadedOn(t *testing.T, list []zones.Zone, sel int) Model {
	t.Helper()
	m := New(nil)
	m.SetSize(100, 40)
	m, _ = m.Update(zonesLoadedMsg{seq: m.refresh.Seq(), zones: list})
	for i := 0; i < sel; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m, _ = m.Update(recordsetsLoadedMsg{zoneID: list[sel].ID, recordsets: []recordsets.RecordSet{{Name: "www"}}})
	if m.selectedZone == nil || m.selectedZone.ID != list[sel].ID || m.loading {
		t.Fatalf("setup: selected=%+v loading=%v", m.selectedZone, m.loading)
	}
	return m
}

func threeZones() []zones.Zone {
	return []zones.Zone{{ID: "zone-a", Name: "a.example."}, {ID: "zone-b", Name: "b.example."}, {ID: "zone-c", Name: "c.example."}}
}

// R must not throw the user back to the first zone: the cursor follows the
// selected zone by ID, wherever the reload puts it.
func TestReloadKeepsCursorOnSelectedZone(t *testing.T) {
	m := loadedOn(t, threeZones(), 2)

	m.ForceRefresh()
	reloaded := append([]zones.Zone{{ID: "zone-0", Name: "0.example."}}, threeZones()...)
	m, cmd := m.Update(zonesLoadedMsg{seq: m.refresh.Seq(), zones: reloaded})
	if m.cursor != 3 || m.selectedZone == nil || m.selectedZone.ID != "zone-c" {
		t.Fatalf("cursor=%d selected=%+v, want cursor 3 on zone-c", m.cursor, m.selectedZone)
	}
	if cmd == nil {
		t.Fatal("reload did not refresh the selected zone's records")
	}
}

// When the selected zone is gone the cursor stays at its position, clamped
// to the shorter list.
func TestReloadClampsCursorWhenSelectedZoneIsGone(t *testing.T) {
	m := loadedOn(t, threeZones(), 2)

	m.ForceRefresh()
	m, _ = m.Update(zonesLoadedMsg{seq: m.refresh.Seq(), zones: threeZones()[:2]})
	if m.cursor != 1 || m.selectedZone == nil || m.selectedZone.ID != "zone-b" {
		t.Fatalf("cursor=%d selected=%+v, want cursor 1 on zone-b", m.cursor, m.selectedZone)
	}
}

// DNS takes part in auto-refresh like the other lists: ticks reload the
// zones, never overlap, and a slow older reply cannot overwrite a newer one.
func TestTickRefreshesZonesWithoutOverlap(t *testing.T) {
	m := loadedOn(t, threeZones(), 1)

	m, first := m.Update(shared.TickMsg{})
	if first == nil {
		t.Fatal("tick did not fetch the zones")
	}
	staleSeq := m.refresh.Seq()
	if _, again := m.Update(shared.TickMsg{}); again != nil {
		t.Fatal("tick started a second fetch while one was in flight")
	}

	m.ForceRefresh()
	newSeq := m.refresh.Seq()
	m, _ = m.Update(zonesLoadedMsg{seq: newSeq, zones: []zones.Zone{{ID: "zone-b", Name: "b.example."}}})
	m, _ = m.Update(zonesLoadedMsg{seq: staleSeq, zones: threeZones()})
	m, _ = m.Update(zonesErrMsg{seq: staleSeq, err: errors.New("stale failure")})
	if len(m.zones) != 1 || m.err != nil {
		t.Fatalf("stale response applied: zones=%v err=%v", m.zones, m.err)
	}
	m, _ = m.Update(recordsetsLoadedMsg{zoneID: "zone-b"})
	if _, next := m.Update(shared.TickMsg{}); next == nil {
		t.Fatal("tick blocked after the newest fetch completed")
	}
}

// The error line takes a zone row, so a cursor on the last visible row must
// scroll back into view when a load fails.
func TestErrorKeepsCursorVisible(t *testing.T) {
	list := make([]zones.Zone, 30)
	for i := range list {
		list[i] = zones.Zone{ID: string(rune('a' + i)), Name: string(rune('a'+i)) + ".example."}
	}
	m := New(nil)
	m.SetSize(100, 14)
	m, _ = m.Update(zonesLoadedMsg{seq: m.refresh.Seq(), zones: list})
	for i := 0; i < m.visibleRows()-1; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.scroll != 0 || m.cursor != m.visibleRows()-1 {
		t.Fatalf("setup: cursor=%d scroll=%d", m.cursor, m.scroll)
	}

	m, _ = m.Update(recordsetsErrMsg{zoneID: m.selectedZone.ID, err: errors.New("boom")})
	if m.cursor < m.scroll || m.cursor >= m.scroll+m.visibleRows() {
		t.Fatalf("cursor %d off screen after a records error: scroll=%d rows=%d", m.cursor, m.scroll, m.visibleRows())
	}

	m.err = nil
	m.scroll = 0
	m.ForceRefresh()
	m, _ = m.Update(zonesErrMsg{seq: m.refresh.Seq(), err: errors.New("boom")})
	if m.cursor < m.scroll || m.cursor >= m.scroll+m.visibleRows() {
		t.Fatalf("cursor %d off screen after a zones error: scroll=%d rows=%d", m.cursor, m.scroll, m.visibleRows())
	}
}

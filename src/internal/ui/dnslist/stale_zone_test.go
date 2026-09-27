package dnslist

import (
	"errors"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
)

func twoZoneModel() Model {
	m := New(nil)
	m.loading = false
	m.zones = []zones.Zone{{ID: "zone-a", Name: "a.example."}, {ID: "zone-b", Name: "b.example."}}
	m.cursor = 1
	m.selectedZone = &m.zones[1]
	m.loading = true // fetch for zone-b in flight
	return m
}

func TestStaleRecordsetsResponseIsDropped(t *testing.T) {
	m := twoZoneModel()
	m, _ = m.Update(recordsetsLoadedMsg{zoneID: "zone-a", recordsets: []recordsets.RecordSet{{Name: "www.a.example."}}})
	if len(m.recordsets) != 0 {
		t.Fatalf("zone-a records shown while zone-b is selected: %+v", m.recordsets)
	}
	if !m.loading {
		t.Fatal("stale response cleared the loading state of the in-flight zone-b fetch")
	}

	m, _ = m.Update(recordsetsLoadedMsg{zoneID: "zone-b", recordsets: []recordsets.RecordSet{{Name: "www.b.example."}}})
	if len(m.recordsets) != 1 || m.recordsets[0].Name != "www.b.example." || m.loading {
		t.Fatalf("current zone response not applied: loading=%v records=%+v", m.loading, m.recordsets)
	}
}

func TestStaleRecordsetsErrorIsDropped(t *testing.T) {
	m := twoZoneModel()
	m, _ = m.Update(recordsetsErrMsg{zoneID: "zone-a", err: errors.New("boom")})
	if m.err != "" || !m.loading {
		t.Fatalf("stale error applied: err=%q loading=%v", m.err, m.loading)
	}

	m, _ = m.Update(recordsetsErrMsg{zoneID: "zone-b", err: errors.New("boom")})
	if m.err != "boom" || m.loading {
		t.Fatalf("current zone error not applied: err=%q loading=%v", m.err, m.loading)
	}
}

func TestRecordsetsWithoutSelectedZoneAreDropped(t *testing.T) {
	m := New(nil)
	m, _ = m.Update(recordsetsLoadedMsg{zoneID: "zone-a", recordsets: []recordsets.RecordSet{{Name: "x"}}})
	if len(m.recordsets) != 0 {
		t.Fatalf("records applied with no selected zone: %+v", m.recordsets)
	}
}

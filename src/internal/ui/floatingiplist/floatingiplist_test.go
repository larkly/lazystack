package floatingiplist

import (
	"testing"
	"time"

	"github.com/larkly/lazystack/internal/network"
)

func TestDescendingSortKeepsTiesStable(t *testing.T) {
	m := New(nil, time.Second)
	m.fips = []network.FloatingIP{
		{ID: "1", FloatingIP: "2001:db8::1", Status: "DOWN"},
		{ID: "2", FloatingIP: "2001:db8::2", Status: "ACTIVE"},
		{ID: "3", FloatingIP: "2001:db8::3", Status: "DOWN"},
		{ID: "4", FloatingIP: "2001:db8::4", Status: "DOWN"},
	}
	m.sortCol = 1 // status
	m.sortAsc = false
	m.sortFIPs()

	want := []string{"1", "3", "4", "2"}
	for i, f := range m.fips {
		if f.ID != want[i] {
			var got []string
			for _, g := range m.fips {
				got = append(got, g.ID)
			}
			t.Fatalf("descending by status = %v, want %v (ties must keep input order)", got, want)
		}
	}
}

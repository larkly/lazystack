package dnslist

import (
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/shared"
)

// Esc on the DNS tab with nothing to clear does nothing in the view; it no
// longer jumps to the server list.
func TestEscWithNothingToClearStaysPut(t *testing.T) {
	m := loadedZones(t)
	if m.Escape() {
		t.Fatal("Escape reported handled with no filter")
	}
	m, cmd := m.Update(escKey)
	if cmd != nil {
		if vc, ok := cmd().(shared.ViewChangeMsg); ok {
			t.Fatalf("esc asked for view %q", vc.View)
		}
	}
	if strings.Contains(m.Hints(), "esc back") {
		t.Fatalf("hints %q still say esc goes back", m.Hints())
	}
}

// Clearing a filter that hid every zone selects the first zone again; the
// root model's follow-up message loads its records.
func TestEscapeThenSelectionChangedLoadsRecords(t *testing.T) {
	m := loadedZones(t)
	m = typeFilter(m, "zzz")
	if !m.Escape() || m.selectedZone == nil {
		t.Fatalf("escape did not restore a selection: %+v", m.selectedZone)
	}
	m.loading = false
	m, cmd := m.Update(shared.SelectionChangedMsg{})
	if cmd == nil || !m.loading {
		t.Fatal("selection change after escape did not load the zone's records")
	}
}

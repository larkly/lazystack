package serverlist

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

// A server just created may only appear after a later refresh; the cursor
// must land on it once it is listed.
func TestSetHighlightSelectsServerOnceListed(t *testing.T) {
	m := New(nil, nil, time.Hour)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.SetHighlight("c")
	m, _ = m.Update(serversLoadedMsg{servers: []compute.Server{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}})
	if s := m.SelectedServer(); s == nil || s.ID != "a" {
		t.Fatalf("cursor moved before the server was listed: %+v", s)
	}
	m, _ = m.Update(serversLoadedMsg{servers: []compute.Server{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}, {ID: "c", Name: "c"}}})
	if s := m.SelectedServer(); s == nil || s.ID != "c" {
		t.Fatalf("cursor on %+v, want the highlighted server", s)
	}
	// The highlight is spent: the user's later moves stick across refreshes.
	m.cursor = 0
	m, _ = m.Update(serversLoadedMsg{servers: []compute.Server{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}, {ID: "c", Name: "c"}}})
	if s := m.SelectedServer(); s == nil || s.ID != "a" {
		t.Fatalf("cursor on %+v after refresh, want a", s)
	}
}

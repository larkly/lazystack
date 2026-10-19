package volumelist

import (
	"testing"
	"time"
)

// Esc clears one layer per press: the filter, then the selection, then it
// is left to the root model.
func TestEscapeClearsFilterThenSelection(t *testing.T) {
	m := New(nil, nil, 5*time.Second)
	m.SetSize(120, 30)
	m, _ = m.Update(volumesLoadedMsg{volumes: filterVols()})
	m.SelectIDs([]string{"v1"})
	m = typeFilter(m, "web")

	if !m.Escape() || m.FilterValue() != "" || m.SelectionCount() != 1 {
		t.Fatalf("first escape: filter=%q selected=%d, want filter cleared and selection kept", m.FilterValue(), m.SelectionCount())
	}
	if !m.Escape() || m.SelectionCount() != 0 {
		t.Fatalf("second escape: selected=%d, want cleared", m.SelectionCount())
	}
	if m.Escape() {
		t.Fatal("third escape reported handled with nothing left to clear")
	}
}

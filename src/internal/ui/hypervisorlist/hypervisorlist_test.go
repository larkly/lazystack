package hypervisorlist

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
)

// The error line takes a table row, so a cursor on the last visible row must
// scroll back into view when a refresh fails.
func TestErrorKeepsCursorVisible(t *testing.T) {
	m := New(nil)
	m.SetSize(80, 12)
	m, _ = m.Update(hypervisorsLoadedMsg{items: make([]compute.Hypervisor, 30)})
	for i := 0; i < m.visibleRows()-1; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.scroll != 0 || m.cursor != m.visibleRows()-1 {
		t.Fatalf("setup: cursor=%d scroll=%d", m.cursor, m.scroll)
	}

	m, _ = m.Update(hypervisorsErrMsg{err: errors.New("boom")})
	if m.cursor < m.scroll || m.cursor >= m.scroll+m.visibleRows() {
		t.Fatalf("cursor %d off screen: scroll=%d rows=%d", m.cursor, m.scroll, m.visibleRows())
	}
}

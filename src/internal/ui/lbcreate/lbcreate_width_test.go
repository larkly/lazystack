package lbcreate

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/testutil"
)

// The form used a fixed 55-cell box that overflowed narrow terminals.
func TestFormFitsNarrowTerminal(t *testing.T) {
	m := New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	m.SetSize(40, 30)
	testutil.AssertRenderSafe(t, m.View(), 40)
}

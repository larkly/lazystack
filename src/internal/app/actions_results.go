package app

import (
	"fmt"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// popNavIfTop pops the local back-navigation entry that brought us into a
// detail view, but only when it points at the view we are returning to, so
// the stack stays balanced without discarding unrelated entries.
func (m *Model) popNavIfTop(dest activeView) {
	if m.nav == nil {
		return
	}
	if top, ok := m.nav.Peek(); ok && top.View == dest {
		m.popNav()
	}
}

// handleResourceActionMsg reports a successful non-server mutation, leaves
// detail views whose resource may no longer exist and immediately refreshes
// the affected view instead of waiting for the next periodic tick.
func (m Model) handleResourceActionMsg(msg shared.ResourceActionMsg) (Model, tea.Cmd) {
	m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
	m.statusBar.Error = ""
	// Navigate back to list view if we were on a detail view
	m.returnToView = 0
	switch m.view {
	case viewVolumeDetail:
		m.popNavIfTop(viewVolumeList)
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
	case viewKeypairDetail:
		m.popNavIfTop(viewKeypairList)
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
	case viewLBView:
		m.statusBar.CurrentView = "lbview"
	case viewImageView:
		m.statusBar.CurrentView = "imageview"
	}
	return m.forceRefreshActiveView()
}

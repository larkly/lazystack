package app

import (
	"fmt"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/vmpassword"
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

// credentialsMsg pairs an action result with transient secrets (rescue or
// evacuation admin passwords). The secrets never enter labels, the status
// bar or the audit log; they are only shown in the masked credentials
// modal, which reveals them on explicit request.
type credentialsMsg struct {
	result tea.Msg
	title  string
	note   string
	creds  []vmpassword.Credential
}

func (m Model) handleCredentials(msg credentialsMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.result != nil {
		var next tea.Model
		next, cmd = m.Update(msg.result)
		m = next.(Model)
	}
	if len(msg.creds) > 0 {
		m.vmPassword = vmpassword.NewCredentials(msg.title, msg.note, msg.creds)
		m.vmPassword.SetSize(m.width, m.height)
	}
	return m, cmd
}

// handleServerActionMsg reports a successful server action and refreshes
// the server data. Deletions arrive as serverDeletedMsg instead, so no
// decision here depends on the (decorated) action label.
func (m Model) handleServerActionMsg(msg shared.ServerActionMsg) (Model, tea.Cmd) {
	m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
	m.statusBar.Error = ""
	// Ensure the resize modal is dismissed once its resize is done; an
	// unrelated action finishing must not close a modal still in use.
	if msg.Action == "Resize" {
		m.serverResize.Active = false
	}
	refreshServers := func() tea.Msg { return shared.RefreshServersMsg{} }
	// Navigate back to the server list from the console log sub-view.
	if m.view == viewConsoleLog {
		m.popNav()
		m.returnToView = 0
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		return m, refreshServers
	}
	// If on detail view, refresh — but skip rapid polling for
	// confirm/revert resize since those use optimistic updates
	if m.view == viewServerDetail {
		if msg.Action == "Confirm resize" || msg.Action == "Revert resize" {
			// Just refresh the server list, let the normal tick update detail
			return m, refreshServers
		}
		id := m.serverDetail.ServerID()
		return m, tea.Batch(
			refreshServers,
			tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return delayedDetailRefreshMsg{id: id}
			}),
			tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return delayedDetailRefreshMsg{id: id}
			}),
		)
	}
	return m, refreshServers
}

// handleResourceActionMsg reports a successful non-server mutation, leaves
// detail views whose resource may no longer exist and immediately refreshes
// the affected view instead of waiting for the next periodic tick.
func (m Model) handleResourceActionMsg(msg shared.ResourceActionMsg) (Model, tea.Cmd) {
	m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
	m.statusBar.Error = ""
	// Navigate back to list view if we were on a detail view
	returnTo := m.returnToView
	m.returnToView = 0
	switch m.view {
	case viewVolumeDetail:
		// A volume opened from another view (server detail's volumes pane)
		// returns there, keeping that view's own origin: the volume list is
		// not the active tab, so it would never receive its refresh reply
		// and would stay loading forever.
		if m.nav != nil {
			if top, ok := m.nav.Peek(); ok && top.View != viewVolumeList {
				entry, _ := m.popNav()
				m.returnToView = returnTo
				m, _ = m.restoreNavEntry(entry)
				return m.forceRefreshActiveView()
			}
		}
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

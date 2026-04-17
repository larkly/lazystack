package app

import (
	"fmt"

	"github.com/atotto/clipboard"
	tea "charm.land/bubbletea/v2"

	"github.com/larkly/lazystack/internal/ui/copypicker"
)

// collectCopyEntries asks the active view for its copyable fields.
// Returns (title, entries) — title describes the selected resource
// (e.g. "Copy — server web01"), entries is empty if nothing is selected.
func (m Model) collectCopyEntries() (string, []copypicker.Entry) {
	switch m.view {
	case viewServerList:
		title, entries := m.serverList.CopyEntries()
		return title, entries
	case viewServerDetail:
		title, entries := m.serverDetail.CopyEntries()
		return title, entries
	case viewVolumeList:
		title, entries := m.volumeList.CopyEntries()
		return title, entries
	case viewVolumeDetail:
		title, entries := m.volumeDetail.CopyEntries()
		return title, entries
	case viewFloatingIPList:
		title, entries := m.floatingIPList.CopyEntries()
		return title, entries
	case viewSecGroupView:
		title, entries := m.secGroupView.CopyEntries()
		return title, entries
	case viewKeypairList:
		title, entries := m.keypairList.CopyEntries()
		return title, entries
	case viewKeypairDetail:
		title, entries := m.keypairDetail.CopyEntries()
		return title, entries
	case viewLBView:
		title, entries := m.lbView.CopyEntries()
		return title, entries
	case viewNetworkList:
		title, entries := m.networkView.CopyEntries()
		return title, entries
	case viewRouterView:
		title, entries := m.routerView.CopyEntries()
		return title, entries
	case viewImageView:
		title, entries := m.imageView.CopyEntries()
		return title, entries
	}
	return "", nil
}

// openCopyPicker opens the copy-field modal for the active view.
// If the view has no selected resource or no copyable fields, a
// hint is shown on the status bar instead.
func (m Model) openCopyPicker() (Model, tea.Cmd) {
	title, entries := m.collectCopyEntries()
	if len(entries) == 0 {
		m.statusBar.StickyHint = "Nothing to copy here"
		return m, nil
	}
	m.copyPicker = copypicker.New(title, entries)
	m.copyPicker.SetSize(m.width, m.height)
	return m, m.copyPicker.Init()
}

// copyToClipboard writes value to the system clipboard and updates
// the status bar. It is the single source of truth for copy feedback.
func (m Model) copyToClipboard(label, value string) Model {
	if err := clipboard.WriteAll(value); err != nil {
		m.statusBar.StickyHint = "Clipboard error: " + err.Error()
		return m
	}
	if label == "" {
		m.statusBar.StickyHint = "Copied: " + value
	} else {
		m.statusBar.StickyHint = fmt.Sprintf("Copied %s: %s", label, value)
	}
	return m
}

package app

import (
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"charm.land/bubbletea/v2"
)

func (m Model) updateActiveView(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case viewCloudPicker:
		m.cloudPicker, cmd = m.cloudPicker.Update(msg)
	case viewServerList:
		m.serverList, cmd = m.serverList.Update(msg)
		m.statusBar.Hint = m.serverList.Hints()
	case viewServerDetail:
		m.serverDetail, cmd = m.serverDetail.Update(msg)
		m.statusBar.Hint = m.serverDetail.Hints()
	case viewServerCreate:
		m.serverCreate, cmd = m.serverCreate.Update(msg)
		m.statusBar.Hint = m.serverCreate.Hints()
	case viewConsoleLog:
		m.consoleLog, cmd = m.consoleLog.Update(msg)
		m.statusBar.Hint = m.consoleLog.Hints()
	case viewActionLog:
		m.actionLog, cmd = m.actionLog.Update(msg)
		m.statusBar.Hint = m.actionLog.Hints()
	case viewVolumeList:
		m.volumeList, cmd = m.volumeList.Update(msg)
		m.statusBar.Hint = m.volumeList.Hints()
	case viewVolumeDetail:
		m.volumeDetail, cmd = m.volumeDetail.Update(msg)
		m.statusBar.Hint = m.volumeDetail.Hints()
	case viewFloatingIPList:
		m.floatingIPList, cmd = m.floatingIPList.Update(msg)
		m.statusBar.Hint = m.floatingIPList.Hints()
	case viewSecGroupView:
		m.secGroupView, cmd = m.secGroupView.Update(msg)
		m.statusBar.Hint = m.secGroupView.Hints()
	case viewKeypairList:
		m.keypairList, cmd = m.keypairList.Update(msg)
		m.statusBar.Hint = m.keypairList.Hints()
	case viewLBList:
		m.lbList, cmd = m.lbList.Update(msg)
		m.statusBar.Hint = m.lbList.Hints()
	case viewLBDetail:
		m.lbDetail, cmd = m.lbDetail.Update(msg)
		m.statusBar.Hint = m.lbDetail.Hints()
	}
	return m, cmd
}

// updateAllViews routes non-key messages to all initialized views so
// background auto-refresh ticks keep firing even when a view isn't active.
func (m Model) updateAllViews(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Always route to server list
	if m.view != viewCloudPicker {
		m.serverList, cmd = m.serverList.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Route to all initialized tab list views
	for i, td := range m.tabs {
		if !m.tabInited[i] {
			continue
		}
		switch td.Key {
		case "volumes":
			m.volumeList, cmd = m.volumeList.Update(msg)
			cmds = append(cmds, cmd)
		case "floatingips":
			m.floatingIPList, cmd = m.floatingIPList.Update(msg)
			cmds = append(cmds, cmd)
		case "secgroups":
			m.secGroupView, cmd = m.secGroupView.Update(msg)
			cmds = append(cmds, cmd)
		case "keypairs":
			m.keypairList, cmd = m.keypairList.Update(msg)
			cmds = append(cmds, cmd)
		case "loadbalancers":
			m.lbList, cmd = m.lbList.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Route to active sub-views (detail, console, etc.)
	switch m.view {
	case viewServerDetail:
		m.serverDetail, cmd = m.serverDetail.Update(msg)
		cmds = append(cmds, cmd)
	case viewVolumeDetail:
		m.volumeDetail, cmd = m.volumeDetail.Update(msg)
		cmds = append(cmds, cmd)
	case viewLBDetail:
		m.lbDetail, cmd = m.lbDetail.Update(msg)
		cmds = append(cmds, cmd)
	case viewConsoleLog:
		m.consoleLog, cmd = m.consoleLog.Update(msg)
		cmds = append(cmds, cmd)
	case viewActionLog:
		m.actionLog, cmd = m.actionLog.Update(msg)
		cmds = append(cmds, cmd)
	case viewServerCreate:
		m.serverCreate, cmd = m.serverCreate.Update(msg)
		cmds = append(cmds, cmd)
	case viewCloudPicker:
		m.cloudPicker, cmd = m.cloudPicker.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) updateModal(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeModal {
	case modalConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	case modalError:
		m.errModal, cmd = m.errModal.Update(msg)
	}
	return m, cmd
}

func (m Model) handleViewChange(msg shared.ViewChangeMsg) (Model, tea.Cmd) {
	switch msg.View {
	case "serverlist":
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }

	case "serverdetail":
		// If coming back from console/resize, return to previous view
		if m.view == viewConsoleLog || m.view == viewActionLog {
			if m.previousView == viewServerDetail && m.serverDetail.ServerID() != "" {
				m.view = viewServerDetail
				m.statusBar.CurrentView = "serverdetail"
				m.statusBar.Hint = m.serverDetail.Hints()
				return m, m.serverDetail.Init()
			}
			// Came from list or no valid detail, go to list
			m.view = viewServerList
			m.statusBar.CurrentView = "serverlist"
			m.statusBar.Hint = m.serverList.Hints()
			return m, func() tea.Msg { return shared.RefreshServersMsg{} }
		}
		if s := m.serverList.SelectedServer(); s != nil {
			m.serverDetail = serverdetail.New(m.client.Compute, s.ID, m.refreshInterval)
			m.serverDetail.SetSize(m.width, m.height)
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, m.serverDetail.Init()
		}
		return m, nil

	case "volumelist":
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
		m.statusBar.Hint = m.volumeList.Hints()
		return m, nil

	case "lblist":
		m.view = viewLBList
		m.statusBar.CurrentView = "lblist"
		m.statusBar.Hint = m.lbList.Hints()
		return m, nil

	case "servercreate":
		m.serverCreate = servercreate.New(m.client.Compute, m.client.Image, m.client.Network)
		m.serverCreate.SetSize(m.width, m.height)
		m.view = viewServerCreate
		m.statusBar.CurrentView = "servercreate"
		m.statusBar.Hint = m.serverCreate.Hints()
		return m, m.serverCreate.Init()

	case "consolelog":
		return m, nil // handled by openConsoleLog

	}
	return m, nil
}

func (m Model) forceRefreshActiveView() (Model, tea.Cmd) {
	switch m.view {
	case viewServerList:
		return m, m.serverList.ForceRefresh()
	case viewServerDetail:
		return m, m.serverDetail.ForceRefresh()
	case viewVolumeList:
		return m, m.volumeList.ForceRefresh()
	case viewVolumeDetail:
		return m, m.volumeDetail.ForceRefresh()
	case viewFloatingIPList:
		return m, m.floatingIPList.ForceRefresh()
	case viewSecGroupView:
		return m, m.secGroupView.ForceRefresh()
	case viewKeypairList:
		return m, m.keypairList.ForceRefresh()
	case viewLBList:
		return m, m.lbList.ForceRefresh()
	case viewLBDetail:
		return m, m.lbDetail.ForceRefresh()
	case viewConsoleLog:
		return m, m.consoleLog.ForceRefresh()
	case viewActionLog:
		return m, m.actionLog.ForceRefresh()
	}
	return m, nil
}

package app

import (
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
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
	case viewVolumeCreate:
		m.volumeCreate, cmd = m.volumeCreate.Update(msg)
		m.statusBar.Hint = m.volumeCreate.Hints()
	case viewKeypairCreate:
		m.keypairCreate, cmd = m.keypairCreate.Update(msg)
		m.statusBar.Hint = m.keypairCreate.Hints()
	case viewKeypairDetail:
		m.keypairDetail, cmd = m.keypairDetail.Update(msg)
		m.statusBar.Hint = m.keypairDetail.Hints()
	case viewKeypairList:
		m.keypairList, cmd = m.keypairList.Update(msg)
		m.statusBar.Hint = m.keypairList.Hints()
	case viewNetworkList:
		m.networkList, cmd = m.networkList.Update(msg)
		m.statusBar.Hint = m.networkList.Hints()
	case viewRouterList:
		m.routerList, cmd = m.routerList.Update(msg)
		m.statusBar.Hint = m.routerList.Hints()
	case viewRouterDetail:
		m.routerDetail, cmd = m.routerDetail.Update(msg)
		m.statusBar.Hint = m.routerDetail.Hints()
	case viewLBList:
		m.lbList, cmd = m.lbList.Update(msg)
		m.statusBar.Hint = m.lbList.Hints()
	case viewLBDetail:
		m.lbDetail, cmd = m.lbDetail.Update(msg)
		m.statusBar.Hint = m.lbDetail.Hints()
	case viewImageList:
		m.imageList, cmd = m.imageList.Update(msg)
		m.statusBar.Hint = m.imageList.Hints()
	case viewImageDetail:
		m.imageDetail, cmd = m.imageDetail.Update(msg)
		m.statusBar.Hint = m.imageDetail.Hints()
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

	// Route to active tab's list view only — inactive views don't need
	// tick messages; their ticker chains restart when they become active.
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) && m.tabInited[m.activeTab] {
		switch m.tabs[m.activeTab].Key {
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
		case "networks":
			m.networkList, cmd = m.networkList.Update(msg)
			cmds = append(cmds, cmd)
		case "routers":
			m.routerList, cmd = m.routerList.Update(msg)
			cmds = append(cmds, cmd)
		case "loadbalancers":
			m.lbList, cmd = m.lbList.Update(msg)
			cmds = append(cmds, cmd)
		case "images":
			m.imageList, cmd = m.imageList.Update(msg)
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
	case viewRouterDetail:
		m.routerDetail, cmd = m.routerDetail.Update(msg)
		cmds = append(cmds, cmd)
	case viewImageDetail:
		m.imageDetail, cmd = m.imageDetail.Update(msg)
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
	case viewVolumeCreate:
		m.volumeCreate, cmd = m.volumeCreate.Update(msg)
		cmds = append(cmds, cmd)
	case viewKeypairCreate:
		m.keypairCreate, cmd = m.keypairCreate.Update(msg)
		cmds = append(cmds, cmd)
	case viewKeypairDetail:
		m.keypairDetail, cmd = m.keypairDetail.Update(msg)
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

func (m Model) handleDetailNavigation(msg shared.NavigateToDetailMsg) (Model, tea.Cmd) {
	switch msg.Resource {
	case "volume":
		m.volumeDetail = volumedetail.New(m.client.BlockStorage, m.client.Compute, msg.ID, m.refreshInterval)
		m.volumeDetail.SetSize(m.width, m.height)
		m.returnToView = m.view
		m.view = viewVolumeDetail
		m.statusBar.CurrentView = "volumedetail"
		m.statusBar.Hint = m.volumeDetail.Hints()
		return m, m.volumeDetail.Init()
	}
	return m, nil
}

func (m Model) handleResourceNavigation(msg shared.NavigateToResourceMsg) (Model, tea.Cmd) {
	// Find the target tab index
	tabIdx := -1
	for i, td := range m.tabs {
		if td.Key == msg.Tab {
			tabIdx = i
			break
		}
	}
	if tabIdx < 0 {
		return m, nil
	}

	m.returnToView = m.view
	m, cmd := m.switchTab(tabIdx)

	// Position cursor on highlighted resource
	switch msg.Tab {
	case "volumes":
		m.volumeList.SetHighlight(msg.Highlight)
	case "secgroups":
		m.secGroupView.ScrollToNames(msg.Highlight)
	case "networks":
		m.networkList.ScrollToNames(msg.Highlight)
	}

	return m, cmd
}

func (m Model) handleViewChange(msg shared.ViewChangeMsg) (Model, tea.Cmd) {
	switch msg.View {
	case "serverlist":
		// If returning from a cross-resource jump, go back to server detail
		if m.returnToView == viewServerDetail && m.serverDetail.ServerID() != "" {
			m.returnToView = 0
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, nil
		}
		m.returnToView = 0
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
			m.serverDetail = serverdetail.New(m.client.Compute, m.client.Network, m.client.BlockStorage, s.ID, m.refreshInterval)
			m.serverDetail.SetSize(m.width, m.height)
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, m.serverDetail.Init()
		}
		return m, nil

	case "volumelist":
		// If returning from a cross-resource jump, go back to server detail
		if m.returnToView == viewServerDetail && m.serverDetail.ServerID() != "" {
			m.returnToView = 0
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, nil
		}
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
		m.statusBar.Hint = m.volumeList.Hints()
		return m, m.volumeList.Init()

	case "volumecreate":
		return m.openVolumeCreate()

	case "routerlist":
		m.view = viewRouterList
		m.statusBar.CurrentView = "routerlist"
		m.statusBar.Hint = m.routerList.Hints()
		return m, m.routerList.Init()

	case "keypairlist":
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
		m.statusBar.Hint = m.keypairList.Hints()
		return m, m.keypairList.Init()

	case "keypaircreate":
		return m.openKeypairCreate()

	case "lblist":
		m.view = viewLBList
		m.statusBar.CurrentView = "lblist"
		m.statusBar.Hint = m.lbList.Hints()
		return m, m.lbList.Init()

	case "imagelist":
		m.view = viewImageList
		m.statusBar.CurrentView = "imagelist"
		m.statusBar.Hint = m.imageList.Hints()
		return m, m.imageList.Init()

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
	case viewNetworkList:
		return m, m.networkList.ForceRefresh()
	case viewRouterList:
		return m, m.routerList.ForceRefresh()
	case viewRouterDetail:
		return m, m.routerDetail.ForceRefresh()
	case viewLBList:
		return m, m.lbList.ForceRefresh()
	case viewLBDetail:
		return m, m.lbDetail.ForceRefresh()
	case viewImageList:
		return m, m.imageList.ForceRefresh()
	case viewImageDetail:
		return m, m.imageDetail.ForceRefresh()
	case viewConsoleLog:
		return m, m.consoleLog.ForceRefresh()
	case viewActionLog:
		return m, m.actionLog.ForceRefresh()
	}
	return m, nil
}

package app

import (
	"time"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
)

// refreshTickCmd returns a single tea.Tick that fires shared.TickMsg after
// the refresh interval. This is the ONLY tick source in the app — views
// must not create their own tick timers.
func (m Model) refreshTickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return shared.TickMsg{}
	})
}

func (m Model) updateActiveView(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.view {
	case viewCloudPicker:
		m.cloudPicker, cmd = m.cloudPicker.Update(msg)
		m.statusBar.Hint = m.cloudPicker.Hints()
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
	case viewHypervisorList:
		m.hypervisorList, cmd = m.hypervisorList.Update(msg)
		m.statusBar.Hint = m.hypervisorList.Hints()
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
		m.networkView, cmd = m.networkView.Update(msg)
		m.statusBar.Hint = m.networkView.Hints()
	case viewRouterView:
		m.routerView, cmd = m.routerView.Update(msg)
		m.statusBar.Hint = m.routerView.Hints()
	case viewLBView:
		m.lbView, cmd = m.lbView.Update(msg)
		m.statusBar.Hint = m.lbView.Hints()
	case viewImageView:
		m.imageView, cmd = m.imageView.Update(msg)
		m.statusBar.Hint = m.imageView.Hints()
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
			m.networkView, cmd = m.networkView.Update(msg)
			cmds = append(cmds, cmd)
		case "routers":
			m.routerView, cmd = m.routerView.Update(msg)
			cmds = append(cmds, cmd)
		case "loadbalancers":
			m.lbView, cmd = m.lbView.Update(msg)
			cmds = append(cmds, cmd)
		case "images":
			m.imageView, cmd = m.imageView.Update(msg)
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
	// viewRouterView, viewLBView, and viewImageView are tab views, not sub-views — handled above
	case viewConsoleLog:
		m.consoleLog, cmd = m.consoleLog.Update(msg)
		cmds = append(cmds, cmd)
	case viewActionLog:
		m.actionLog, cmd = m.actionLog.Update(msg)
		cmds = append(cmds, cmd)
	case viewHypervisorList:
		m.hypervisorList, cmd = m.hypervisorList.Update(msg)
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
	shared.Debugf("[app] handleDetailNavigation: resource=%s id=%s", msg.Resource, msg.ID)
	switch msg.Resource {
	case "volume":
		m.volumeDetail = volumedetail.New(m.client.BlockStorage, m.client.Compute, msg.ID)
		m.volumeDetail.SetSize(m.width, m.height)
		m.nav.Push(m.view, m.activeTab)
		m.view = viewVolumeDetail
		m.statusBar.CurrentView = "volumedetail"
		m.statusBar.Hint = m.volumeDetail.Hints()
		return m, m.volumeDetail.Init()
	case "server":
		m.serverDetail = serverdetail.New(m.client.Compute, m.client.Network, m.client.BlockStorage, msg.ID, m.refreshInterval)
		m.serverDetail.SetSize(m.width, m.height)
		m.returnToView = m.view
		m.view = viewServerDetail
		m.statusBar.CurrentView = "serverdetail"
		m.statusBar.Hint = m.serverDetail.Hints()
		return m, m.serverDetail.Init()
	}
	return m, nil
}

func (m Model) handleResourceNavigation(msg shared.NavigateToResourceMsg) (Model, tea.Cmd) {
	shared.Debugf("[app] handleResourceNavigation: tab=%s", msg.Tab)
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
		m.networkView.ScrollToNames(msg.Highlight)
	}

	return m, cmd
}

func (m Model) handleViewChange(msg shared.ViewChangeMsg) (Model, tea.Cmd) {
	shared.Debugf("[app] handleViewChange: target=%s", msg.View)
	switch msg.View {
	case "serverlist":
		// If returning from a cross-resource jump, go back to the originating view
		if m.returnToView == viewServerDetail && m.serverDetail.ServerID() != "" {
			m.returnToView = 0
			m.view = viewServerDetail
			m.statusBar.CurrentView = "serverdetail"
			m.statusBar.Hint = m.serverDetail.Hints()
			return m, nil
		}
		if m.returnToView == viewSecGroupView {
			m.returnToView = 0
			m.view = viewSecGroupView
			m.statusBar.CurrentView = "secgroupview"
			m.statusBar.Hint = m.secGroupView.Hints()
			return m, nil
		}
		if m.returnToView == viewImageView {
			m.returnToView = 0
			m.view = viewImageView
			m.statusBar.CurrentView = "imageview"
			m.statusBar.Hint = m.imageView.Hints()
			return m, nil
		}
		m.returnToView = 0
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }

	case "serverdetail":
		if entry, ok := m.nav.Pop(); ok {
			m.view = entry.View
			m.activeTab = entry.Tab
			m.statusBar.CurrentView = m.viewName()
			return m, nil
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
		m.view = viewRouterView
		m.statusBar.CurrentView = "routerview"
		m.statusBar.Hint = m.routerView.Hints()
		return m, m.routerView.ForceRefresh()

	case "keypairlist":
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
		m.statusBar.Hint = m.keypairList.Hints()
		return m, m.keypairList.Init()

	case "keypaircreate":
		return m.openKeypairCreate()

	case "lblist":
		m.view = viewLBView
		m.statusBar.CurrentView = "lbview"
		m.statusBar.Hint = m.lbView.Hints()
		return m, m.lbView.ForceRefresh()

	case "imagelist":
		m.view = viewImageView
		m.statusBar.CurrentView = "imageview"
		m.statusBar.Hint = m.imageView.Hints()
		return m, m.imageView.ForceRefresh()

	case "servercreate":
		m.serverCreate = servercreate.New(m.client.Compute, m.client.Image, m.client.Network)
		m.serverCreate.SetSize(m.width, m.height)
		m.view = viewServerCreate
		m.statusBar.CurrentView = "servercreate"
		m.statusBar.Hint = m.serverCreate.Hints()
		return m, m.serverCreate.Init()

	case "secgroupview":
		m.returnToView = 0
		m.view = viewSecGroupView
		m.statusBar.CurrentView = "secgroupview"
		m.statusBar.Hint = m.secGroupView.Hints()
		return m, m.secGroupView.ForceRefresh()

	case "consolelog":
		return m, nil // handled by openConsoleLog

	}
	return m, nil
}

func (m Model) forceRefreshActiveView() (Model, tea.Cmd) {
	shared.Debugf("[app] forceRefreshActiveView: view=%d", m.view)
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
		return m, m.networkView.ForceRefresh()
	case viewRouterView:
		return m, m.routerView.ForceRefresh()
	case viewLBView:
		return m, m.lbView.ForceRefresh()
	case viewImageView:
		return m, m.imageView.ForceRefresh()
	case viewConsoleLog:
		return m, m.consoleLog.ForceRefresh()
	case viewActionLog:
		return m, m.actionLog.ForceRefresh()
	case viewHypervisorList:
		return m, m.hypervisorList.ForceRefresh()
	}
	return m, nil
}

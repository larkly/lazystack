package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bosse/lazystack/internal/cloud"
	"github.com/bosse/lazystack/internal/compute"
	"github.com/bosse/lazystack/internal/network"
	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/volume"
	"github.com/bosse/lazystack/internal/ui/actionlog"
	"github.com/bosse/lazystack/internal/ui/cloudpicker"
	"github.com/bosse/lazystack/internal/ui/consolelog"
	"github.com/bosse/lazystack/internal/ui/fippicker"
	"github.com/bosse/lazystack/internal/ui/floatingiplist"
	"github.com/bosse/lazystack/internal/ui/help"
	"github.com/bosse/lazystack/internal/ui/keypairlist"
	"github.com/bosse/lazystack/internal/ui/modal"
	"github.com/bosse/lazystack/internal/ui/secgroupview"
	"github.com/bosse/lazystack/internal/ui/servercreate"
	"github.com/bosse/lazystack/internal/ui/serverdetail"
	"github.com/bosse/lazystack/internal/ui/serverlist"
	"github.com/bosse/lazystack/internal/ui/serverresize"
	"github.com/bosse/lazystack/internal/ui/statusbar"
	"github.com/bosse/lazystack/internal/ui/volumedetail"
	"github.com/bosse/lazystack/internal/ui/volumelist"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

type activeTab int

const (
	tabServers activeTab = iota
	tabVolumes
	tabFloatingIPs
	tabSecGroups
	tabKeypairs
)

var tabNames = []string{"Servers", "Volumes", "Floating IPs", "Sec Groups", "Key Pairs"}

type activeView int

const (
	viewCloudPicker activeView = iota
	viewServerList
	viewServerDetail
	viewServerCreate
	viewConsoleLog
	viewActionLog
	viewVolumeList
	viewVolumeDetail
	viewFloatingIPList
	viewSecGroupView
	viewKeypairList
)

type modalType int

type delayedDetailRefreshMsg struct {
	id string
}

type serverDetailRefreshedMsg struct {
	server *compute.Server
}

const (
	modalNone modalType = iota
	modalConfirm
	modalError
)

// Model is the root application model.
type Model struct {
	view         activeView
	width        int
	height       int
	client       *cloud.Client
	cloudPicker  cloudpicker.Model
	serverList   serverlist.Model
	serverDetail serverdetail.Model
	serverCreate servercreate.Model
	consoleLog    consolelog.Model
	actionLog     actionlog.Model
	serverResize  serverresize.Model
	fipPicker     fippicker.Model
	volumeList    volumelist.Model
	volumeDetail  volumedetail.Model
	floatingIPList floatingiplist.Model
	secGroupView  secgroupview.Model
	keypairList   keypairlist.Model
	statusBar     statusbar.Model
	activeTab     activeTab
	tabsInited    map[activeTab]bool
	help         help.Model
	confirm      modal.ConfirmModel
	errModal     modal.ErrorModel
	activeModal  modalType
	cloudName       string
	autoCloud       string
	previousView    activeView
	refreshInterval time.Duration
	minWidth        int
	minHeight    int
	tooSmall     bool
	restart      bool
	version      string
}

// ShouldRestart returns true if the app quit due to a restart request.
func (m Model) ShouldRestart() bool {
	return m.restart
}

// Options configures the application.
type Options struct {
	AlwaysPickCloud bool
	RefreshInterval time.Duration
	Version         string
}

// New creates the root model.
func New(opts Options) Model {
	clouds, err := cloud.ListCloudNames()

	refresh := opts.RefreshInterval
	if refresh == 0 {
		refresh = 5 * time.Second
	}

	// Auto-select if exactly one cloud and not forced to pick
	if err == nil && len(clouds) == 1 && !opts.AlwaysPickCloud {
		return Model{
			view:            viewCloudPicker,
			cloudPicker:     cloudpicker.New(clouds, nil),
			statusBar:       statusbar.New(opts.Version),
			help:            help.New(),
			minWidth:        80,
			minHeight:       20,
			autoCloud:       clouds[0],
			refreshInterval: refresh,
			version:         opts.Version,
			tabsInited:      make(map[activeTab]bool),
		}
	}

	cp := cloudpicker.New(clouds, err)
	return Model{
		view:            viewCloudPicker,
		cloudPicker:     cp,
		statusBar:       statusbar.New(opts.Version),
		help:            help.New(),
		minWidth:        80,
		minHeight:       20,
		refreshInterval: refresh,
		version:         opts.Version,
		tabsInited:      make(map[activeTab]bool),
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	if m.autoCloud != "" {
		name := m.autoCloud
		return func() tea.Msg {
			return shared.CloudSelectedMsg{CloudName: name}
		}
	}
	return nil
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tooSmall = m.width < m.minWidth || m.height < m.minHeight
		m.cloudPicker.SetSize(m.width, m.height)
		m.confirm.SetSize(m.width, m.height)
		m.errModal.SetSize(m.width, m.height)
		m.help.Width = m.width
		m.help.Height = m.height
		m.serverResize.SetSize(m.width, m.height)
		m.fipPicker.SetSize(m.width, m.height)
		m.statusBar.Width = m.width
		return m.updateActiveView(msg)

	case tea.KeyMsg:
		if m.help.Visible {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}

		// Restart works from anywhere
		if key.Matches(msg, shared.Keys.Restart) {
			m.restart = true
			return m, tea.Quit
		}

		if m.activeModal != modalNone {
			return m.updateModal(msg)
		}

		// Resize modal intercepts all keys when active
		if m.serverResize.Active {
			var cmd tea.Cmd
			m.serverResize, cmd = m.serverResize.Update(msg)
			return m, cmd
		}

		// FIP picker modal intercepts all keys when active
		if m.fipPicker.Active {
			var cmd tea.Cmd
			m.fipPicker, cmd = m.fipPicker.Update(msg)
			return m, cmd
		}

		if m.view != viewServerCreate {
			switch {
			case key.Matches(msg, shared.Keys.Quit) && m.view != viewCloudPicker:
				return m, tea.Quit
			case key.Matches(msg, shared.Keys.Help):
				m.help.Visible = true
				m.help.View = m.viewName()
				return m, nil
			case key.Matches(msg, shared.Keys.CloudPick) && m.view != viewCloudPicker:
				return m.switchToCloudPicker()
			}

			// Tab switching (only from top-level list views)
			if m.isTopLevelView() {
				switch {
				case key.Matches(msg, shared.Keys.Tab1):
					return m.switchTab(tabServers)
				case key.Matches(msg, shared.Keys.Tab2):
					return m.switchTab(tabVolumes)
				case key.Matches(msg, shared.Keys.Tab3):
					return m.switchTab(tabFloatingIPs)
				case key.Matches(msg, shared.Keys.Tab4):
					return m.switchTab(tabSecGroups)
				case key.Matches(msg, shared.Keys.Tab5):
					return m.switchTab(tabKeypairs)
				case key.Matches(msg, shared.Keys.Right):
					next := (m.activeTab + 1) % activeTab(len(tabNames))
					return m.switchTab(next)
				case key.Matches(msg, shared.Keys.Left):
					prev := (m.activeTab - 1 + activeTab(len(tabNames))) % activeTab(len(tabNames))
					return m.switchTab(prev)
				}
			}
		}

		// Global force refresh
		if key.Matches(msg, shared.Keys.Refresh) && m.view != viewCloudPicker {
			return m.forceRefreshActiveView()
		}

		if m.view == viewServerList || m.view == viewServerDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Reboot) {
				return m.openRebootConfirm("soft reboot")
			}
			if key.Matches(msg, shared.Keys.HardReboot) {
				return m.openRebootConfirm("hard reboot")
			}
			if key.Matches(msg, shared.Keys.Pause) {
				return m.openToggleConfirm("pause/unpause")
			}
			if key.Matches(msg, shared.Keys.Suspend) {
				return m.openToggleConfirm("suspend/resume")
			}
			if key.Matches(msg, shared.Keys.Shelve) {
				return m.openToggleConfirm("shelve/unshelve")
			}
			if key.Matches(msg, shared.Keys.Console) {
				return m.openConsoleLog()
			}
			if key.Matches(msg, shared.Keys.Actions) {
				return m.openActionLog()
			}
			if key.Matches(msg, shared.Keys.Resize) {
				return m.openResize()
			}
			if key.Matches(msg, shared.Keys.ConfirmResize) {
				return m.doConfirmResize()
			}
			if key.Matches(msg, shared.Keys.RevertResize) {
				return m.doRevertResize()
			}
			if key.Matches(msg, shared.Keys.Attach) {
				return m.doAllocateAndAssociateFIP()
			}
		}

		// Volume list: Enter to open detail, ctrl+d to delete
		if m.view == viewVolumeList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openVolumeDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openVolumeDeleteConfirm()
			}
		}

		// Volume detail: ctrl+d delete, ctrl+a attach, ctrl+t detach
		if m.view == viewVolumeDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openVolumeDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Attach) {
				return m.openVolumeAttach()
			}
			if key.Matches(msg, shared.Keys.Detach) {
				return m.openVolumeDetach()
			}
		}

		// Floating IP list: ctrl+d release, ctrl+n allocate, ctrl+t disassociate
		if m.view == viewFloatingIPList {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openFIPReleaseConfirm()
			}
			if key.Matches(msg, shared.Keys.Allocate) {
				return m.doAllocateFIP()
			}
			if key.Matches(msg, shared.Keys.Detach) {
				return m.openFIPDisassociateConfirm()
			}
		}

		// Security group view: ctrl+d delete rule (when on a rule)
		if m.view == viewSecGroupView {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openSGRuleDeleteConfirm()
			}
		}

		// Key pair list: ctrl+d delete
		if m.view == viewKeypairList {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openKeyPairDeleteConfirm()
			}
		}

		return m.updateActiveView(msg)

	case shared.CloudSelectedMsg:
		m.cloudName = msg.CloudName
		m.statusBar.Hint = "Connecting..."
		return m, m.connectToCloud(msg.CloudName)

	case shared.CloudConnectedMsg:
		m.client = &cloud.Client{
			CloudName:    m.cloudName,
			Compute:      msg.ComputeClient,
			Image:        msg.ImageClient,
			Network:      msg.NetworkClient,
			BlockStorage: msg.BlockStorageClient,
		}
		// Reset tab state for new cloud connection
		m.tabsInited = make(map[activeTab]bool)
		m.activeTab = tabServers
		m.statusBar.CloudName = m.cloudName
		m.statusBar.Region = msg.Region
		m.serverList = serverlist.New(msg.ComputeClient, msg.ImageClient, m.refreshInterval)
		m.serverList.SetSize(m.width, m.height)
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, m.serverList.Init()

	case shared.CloudConnectErrMsg:
		m.errModal = modal.NewError("Cloud Connection", msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case shared.ViewChangeMsg:
		return m.handleViewChange(msg)

	case modal.ConfirmAction:
		m.activeModal = modalNone
		if msg.Confirm {
			return m.executeAction(msg)
		}
		return m, nil

	case modal.ErrorDismissedMsg:
		m.activeModal = modalNone
		return m, nil

	case shared.ServerActionMsg:
		m.statusBar.Hint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
		m.statusBar.Error = ""
		// Ensure resize modal is dismissed
		m.serverResize.Active = false
		// Navigate back to server list if on a sub-view, or after delete
		if m.view == viewConsoleLog || (m.view == viewServerDetail && msg.Action == "Delete") {
			m.view = viewServerList
			m.statusBar.CurrentView = "serverlist"
			return m, func() tea.Msg { return shared.RefreshServersMsg{} }
		}
		// If on detail view, refresh — but skip rapid polling for
		// confirm/revert resize since those use optimistic updates
		if m.view == viewServerDetail {
			if msg.Action == "Confirm resize" || msg.Action == "Revert resize" {
				// Just refresh the server list, let the normal tick update detail
				return m, func() tea.Msg { return shared.RefreshServersMsg{} }
			}
			id := m.serverDetail.ServerID()
			return m, tea.Batch(
				func() tea.Msg { return shared.RefreshServersMsg{} },
				tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
					return delayedDetailRefreshMsg{id: id}
				}),
				tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return delayedDetailRefreshMsg{id: id}
				}),
			)
		}
		return m, func() tea.Msg { return shared.RefreshServersMsg{} }

	case shared.ResourceActionMsg:
		m.statusBar.Hint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
		m.statusBar.Error = ""
		// Navigate back to list view if we were on a detail view
		if m.view == viewVolumeDetail {
			m.view = viewVolumeList
			m.statusBar.CurrentView = "volumelist"
			m.statusBar.Hint = m.volumeList.Hints()
		}
		return m, nil

	case shared.ResourceActionErrMsg:
		m.errModal = modal.NewError(
			fmt.Sprintf("%s %s", msg.Action, msg.Name), msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case shared.ServerActionErrMsg:
		m.errModal = modal.NewError(
			fmt.Sprintf("%s %s", msg.Action, msg.Name), msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case delayedDetailRefreshMsg:
		if m.view == viewServerDetail && m.serverDetail.ServerID() == msg.id {
			client := m.client.Compute
			id := msg.id
			return m, func() tea.Msg {
				srv, err := compute.GetServer(context.Background(), client, id)
				if err != nil {
					return shared.ErrMsg{Err: err}
				}
				return serverDetailRefreshedMsg{server: srv}
			}
		}
		return m, nil

	case serverDetailRefreshedMsg:
		if m.view == viewServerDetail && msg.server != nil {
			m.serverDetail.SetServer(msg.server)
		}
		return m, nil

	default:
		// Route to all views first so background ticks keep firing
		m2, viewCmd := m.updateAllViews(msg)
		m = m2
		// Also route to active modals (for spinner, loaded msgs)
		if m.serverResize.Active {
			var cmd tea.Cmd
			m.serverResize, cmd = m.serverResize.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.fipPicker.Active {
			var cmd tea.Cmd
			m.fipPicker, cmd = m.fipPicker.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		return m, viewCmd
	}
}

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
	if m.tabsInited[tabVolumes] {
		m.volumeList, cmd = m.volumeList.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.tabsInited[tabFloatingIPs] {
		m.floatingIPList, cmd = m.floatingIPList.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.tabsInited[tabSecGroups] {
		m.secGroupView, cmd = m.secGroupView.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.tabsInited[tabKeypairs] {
		m.keypairList, cmd = m.keypairList.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Route to active sub-views (detail, console, etc.)
	switch m.view {
	case viewServerDetail:
		m.serverDetail, cmd = m.serverDetail.Update(msg)
		cmds = append(cmds, cmd)
	case viewVolumeDetail:
		m.volumeDetail, cmd = m.volumeDetail.Update(msg)
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
	case viewConsoleLog:
		return m, m.consoleLog.ForceRefresh()
	case viewActionLog:
		return m, m.actionLog.ForceRefresh()
	}
	return m, nil
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

func (m Model) isTopLevelView() bool {
	switch m.view {
	case viewServerList, viewVolumeList, viewFloatingIPList, viewSecGroupView, viewKeypairList:
		return true
	}
	return false
}

func (m Model) switchTab(tab activeTab) (Model, tea.Cmd) {
	if tab == m.activeTab && m.isTopLevelView() {
		return m, nil
	}
	m.activeTab = tab

	switch tab {
	case tabServers:
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, nil

	case tabVolumes:
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
		if !m.tabsInited[tabVolumes] {
			m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, m.refreshInterval)
			m.volumeList.SetSize(m.width, m.height)
			m.tabsInited[tabVolumes] = true
			m.statusBar.Hint = m.volumeList.Hints()
			return m, m.volumeList.Init()
		}
		m.statusBar.Hint = m.volumeList.Hints()
		return m, nil

	case tabFloatingIPs:
		m.view = viewFloatingIPList
		m.statusBar.CurrentView = "floatingiplist"
		if !m.tabsInited[tabFloatingIPs] {
			m.floatingIPList = floatingiplist.New(m.client.Network, m.refreshInterval)
			m.floatingIPList.SetSize(m.width, m.height)
			m.tabsInited[tabFloatingIPs] = true
			m.statusBar.Hint = m.floatingIPList.Hints()
			return m, m.floatingIPList.Init()
		}
		m.statusBar.Hint = m.floatingIPList.Hints()
		return m, nil

	case tabSecGroups:
		m.view = viewSecGroupView
		m.statusBar.CurrentView = "secgroupview"
		if !m.tabsInited[tabSecGroups] {
			m.secGroupView = secgroupview.New(m.client.Network, m.refreshInterval)
			m.secGroupView.SetSize(m.width, m.height)
			m.tabsInited[tabSecGroups] = true
			m.statusBar.Hint = m.secGroupView.Hints()
			return m, m.secGroupView.Init()
		}
		m.statusBar.Hint = m.secGroupView.Hints()
		return m, nil

	case tabKeypairs:
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
		if !m.tabsInited[tabKeypairs] {
			m.keypairList = keypairlist.New(m.client.Compute)
			m.keypairList.SetSize(m.width, m.height)
			m.tabsInited[tabKeypairs] = true
			m.statusBar.Hint = m.keypairList.Hints()
			return m, m.keypairList.Init()
		}
		m.statusBar.Hint = m.keypairList.Hints()
		return m, nil
	}
	return m, nil
}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if activeTab(i) == m.activeTab {
			tabs = append(tabs, lipgloss.NewStyle().
				Background(shared.ColorPrimary).
				Foreground(shared.ColorBg).
				Bold(true).
				Render(label))
		} else {
			tabs = append(tabs, lipgloss.NewStyle().
				Foreground(shared.ColorMuted).
				Render(label))
		}
	}
	return strings.Join(tabs, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("│"))
}

func (m Model) switchToCloudPicker() (Model, tea.Cmd) {
	clouds, err := cloud.ListCloudNames()
	m.cloudPicker = cloudpicker.New(clouds, err)
	m.cloudPicker.SetSize(m.width, m.height)
	m.view = viewCloudPicker
	m.statusBar.CurrentView = "cloudpicker"
	m.statusBar.Hint = "Select a cloud to connect"
	return m, nil
}

func (m Model) openDeleteConfirm() (Model, tea.Cmd) {
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		servers := m.serverList.SelectedServers()
		refs := make([]modal.ServerRef, len(servers))
		for i, s := range servers {
			refs[i] = modal.ServerRef{ID: s.ID, Name: s.Name}
		}
		m.confirm = modal.NewBulkConfirm("delete", refs)
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil
	}
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete", id, name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openRebootConfirm(action string) (Model, tea.Cmd) {
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		servers := m.serverList.SelectedServers()
		refs := make([]modal.ServerRef, len(servers))
		for i, s := range servers {
			refs[i] = modal.ServerRef{ID: s.ID, Name: s.Name}
		}
		m.confirm = modal.NewBulkConfirm(action, refs)
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil
	}
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm(action, id, name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openToggleConfirm(action string) (Model, tea.Cmd) {
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		servers := m.serverList.SelectedServers()
		// For toggle actions, determine the action from the first server's status
		actualAction := action
		if len(servers) > 0 {
			status := servers[0].Status
			switch action {
			case "pause/unpause":
				if status == "PAUSED" {
					actualAction = "unpause"
				} else {
					actualAction = "pause"
				}
			case "suspend/resume":
				if status == "SUSPENDED" {
					actualAction = "resume"
				} else {
					actualAction = "suspend"
				}
			case "shelve/unshelve":
				if status == "SHELVED" || status == "SHELVED_OFFLOADED" {
					actualAction = "unshelve"
				} else {
					actualAction = "shelve"
				}
			}
		}
		refs := make([]modal.ServerRef, len(servers))
		for i, s := range servers {
			refs[i] = modal.ServerRef{ID: s.ID, Name: s.Name}
		}
		m.confirm = modal.NewBulkConfirm(actualAction, refs)
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil
	}
	var id, name, status string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name, status = s.ID, s.Name, s.Status
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
		status = m.serverDetail.ServerStatus()
	}
	if id == "" {
		return m, nil
	}

	// Determine the actual action based on current state
	actualAction := action
	switch action {
	case "pause/unpause":
		if status == "PAUSED" {
			actualAction = "unpause"
		} else {
			actualAction = "pause"
		}
	case "suspend/resume":
		if status == "SUSPENDED" {
			actualAction = "resume"
		} else {
			actualAction = "suspend"
		}
	case "shelve/unshelve":
		if status == "SHELVED" || status == "SHELVED_OFFLOADED" {
			actualAction = "unshelve"
		} else {
			actualAction = "shelve"
		}
	}

	m.confirm = modal.NewConfirm(actualAction, id, name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openConsoleLog() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.consoleLog = consolelog.New(m.client.Compute, id, name)
	m.consoleLog.SetSize(m.width, m.height)
	m.previousView = m.view
	m.view = viewConsoleLog
	m.statusBar.CurrentView = "consolelog"
	m.statusBar.Hint = m.consoleLog.Hints()
	return m, m.consoleLog.Init()
}

func (m Model) openActionLog() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.actionLog = actionlog.New(m.client.Compute, id, name)
	m.actionLog.SetSize(m.width, m.height)
	m.previousView = m.view
	m.view = viewActionLog
	m.statusBar.CurrentView = "actionlog"
	m.statusBar.Hint = m.actionLog.Hints()
	return m, m.actionLog.Init()
}

func (m Model) openResize() (Model, tea.Cmd) {
	// Bulk resize
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		servers := m.serverList.SelectedServers()
		ids := make([]string, len(servers))
		for i, s := range servers {
			ids[i] = s.ID
		}
		// Use first server's flavor as current (best effort)
		currentFlavor := ""
		if len(servers) > 0 {
			currentFlavor = servers[0].FlavorName
		}
		m.serverResize = serverresize.NewBulk(m.client.Compute, ids, currentFlavor)
		m.serverResize.SetSize(m.width, m.height)
		m.serverList.ClearSelection()
		return m, m.serverResize.Init()
	}

	var id, name, flavor string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name, flavor = s.ID, s.Name, s.FlavorName
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
		flavor = m.serverDetail.ServerFlavor()
	}
	if id == "" {
		return m, nil
	}
	m.serverResize = serverresize.New(m.client.Compute, id, name, flavor)
	m.serverResize.SetSize(m.width, m.height)
	return m, m.serverResize.Init()
}

func (m Model) getSelectedServerInfo() (id, name string) {
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name = s.ID, s.Name
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	return
}

func (m Model) doConfirmResize() (Model, tea.Cmd) {
	// Bulk confirm
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		srvs := m.serverList.SelectedServers()
		ids := make([]string, 0, len(srvs))
		for _, s := range srvs {
			if s.Status == "VERIFY_RESIZE" {
				ids = append(ids, s.ID)
			}
		}
		if len(ids) == 0 {
			return m, nil
		}
		m.serverList.ClearSelection()
		m.statusBar.Hint = fmt.Sprintf("✓ Confirm resize %d servers", len(ids))
		client := m.client.Compute
		return m, func() tea.Msg {
			var errs []string
			for _, id := range ids {
				if err := compute.ConfirmResize(context.Background(), client, id); err != nil {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				return shared.ServerActionErrMsg{
					Action: "Confirm resize",
					Name:   fmt.Sprintf("%d servers", len(ids)),
					Err:    fmt.Errorf("%s", strings.Join(errs, "; ")),
				}
			}
			return shared.ServerActionMsg{Action: "Confirm resize", Name: fmt.Sprintf("%d servers", len(ids))}
		}
	}

	id, name := m.getSelectedServerInfo()
	if id == "" {
		return m, nil
	}
	if m.view == viewServerDetail {
		m.serverDetail.SetPendingAction("Resize confirmed")
	}
	m.statusBar.Hint = fmt.Sprintf("✓ Confirm resize %s", name)
	client := m.client.Compute
	return m, func() tea.Msg {
		err := compute.ConfirmResize(context.Background(), client, id)
		if err != nil {
			return shared.ServerActionErrMsg{Action: "Confirm resize", Name: name, Err: err}
		}
		return shared.ServerActionMsg{Action: "Confirm resize", Name: name}
	}
}

func (m Model) doRevertResize() (Model, tea.Cmd) {
	// Bulk revert
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		srvs := m.serverList.SelectedServers()
		ids := make([]string, 0, len(srvs))
		for _, s := range srvs {
			if s.Status == "VERIFY_RESIZE" {
				ids = append(ids, s.ID)
			}
		}
		if len(ids) == 0 {
			return m, nil
		}
		m.serverList.ClearSelection()
		m.statusBar.Hint = fmt.Sprintf("✓ Revert resize %d servers", len(ids))
		client := m.client.Compute
		return m, func() tea.Msg {
			var errs []string
			for _, id := range ids {
				if err := compute.RevertResize(context.Background(), client, id); err != nil {
					errs = append(errs, err.Error())
				}
			}
			if len(errs) > 0 {
				return shared.ServerActionErrMsg{
					Action: "Revert resize",
					Name:   fmt.Sprintf("%d servers", len(ids)),
					Err:    fmt.Errorf("%s", strings.Join(errs, "; ")),
				}
			}
			return shared.ServerActionMsg{Action: "Revert resize", Name: fmt.Sprintf("%d servers", len(ids))}
		}
	}

	id, name := m.getSelectedServerInfo()
	if id == "" {
		return m, nil
	}
	if m.view == viewServerDetail {
		m.serverDetail.SetPendingAction("Resize reverted")
	}
	m.statusBar.Hint = fmt.Sprintf("✓ Revert resize %s", name)
	client := m.client.Compute
	return m, func() tea.Msg {
		err := compute.RevertResize(context.Background(), client, id)
		if err != nil {
			return shared.ServerActionErrMsg{Action: "Revert resize", Name: name, Err: err}
		}
		return shared.ServerActionMsg{Action: "Revert resize", Name: name}
	}
}

// --- Volume actions ---

func (m Model) openVolumeDetail() (Model, tea.Cmd) {
	v := m.volumeList.SelectedVolume()
	if v == nil {
		return m, nil
	}
	m.volumeDetail = volumedetail.New(m.client.BlockStorage, m.client.Compute, v.ID, m.refreshInterval)
	m.volumeDetail.SetSize(m.width, m.height)
	m.view = viewVolumeDetail
	m.statusBar.CurrentView = "volumedetail"
	m.statusBar.Hint = m.volumeDetail.Hints()
	return m, m.volumeDetail.Init()
}

func (m Model) openVolumeDeleteConfirm() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewVolumeList:
		if v := m.volumeList.SelectedVolume(); v != nil {
			id, name = v.ID, v.Name
			if name == "" {
				name = id
			}
		}
	case viewVolumeDetail:
		id = m.volumeDetail.SelectedVolumeID()
		name = m.volumeDetail.SelectedVolumeName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_volume", id, name)
	m.confirm.Title = "Delete Volume"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete volume %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openVolumeAttach() (Model, tea.Cmd) {
	// Attach requires a server ID — for now, show an error that this needs CLI
	// A full implementation would need a server picker modal
	m.errModal = modal.NewError("Attach Volume", fmt.Errorf("use 'openstack server add volume' CLI to attach volumes"))
	m.errModal.SetSize(m.width, m.height)
	m.activeModal = modalError
	return m, nil
}

func (m Model) openVolumeDetach() (Model, tea.Cmd) {
	if m.view != viewVolumeDetail {
		return m, nil
	}
	id := m.volumeDetail.SelectedVolumeID()
	name := m.volumeDetail.SelectedVolumeName()
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("detach_volume", id, name)
	m.confirm.Title = "Detach Volume"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to detach volume %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Floating IP actions ---

func (m Model) openFIPReleaseConfirm() (Model, tea.Cmd) {
	fip := m.floatingIPList.SelectedFIP()
	if fip == nil {
		return m, nil
	}
	m.confirm = modal.NewConfirm("release_fip", fip.ID, fip.FloatingIP)
	m.confirm.Title = "Release Floating IP"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to release floating IP %s?", fip.FloatingIP)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) doAllocateFIP() (Model, tea.Cmd) {
	m.statusBar.Hint = "Allocating floating IP..."
	networkClient := m.client.Network
	return m, func() tea.Msg {
		nets, err := network.ListExternalNetworks(context.Background(), networkClient)
		if err != nil {
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: err}
		}
		if len(nets) == 0 {
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: fmt.Errorf("no external networks available")}
		}
		fip, err := network.AllocateFloatingIP(context.Background(), networkClient, nets[0].ID)
		if err != nil {
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: err}
		}
		return shared.ResourceActionMsg{Action: "Allocated", Name: fip.FloatingIP}
	}
}

func (m Model) doAllocateAndAssociateFIP() (Model, tea.Cmd) {
	var serverID, serverName string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			serverID, serverName = s.ID, s.Name
		}
	case viewServerDetail:
		serverID = m.serverDetail.ServerID()
		serverName = m.serverDetail.ServerName()
	}
	if serverID == "" {
		return m, nil
	}
	m.fipPicker = fippicker.New(m.client.Network, serverID, serverName)
	m.fipPicker.SetSize(m.width, m.height)
	return m, m.fipPicker.Init()
}

func (m Model) openFIPDisassociateConfirm() (Model, tea.Cmd) {
	fip := m.floatingIPList.SelectedFIP()
	if fip == nil || fip.PortID == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("disassociate_fip", fip.ID, fip.FloatingIP)
	m.confirm.Title = "Disassociate Floating IP"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to disassociate floating IP %s?", fip.FloatingIP)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Security Group actions ---

func (m Model) openSGRuleDeleteConfirm() (Model, tea.Cmd) {
	ruleID := m.secGroupView.SelectedRule()
	if ruleID == "" {
		return m, nil
	}
	groupName := m.secGroupView.SelectedGroupName()
	m.confirm = modal.NewConfirm("delete_sg_rule", ruleID, groupName)
	m.confirm.Title = "Delete Security Group Rule"
	m.confirm.Body = fmt.Sprintf("Delete rule from security group %q?", groupName)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Key Pair actions ---

func (m Model) openKeyPairDeleteConfirm() (Model, tea.Cmd) {
	kp := m.keypairList.SelectedKeyPair()
	if kp == nil {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_keypair", kp.Name, kp.Name)
	m.confirm.Title = "Delete Key Pair"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete key pair %q?", kp.Name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) executeAction(action modal.ConfirmAction) (Model, tea.Cmd) {
	client := m.client.Compute

	// Bulk actions
	if len(action.Servers) > 0 {
		m.serverList.ClearSelection()
		return m, m.executeBulkAction(client, action)
	}

	switch action.Action {
	case "delete":
		return m, func() tea.Msg {
			err := compute.DeleteServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Delete", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Delete", Name: action.Name}
		}
	case "soft reboot":
		return m, func() tea.Msg {
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.SoftReboot)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Reboot", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Reboot", Name: action.Name}
		}
	case "hard reboot":
		return m, func() tea.Msg {
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.HardReboot)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Hard reboot", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Hard reboot", Name: action.Name}
		}
	case "pause":
		return m, func() tea.Msg {
			err := compute.PauseServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Pause", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Pause", Name: action.Name}
		}
	case "unpause":
		return m, func() tea.Msg {
			err := compute.UnpauseServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Unpause", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Unpause", Name: action.Name}
		}
	case "suspend":
		return m, func() tea.Msg {
			err := compute.SuspendServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Suspend", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Suspend", Name: action.Name}
		}
	case "resume":
		return m, func() tea.Msg {
			err := compute.ResumeServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Resume", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Resume", Name: action.Name}
		}
	case "shelve":
		return m, func() tea.Msg {
			err := compute.ShelveServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Shelve", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Shelve", Name: action.Name}
		}
	case "unshelve":
		return m, func() tea.Msg {
			err := compute.UnshelveServer(context.Background(), client, action.ServerID)
			if err != nil {
				return shared.ServerActionErrMsg{Action: "Unshelve", Name: action.Name, Err: err}
			}
			return shared.ServerActionMsg{Action: "Unshelve", Name: action.Name}
		}
	case "delete_volume":
		bsClient := m.client.BlockStorage
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			err := volume.DeleteVolume(context.Background(), bsClient, id)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Delete volume", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Deleted volume", Name: name}
		}
	case "detach_volume":
		computeC := m.client.Compute
		volID := action.ServerID
		name := action.Name
		bsClient := m.client.BlockStorage
		return m, func() tea.Msg {
			vol, err := volume.GetVolume(context.Background(), bsClient, volID)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: err}
			}
			if vol.AttachedServerID == "" {
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: fmt.Errorf("volume is not attached")}
			}
			err = volume.DetachVolume(context.Background(), computeC, vol.AttachedServerID, volID)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Detached volume", Name: name}
		}
	case "release_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			err := network.ReleaseFloatingIP(context.Background(), netClient, id)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Release FIP", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Released", Name: name}
		}
	case "disassociate_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			err := network.DisassociateFloatingIP(context.Background(), netClient, id)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Disassociate FIP", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Disassociated", Name: name}
		}
	case "delete_sg_rule":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			err := network.DeleteSecurityGroupRule(context.Background(), netClient, id)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Delete rule", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Deleted rule from", Name: name}
		}
	case "delete_keypair":
		computeC := m.client.Compute
		name := action.ServerID // keypair name is stored in ServerID
		return m, func() tea.Msg {
			err := compute.DeleteKeyPair(context.Background(), computeC, name)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Delete keypair", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Deleted keypair", Name: name}
		}
	}
	return m, nil
}

func (m Model) executeBulkAction(client *gophercloud.ServiceClient, action modal.ConfirmAction) tea.Cmd {
	targets := action.Servers
	act := action.Action
	return func() tea.Msg {
		var errs []string
		for _, s := range targets {
			var err error
			switch act {
			case "delete":
				err = compute.DeleteServer(context.Background(), client, s.ID)
			case "soft reboot":
				err = compute.RebootServer(context.Background(), client, s.ID, servers.SoftReboot)
			case "pause":
				err = compute.PauseServer(context.Background(), client, s.ID)
			case "unpause":
				err = compute.UnpauseServer(context.Background(), client, s.ID)
			case "suspend":
				err = compute.SuspendServer(context.Background(), client, s.ID)
			case "resume":
				err = compute.ResumeServer(context.Background(), client, s.ID)
			case "shelve":
				err = compute.ShelveServer(context.Background(), client, s.ID)
			case "unshelve":
				err = compute.UnshelveServer(context.Background(), client, s.ID)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
			}
		}
		if len(errs) > 0 {
			return shared.ServerActionErrMsg{
				Action: act,
				Name:   fmt.Sprintf("%d servers", len(targets)),
				Err:    fmt.Errorf("%s", strings.Join(errs, "; ")),
			}
		}
		return shared.ServerActionMsg{
			Action: act,
			Name:   fmt.Sprintf("%d servers", len(targets)),
		}
	}
}

func (m Model) connectToCloud(name string) tea.Cmd {
	return func() tea.Msg {
		client, err := cloud.Connect(context.Background(), name)
		if err != nil {
			return shared.CloudConnectErrMsg{Err: err}
		}
		return shared.CloudConnectedMsg{
			ComputeClient:      client.Compute,
			ImageClient:        client.Image,
			NetworkClient:      client.Network,
			BlockStorageClient: client.BlockStorage,
			Region:             client.Region,
		}
	}
}

func (m Model) viewName() string {
	switch m.view {
	case viewCloudPicker:
		return "cloudpicker"
	case viewServerList:
		return "serverlist"
	case viewServerDetail:
		return "serverdetail"
	case viewServerCreate:
		return "servercreate"
	case viewConsoleLog:
		return "consolelog"
	case viewActionLog:
		return "actionlog"
	case viewVolumeList:
		return "volumelist"
	case viewVolumeDetail:
		return "volumedetail"
	case viewFloatingIPList:
		return "floatingiplist"
	case viewSecGroupView:
		return "secgroupview"
	case viewKeypairList:
		return "keypairlist"
	}
	return ""
}

// View renders the full UI.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.tooSmall {
		msg := fmt.Sprintf("Terminal too small (%dx%d). Need at least %dx%d.",
			m.width, m.height, m.minWidth, m.minHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(msg))
	}

	if m.help.Visible {
		return m.help.Render()
	}

	if m.activeModal == modalConfirm {
		return m.confirm.View()
	}
	if m.activeModal == modalError {
		return m.errModal.View()
	}
	if m.fipPicker.Active {
		return m.fipPicker.View()
	}
	if m.serverResize.Active {
		return m.serverResize.View()
	}

	var content string
	switch m.view {
	case viewCloudPicker:
		return m.cloudPicker.View()
	case viewServerList:
		content = m.serverList.View()
	case viewServerDetail:
		content = m.serverDetail.View()
	case viewServerCreate:
		content = m.serverCreate.View()
	case viewConsoleLog:
		content = m.consoleLog.View()
	case viewActionLog:
		content = m.actionLog.View()
	case viewVolumeList:
		content = m.volumeList.View()
	case viewVolumeDetail:
		content = m.volumeDetail.View()
	case viewFloatingIPList:
		content = m.floatingIPList.View()
	case viewSecGroupView:
		content = m.secGroupView.View()
	case viewKeypairList:
		content = m.keypairList.View()
	}

	// Add tab bar for top-level views
	if m.isTopLevelView() {
		content = m.renderTabBar() + "\n" + content
	}

	// Overlay app name + version on top-right (lines 0 and 1)
	appName := lipgloss.NewStyle().
		Foreground(shared.ColorBg).
		Background(shared.ColorPrimary).
		Bold(true).
		Padding(0, 1).
		Render("LAZYSTACK")
	versionStr := ""
	if m.version != "" {
		versionStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(m.version)
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]
		firstW := lipgloss.Width(firstLine)
		nameW := lipgloss.Width(appName)
		pad := m.width - firstW - nameW
		if pad > 0 {
			lines[0] = firstLine + strings.Repeat(" ", pad) + appName
		}
	}
	if len(lines) > 1 && versionStr != "" {
		secondLine := lines[1]
		secondW := lipgloss.Width(secondLine)
		verW := lipgloss.Width(versionStr)
		pad := m.width - secondW - verW
		if pad > 0 {
			lines[1] = secondLine + strings.Repeat(" ", pad) + versionStr
		}
	}
	content = strings.Join(lines, "\n")

	contentHeight := m.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	padded := lipgloss.NewStyle().Height(contentHeight).Render(content)
	return padded + "\n" + m.statusBar.Render()
}

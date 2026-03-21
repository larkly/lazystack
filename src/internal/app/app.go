package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bosse/lazystack/internal/cloud"
	"github.com/bosse/lazystack/internal/compute"
	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/ui/actionlog"
	"github.com/bosse/lazystack/internal/ui/cloudpicker"
	"github.com/bosse/lazystack/internal/ui/consolelog"
	"github.com/bosse/lazystack/internal/ui/fippicker"
	"github.com/bosse/lazystack/internal/ui/floatingiplist"
	"github.com/bosse/lazystack/internal/ui/help"
	"github.com/bosse/lazystack/internal/ui/keypairlist"
	"github.com/bosse/lazystack/internal/ui/lbdetail"
	"github.com/bosse/lazystack/internal/ui/lblist"
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
)

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
	viewLBList
	viewLBDetail
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
	lbList        lblist.Model
	lbDetail      lbdetail.Model
	statusBar     statusbar.Model
	tabs      []TabDef
	activeTab int
	tabInited []bool
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

	tabs := DefaultTabs()

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
			tabs:            tabs,
			tabInited:       make([]bool, len(tabs)),
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
		tabs:            tabs,
		tabInited:       make([]bool, len(tabs)),
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
				// Number keys 1-9 map to tab indices
				if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
					idx := int(s[0] - '1')
					if idx < len(m.tabs) {
						return m.switchTab(idx)
					}
				}
				switch {
				case key.Matches(msg, shared.Keys.Right):
					next := (m.activeTab + 1) % len(m.tabs)
					return m.switchTab(next)
				case key.Matches(msg, shared.Keys.Left):
					prev := (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
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

		// Load balancer list: Enter to open detail, ctrl+d to delete
		if m.view == viewLBList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openLBDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openLBDeleteConfirm()
			}
		}

		// Load balancer detail: ctrl+d delete
		if m.view == viewLBDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openLBDeleteConfirm()
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
			LoadBalancer: msg.LoadBalancerClient,
		}
		// Build tabs conditionally based on available services
		m.tabs = []TabDef{{Name: "Servers", Key: "servers"}}
		if msg.BlockStorageClient != nil {
			m.tabs = append(m.tabs, TabDef{Name: "Volumes", Key: "volumes"})
		}
		m.tabs = append(m.tabs, TabDef{Name: "Floating IPs", Key: "floatingips"})
		m.tabs = append(m.tabs, TabDef{Name: "Sec Groups", Key: "secgroups"})
		if msg.LoadBalancerClient != nil {
			m.tabs = append(m.tabs, TabDef{Name: "Load Balancers", Key: "loadbalancers"})
		}
		m.tabs = append(m.tabs, TabDef{Name: "Key Pairs", Key: "keypairs"})
		m.tabInited = make([]bool, len(m.tabs))
		m.activeTab = 0
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
		if m.view == viewLBDetail {
			m.view = viewLBList
			m.statusBar.CurrentView = "lblist"
			m.statusBar.Hint = m.lbList.Hints()
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

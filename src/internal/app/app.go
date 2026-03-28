package app

import (
	"context"
	"fmt"
	"time"

	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/selfupdate"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/cloudpicker"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/fippicker"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/help"
	"github.com/larkly/lazystack/internal/ui/keypaircreate"
	"github.com/larkly/lazystack/internal/ui/keypairdetail"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/lbdetail"
	"github.com/larkly/lazystack/internal/ui/lblist"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/networkcreate"
	"github.com/larkly/lazystack/internal/ui/routercreate"
	"github.com/larkly/lazystack/internal/ui/routerdetail"
	"github.com/larkly/lazystack/internal/ui/routerlist"
	"github.com/larkly/lazystack/internal/ui/subnetpicker"
	"github.com/larkly/lazystack/internal/ui/networklist"
	"github.com/larkly/lazystack/internal/ui/subnetcreate"
	"github.com/larkly/lazystack/internal/ui/projectpicker"
	"github.com/larkly/lazystack/internal/ui/quotaview"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/sgcreate"
	"github.com/larkly/lazystack/internal/ui/sgrulecreate"
	"github.com/larkly/lazystack/internal/ui/serverpicker"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverrename"
	"github.com/larkly/lazystack/internal/ui/serverrebuild"
	"github.com/larkly/lazystack/internal/ui/serverrescue"
	"github.com/larkly/lazystack/internal/ui/serversnapshot"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/serverresize"
	"github.com/larkly/lazystack/internal/ui/statusbar"
	"github.com/larkly/lazystack/internal/ui/volumecreate"
	"github.com/larkly/lazystack/internal/ui/imagedetail"
	"github.com/larkly/lazystack/internal/ui/imagelist"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
	"github.com/larkly/lazystack/internal/ui/volumelist"
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
	viewVolumeCreate
	viewKeypairCreate
	viewNetworkList
	viewKeypairDetail
	viewRouterList
	viewRouterDetail
	viewImageList
	viewImageDetail
)

type modalType int

// UpdateAvailableMsg is sent when a newer version is found.
type UpdateAvailableMsg struct {
	Latest      string
	DownloadURL string
	ChecksumsURL string
}

// UpdateResultMsg is sent after selfupdate.Apply completes.
type UpdateResultMsg struct {
	Err error
}

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
	serverRename    serverrename.Model
	serverRebuild   serverrebuild.Model
	serverRescue    serverrescue.Model
	serverSnapshot  serversnapshot.Model
	serverResize    serverresize.Model
	fipPicker     fippicker.Model
	serverPicker  serverpicker.Model
	sgCreate       sgcreate.Model
	networkCreate  networkcreate.Model
	subnetCreate   subnetcreate.Model
	routerList     routerlist.Model
	routerDetail   routerdetail.Model
	routerCreate   routercreate.Model
	subnetPicker   subnetpicker.Model
	sgRuleCreate  sgrulecreate.Model
	projectPicker projectpicker.Model
	volumeList    volumelist.Model
	volumeDetail  volumedetail.Model
	volumeCreate  volumecreate.Model
	floatingIPList floatingiplist.Model
	secGroupView  secgroupview.Model
	keypairList    keypairlist.Model
	keypairCreate  keypaircreate.Model
	keypairDetail  keypairdetail.Model
	imageList      imagelist.Model
	imageDetail    imagedetail.Model
	networkList   networklist.Model
	lbList        lblist.Model
	lbDetail      lbdetail.Model
	statusBar     statusbar.Model
	tabs      []TabDef
	activeTab int
	tabInited []bool
	help         help.Model
	quotaView    quotaview.Model
	confirm      modal.ConfirmModel
	errModal     modal.ErrorModel
	activeModal  modalType
	projects         []shared.ProjectInfo
	currentProjectID string
	cloudName        string
	autoCloud        string
	previousView    activeView
	refreshInterval time.Duration
	minWidth        int
	minHeight    int
	tooSmall     bool
	restart        bool
	version        string
	checkUpdate    bool
	updating       bool
	idleTimeout    time.Duration
	lastActivity   time.Time
	idlePaused     bool
	latestVersion  string
	downloadURL    string
	checksumsURL   string
}

// ShouldRestart returns true if the app quit due to a restart request.
func (m Model) ShouldRestart() bool {
	return m.restart
}

// Options configures the application.
type Options struct {
	AlwaysPickCloud bool
	Cloud           string
	RefreshInterval time.Duration
	IdleTimeout     time.Duration
	Version         string
	CheckUpdate     bool
}

// New creates the root model.
func New(opts Options) Model {
	clouds, err := cloud.ListCloudNames()

	refresh := opts.RefreshInterval
	if refresh == 0 {
		refresh = 5 * time.Second
	}

	tabs := DefaultTabs()

	// Auto-select if --cloud flag is set, or exactly one cloud and not forced to pick
	if opts.Cloud != "" || (err == nil && len(clouds) == 1 && !opts.AlwaysPickCloud) {
		autoName := opts.Cloud
		if autoName == "" {
			autoName = clouds[0]
		}
		return Model{
			view:            viewCloudPicker,
			cloudPicker:     cloudpicker.New(clouds, nil),
			statusBar:       statusbar.New(opts.Version),
			help:            help.New(),
			quotaView:       quotaview.New(),
			minWidth:        80,
			minHeight:       20,
			autoCloud:       autoName,
			refreshInterval: refresh,
			idleTimeout:     opts.IdleTimeout,
			version:         opts.Version,
			checkUpdate:     opts.CheckUpdate,
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
		quotaView:       quotaview.New(),
		minWidth:        80,
		minHeight:       20,
		refreshInterval: refresh,
		idleTimeout:     opts.IdleTimeout,
		version:         opts.Version,
		checkUpdate:     opts.CheckUpdate,
		tabs:            tabs,
		tabInited:       make([]bool, len(tabs)),
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.autoCloud != "" {
		name := m.autoCloud
		cmds = append(cmds, func() tea.Msg {
			return shared.CloudSelectedMsg{CloudName: name}
		})
	}
	if m.checkUpdate && m.version != "dev" {
		ver := m.version
		cmds = append(cmds, func() tea.Msg {
			latest, dlURL, csURL, err := selfupdate.CheckLatest(ver)
			if err != nil || latest == "" {
				return nil
			}
			return UpdateAvailableMsg{Latest: latest, DownloadURL: dlURL, ChecksumsURL: csURL}
		})
	}
	return tea.Batch(cmds...)
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
		m.quotaView.Width = m.width
		m.quotaView.Height = m.height
		m.serverRename.SetSize(m.width, m.height)
		m.serverRebuild.SetSize(m.width, m.height)
		m.serverRescue.SetSize(m.width, m.height)
		m.serverSnapshot.SetSize(m.width, m.height)
		m.serverResize.SetSize(m.width, m.height)
		m.fipPicker.SetSize(m.width, m.height)
		m.serverPicker.SetSize(m.width, m.height)
		m.sgCreate.SetSize(m.width, m.height)
		m.sgRuleCreate.SetSize(m.width, m.height)
		m.networkCreate.SetSize(m.width, m.height)
		m.subnetCreate.SetSize(m.width, m.height)
		m.routerCreate.SetSize(m.width, m.height)
		m.subnetPicker.SetSize(m.width, m.height)
		m.projectPicker.SetSize(m.width, m.height)
		m.statusBar.Width = m.width
		return m.updateActiveView(msg)

	case tea.KeyMsg:
		m.lastActivity = time.Now()
		m.statusBar.StickyHint = ""
		if m.idlePaused {
			m.idlePaused = false
			m.statusBar.Hint = ""
			return m, func() tea.Msg { return shared.TickMsg{} }
		}

		if m.help.Visible {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}

		if m.quotaView.Visible {
			var cmd tea.Cmd
			m.quotaView, cmd = m.quotaView.Update(msg)
			return m, cmd
		}

		// Restart works from anywhere
		if key.Matches(msg, shared.Keys.Restart) {
			m.restart = true
			return m, tea.Quit
		}

		if m.activeModal != modalNone {
			if m.updating {
				return m, nil // swallow keys while update is downloading
			}
			return m.updateModal(msg)
		}

		// Rename modal intercepts all keys when active
		if m.serverRename.Active {
			var cmd tea.Cmd
			m.serverRename, cmd = m.serverRename.Update(msg)
			return m, cmd
		}

		// Rebuild modal intercepts all keys when active
		if m.serverRebuild.Active {
			var cmd tea.Cmd
			m.serverRebuild, cmd = m.serverRebuild.Update(msg)
			return m, cmd
		}

		// Rescue modal intercepts all keys when active
		if m.serverRescue.Active {
			var cmd tea.Cmd
			m.serverRescue, cmd = m.serverRescue.Update(msg)
			return m, cmd
		}

		// Snapshot modal intercepts all keys when active
		if m.serverSnapshot.Active {
			var cmd tea.Cmd
			m.serverSnapshot, cmd = m.serverSnapshot.Update(msg)
			return m, cmd
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

		// Server picker modal intercepts all keys when active
		if m.serverPicker.Active {
			var cmd tea.Cmd
			m.serverPicker, cmd = m.serverPicker.Update(msg)
			return m, cmd
		}

		// Router create modal intercepts all keys when active
		if m.routerCreate.Active {
			var cmd tea.Cmd
			m.routerCreate, cmd = m.routerCreate.Update(msg)
			return m, cmd
		}

		// Subnet picker modal intercepts all keys when active
		if m.subnetPicker.Active {
			var cmd tea.Cmd
			m.subnetPicker, cmd = m.subnetPicker.Update(msg)
			return m, cmd
		}

		// Network create modal intercepts all keys when active
		if m.networkCreate.Active {
			var cmd tea.Cmd
			m.networkCreate, cmd = m.networkCreate.Update(msg)
			return m, cmd
		}

		// Subnet create modal intercepts all keys when active
		if m.subnetCreate.Active {
			var cmd tea.Cmd
			m.subnetCreate, cmd = m.subnetCreate.Update(msg)
			return m, cmd
		}

		// SG create modal intercepts all keys when active
		if m.sgCreate.Active {
			var cmd tea.Cmd
			m.sgCreate, cmd = m.sgCreate.Update(msg)
			return m, cmd
		}

		// SG rule create modal intercepts all keys when active
		if m.sgRuleCreate.Active {
			var cmd tea.Cmd
			m.sgRuleCreate, cmd = m.sgRuleCreate.Update(msg)
			return m, cmd
		}

		// Project picker modal intercepts all keys when active
		if m.projectPicker.Active {
			var cmd tea.Cmd
			m.projectPicker, cmd = m.projectPicker.Update(msg)
			return m, cmd
		}

		if m.view != viewServerCreate && m.view != viewVolumeCreate && m.view != viewKeypairCreate {
			switch {
			case key.Matches(msg, shared.Keys.Quit) && m.view != viewCloudPicker:
				return m, tea.Quit
			case key.Matches(msg, shared.Keys.Help):
				m.help.Visible = true
				m.help.View = m.viewName()
				return m, nil
			case key.Matches(msg, shared.Keys.CloudPick) && m.view != viewCloudPicker:
				return m.switchToCloudPicker()
			case key.Matches(msg, shared.Keys.ProjectPick) && m.view != viewCloudPicker && len(m.projects) > 1:
				m.projectPicker = projectpicker.New(m.projects, m.currentProjectID)
				m.projectPicker.SetSize(m.width, m.height)
				return m, nil
			case key.Matches(msg, shared.Keys.Quota) && m.view != viewCloudPicker && m.view != viewServerCreate:
				m.quotaView.Width = m.width
				m.quotaView.Height = m.height
				if m.currentProjectID != "" {
					m.quotaView.SetProjectID(m.currentProjectID)
				}
				cmd := m.quotaView.Open()
				return m, cmd
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
			if key.Matches(msg, shared.Keys.StopStart) {
				return m.openToggleConfirm("stop/start")
			}
			if key.Matches(msg, shared.Keys.Lock) {
				return m.openToggleConfirm("lock/unlock")
			}
			if key.Matches(msg, shared.Keys.Rescue) {
				return m.openRescue()
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
			if key.Matches(msg, shared.Keys.Rename) {
				return m.openRename()
			}
			if key.Matches(msg, shared.Keys.Rebuild) {
				return m.openRebuild()
			}
			if key.Matches(msg, shared.Keys.Snapshot) {
				return m.openSnapshot()
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

		// Volume list: Enter to open detail, ctrl+d to delete, ctrl+n to create
		if m.view == viewVolumeList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openVolumeDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openVolumeDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				return m.openVolumeCreate()
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

		// Security group view: context-sensitive create/delete
		if m.view == viewSecGroupView {
			if key.Matches(msg, shared.Keys.Delete) {
				if m.secGroupView.InRules() {
					return m.openSGRuleDeleteConfirm()
				}
				return m.openSGDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				if m.secGroupView.InRules() {
					return m.openSGRuleCreate()
				}
				return m.openSGCreate()
			}
		}

		// Network list: context-sensitive create/delete for networks and subnets
		// When expanded: ctrl+n creates subnet, ctrl+d on subnet deletes subnet
		// When collapsed: ctrl+n creates network, ctrl+d deletes network
		if m.view == viewNetworkList {
			if key.Matches(msg, shared.Keys.Delete) {
				if m.networkList.InSubnets() {
					return m.openSubnetDeleteConfirm()
				}
				return m.openNetworkDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				if m.networkList.InSubnets() || m.networkList.IsExpanded() {
					return m.openSubnetCreate()
				}
				return m.openNetworkCreate()
			}
		}

		// Router list: Enter detail, ctrl+n create, ctrl+d delete
		if m.view == viewRouterList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openRouterDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openRouterDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				return m.openRouterCreate()
			}
		}

		// Router detail: ctrl+a add interface, ctrl+t remove interface, ctrl+d delete
		if m.view == viewRouterDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openRouterDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Attach) {
				return m.openAddRouterInterface()
			}
			if key.Matches(msg, shared.Keys.Detach) {
				return m.openRemoveRouterInterfaceConfirm()
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

		// Key pair list: enter detail, ctrl+d delete, ctrl+n create
		if m.view == viewKeypairList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openKeypairDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openKeyPairDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				return m.openKeypairCreate()
			}
		}

		// Key pair detail: ctrl+d delete
		if m.view == viewKeypairDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openKeyPairDeleteConfirm()
			}
		}

		// Image list: Enter to open detail, ctrl+d to delete, d to deactivate/reactivate
		if m.view == viewImageList {
			if key.Matches(msg, shared.Keys.Enter) {
				return m.openImageDetail()
			}
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openImageDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Deactivate) {
				return m.openImageDeactivateConfirm()
			}
		}

		// Image detail: ctrl+d delete, d to deactivate/reactivate
		if m.view == viewImageDetail {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openImageDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Deactivate) {
				return m.openImageDeactivateConfirm()
			}
		}

		return m.updateActiveView(msg)

	case shared.CloudSelectedMsg:
		m.lastActivity = time.Now()
		m.cloudName = msg.CloudName
		m.statusBar.Hint = "Connecting..."
		return m, m.connectToCloud(msg.CloudName)

	case shared.CloudConnectedMsg:
		m.lastActivity = time.Now()
		m.idlePaused = false
		m.client = &cloud.Client{
			CloudName:      m.cloudName,
			Compute:        msg.ComputeClient,
			Image:          msg.ImageClient,
			Network:        msg.NetworkClient,
			BlockStorage:   msg.BlockStorageClient,
			LoadBalancer:   msg.LoadBalancerClient,
			ProviderClient: msg.ProviderClient,
			EndpointOpts:   msg.EndpointOpts,
		}
		// Build tabs conditionally based on available services
		m.tabs = []TabDef{{Name: "Servers", Key: "servers"}}
		if msg.BlockStorageClient != nil {
			m.tabs = append(m.tabs, TabDef{Name: "Volumes", Key: "volumes"})
		}
		m.tabs = append(m.tabs, TabDef{Name: "Images", Key: "images"})
		m.tabs = append(m.tabs, TabDef{Name: "Floating IPs", Key: "floatingips"})
		m.tabs = append(m.tabs, TabDef{Name: "Sec Groups", Key: "secgroups"})
		m.tabs = append(m.tabs, TabDef{Name: "Networks", Key: "networks"})
		m.tabs = append(m.tabs, TabDef{Name: "Routers", Key: "routers"})
		if msg.LoadBalancerClient != nil {
			m.tabs = append(m.tabs, TabDef{Name: "Load Balancers", Key: "loadbalancers"})
		}
		m.tabs = append(m.tabs, TabDef{Name: "Key Pairs", Key: "keypairs"})
		m.tabInited = make([]bool, len(m.tabs))
		m.activeTab = 0
		m.statusBar.CloudName = m.cloudName
		m.statusBar.Region = msg.Region
		m.quotaView.SetClients(msg.ComputeClient, msg.NetworkClient, msg.BlockStorageClient, "")
		m.serverList = serverlist.New(msg.ComputeClient, msg.ImageClient, m.refreshInterval)
		m.serverList.SetSize(m.width, m.height)
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		cmds := []tea.Cmd{m.serverList.Init()}
		// Background-fetch accessible projects for project switching
		if msg.ProviderClient != nil {
			pc := msg.ProviderClient
			eo := msg.EndpointOpts
			cmds = append(cmds, func() tea.Msg {
				projs, err := cloud.ListAccessibleProjects(context.Background(), pc, eo)
				if err != nil {
					return nil
				}
				var infos []shared.ProjectInfo
				for _, p := range projs {
					infos = append(infos, shared.ProjectInfo{ID: p.ID, Name: p.Name})
				}
				// Try to get current project ID from the auth scope
				currentID := ""
				if pc.GetAuthResult() != nil {
					// The token should have project scope info
					// We'll match by checking project IDs
				}
				return shared.ProjectsLoadedMsg{Projects: infos, CurrentID: currentID}
			})
		}
		return m, tea.Batch(cmds...)

	case shared.ProjectsLoadedMsg:
		m.projects = msg.Projects
		// Find current project name for status bar
		for _, p := range msg.Projects {
			if p.ID == msg.CurrentID {
				m.statusBar.ProjectName = p.Name
				m.currentProjectID = p.ID
				break
			}
		}
		// If we couldn't identify the current project but have only one, use it
		if m.statusBar.ProjectName == "" && len(msg.Projects) == 1 {
			m.statusBar.ProjectName = msg.Projects[0].Name
			m.currentProjectID = msg.Projects[0].ID
		}
		// If we have a current project ID set from project switching, preserve the name
		if m.currentProjectID != "" && m.statusBar.ProjectName == "" {
			for _, p := range msg.Projects {
				if p.ID == m.currentProjectID {
					m.statusBar.ProjectName = p.Name
					break
				}
			}
		}
		if m.currentProjectID != "" {
			m.quotaView.SetProjectID(m.currentProjectID)
		}
		return m, nil

	case shared.ProjectSelectedMsg:
		m.lastActivity = time.Now()
		m.projectPicker.Active = false
		m.statusBar.Hint = fmt.Sprintf("Switching to project %s...", msg.ProjectName)
		m.currentProjectID = msg.ProjectID
		m.statusBar.ProjectName = msg.ProjectName
		cloudName := m.cloudName
		projectID := msg.ProjectID
		return m, func() tea.Msg {
			client, err := cloud.ConnectWithProject(context.Background(), cloudName, projectID)
			if err != nil {
				return shared.CloudConnectErrMsg{Err: err}
			}
			return shared.CloudConnectedMsg{
				ComputeClient:      client.Compute,
				ImageClient:        client.Image,
				NetworkClient:      client.Network,
				BlockStorageClient: client.BlockStorage,
				LoadBalancerClient: client.LoadBalancer,
				ProviderClient:     client.ProviderClient,
				EndpointOpts:       client.EndpointOpts,
				Region:             client.Region,
			}
		}

	case shared.CloudConnectErrMsg:
		m.errModal = modal.NewError("Cloud Connection", msg.Err)
		m.errModal.SetSize(m.width, m.height)
		m.activeModal = modalError
		return m, nil

	case UpdateAvailableMsg:
		m.latestVersion = msg.Latest
		m.downloadURL = msg.DownloadURL
		m.checksumsURL = msg.ChecksumsURL
		m.confirm = modal.ConfirmModel{
			Action: "update",
			Title:  "Update Available",
			Body:   fmt.Sprintf("New version available: %s (current: %s). Upgrade now?", msg.Latest, m.version),
		}
		m.confirm.SetSize(m.width, m.height)
		m.activeModal = modalConfirm
		return m, nil

	case UpdateResultMsg:
		m.updating = false
		if msg.Err != nil {
			m.errModal = modal.NewError("Update failed", msg.Err)
			m.errModal.SetSize(m.width, m.height)
			m.activeModal = modalError
			return m, nil
		}
		m.restart = true
		return m, tea.Quit

	case shared.ViewChangeMsg:
		return m.handleViewChange(msg)

	case modal.ConfirmAction:
		if msg.Confirm && msg.Action == "update" {
			m.updating = true
			m.confirm.Title = "Updating"
			m.confirm.Body = fmt.Sprintf("Downloading %s, please wait...", m.latestVersion)
			dlURL := m.downloadURL
			csURL := m.checksumsURL
			return m, func() tea.Msg {
				return UpdateResultMsg{Err: selfupdate.Apply(dlURL, csURL)}
			}
		}
		m.activeModal = modalNone
		if msg.Confirm {
			return m.executeAction(msg)
		}
		if msg.Action == "update" {
			m.statusBar.Hint = fmt.Sprintf("Upgrade available: %s — use --update", m.latestVersion)
		}
		return m, nil

	case modal.ErrorDismissedMsg:
		m.activeModal = modalNone
		return m, nil

	case shared.ServerActionMsg:
		m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
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
		m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
		m.statusBar.Error = ""
		// Navigate back to list view if we were on a detail view
		if m.view == viewVolumeDetail {
			m.view = viewVolumeList
			m.statusBar.CurrentView = "volumelist"
		}
		if m.view == viewKeypairDetail {
			m.view = viewKeypairList
			m.statusBar.CurrentView = "keypairlist"
		}
		if m.view == viewRouterDetail {
			m.view = viewRouterList
			m.statusBar.CurrentView = "routerlist"
		}
		if m.view == viewLBDetail {
			m.view = viewLBList
			m.statusBar.CurrentView = "lblist"
		}
		if m.view == viewImageDetail {
			m.view = viewImageList
			m.statusBar.CurrentView = "imagelist"
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
		// Idle timeout: swallow ticks when paused, or pause if idle too long
		if _, ok := msg.(shared.TickMsg); ok {
			if m.idlePaused {
				return m, nil
			}
			if m.idleTimeout > 0 && !m.lastActivity.IsZero() && time.Since(m.lastActivity) > m.idleTimeout {
				m.idlePaused = true
				m.statusBar.Hint = "⏸ Paused — press any key to resume"
				return m, nil
			}
		}
		// Route to all views first so background ticks keep firing
		m2, viewCmd := m.updateAllViews(msg)
		m = m2
		// Route to quota view for spinner/loaded messages
		if m.quotaView.Visible {
			var cmd tea.Cmd
			m.quotaView, cmd = m.quotaView.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		// Also route to active modals (for spinner, loaded msgs)
		if m.serverRename.Active {
			var cmd tea.Cmd
			m.serverRename, cmd = m.serverRename.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.serverRebuild.Active {
			var cmd tea.Cmd
			m.serverRebuild, cmd = m.serverRebuild.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.serverRescue.Active {
			var cmd tea.Cmd
			m.serverRescue, cmd = m.serverRescue.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.serverSnapshot.Active {
			var cmd tea.Cmd
			m.serverSnapshot, cmd = m.serverSnapshot.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
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
		if m.serverPicker.Active {
			var cmd tea.Cmd
			m.serverPicker, cmd = m.serverPicker.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.routerCreate.Active {
			var cmd tea.Cmd
			m.routerCreate, cmd = m.routerCreate.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.subnetPicker.Active {
			var cmd tea.Cmd
			m.subnetPicker, cmd = m.subnetPicker.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.networkCreate.Active {
			var cmd tea.Cmd
			m.networkCreate, cmd = m.networkCreate.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.subnetCreate.Active {
			var cmd tea.Cmd
			m.subnetCreate, cmd = m.subnetCreate.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.sgCreate.Active {
			var cmd tea.Cmd
			m.sgCreate, cmd = m.sgCreate.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.sgRuleCreate.Active {
			var cmd tea.Cmd
			m.sgRuleCreate, cmd = m.sgRuleCreate.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		if m.projectPicker.Active {
			var cmd tea.Cmd
			m.projectPicker, cmd = m.projectPicker.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		return m, viewCmd
	}
}

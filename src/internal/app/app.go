package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/selfupdate"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ssh"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/cloneprogress"
	"github.com/larkly/lazystack/internal/ui/cloudpicker"
	"github.com/larkly/lazystack/internal/ui/configview"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/consoleurl"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"github.com/larkly/lazystack/internal/ui/dnslist"
	"github.com/larkly/lazystack/internal/ui/fippicker"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/help"
	"github.com/larkly/lazystack/internal/ui/hypervisorlist"
	"github.com/larkly/lazystack/internal/ui/imagecreate"
	"github.com/larkly/lazystack/internal/ui/imagedownload"
	"github.com/larkly/lazystack/internal/ui/imageedit"
	"github.com/larkly/lazystack/internal/ui/imageview"
	"github.com/larkly/lazystack/internal/ui/keypaircreate"
	"github.com/larkly/lazystack/internal/ui/keypairdetail"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/lbcreate"
	"github.com/larkly/lazystack/internal/ui/lblistenercreate"
	"github.com/larkly/lazystack/internal/ui/lbmembercreate"
	"github.com/larkly/lazystack/internal/ui/lbmonitorcreate"
	"github.com/larkly/lazystack/internal/ui/lbpoolcreate"
	"github.com/larkly/lazystack/internal/ui/lbview"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/networkcreate"
	"github.com/larkly/lazystack/internal/ui/networkview"
	"github.com/larkly/lazystack/internal/ui/portcreate"
	"github.com/larkly/lazystack/internal/ui/portedit"
	"github.com/larkly/lazystack/internal/ui/projectpicker"
	"github.com/larkly/lazystack/internal/ui/quotaview"
	"github.com/larkly/lazystack/internal/ui/routercreate"
	"github.com/larkly/lazystack/internal/ui/routerview"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverdetail"
	"github.com/larkly/lazystack/internal/ui/serverlist"
	"github.com/larkly/lazystack/internal/ui/servicecatalog"
	"github.com/larkly/lazystack/internal/ui/serverpicker"
	"github.com/larkly/lazystack/internal/ui/serverrebuild"
	"github.com/larkly/lazystack/internal/ui/serverrename"
	"github.com/larkly/lazystack/internal/ui/serverresize"
	"github.com/larkly/lazystack/internal/ui/serversnapshot"
	"github.com/larkly/lazystack/internal/ui/sgcreate"
	"github.com/larkly/lazystack/internal/ui/sgrulecreate"
	"github.com/larkly/lazystack/internal/ui/sshprompt"
	"github.com/larkly/lazystack/internal/ui/statusbar"
	"github.com/larkly/lazystack/internal/ui/subnetcreate"
	"github.com/larkly/lazystack/internal/ui/subnetedit"
	"github.com/larkly/lazystack/internal/ui/subnetpicker"
	"github.com/larkly/lazystack/internal/ui/vmpassword"
	"github.com/larkly/lazystack/internal/ui/volumecreate"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
	"github.com/larkly/lazystack/internal/ui/volumelist"
	"github.com/larkly/lazystack/internal/ui/volumepicker"
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
	viewLBView
	viewVolumeCreate
	viewKeypairCreate
	viewNetworkList
	viewKeypairDetail
	viewRouterView
	viewImageView
	viewHypervisorList
	viewServiceCatalog
	viewDNSList
)

type modalType int

// UpdateAvailableMsg is sent when a newer version is found.
type UpdateAvailableMsg struct {
	Latest       string
	DownloadURL  string
	ChecksumsURL string
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
	view                activeView
	width               int
	height              int
	client              *cloud.Client
	cloudPicker         cloudpicker.Model
	copyPicker          copypicker.Model
	serverList          serverlist.Model
	serverDetail        serverdetail.Model
	serverCreate        servercreate.Model
	consoleLog          consolelog.Model
	actionLog           actionlog.Model
	serverRename        serverrename.Model
	serverRebuild       serverrebuild.Model
	serverSnapshot      serversnapshot.Model
	serverResize        serverresize.Model
	sshPrompt           sshprompt.Model
	consoleURL          consoleurl.Model
	vmPassword          vmpassword.Model
	fipPicker           fippicker.Model
	serverPicker        serverpicker.Model
	volumePicker        volumepicker.Model
	sgCreate            sgcreate.Model
	networkCreate       networkcreate.Model
	subnetCreate        subnetcreate.Model
	subnetEdit          subnetedit.Model
	portCreate          portcreate.Model
	portEdit            portedit.Model
	routerView          routerview.Model
	routerCreate        routercreate.Model
	subnetPicker        subnetpicker.Model
	sgRuleCreate        sgrulecreate.Model
	projectPicker       projectpicker.Model
	volumeList          volumelist.Model
	volumeDetail        volumedetail.Model
	volumeCreate        volumecreate.Model
	floatingIPList      floatingiplist.Model
	secGroupView        secgroupview.Model
	keypairList         keypairlist.Model
	keypairCreate       keypaircreate.Model
	keypairDetail       keypairdetail.Model
	imageView           imageview.Model
	imageEdit           imageedit.Model
	imageCreate         imagecreate.Model
	imageDownload       imagedownload.Model
	networkView         networkview.Model
	lbView              lbview.Model
	lbCreate            lbcreate.Model
	lbListenerCreate    lblistenercreate.Model
	lbPoolCreate        lbpoolcreate.Model
	lbMemberCreate      lbmembercreate.Model
	lbMonitorCreate     lbmonitorcreate.Model
	cloneProgress       cloneprogress.Model
	statusBar           statusbar.Model
	tabs                []TabDef
	activeTab           int
	tabInited           []bool
	help                help.Model
	quotaView           quotaview.Model
	configView          configview.Model
	confirm             modal.ConfirmModel
	hypervisorList      hypervisorlist.Model
	serviceCatalog      servicecatalog.Model
	dnsList             dnslist.Model
	errModal            modal.ErrorModel
	activeModal         modalType
	projects            []shared.ProjectInfo
	currentProjectID    string
	cloudName           string
	autoCloud           string
	previousView        activeView
	returnToView        activeView // for cross-resource navigation back-nav
	nav                 NavStack   // explicit navigation stack (H2)
	refreshInterval     time.Duration
	minWidth            int
	minHeight           int
	tooSmall            bool
	restart             bool
	version             string
	checkUpdate         bool
	idleTimeout         time.Duration
	lastActivity        time.Time
	idlePaused          bool
	latestVersion       string
	downloadURL         string
	checksumsURL        string
	updateCheckInterval time.Duration
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
	Plain           bool
	Config          *config.Config
}

// New creates the root model.
func New(opts Options) Model {
	shared.PlainMode = opts.Plain
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
			view:                viewCloudPicker,
			cloudPicker:         cloudpicker.New(clouds, nil),
			statusBar:           statusbar.New(opts.Version),
			help:                help.New(),
			quotaView:           quotaview.New(),
			configView:          configview.New(opts.Config),
			minWidth:            80,
			minHeight:           20,
			autoCloud:           autoName,
			refreshInterval:     refresh,
			idleTimeout:         opts.IdleTimeout,
			version:             opts.Version,
			checkUpdate:         opts.CheckUpdate,
			updateCheckInterval: time.Duration(opts.Config.General.UpdateCheckInterval) * time.Hour,
			tabs:                tabs,
			tabInited:           make([]bool, len(tabs)),
		}
	}

	cp := cloudpicker.New(clouds, err)
	return Model{
		view:                viewCloudPicker,
		cloudPicker:         cp,
		statusBar:           statusbar.New(opts.Version),
		help:                help.New(),
		quotaView:           quotaview.New(),
		configView:          configview.New(opts.Config),
		minWidth:            80,
		minHeight:           20,
		refreshInterval:     refresh,
		idleTimeout:         opts.IdleTimeout,
		version:             opts.Version,
		checkUpdate:         opts.CheckUpdate,
		updateCheckInterval: time.Duration(opts.Config.General.UpdateCheckInterval) * time.Hour,
		tabs:                tabs,
		tabInited:           make([]bool, len(tabs)),
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
		ttl := m.updateCheckInterval
		cmds = append(cmds, func() tea.Msg {
			latest, dlURL, csURL, err := selfupdate.CheckLatestCached(context.Background(), ver, ttl)
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
		m.setSizeAllModals(m.width, m.height)
		m.help.Width = m.width
		m.help.Height = m.height
		m.quotaView.Width = m.width
		m.quotaView.Height = m.height
		m.configView.Width = m.width
		m.configView.Height = m.height
		m.statusBar.Width = m.width
		return m.updateActiveView(msg)

	case tea.KeyMsg:
		m.lastActivity = time.Now()
		m.statusBar.StickyHint = ""
		if m.idlePaused {
			m.idlePaused = false
			m.statusBar.Hint = ""
			shared.Debugf("[app] resuming from idle, restarting tick")
			return m, m.refreshTickCmd()
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

		if m.configView.Visible {
			var cmd tea.Cmd
			m.configView, cmd = m.configView.Update(msg)
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

		if ok, cmd := m.updateAnyModal(msg); ok {
			return m, cmd
		}

		if m.view != viewServerCreate && m.view != viewVolumeCreate && m.view != viewKeypairCreate {
			// Server list filter mode: esc should clear filter before global back-nav/tab handlers.
			if m.view == viewServerList && m.serverList.IsFiltering() && (key.Matches(msg, shared.Keys.Back) || msg.String() == "esc") {
				return m.updateActiveView(msg)
			}

			switch {
			case key.Matches(msg, shared.Keys.Quit) && m.view != viewCloudPicker:
				return m, tea.Quit
			case key.Matches(msg, shared.Keys.Help):
				m.help.Open(m.viewName())
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
			case key.Matches(msg, shared.Keys.Config) && m.view != viewCloudPicker:
				m.configView.Width = m.width
				m.configView.Height = m.height
				m.configView.Open()
				return m, nil
			case key.Matches(msg, shared.Keys.Hypervisors) && m.view != viewCloudPicker:
				return m.openHypervisorList()
			case key.Matches(msg, shared.Keys.Browse) && m.view != viewCloudPicker:
				return m.openServiceCatalog()
			}

			// Back-navigation from cross-resource jump
			if m.isTopLevelView() && m.returnToView == viewServerDetail && key.Matches(msg, shared.Keys.Back) {
				if m.serverDetail.ServerID() != "" {
					m.returnToView = 0
					m.view = viewServerDetail
					m.statusBar.CurrentView = "serverdetail"
					m.statusBar.Hint = m.serverDetail.Hints()
					return m, nil
				}
			}

			// Tab switching (only from top-level list views)
			if m.isTopLevelView() {
				// Number keys 1-9 map to tab indices
				if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
					idx := int(s[0] - '1')
					if idx < len(m.tabs) {
						m.returnToView = 0 // clear cross-resource back-nav
						return m.switchTab(idx)
					}
				}
				switch {
				case key.Matches(msg, shared.Keys.Right):
					m.returnToView = 0
					next := (m.activeTab + 1) % len(m.tabs)
					return m.switchTab(next)
				case key.Matches(msg, shared.Keys.Left):
					m.returnToView = 0
					prev := (m.activeTab - 1 + len(m.tabs)) % len(m.tabs)
					return m.switchTab(prev)
				}
			}
		}

		// Global force refresh
		if key.Matches(msg, shared.Keys.Refresh) && m.view != viewCloudPicker {
			return m.forceRefreshActiveView()
		}

		// Global copy-field picker
		if key.Matches(msg, shared.Keys.Copy) && m.view != viewCloudPicker {
			return m.openCopyPicker()
		}

		if m.view == viewServerList || m.view == viewServerDetail {
			// Allow read-only actions and lock/unlock regardless of lock state
			if key.Matches(msg, shared.Keys.Lock) {
				return m.openToggleConfirm("lock/unlock")
			}
			if key.Matches(msg, shared.Keys.CopySSH) {
				return m.copySSHCommand()
			}
			if key.Matches(msg, shared.Keys.ConsoleURL) {
				return m.openConsoleURL()
			}
			if key.Matches(msg, shared.Keys.GetPassword) {
				return m.openVMPassword()
			}
			if key.Matches(msg, shared.Keys.Console) {
				return m.openConsoleLog()
			}
			if key.Matches(msg, shared.Keys.Actions) {
				return m.openActionLog()
			}

			// Volume detach from server detail volumes pane
			if m.view == viewServerDetail && m.serverDetail.FocusedOnVolumes() {
				if key.Matches(msg, shared.Keys.Detach) {
					if !m.blockStorageAvailable() {
						m.statusBar.StickyHint = "Block storage unavailable in this cloud"
						return m, nil
					}
					return m.openServerVolumeDetach()
				}
			}

			// Block all mutating actions on locked servers
			if m.isSelectedServerLocked() && key.Matches(msg,
				shared.Keys.Delete, shared.Keys.Reboot, shared.Keys.HardReboot,
				shared.Keys.Pause, shared.Keys.Suspend, shared.Keys.Shelve,
				shared.Keys.StopStart, shared.Keys.Rescue,
				shared.Keys.Resize, shared.Keys.Rename, shared.Keys.Rebuild,
				shared.Keys.Snapshot, shared.Keys.ConfirmResize, shared.Keys.RevertResize,
				shared.Keys.Attach, shared.Keys.Clone,
			) {
				m.statusBar.StickyHint = "Server is locked. Unlock it first with Ctrl+L."
				return m, nil
			}

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
			if key.Matches(msg, shared.Keys.Rescue) {
				return m.openToggleConfirm("rescue/unrescue")
			}
			if key.Matches(msg, shared.Keys.SSH) {
				return m.openSSH()
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
				if !m.blockStorageAvailable() {
					m.statusBar.StickyHint = "Block storage unavailable in this cloud"
					return m, nil
				}
				return m.openServerVolumeAttach()
			}
			if key.Matches(msg, shared.Keys.AssignFIP) {
				return m.doAllocateAndAssociateFIP()
			}
			if key.Matches(msg, shared.Keys.Clone) {
				return m.openClone()
			}
		}

		// Volume list: Enter to open detail, ctrl+d to delete, ctrl+n to create, ctrl+a attach, ctrl+t detach
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
			if key.Matches(msg, shared.Keys.Attach) {
				return m.openVolumeAttach()
			}
			if key.Matches(msg, shared.Keys.Detach) {
				return m.openVolumeDetach()
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

		// Security group view: context-sensitive actions based on focused pane
		if m.view == viewSecGroupView {
			pane := m.secGroupView.FocusedPane()
			switch {
			case key.Matches(msg, shared.Keys.Enter):
				if serverID := m.secGroupView.SelectedServerID(); serverID != "" {
					return m.handleDetailNavigation(shared.NavigateToDetailMsg{Resource: "server", ID: serverID})
				}
				if r := m.secGroupView.SelectedRule(); r != nil {
					return m.openSGRuleEdit()
				}
			case key.Matches(msg, shared.Keys.Delete):
				if pane == secgroupview.FocusRules && m.secGroupView.SelectedRuleID() != "" {
					return m.openSGRuleDeleteConfirm()
				}
				if (pane == secgroupview.FocusSelector || pane == secgroupview.FocusRules) && m.secGroupView.SelectedGroupName() != "default" {
					return m.openSGDeleteConfirm()
				}
			case key.Matches(msg, shared.Keys.Create):
				if pane == secgroupview.FocusRules {
					return m.openSGRuleCreate()
				}
				return m.openSGCreate()
			case key.Matches(msg, shared.Keys.Rename) && m.secGroupView.SelectedGroupName() != "default":
				return m.openSGRename()
			case key.Matches(msg, shared.Keys.Clone):
				return m.openSGClone()
			}
		}

		// Network view: context-sensitive create/delete for networks and subnets
		// When subnets pane focused: ctrl+n creates subnet, ctrl+d deletes subnet
		// Otherwise: ctrl+n creates network, ctrl+d deletes network
		if m.view == viewNetworkList {
			if key.Matches(msg, shared.Keys.Delete) {
				if m.networkView.InSubnets() {
					return m.openSubnetDeleteConfirm()
				}
				if m.networkView.InPorts() {
					return m.openPortDeleteConfirm()
				}
				return m.openNetworkDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				if m.networkView.InSubnets() {
					return m.openSubnetCreate()
				}
				if m.networkView.InPorts() {
					return m.openPortCreate()
				}
				return m.openNetworkCreate()
			}
			if key.Matches(msg, shared.Keys.Enter) && m.networkView.InSubnets() {
				return m.openSubnetEdit()
			}
			if key.Matches(msg, shared.Keys.Enter) && m.networkView.InPorts() {
				return m.openPortEdit()
			}
		}

		// Router view: context-sensitive actions based on focused pane
		if m.view == viewRouterView {
			if key.Matches(msg, shared.Keys.Delete) {
				return m.openRouterDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) {
				if m.routerView.InInterfaces() {
					return m.openAddRouterInterface()
				}
				return m.openRouterCreate()
			}
			if key.Matches(msg, shared.Keys.Attach) {
				return m.openAddRouterInterface()
			}
			if key.Matches(msg, shared.Keys.Detach) {
				return m.openRemoveRouterInterfaceConfirm()
			}
		}

		// Load balancer view: context-sensitive CRUD based on focused pane
		if m.view == viewLBView {
			// Block ALL mutation keys during PENDING state
			if lb := m.lbView.LB(); lb != nil && strings.HasPrefix(lb.ProvisioningStatus, "PENDING_") {
				if key.Matches(msg, shared.Keys.Delete) ||
					key.Matches(msg, shared.Keys.Create) ||
					key.Matches(msg, shared.Keys.Enter) ||
					key.Matches(msg, shared.Keys.StopStart) ||
					msg.String() == "ctrl+h" {
					m.statusBar.StickyHint = "Load balancer is " + lb.ProvisioningStatus + ", please wait..."
					return m, nil
				}
			}

			pane := m.lbView.FocusedPane()

			// Health monitor: ctrl+h on pools pane
			if msg.String() == "ctrl+h" && pane == lbview.FocusPools {
				if pool := m.lbView.SelectedPool(); pool != nil {
					if pool.MonitorID != "" {
						return m.openLBMonitorEdit()
					}
					return m.openLBMonitorCreate()
				}
			}

			// Admin state toggle: 'o' key
			if key.Matches(msg, shared.Keys.StopStart) {
				switch pane {
				case lbview.FocusInfo:
					return m.toggleLBAdminState()
				case lbview.FocusListeners:
					return m.toggleListenerAdminState()
				case lbview.FocusPools:
					return m.togglePoolAdminState()
				case lbview.FocusMembers:
					return m.toggleMemberAdminState()
				}
			}

			// Bulk member selection: space toggles, x toggles all
			if key.Matches(msg, shared.Keys.Select) && pane == lbview.FocusMembers {
				m.lbView.ToggleMemberSelection()
				return m, nil
			}
			if msg.String() == "x" && pane == lbview.FocusMembers {
				m.lbView.ToggleAllMemberSelection()
				return m, nil
			}

			// Drain member: 'w' key on members pane
			if msg.String() == "w" && pane == lbview.FocusMembers {
				if m.lbView.SelectedMemberCount() > 0 {
					return m.drainLBMembersBulk()
				}
				if mem := m.lbView.SelectedMember(); mem != nil && mem.Weight > 0 {
					return m.drainLBMember()
				}
			}

			switch {
			case key.Matches(msg, shared.Keys.Delete):
				switch pane {
				case lbview.FocusSelector:
					return m.openLBDeleteConfirm()
				case lbview.FocusListeners:
					if m.lbView.SelectedListenerID() != "" {
						return m.openLBListenerDeleteConfirm()
					}
					return m.openLBDeleteConfirm()
				case lbview.FocusPools:
					if pool := m.lbView.SelectedPool(); pool != nil && pool.MonitorID != "" {
						return m.openLBMonitorDeleteConfirm()
					}
					if m.lbView.SelectedPoolID() != "" {
						return m.openLBPoolDeleteConfirm()
					}
					return m.openLBDeleteConfirm()
				case lbview.FocusMembers:
					if m.lbView.SelectedMemberCount() > 0 {
						return m.openLBBulkMemberDeleteConfirm()
					}
					if m.lbView.SelectedMemberID() != "" {
						return m.openLBMemberDeleteConfirm()
					}
				default:
					return m.openLBDeleteConfirm()
				}
			case key.Matches(msg, shared.Keys.Create):
				switch pane {
				case lbview.FocusSelector:
					return m.openLBCreate()
				case lbview.FocusListeners:
					return m.openLBListenerCreate()
				case lbview.FocusPools:
					return m.openLBPoolCreate()
				case lbview.FocusMembers:
					if m.lbView.SelectedPoolID() != "" {
						return m.openLBMemberCreate()
					}
				}
			case key.Matches(msg, shared.Keys.Enter):
				switch pane {
				case lbview.FocusInfo:
					return m.openLBEdit()
				case lbview.FocusListeners:
					if m.lbView.SelectedListenerID() != "" {
						return m.openLBListenerEdit()
					}
				case lbview.FocusPools:
					if pool := m.lbView.SelectedPool(); pool != nil {
						return m.openLBPoolEdit()
					}
				case lbview.FocusMembers:
					if m.lbView.SelectedMemberID() != "" {
						return m.openLBMemberEdit()
					}
				}
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

		// Image view: context-sensitive actions based on focused pane
		if m.view == viewImageView {
			pane := m.imageView.FocusedPane()

			if key.Matches(msg, shared.Keys.Delete) {
				return m.openImageDeleteConfirm()
			}
			if key.Matches(msg, shared.Keys.Deactivate) && (pane == imageview.FocusSelector || pane == imageview.FocusInfo) {
				return m.openImageDeactivateConfirm()
			}
			if key.Matches(msg, shared.Keys.Create) && pane == imageview.FocusSelector {
				return m.openImageUpload()
			}
			if msg.String() == "ctrl+g" && pane == imageview.FocusProperties {
				return m.openImageDownload()
			}
			if key.Matches(msg, shared.Keys.Enter) {
				switch pane {
				case imageview.FocusInfo:
					return m.openImageEdit()
				case imageview.FocusServers:
					if serverID := m.imageView.SelectedServerID(); serverID != "" {
						return m.handleDetailNavigation(shared.NavigateToDetailMsg{Resource: "server", ID: serverID})
					}
				}
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
			DNS:            msg.DNSClient,
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
		if msg.DNSClient != nil {
			m.tabs = append(m.tabs, TabDef{Name: "DNS", Key: "dns"})
		}
		m.tabs = append(m.tabs, TabDef{Name: "Key Pairs", Key: "keypairs"})
		m.tabInited = make([]bool, len(m.tabs))
		m.activeTab = 0
		m.statusBar.CloudName = m.cloudName
		m.statusBar.Region = msg.Region
		m.quotaView.SetClients(msg.ComputeClient, msg.NetworkClient, msg.BlockStorageClient, "")
		m.serverList = serverlist.New(msg.ComputeClient, msg.ImageClient, m.refreshInterval)
		m.serverList.SetConfig(m.configView.Cfg())
		m.serverList.SetSize(m.width, m.height)
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		cmds := []tea.Cmd{m.serverList.Init(), m.refreshTickCmd()}
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
				// Extract current project ID from the auth token scope
				currentID := ""
				if ar, ok := pc.GetAuthResult().(interface {
					ExtractProject() (*tokens.Project, error)
				}); ok {
					if proj, err := ar.ExtractProject(); err == nil && proj != nil {
						currentID = proj.ID
					}
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
		if msg.ChecksumsURL == "" {
			m.statusBar.StickyHint = fmt.Sprintf("update %s skipped: checksums unavailable", msg.Latest)
		} else {
			m.statusBar.Hint = fmt.Sprintf("Upgrade available: %s", msg.Latest)
		}
		return m, nil

	case shared.ConfigChangedMsg:
		m.refreshInterval = time.Duration(m.configView.Cfg().General.RefreshInterval) * time.Second
		m.idleTimeout = time.Duration(m.configView.Cfg().General.IdleTimeout) * time.Minute
		return m, nil

	case shared.ViewChangeMsg:
		return m.handleViewChange(msg)

	case shared.NavigateToResourceMsg:
		return m.handleResourceNavigation(msg)

	case shared.NavigateToDetailMsg:
		return m.handleDetailNavigation(msg)

	case modal.ConfirmAction:
		m.activeModal = modalNone
		if msg.Confirm {
			return m.executeAction(msg)
		}
		return m, nil

	case modal.ErrorDismissedMsg:
		m.activeModal = modalNone
		return m, nil

	case copypicker.ChosenMsg:
		return m.copyToClipboard(msg.Label, msg.Value), nil

	case copypicker.CancelledMsg:
		return m, nil

	case sshprompt.SSHConnectMsg:
		m.sshPrompt.Active = false
		args := ssh.BuildArgs(ssh.Options{
			User:           msg.User,
			IP:             msg.IP,
			KeyPath:        msg.KeyPath,
			Debug:          msg.Debug,
			IgnoreHostKeys: msg.IgnoreHostKeys,
		})
		c := exec.Command("ssh", args...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return shared.SSHFinishedMsg{Err: err}
		})

	case shared.SSHFinishedMsg:
		if msg.Err != nil {
			m.statusBar.StickyHint = "SSH error: " + msg.Err.Error()
		}
		return m, nil

	case shared.ConsoleURLMsg:
		m.statusBar.StickyHint = ""
		m.consoleURL = consoleurl.New(msg.URL, msg.ServerName)
		m.consoleURL.SetSize(m.width, m.height)
		return m, nil

	case shared.ConsoleURLErrMsg:
		m.statusBar.StickyHint = "Console URL error: " + msg.Err.Error()
		return m, nil

	case shared.VMPasswordMsg:
		m.statusBar.StickyHint = ""
		m.vmPassword = vmpassword.New(msg.ServerName, msg.KeyName, msg.KeyPath, msg.Plain, msg.Encrypted, msg.Note)
		m.vmPassword.SetSize(m.width, m.height)
		return m, nil

	case shared.VMPasswordErrMsg:
		m.statusBar.StickyHint = "Password error: " + msg.Err.Error()
		return m, nil

	case shared.ServerActionMsg:
		m.statusBar.StickyHint = fmt.Sprintf("✓ %s %s", msg.Action, msg.Name)
		m.statusBar.Error = ""
		// Ensure resize modal is dismissed
		m.serverResize.Active = false
		// Navigate back to server list if on a sub-view, or after delete
		if m.view == viewConsoleLog || (m.view == viewServerDetail && msg.Action == "Delete") {
			m.returnToView = 0
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
		m.returnToView = 0
		if m.view == viewVolumeDetail {
			m.view = viewVolumeList
			m.statusBar.CurrentView = "volumelist"
		}
		if m.view == viewKeypairDetail {
			m.view = viewKeypairList
			m.statusBar.CurrentView = "keypairlist"
		}
		if m.view == viewLBView {
			m.statusBar.CurrentView = "lbview"
			return m, m.lbView.ForceRefresh()
		}
		if m.view == viewImageView {
			m.statusBar.CurrentView = "imageview"
			return m, m.imageView.ForceRefresh()
		}
		if m.view == viewSecGroupView {
			return m, m.secGroupView.ForceRefresh()
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

	case servercreate.ServerCloneCreatedMsg:
		// Server created in clone mode with volume cloning — start volume clone progress
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"

		// Build volume operations — names will be resolved async in Init
		var ops []cloneprogress.VolumeOp
		for _, vid := range msg.VolumeIDs {
			ops = append(ops, cloneprogress.VolumeOp{
				SourceVolID: vid,
				SourceName:  vid,
				CloneName:   vid, // placeholder, resolved in Init
				Status:      "pending",
			})
		}

		m.cloneProgress = cloneprogress.New(m.client.Compute, m.client.BlockStorage, msg.Server.ID, msg.Server.Name, ops)
		m.cloneProgress.SetSize(m.width, m.height)
		return m, tea.Batch(
			m.cloneProgress.Init(),
			func() tea.Msg { return shared.RefreshServersMsg{} },
		)

	case cloneprogress.AllCompleteMsg:
		m.cloneProgress.Active = false
		m.statusBar.StickyHint = fmt.Sprintf("✓ Clone complete — all volumes attached to %s", m.cloneProgress.ServerName())
		return m, nil

	case cloneprogress.RollbackCompleteMsg:
		m.cloneProgress.Active = false
		m.errModal = modal.NewError("Clone Failed", msg.Cause)
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
				shared.Debugf("[app] TickMsg: swallowed (idle paused)")
				return m, nil
			}
			if m.idleTimeout > 0 && !m.lastActivity.IsZero() && time.Since(m.lastActivity) > m.idleTimeout {
				m.idlePaused = true
				m.statusBar.Hint = "⏸ Paused — press any key to resume"
				shared.Debugf("[app] TickMsg: pausing due to idle timeout")
				return m, nil
			}
		}
		// For tick messages, dispatch to views then chain the next tick.
		// This is the ONLY place tick chaining happens — views don't manage ticks.
		if _, isTick := msg.(shared.TickMsg); isTick {
			shared.Debugf("[app] TickMsg: dispatching to views and chaining next tick")
			m2, viewCmd := m.updateAllViews(msg)
			m = m2
			return m, tea.Batch(viewCmd, m.refreshTickCmd())
		}

		// Route to all views first so background messages keep flowing
		m2, viewCmd := m.updateAllViews(msg)
		m = m2
		// Route to quota view for spinner/loaded messages
		if m.quotaView.Visible {
			var cmd tea.Cmd
			m.quotaView, cmd = m.quotaView.Update(msg)
			return m, tea.Batch(viewCmd, cmd)
		}
		// Route to any active modals for background messages
		if modalCmd := m.updateAnyModalBackground(msg); modalCmd != nil {
			return m, tea.Batch(viewCmd, modalCmd)
		}
		return m, viewCmd
	}
}

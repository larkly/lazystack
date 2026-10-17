package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/cloud"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ssh"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/auditlog"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/fippicker"
	"github.com/larkly/lazystack/internal/ui/hypervisorlist"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/serveradminact"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/servermetadata"
	"github.com/larkly/lazystack/internal/ui/serverrebuild"
	"github.com/larkly/lazystack/internal/ui/serverrename"
	"github.com/larkly/lazystack/internal/ui/serverresize"
	"github.com/larkly/lazystack/internal/ui/serversnapshot"
	"github.com/larkly/lazystack/internal/ui/servicecatalog"
	"github.com/larkly/lazystack/internal/ui/sshprompt"
	"github.com/larkly/lazystack/internal/ui/usermanagement"
	"github.com/larkly/lazystack/internal/ui/vmpassword"
	"github.com/larkly/lazystack/internal/volume"
)

func resolveToggleAction(toggleAction, status string, locked bool) string {
	switch toggleAction {
	case "pause/unpause":
		if status == "PAUSED" {
			return "unpause"
		}
		return "pause"
	case "suspend/resume":
		if status == "SUSPENDED" {
			return "resume"
		}
		return "suspend"
	case "shelve/unshelve":
		if status == "SHELVED" || status == "SHELVED_OFFLOADED" {
			return "unshelve"
		}
		return "shelve"
	case "stop/start":
		if status == "SHUTOFF" {
			return "start"
		}
		return "stop"
	case "lock/unlock":
		if locked {
			return "unlock"
		}
		return "lock"
	case "rescue/unrescue":
		if status == "RESCUE" {
			return "unrescue"
		}
		return "rescue"
	default:
		return toggleAction
	}
}

func countBulkActions(servers []modal.ServerRef, fallbackAction string) map[string]int {
	counts := make(map[string]int)
	for _, s := range servers {
		act := s.Action
		if act == "" {
			act = fallbackAction
		}
		counts[act]++
	}
	return counts
}

func formatActionCounts(counts map[string]int) string {
	ordered := []string{"start", "stop", "pause", "unpause", "suspend", "resume", "shelve", "unshelve", "lock", "unlock", "rescue", "unrescue"}
	seen := make(map[string]bool, len(counts))
	var parts []string
	for _, act := range ordered {
		if n, ok := counts[act]; ok {
			parts = append(parts, fmt.Sprintf("%s:%d", act, n))
			seen[act] = true
		}
	}
	var extra []string
	for act := range counts {
		if !seen[act] {
			extra = append(extra, act)
		}
	}
	sort.Strings(extra)
	for _, act := range extra {
		parts = append(parts, fmt.Sprintf("%s:%d", act, counts[act]))
	}
	return strings.Join(parts, ", ")
}

func (m Model) isSelectedServerLocked() bool {
	if m.view == viewServerList && m.serverList.SelectionCount() > 0 {
		for _, s := range m.serverList.SelectedServers() {
			if s.Locked {
				return true
			}
		}
		return false
	}
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			return s.Locked
		}
	case viewServerDetail:
		return m.serverDetail.ServerLocked()
	}
	return false
}

func (m Model) openClone() (Model, tea.Cmd) {
	var srv *compute.Server
	switch m.view {
	case viewServerList:
		srv = m.serverList.SelectedServer()
	case viewServerDetail:
		srv = m.serverDetail.Server()
	}
	if srv == nil {
		return m, nil
	}

	// Deduplicate server name using existing server list
	cloneName := shared.DeduplicateName(srv.Name, m.serverList.ServerNames())

	cfg := servercreate.CloneConfig{
		SourceName:    cloneName,
		ImageID:       srv.ImageID,
		FlavorID:      srv.FlavorID,
		FlavorName:    srv.FlavorName,
		KeyName:       srv.KeyName,
		SecGroupNames: srv.SecGroups,
		NetworkNames:  srv.Networks,
		VolumeIDs:     compute.VolumeAttachmentIDs(srv.VolAttach),
	}

	m.serverCreate = servercreate.NewClone(m.client.Compute, m.client.Image, m.client.Network, cfg)
	m.serverCreate.SetSize(m.width, m.height)
	m.view = viewServerCreate
	m.statusBar.CurrentView = "servercreate"
	m.statusBar.Hint = m.serverCreate.Hints()
	return m, m.serverCreate.Init()
}

func (m Model) openRename() (Model, tea.Cmd) {
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
	m.serverRename = serverrename.New(m.client.Compute, id, name)
	m.serverRename.SetSize(m.width, m.height)
	return m, m.serverRename.Init()
}

func (m Model) openSnapshot() (Model, tea.Cmd) {
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
	m.serverSnapshot = serversnapshot.New(m.client.Compute, id, name)
	m.serverSnapshot.SetSize(m.width, m.height)
	return m, m.serverSnapshot.Init()
}

func (m Model) openRebuild() (Model, tea.Cmd) {
	var id, name, imageID string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name, imageID = s.ID, s.Name, s.ImageID
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
		imageID = m.serverDetail.ServerImageID()
	}
	if id == "" {
		return m, nil
	}
	m.serverRebuild = serverrebuild.New(m.client.Compute, m.client.Image, id, name, imageID)
	m.serverRebuild.SetSize(m.width, m.height)
	return m, m.serverRebuild.Init()
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
	var srv *compute.Server
	switch m.view {
	case viewServerList:
		srv = m.serverList.SelectedServer()
	case viewServerDetail:
		srv = m.serverDetail.Server()
	}
	if srv == nil {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete", srv.ID, srv.Name)
	m.confirm.VolumeIDs = compute.VolumeAttachmentIDs(srv.VolAttach)
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
		refs := make([]modal.ServerRef, len(servers))
		for i, s := range servers {
			refs[i] = modal.ServerRef{
				ID:     s.ID,
				Name:   s.Name,
				Action: resolveToggleAction(action, s.Status, s.Locked),
			}
		}
		counts := countBulkActions(refs, action)
		bulkAction := action
		if len(refs) > 0 {
			bulkAction = refs[0].Action
		}
		m.confirm = modal.NewBulkConfirm(bulkAction, refs)
		if len(counts) > 1 {
			summary := formatActionCounts(counts)
			m.confirm.Title = fmt.Sprintf("Confirm mixed %s", action)
			m.confirm.Body = fmt.Sprintf("Apply %s to %d servers (%s): %s", action, len(refs), summary, modal.ServerNames(refs))
		}
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

	var locked bool
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			locked = s.Locked
		}
	case viewServerDetail:
		locked = m.serverDetail.ServerLocked()
	}
	actualAction := resolveToggleAction(action, status, locked)

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
	m.pushNav(m.view, m.activeTab)
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
	m.pushNav(m.view, m.activeTab)
	m.view = viewActionLog
	m.statusBar.CurrentView = "actionlog"
	m.statusBar.Hint = m.actionLog.Hints()
	return m, m.actionLog.Init()
}

func (m Model) openAuditLog() (Model, tea.Cmd) {
	m.auditLog = auditlog.New()
	m.auditLog.SetSize(m.width, m.height)
	m.nav.Push(m.view, m.activeTab)
	m.view = viewAuditLog
	m.statusBar.CurrentView = "auditlog"
	m.statusBar.Hint = m.auditLog.Hints()
	return m, tea.Batch(m.auditLog.Init(), m.openAuditLogCmd())
}

func (m Model) openAuditLogCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := audit.ReadEntries(audit.DefaultPath(), 500)
		if err != nil {
			return auditLogLoadedMsg{err: err}
		}
		return auditLogLoadedMsg{entries: entries}
	}
}

type auditLogLoadedMsg struct {
	entries []audit.Entry
	err     error
}

func (m Model) openHypervisorList() (Model, tea.Cmd) {
	m.hypervisorList = hypervisorlist.New(m.client.Compute)
	m.hypervisorList.SetSize(m.width, m.height)
	m.nav.Push(m.view, m.activeTab)
	m.view = viewHypervisorList
	m.statusBar.CurrentView = "hypervisorlist"
	m.statusBar.Hint = m.hypervisorList.Hints()
	return m, m.hypervisorList.Init()
}

func (m Model) openServiceCatalog() (Model, tea.Cmd) {
	m.serviceCatalog = servicecatalog.New(m.client.ProviderClient, m.client.EndpointOpts)
	m.serviceCatalog.SetSize(m.width, m.height)
	m.nav.Push(m.view, m.activeTab)
	m.view = viewServiceCatalog
	m.statusBar.CurrentView = "servicecatalog"
	m.statusBar.Hint = m.serviceCatalog.Hints()
	return m, m.serviceCatalog.Init()
}

func (m Model) openUserManagement() (Model, tea.Cmd) {
	m.userManagement = usermanagement.New(m.client.ProviderClient, m.client.EndpointOpts)
	m.userManagement.SetSize(m.width, m.height)
	m.nav.Push(m.view, m.activeTab)
	m.view = viewUserManagement
	m.statusBar.CurrentView = "usermanagement"
	m.statusBar.Hint = m.userManagement.Hints()
	return m, m.userManagement.Init()
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
		names := make(map[string]string, len(servers))
		for _, s := range servers {
			names[s.ID] = s.Name
		}
		m.serverResize = serverresize.NewBulk(m.client.Compute, ids, currentFlavor)
		m.serverResize.Track = m.resizeTracker(names)
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
	m.serverResize.Track = m.resizeTracker(map[string]string{id: name})
	m.serverResize.SetSize(m.width, m.height)
	return m, m.serverResize.Init()
}

// resizeTracker makes the resize modal's submission honor the per-server
// in-flight locks: a resize of a busy server is refused, and an accepted
// one holds its servers' locks until the request finishes. The modal calls
// it synchronously from Update, so the shared action state is only ever
// touched from the Update goroutine.
func (m Model) resizeTracker(names map[string]string) func([]string, tea.Cmd) (tea.Cmd, string) {
	m.ensureActions()
	return func(ids []string, cmd tea.Cmd) (tea.Cmd, string) {
		locks := make([]actionLock, 0, len(ids))
		for _, id := range ids {
			locks = append(locks, m.lock("server", id, names[id]))
		}
		if busy := m.busy(locks); busy != "" {
			return nil, fmt.Sprintf("%s already has an operation in progress; wait for it to finish", busy)
		}
		_, tracked, _ := m.trackAction(locks, cmd)
		return tracked, ""
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

func (m Model) executeAction(action modal.ConfirmAction) (Model, tea.Cmd) {
	client := m.client.Compute

	// Resource-specific bulk actions (must come before the generic server bulk guard)
	switch action.Action {
	case "delete_volumes_bulk":
		return m, m.executeDeleteVolumesBulk(action.Servers)
	case "detach_volumes_bulk":
		return m, m.executeDetachVolumesBulk(action.Servers)
	case "delete_images_bulk":
		return m, m.executeDeleteImagesBulk(action.Servers)
	case "delete_lb_members_bulk":
		m.lbView.ClearMemberSelection()
		return m, m.executeDeleteLBMembersBulk(action)
	}

	// Bulk actions (server-only)
	if len(action.Servers) > 0 {
		m.serverList.ClearSelection()
		return m, m.executeBulkAction(client, action)
	}

	if op, ok := serverOps[action.Action]; ok {
		name := action.Name
		if op.nameIsID {
			name = action.ServerID
		}
		return m, m.serverOpCmd(op, action.ServerID, name)
	}

	switch action.Action {
	case "delete":
		return m, m.deleteServerCmd(action)
	case "rescue":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] rescuing server %s", action.Name)
			adminPass, err := compute.RescueServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] rescue server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionRescue, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Rescue", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] rescued server %s", action.Name)
			m.logAudit(audit.ActionRescue, "server", action.ServerID, action.Name, "success", "")
			msg := shared.ServerActionMsg{Action: "Rescue", Name: action.Name}
			if adminPass == "" {
				return msg
			}
			return credentialsMsg{
				result: msg,
				title:  "Rescue Password",
				note:   "Temporary rescue-mode password; it is not stored anywhere.",
				creds:  []vmpassword.Credential{{Server: action.Name, Secret: adminPass}},
			}
		}
	case "detach_volume":
		computeC := m.client.Compute
		volID := action.ServerID
		name := action.Name
		bsClient := m.client.BlockStorage
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] detaching volume %s", name)
			vol, err := volume.GetVolume(ctx, bsClient, volID)
			if err != nil {
				shared.Debugf("[action] detach volume %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: err}
			}
			if !vol.IsAttached() {
				shared.Debugf("[action] detach volume %s failed: volume is not attached", name)
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: fmt.Errorf("volume is not attached")}
			}
			var detachErrs []error
			for _, att := range vol.Attachments {
				err = volume.DetachVolume(ctx, computeC, att.ServerID, volID)
				if err != nil {
					shared.Debugf("[action] detach volume %s from server %s failed: %s", name, att.ServerID, err)
					detachErrs = append(detachErrs, fmt.Errorf("server %s: %w", att.ServerID, err))
				}
			}
			if len(detachErrs) > 0 {
				shared.Debugf("[action] detach volume %s partially failed: %v", name, detachErrs)
				m.logAudit(audit.ActionDetachVolume, "volume", volID, name, "error", fmt.Sprintf("detach errors: %v", detachErrs))
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: fmt.Errorf("detach errors: %v", detachErrs)}
			}
			shared.Debugf("[action] detached volume %s", name)
			m.logAudit(audit.ActionDetachVolume, "volume", volID, name, "success", "")
			return shared.ResourceActionMsg{Action: "Detached volume", Name: name}
		}
	case "remove_router_interface":
		target, ok := decodeRouterInterfaceTarget(action.ServerID)
		if !ok {
			return m, nil
		}
		return m, m.removeRouterInterfaceCmd(target, action.Name)
	case "delete_lb_member":
		lbClient := m.client.LoadBalancer
		name := action.Name
		// ServerID encodes "poolID|memberID" captured at confirm time
		parts := strings.SplitN(action.ServerID, "|", 2)
		if len(parts) != 2 {
			return m, nil
		}
		poolID, memberID := parts[0], parts[1]
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			err := loadbalancer.DeleteMember(ctx, lbClient, poolID, memberID)
			if err != nil {
				m.logAudit(audit.ActionDeleteLB, "lb_member", memberID, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete member", Name: name, Err: err}
			}
			m.logAudit(audit.ActionDeleteLB, "lb_member", memberID, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted member", Name: name}
		}
	case "deactivate_image":
		return m, m.doDeactivateImage(action.ServerID, action.Name)
	case "reactivate_image":
		return m, m.doReactivateImage(action.ServerID, action.Name)
	}
	return m, nil
}

// serverOp is a single-resource confirm action that makes one service call,
// writes one audit record and reports the outcome. Actions with extra
// results (rescue's password, detach's per-attachment errors, LB members'
// composite IDs) keep dedicated code in executeAction.
type serverOp struct {
	// svc picks the service client run is called with.
	svc func(*cloud.Client) *gophercloud.ServiceClient
	run func(ctx context.Context, sc *gophercloud.ServiceClient, id string) error
	// audit and resource are the audit record's action and resource type.
	audit    audit.ActionType
	resource string
	// label names the action in result messages. Server lifecycle ops use
	// it for ServerActionMsg and ServerActionErrMsg alike.
	label string
	// done, when set, makes this a resource op: failures report a
	// ResourceActionErrMsg with label, success a ResourceActionMsg with done.
	done string
	// nameIsID marks resources identified by name (keypairs): the confirm
	// action carries the name in ServerID.
	nameIsID bool
}

func computeSvc(c *cloud.Client) *gophercloud.ServiceClient { return c.Compute }
func networkSvc(c *cloud.Client) *gophercloud.ServiceClient { return c.Network }
func volumeSvc(c *cloud.Client) *gophercloud.ServiceClient  { return c.BlockStorage }
func lbSvc(c *cloud.Client) *gophercloud.ServiceClient      { return c.LoadBalancer }
func imageSvc(c *cloud.Client) *gophercloud.ServiceClient   { return c.Image }

func rebootServer(how servers.RebootMethod) func(context.Context, *gophercloud.ServiceClient, string) error {
	return func(ctx context.Context, sc *gophercloud.ServiceClient, id string) error {
		return compute.RebootServer(ctx, sc, id, how)
	}
}

// deleteRouter explains the 409 Neutron returns while a router still has
// interfaces. Not wrapped with %w: the generic 409 text ("resource is busy")
// would otherwise replace this actionable message in the modal.
func deleteRouter(ctx context.Context, sc *gophercloud.ServiceClient, id string) error {
	err := network.DeleteRouter(ctx, sc, id)
	if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
		err = fmt.Errorf("router still in use; remove its interfaces first (Interfaces pane, %s): %v", shared.Keys.Detach.Help().Key, err)
	}
	return err
}

// serverOps maps confirm action names to their single-call operation.
var serverOps = map[string]serverOp{
	"soft reboot": {svc: computeSvc, run: rebootServer(servers.SoftReboot), audit: audit.ActionReboot, resource: "server", label: "Reboot"},
	"hard reboot": {svc: computeSvc, run: rebootServer(servers.HardReboot), audit: audit.ActionReboot, resource: "server", label: "Hard reboot"},
	"pause":       {svc: computeSvc, run: compute.PauseServer, audit: audit.ActionPause, resource: "server", label: "Pause"},
	"unpause":     {svc: computeSvc, run: compute.UnpauseServer, audit: audit.ActionUnpause, resource: "server", label: "Unpause"},
	"suspend":     {svc: computeSvc, run: compute.SuspendServer, audit: audit.ActionSuspend, resource: "server", label: "Suspend"},
	"resume":      {svc: computeSvc, run: compute.ResumeServer, audit: audit.ActionResume, resource: "server", label: "Resume"},
	"shelve":      {svc: computeSvc, run: compute.ShelveServer, audit: audit.ActionShelve, resource: "server", label: "Shelve"},
	"unshelve":    {svc: computeSvc, run: compute.UnshelveServer, audit: audit.ActionUnshelve, resource: "server", label: "Unshelve"},
	"stop":        {svc: computeSvc, run: compute.StopServer, audit: audit.ActionStop, resource: "server", label: "Stop"},
	"start":       {svc: computeSvc, run: compute.StartServer, audit: audit.ActionStart, resource: "server", label: "Start"},
	"lock":        {svc: computeSvc, run: compute.LockServer, audit: audit.ActionLock, resource: "server", label: "Lock"},
	"unlock":      {svc: computeSvc, run: compute.UnlockServer, audit: audit.ActionUnlock, resource: "server", label: "Unlock"},
	"unrescue":    {svc: computeSvc, run: compute.UnrescueServer, audit: audit.ActionUnrescue, resource: "server", label: "Unrescue"},

	"delete_volume":      {svc: volumeSvc, run: volume.DeleteVolume, audit: audit.ActionDelete, resource: "volume", label: "Delete volume", done: "Deleted volume"},
	"release_fip":        {svc: networkSvc, run: network.ReleaseFloatingIP, audit: audit.ActionReleaseFIP, resource: "floating_ip", label: "Release FIP", done: "Released"},
	"disassociate_fip":   {svc: networkSvc, run: network.DisassociateFloatingIP, audit: audit.ActionDetachFIP, resource: "floating_ip", label: "Disassociate FIP", done: "Disassociated"},
	"delete_router":      {svc: networkSvc, run: deleteRouter, audit: audit.ActionDeleteRouter, resource: "router", label: "Delete router", done: "Deleted router"},
	"delete_port":        {svc: networkSvc, run: network.DeletePort, audit: audit.ActionDeletePort, resource: "port", label: "Delete port", done: "Deleted port"},
	"delete_network":     {svc: networkSvc, run: network.DeleteNetwork, audit: audit.ActionDeleteNet, resource: "network", label: "Delete network", done: "Deleted network"},
	"delete_subnet":      {svc: networkSvc, run: network.DeleteSubnet, audit: audit.ActionDeleteSubnet, resource: "subnet", label: "Delete subnet", done: "Deleted subnet"},
	"delete_sg":          {svc: networkSvc, run: network.DeleteSecurityGroup, audit: audit.ActionDeleteNet, resource: "security_group", label: "Delete security group", done: "Deleted security group"},
	"delete_sg_rule":     {svc: networkSvc, run: network.DeleteSecurityGroupRule, audit: audit.ActionDeleteNet, resource: "security_group_rule", label: "Delete rule", done: "Deleted rule from"},
	"delete_lb":          {svc: lbSvc, run: loadbalancer.DeleteLoadBalancer, audit: audit.ActionDeleteLB, resource: "load_balancer", label: "Delete LB", done: "Deleted LB"},
	"delete_lb_listener": {svc: lbSvc, run: loadbalancer.DeleteListener, audit: audit.ActionDeleteLB, resource: "lb_listener", label: "Delete listener", done: "Deleted listener"},
	"delete_lb_pool":     {svc: lbSvc, run: loadbalancer.DeletePool, audit: audit.ActionDeleteLB, resource: "lb_pool", label: "Delete pool", done: "Deleted pool"},
	"delete_lb_monitor":  {svc: lbSvc, run: loadbalancer.DeleteHealthMonitor, audit: audit.ActionDeleteLB, resource: "lb_monitor", label: "Delete monitor", done: "Removed monitor from"},
	"delete_keypair":     {svc: computeSvc, run: compute.DeleteKeyPair, audit: audit.ActionDeleteKey, resource: "keypair", label: "Delete keypair", done: "Deleted keypair", nameIsID: true},
	"delete_image":       {svc: imageSvc, run: image.DeleteImage, audit: audit.ActionDeleteImage, resource: "image", label: "Delete image", done: "Deleted image"},
}

// serverOpCmd runs op against the resource id, audits the outcome under
// name and reports it.
func (m Model) serverOpCmd(op serverOp, id, name string) tea.Cmd {
	sc := op.svc(m.client)
	return func() tea.Msg {
		ctx, cancel := actionCtx()
		defer cancel()
		shared.Debugf("[action] %s %s %s: start", op.label, op.resource, name)
		if err := op.run(ctx, sc, id); err != nil {
			shared.Debugf("[action] %s %s %s failed: %s", op.label, op.resource, name, err)
			m.logAudit(op.audit, op.resource, id, name, "error", err.Error())
			if op.done == "" {
				return shared.ServerActionErrMsg{Action: op.label, Name: name, Err: err}
			}
			return shared.ResourceActionErrMsg{Action: op.label, Name: name, Err: err}
		}
		shared.Debugf("[action] %s %s %s: done", op.label, op.resource, name)
		m.logAudit(op.audit, op.resource, id, name, "success", "")
		if op.done == "" {
			return shared.ServerActionMsg{Action: op.label, Name: name}
		}
		return shared.ResourceActionMsg{Action: op.done, Name: name}
	}
}

func (m Model) getServerSSHInfo() (name, keyName string, floatingIPs, ipv6, ipv4 []string) {
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			return s.Name, s.KeyName, s.FloatingIP, s.IPv6, s.IPv4
		}
	case viewServerDetail:
		return m.serverDetail.ServerName(), m.serverDetail.ServerKeyName(),
			m.serverDetail.ServerFloatingIPs(), m.serverDetail.ServerIPv6(), m.serverDetail.ServerIPv4()
	}
	return "", "", nil, nil, nil
}

func (m Model) openSSH() (Model, tea.Cmd) {
	name, keyName, floatingIPs, ipv6, ipv4 := m.getServerSSHInfo()
	if name == "" {
		return m, nil
	}
	if len(floatingIPs) == 0 && len(ipv6) == 0 && len(ipv4) == 0 {
		m.statusBar.StickyHint = "No IP address available for SSH"
		return m, nil
	}
	keyPath := ssh.FindKeyPath(keyName)
	ignoreHostKeysDefault := false
	if cfg := m.configView.Cfg(); cfg != nil {
		ignoreHostKeysDefault = cfg.General.IgnoreSSHHostKeys
	}
	m.sshPrompt = sshprompt.New(name, floatingIPs, ipv6, ipv4, keyPath, ignoreHostKeysDefault)
	m.sshPrompt.SetSize(m.width, m.height)
	return m, m.sshPrompt.Init()
}

func (m Model) copySSHCommand() (Model, tea.Cmd) {
	_, keyName, floatingIPs, ipv6, ipv4 := m.getServerSSHInfo()
	ip := ssh.ChooseIP(floatingIPs, ipv6, ipv4)
	if ip == "" {
		m.statusBar.StickyHint = "No IP address available for SSH"
		return m, nil
	}
	keyPath := ssh.FindKeyPath(keyName)
	cmdStr := ssh.BuildCommandString(ssh.Options{User: "USER", IP: ip, KeyPath: keyPath})
	return m.copyToClipboard("", cmdStr)
}

func (m Model) openVMPassword() (Model, tea.Cmd) {
	var id, name, keyName string
	switch m.view {
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			id, name, keyName = s.ID, s.Name, s.KeyName
		}
	case viewServerDetail:
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
		keyName = m.serverDetail.ServerKeyName()
	}
	if id == "" {
		return m, nil
	}
	m.statusBar.StickyHint = "Fetching admin password..."
	client := m.client.Compute
	serverName := name
	kn := keyName
	return m, func() tea.Msg {
		ctx, cancel := actionCtx()
		defer cancel()
		keyPath := ssh.FindKeyPath(kn)
		plain, encrypted, err := compute.GetPassword(ctx, client, id, keyPath)
		if err != nil && encrypted == "" {
			return shared.VMPasswordErrMsg{Err: err, ServerName: serverName}
		}
		msg := shared.VMPasswordMsg{
			ServerName: serverName,
			KeyName:    kn,
			KeyPath:    keyPath,
			Plain:      plain,
			Encrypted:  encrypted,
		}
		switch {
		case encrypted == "":
			msg.Note = "No password set (Linux instance, or not yet generated)."
		case kn == "":
			msg.Note = "Server has no keypair — cannot decrypt."
		case keyPath == "":
			msg.Note = "No private key found in ~/.ssh/ matching keypair \"" + kn + "\"."
		case err != nil:
			msg.Note = "Decryption failed: " + err.Error()
		}
		return msg
	}
}

func (m Model) openConsoleURL() (Model, tea.Cmd) {
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
	m.statusBar.StickyHint = "Fetching console URL..."
	client := m.client.Compute
	serverName := name
	return m, func() tea.Msg {
		ctx, cancel := actionCtx()
		defer cancel()
		url, err := compute.GetRemoteConsole(ctx, client, id)
		if err != nil {
			return shared.ConsoleURLErrMsg{Err: err, ServerName: serverName}
		}
		return shared.ConsoleURLMsg{URL: url, ServerName: serverName}
	}
}

func (m Model) openAdminActions() (Model, tea.Cmd) {
	var id, name string
	if m.view == viewServerDetail {
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
	}
	if id == "" {
		return m, nil
	}
	m.serverAdminAct = serveradminact.New(id, name)
	m.serverAdminAct.SetSize(m.width, m.height)
	return m, m.serverAdminAct.Init()
}

func (m Model) openServerMetadata() (Model, tea.Cmd) {
	var id, name string
	var meta map[string]string
	if m.view == viewServerDetail {
		id = m.serverDetail.ServerID()
		name = m.serverDetail.ServerName()
		s := m.serverDetail.Server()
		if s != nil {
			meta = s.Metadata
		}
	}
	if id == "" {
		return m, nil
	}
	if meta == nil {
		meta = make(map[string]string)
	}
	m.serverMetadata = servermetadata.New(m.client.Compute, id, name, meta)
	m.serverMetadata.SetSize(m.width, m.height)
	return m, m.serverMetadata.Init()
}

package app

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/audit"
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
			m.confirm.Body = fmt.Sprintf("Apply %s to %d servers (%s)?", action, len(refs), summary)
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
			return auditLogLoadedMsg{err: err.Error()}
		}
		return auditLogLoadedMsg{entries: entries}
	}
}

type auditLogLoadedMsg struct {
	entries []audit.Entry
	err     string
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

	switch action.Action {
	case "delete":
		return m, m.deleteServerCmd(action)
	case "soft reboot":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] rebooting server %s", action.Name)
			err := compute.RebootServer(ctx, client, action.ServerID, servers.SoftReboot)
			if err != nil {
				shared.Debugf("[action] reboot server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionReboot, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Reboot", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] rebooted server %s", action.Name)
			m.logAudit(audit.ActionReboot, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Reboot", Name: action.Name}
		}
	case "hard reboot":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] hard rebooting server %s", action.Name)
			err := compute.RebootServer(ctx, client, action.ServerID, servers.HardReboot)
			if err != nil {
				shared.Debugf("[action] hard reboot server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionReboot, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Hard reboot", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] hard rebooted server %s", action.Name)
			m.logAudit(audit.ActionReboot, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Hard reboot", Name: action.Name}
		}
	case "pause":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] pausing server %s", action.Name)
			err := compute.PauseServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] pause server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionPause, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Pause", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] paused server %s", action.Name)
			m.logAudit(audit.ActionPause, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Pause", Name: action.Name}
		}
	case "unpause":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] unpausing server %s", action.Name)
			err := compute.UnpauseServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unpause server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionUnpause, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Unpause", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unpaused server %s", action.Name)
			m.logAudit(audit.ActionUnpause, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Unpause", Name: action.Name}
		}
	case "suspend":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] suspending server %s", action.Name)
			err := compute.SuspendServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] suspend server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionSuspend, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Suspend", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] suspended server %s", action.Name)
			m.logAudit(audit.ActionSuspend, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Suspend", Name: action.Name}
		}
	case "resume":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] resuming server %s", action.Name)
			err := compute.ResumeServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] resume server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionResume, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Resume", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] resumed server %s", action.Name)
			m.logAudit(audit.ActionResume, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Resume", Name: action.Name}
		}
	case "shelve":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] shelving server %s", action.Name)
			err := compute.ShelveServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] shelve server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionShelve, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Shelve", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] shelved server %s", action.Name)
			m.logAudit(audit.ActionShelve, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Shelve", Name: action.Name}
		}
	case "unshelve":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] unshelving server %s", action.Name)
			err := compute.UnshelveServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unshelve server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionUnshelve, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Unshelve", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unshelved server %s", action.Name)
			m.logAudit(audit.ActionUnshelve, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Unshelve", Name: action.Name}
		}
	case "stop":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] stopping server %s", action.Name)
			err := compute.StopServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] stop server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionStop, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Stop", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] stopped server %s", action.Name)
			m.logAudit(audit.ActionStop, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Stop", Name: action.Name}
		}
	case "start":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] starting server %s", action.Name)
			err := compute.StartServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] start server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionStart, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Start", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] started server %s", action.Name)
			m.logAudit(audit.ActionStart, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Start", Name: action.Name}
		}
	case "lock":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] locking server %s", action.Name)
			err := compute.LockServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] lock server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionLock, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Lock", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] locked server %s", action.Name)
			m.logAudit(audit.ActionLock, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Lock", Name: action.Name}
		}
	case "unlock":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] unlocking server %s", action.Name)
			err := compute.UnlockServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unlock server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionUnlock, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Unlock", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unlocked server %s", action.Name)
			m.logAudit(audit.ActionUnlock, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Unlock", Name: action.Name}
		}
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
	case "unrescue":
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] unrescuing server %s", action.Name)
			err := compute.UnrescueServer(ctx, client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unrescue server %s failed: %s", action.Name, err)
				m.logAudit(audit.ActionUnrescue, "server", action.ServerID, action.Name, "error", err.Error())
				return shared.ServerActionErrMsg{Action: "Unrescue", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unrescued server %s", action.Name)
			m.logAudit(audit.ActionUnrescue, "server", action.ServerID, action.Name, "success", "")
			return shared.ServerActionMsg{Action: "Unrescue", Name: action.Name}
		}
	case "delete_volume":
		bsClient := m.client.BlockStorage
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting volume %s", name)
			err := volume.DeleteVolume(ctx, bsClient, id)
			if err != nil {
				shared.Debugf("[action] delete volume %s failed: %s", name, err)
				m.logAudit(audit.ActionDelete, "volume", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete volume", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted volume %s", name)
			m.logAudit(audit.ActionDelete, "volume", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted volume", Name: name}
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
	case "release_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] releasing floating IP %s", name)
			err := network.ReleaseFloatingIP(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] release floating IP %s failed: %s", name, err)
				m.logAudit(audit.ActionDetachFIP, "floating_ip", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Release FIP", Name: name, Err: err}
			}
			shared.Debugf("[action] released floating IP %s", name)
			m.logAudit(audit.ActionDetachFIP, "floating_ip", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Released", Name: name}
		}
	case "disassociate_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] disassociating floating IP %s", name)
			err := network.DisassociateFloatingIP(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] disassociate floating IP %s failed: %s", name, err)
				m.logAudit(audit.ActionDetachFIP, "floating_ip", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Disassociate FIP", Name: name, Err: err}
			}
			shared.Debugf("[action] disassociated floating IP %s", name)
			m.logAudit(audit.ActionDetachFIP, "floating_ip", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Disassociated", Name: name}
		}
	case "delete_router":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting router %s", name)
			err := network.DeleteRouter(ctx, netClient, id)
			if err != nil {
				// Not wrapped with %w: the generic 409 text ("resource is busy")
				// would otherwise replace this actionable message in the modal.
				if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
					err = fmt.Errorf("router still in use; remove its interfaces first (Interfaces pane, %s): %v", shared.Keys.Detach.Help().Key, err)
				}
				shared.Debugf("[action] delete router %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteRouter, "router", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete router", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted router %s", name)
			m.logAudit(audit.ActionDeleteRouter, "router", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted router", Name: name}
		}
	case "remove_router_interface":
		target, ok := decodeRouterInterfaceTarget(action.ServerID)
		if !ok {
			return m, nil
		}
		return m, m.removeRouterInterfaceCmd(target, action.Name)
	case "delete_port":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting port %s", name)
			err := network.DeletePort(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] delete port %s failed: %s", name, err)
				m.logAudit(audit.ActionDeletePort, "port", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete port", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted port %s", name)
			m.logAudit(audit.ActionDeletePort, "port", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted port", Name: name}
		}
	case "delete_network":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting network %s", name)
			err := network.DeleteNetwork(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] delete network %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteNet, "network", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete network", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted network %s", name)
			m.logAudit(audit.ActionDeleteNet, "network", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted network", Name: name}
		}
	case "delete_subnet":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting subnet %s", name)
			err := network.DeleteSubnet(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] delete subnet %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteSubnet, "subnet", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete subnet", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted subnet %s", name)
			m.logAudit(audit.ActionDeleteSubnet, "subnet", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted subnet", Name: name}
		}
	case "delete_sg":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting security group %s", name)
			err := network.DeleteSecurityGroup(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] delete security group %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteNet, "security_group", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete security group", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted security group %s", name)
			m.logAudit(audit.ActionDeleteNet, "security_group", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted security group", Name: name}
		}
	case "delete_sg_rule":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting security group rule from %s", name)
			err := network.DeleteSecurityGroupRule(ctx, netClient, id)
			if err != nil {
				shared.Debugf("[action] delete security group rule from %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteNet, "security_group_rule", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete rule", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted security group rule from %s", name)
			m.logAudit(audit.ActionDeleteNet, "security_group_rule", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted rule from", Name: name}
		}
	case "delete_lb":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting load balancer %s", name)
			err := loadbalancer.DeleteLoadBalancer(ctx, lbClient, id)
			if err != nil {
				shared.Debugf("[action] delete load balancer %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteLB, "load_balancer", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete LB", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted load balancer %s", name)
			m.logAudit(audit.ActionDeleteLB, "load_balancer", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted LB", Name: name}
		}
	case "delete_lb_listener":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			err := loadbalancer.DeleteListener(ctx, lbClient, id)
			if err != nil {
				m.logAudit(audit.ActionDeleteLB, "lb_listener", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete listener", Name: name, Err: err}
			}
			m.logAudit(audit.ActionDeleteLB, "lb_listener", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted listener", Name: name}
		}
	case "delete_lb_pool":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			err := loadbalancer.DeletePool(ctx, lbClient, id)
			if err != nil {
				m.logAudit(audit.ActionDeleteLB, "lb_pool", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete pool", Name: name, Err: err}
			}
			m.logAudit(audit.ActionDeleteLB, "lb_pool", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted pool", Name: name}
		}
	case "delete_lb_monitor":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting health monitor from %s", name)
			err := loadbalancer.DeleteHealthMonitor(ctx, lbClient, id)
			if err != nil {
				shared.Debugf("[action] delete health monitor from %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteLB, "lb_monitor", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete monitor", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted health monitor from %s", name)
			m.logAudit(audit.ActionDeleteLB, "lb_monitor", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Removed monitor from", Name: name}
		}
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
	case "delete_keypair":
		computeC := m.client.Compute
		name := action.ServerID // keypair name is stored in ServerID
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting keypair %s", name)
			err := compute.DeleteKeyPair(ctx, computeC, name)
			if err != nil {
				shared.Debugf("[action] delete keypair %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteKey, "keypair", name, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete keypair", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted keypair %s", name)
			m.logAudit(audit.ActionDeleteKey, "keypair", name, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted keypair", Name: name}
		}
	case "delete_image":
		imgClient := m.client.Image
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			ctx, cancel := actionCtx()
			defer cancel()
			shared.Debugf("[action] deleting image %s", name)
			err := image.DeleteImage(ctx, imgClient, id)
			if err != nil {
				shared.Debugf("[action] delete image %s failed: %s", name, err)
				m.logAudit(audit.ActionDeleteImage, "image", id, name, "error", err.Error())
				return shared.ResourceActionErrMsg{Action: "Delete image", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted image %s", name)
			m.logAudit(audit.ActionDeleteImage, "image", id, name, "success", "")
			return shared.ResourceActionMsg{Action: "Deleted image", Name: name}
		}
	case "deactivate_image":
		return m, m.doDeactivateImage(action.ServerID, action.Name)
	case "reactivate_image":
		return m, m.doReactivateImage(action.ServerID, action.Name)
	}
	return m, nil
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
	if err := clipboard.WriteAll(cmdStr); err != nil {
		m.statusBar.StickyHint = "Clipboard error: " + err.Error()
	} else {
		m.statusBar.StickyHint = "Copied: " + cmdStr
	}
	return m, nil
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

package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ssh"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/fippicker"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/servercreate"
	"github.com/larkly/lazystack/internal/ui/serverrebuild"
	"github.com/larkly/lazystack/internal/ui/serverrename"
	"github.com/larkly/lazystack/internal/ui/serversnapshot"
	"github.com/larkly/lazystack/internal/ui/serverresize"
	"github.com/larkly/lazystack/internal/ui/sshprompt"
	"github.com/larkly/lazystack/internal/volume"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"charm.land/bubbletea/v2"
)

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
			case "stop/start":
				if status == "SHUTOFF" {
					actualAction = "start"
				} else {
					actualAction = "stop"
				}
			case "lock/unlock":
				if len(servers) > 0 && servers[0].Locked {
					actualAction = "unlock"
				} else {
					actualAction = "lock"
				}
			case "rescue/unrescue":
				if status == "RESCUE" {
					actualAction = "unrescue"
				} else {
					actualAction = "rescue"
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
	case "stop/start":
		if status == "SHUTOFF" {
			actualAction = "start"
		} else {
			actualAction = "stop"
		}
	case "lock/unlock":
		// Lock status needs to be checked from the server object
		var locked bool
		switch m.view {
		case viewServerList:
			if s := m.serverList.SelectedServer(); s != nil {
				locked = s.Locked
			}
		case viewServerDetail:
			locked = m.serverDetail.ServerLocked()
		}
		if locked {
			actualAction = "unlock"
		} else {
			actualAction = "lock"
		}
	case "rescue/unrescue":
		if status == "RESCUE" {
			actualAction = "unrescue"
		} else {
			actualAction = "rescue"
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

func (m Model) executeAction(action modal.ConfirmAction) (Model, tea.Cmd) {
	client := m.client.Compute

	// Bulk actions
	if len(action.Servers) > 0 {
		m.serverList.ClearSelection()
		return m, m.executeBulkAction(client, action)
	}

	switch action.Action {
	case "delete":
		bsClient := m.client.BlockStorage
		computeC := m.client.Compute
		deleteVols := action.DeleteVolumes
		volIDs := action.VolumeIDs
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting server %s", action.Name)
			// Detach volumes before deleting the server
			var volErrs []string
			if deleteVols && bsClient != nil {
				for _, vid := range volIDs {
					if err := volume.DetachVolume(context.Background(), computeC, action.ServerID, vid); err != nil {
						volErrs = append(volErrs, fmt.Sprintf("detach %s: %v", vid, err))
					}
				}
				// Wait for volumes to detach (up to 30s)
				for range 10 {
					allDetached := true
					for _, vid := range volIDs {
						v, err := volume.GetVolume(context.Background(), bsClient, vid)
						if err == nil && v.Status != "available" {
							allDetached = false
							break
						}
					}
					if allDetached {
						break
					}
					time.Sleep(3 * time.Second)
				}
			}

			err := compute.DeleteServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] delete server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Delete", Name: action.Name, Err: err}
			}

			if deleteVols && bsClient != nil {
				for _, vid := range volIDs {
					if err := volume.DeleteVolume(context.Background(), bsClient, vid); err != nil {
						volErrs = append(volErrs, fmt.Sprintf("delete %s: %v", vid, err))
					}
				}
			}
			shared.Debugf("[action] deleted server %s", action.Name)
			msg := shared.ServerActionMsg{Action: "Delete", Name: action.Name}
			if len(volErrs) > 0 {
				msg.Action = fmt.Sprintf("Delete (warning: %d volume error(s))", len(volErrs))
			}
			return msg
		}
	case "soft reboot":
		return m, func() tea.Msg {
			shared.Debugf("[action] rebooting server %s", action.Name)
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.SoftReboot)
			if err != nil {
				shared.Debugf("[action] reboot server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Reboot", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] rebooted server %s", action.Name)
			return shared.ServerActionMsg{Action: "Reboot", Name: action.Name}
		}
	case "hard reboot":
		return m, func() tea.Msg {
			shared.Debugf("[action] hard rebooting server %s", action.Name)
			err := compute.RebootServer(context.Background(), client, action.ServerID, servers.HardReboot)
			if err != nil {
				shared.Debugf("[action] hard reboot server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Hard reboot", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] hard rebooted server %s", action.Name)
			return shared.ServerActionMsg{Action: "Hard reboot", Name: action.Name}
		}
	case "pause":
		return m, func() tea.Msg {
			shared.Debugf("[action] pausing server %s", action.Name)
			err := compute.PauseServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] pause server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Pause", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] paused server %s", action.Name)
			return shared.ServerActionMsg{Action: "Pause", Name: action.Name}
		}
	case "unpause":
		return m, func() tea.Msg {
			shared.Debugf("[action] unpausing server %s", action.Name)
			err := compute.UnpauseServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unpause server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Unpause", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unpaused server %s", action.Name)
			return shared.ServerActionMsg{Action: "Unpause", Name: action.Name}
		}
	case "suspend":
		return m, func() tea.Msg {
			shared.Debugf("[action] suspending server %s", action.Name)
			err := compute.SuspendServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] suspend server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Suspend", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] suspended server %s", action.Name)
			return shared.ServerActionMsg{Action: "Suspend", Name: action.Name}
		}
	case "resume":
		return m, func() tea.Msg {
			shared.Debugf("[action] resuming server %s", action.Name)
			err := compute.ResumeServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] resume server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Resume", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] resumed server %s", action.Name)
			return shared.ServerActionMsg{Action: "Resume", Name: action.Name}
		}
	case "shelve":
		return m, func() tea.Msg {
			shared.Debugf("[action] shelving server %s", action.Name)
			err := compute.ShelveServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] shelve server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Shelve", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] shelved server %s", action.Name)
			return shared.ServerActionMsg{Action: "Shelve", Name: action.Name}
		}
	case "unshelve":
		return m, func() tea.Msg {
			shared.Debugf("[action] unshelving server %s", action.Name)
			err := compute.UnshelveServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unshelve server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Unshelve", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unshelved server %s", action.Name)
			return shared.ServerActionMsg{Action: "Unshelve", Name: action.Name}
		}
	case "stop":
		return m, func() tea.Msg {
			shared.Debugf("[action] stopping server %s", action.Name)
			err := compute.StopServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] stop server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Stop", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] stopped server %s", action.Name)
			return shared.ServerActionMsg{Action: "Stop", Name: action.Name}
		}
	case "start":
		return m, func() tea.Msg {
			shared.Debugf("[action] starting server %s", action.Name)
			err := compute.StartServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] start server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Start", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] started server %s", action.Name)
			return shared.ServerActionMsg{Action: "Start", Name: action.Name}
		}
	case "lock":
		return m, func() tea.Msg {
			shared.Debugf("[action] locking server %s", action.Name)
			err := compute.LockServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] lock server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Lock", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] locked server %s", action.Name)
			return shared.ServerActionMsg{Action: "Lock", Name: action.Name}
		}
	case "unlock":
		return m, func() tea.Msg {
			shared.Debugf("[action] unlocking server %s", action.Name)
			err := compute.UnlockServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unlock server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Unlock", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unlocked server %s", action.Name)
			return shared.ServerActionMsg{Action: "Unlock", Name: action.Name}
		}
	case "rescue":
		return m, func() tea.Msg {
			shared.Debugf("[action] rescuing server %s", action.Name)
			adminPass, err := compute.RescueServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] rescue server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Rescue", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] rescued server %s", action.Name)
			msg := shared.ServerActionMsg{Action: "Rescue", Name: action.Name}
			if adminPass != "" {
				msg.Action = fmt.Sprintf("Rescue (password: %s)", adminPass)
			}
			return msg
		}
	case "unrescue":
		return m, func() tea.Msg {
			shared.Debugf("[action] unrescuing server %s", action.Name)
			err := compute.UnrescueServer(context.Background(), client, action.ServerID)
			if err != nil {
				shared.Debugf("[action] unrescue server %s failed: %s", action.Name, err)
				return shared.ServerActionErrMsg{Action: "Unrescue", Name: action.Name, Err: err}
			}
			shared.Debugf("[action] unrescued server %s", action.Name)
			return shared.ServerActionMsg{Action: "Unrescue", Name: action.Name}
		}
	case "delete_volume":
		bsClient := m.client.BlockStorage
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting volume %s", name)
			err := volume.DeleteVolume(context.Background(), bsClient, id)
			if err != nil {
				shared.Debugf("[action] delete volume %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete volume", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted volume %s", name)
			return shared.ResourceActionMsg{Action: "Deleted volume", Name: name}
		}
	case "detach_volume":
		computeC := m.client.Compute
		volID := action.ServerID
		name := action.Name
		bsClient := m.client.BlockStorage
		return m, func() tea.Msg {
			shared.Debugf("[action] detaching volume %s", name)
			vol, err := volume.GetVolume(context.Background(), bsClient, volID)
			if err != nil {
				shared.Debugf("[action] detach volume %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: err}
			}
			if vol.AttachedServerID == "" {
				shared.Debugf("[action] detach volume %s failed: volume is not attached", name)
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: fmt.Errorf("volume is not attached")}
			}
			err = volume.DetachVolume(context.Background(), computeC, vol.AttachedServerID, volID)
			if err != nil {
				shared.Debugf("[action] detach volume %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Detach volume", Name: name, Err: err}
			}
			shared.Debugf("[action] detached volume %s", name)
			return shared.ResourceActionMsg{Action: "Detached volume", Name: name}
		}
	case "release_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] releasing floating IP %s", name)
			err := network.ReleaseFloatingIP(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] release floating IP %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Release FIP", Name: name, Err: err}
			}
			shared.Debugf("[action] released floating IP %s", name)
			return shared.ResourceActionMsg{Action: "Released", Name: name}
		}
	case "disassociate_fip":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] disassociating floating IP %s", name)
			err := network.DisassociateFloatingIP(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] disassociate floating IP %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Disassociate FIP", Name: name, Err: err}
			}
			shared.Debugf("[action] disassociated floating IP %s", name)
			return shared.ResourceActionMsg{Action: "Disassociated", Name: name}
		}
	case "delete_router":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting router %s", name)
			err := network.DeleteRouter(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] delete router %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete router", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted router %s", name)
			return shared.ResourceActionMsg{Action: "Deleted router", Name: name}
		}
	case "remove_router_interface":
		netClient := m.client.Network
		routerID := action.ServerID
		name := action.Name
		// Get the selected subnet ID from the router view
		subnetID := m.routerView.SelectedInterfaceSubnetID()
		return m, func() tea.Msg {
			shared.Debugf("[action] removing router interface from %s", name)
			err := network.RemoveRouterInterface(context.Background(), netClient, routerID, subnetID)
			if err != nil {
				shared.Debugf("[action] remove router interface from %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Remove interface", Name: name, Err: err}
			}
			shared.Debugf("[action] removed router interface from %s", name)
			return shared.ResourceActionMsg{Action: "Removed interface from", Name: name}
		}
	case "delete_network":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting network %s", name)
			err := network.DeleteNetwork(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] delete network %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete network", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted network %s", name)
			return shared.ResourceActionMsg{Action: "Deleted network", Name: name}
		}
	case "delete_subnet":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting subnet %s", name)
			err := network.DeleteSubnet(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] delete subnet %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete subnet", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted subnet %s", name)
			return shared.ResourceActionMsg{Action: "Deleted subnet", Name: name}
		}
	case "delete_sg":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting security group %s", name)
			err := network.DeleteSecurityGroup(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] delete security group %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete security group", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted security group %s", name)
			return shared.ResourceActionMsg{Action: "Deleted security group", Name: name}
		}
	case "delete_sg_rule":
		netClient := m.client.Network
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting security group rule from %s", name)
			err := network.DeleteSecurityGroupRule(context.Background(), netClient, id)
			if err != nil {
				shared.Debugf("[action] delete security group rule from %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete rule", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted security group rule from %s", name)
			return shared.ResourceActionMsg{Action: "Deleted rule from", Name: name}
		}
	case "delete_lb":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting load balancer %s", name)
			err := loadbalancer.DeleteLoadBalancer(context.Background(), lbClient, id)
			if err != nil {
				shared.Debugf("[action] delete load balancer %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete LB", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted load balancer %s", name)
			return shared.ResourceActionMsg{Action: "Deleted LB", Name: name}
		}
	case "delete_keypair":
		computeC := m.client.Compute
		name := action.ServerID // keypair name is stored in ServerID
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting keypair %s", name)
			err := compute.DeleteKeyPair(context.Background(), computeC, name)
			if err != nil {
				shared.Debugf("[action] delete keypair %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete keypair", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted keypair %s", name)
			return shared.ResourceActionMsg{Action: "Deleted keypair", Name: name}
		}
	case "delete_image":
		imgClient := m.client.Image
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			shared.Debugf("[action] deleting image %s", name)
			err := image.DeleteImage(context.Background(), imgClient, id)
			if err != nil {
				shared.Debugf("[action] delete image %s failed: %s", name, err)
				return shared.ResourceActionErrMsg{Action: "Delete image", Name: name, Err: err}
			}
			shared.Debugf("[action] deleted image %s", name)
			return shared.ResourceActionMsg{Action: "Deleted image", Name: name}
		}
	case "deactivate_image":
		return m, m.doDeactivateImage(action.ServerID, action.Name)
	case "reactivate_image":
		return m, m.doReactivateImage(action.ServerID, action.Name)
	}
	return m, nil
}

func (m Model) executeBulkAction(client *gophercloud.ServiceClient, action modal.ConfirmAction) tea.Cmd {
	targets := action.Servers
	act := action.Action
	return func() tea.Msg {
		var errs []string
		var passwords []string
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
			case "stop":
				err = compute.StopServer(context.Background(), client, s.ID)
			case "start":
				err = compute.StartServer(context.Background(), client, s.ID)
			case "lock":
				err = compute.LockServer(context.Background(), client, s.ID)
			case "unlock":
				err = compute.UnlockServer(context.Background(), client, s.ID)
			case "rescue":
				var adminPass string
				adminPass, err = compute.RescueServer(context.Background(), client, s.ID)
				if err == nil && adminPass != "" {
					passwords = append(passwords, fmt.Sprintf("%s: %s", s.Name, adminPass))
				}
			case "unrescue":
				err = compute.UnrescueServer(context.Background(), client, s.ID)
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
		msg := shared.ServerActionMsg{
			Action: act,
			Name:   fmt.Sprintf("%d servers", len(targets)),
		}
		if len(passwords) > 0 {
			msg.Action = fmt.Sprintf("Rescue (passwords: %s)", strings.Join(passwords, ", "))
		}
		return msg
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
	m.sshPrompt = sshprompt.New(name, floatingIPs, ipv6, ipv4, keyPath)
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
		url, err := compute.GetRemoteConsole(context.Background(), client, id)
		if err != nil {
			return shared.ConsoleURLErrMsg{Err: err, ServerName: serverName}
		}
		return shared.ConsoleURLMsg{URL: url, ServerName: serverName}
	}
}

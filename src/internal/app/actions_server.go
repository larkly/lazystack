package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/actionlog"
	"github.com/larkly/lazystack/internal/ui/consolelog"
	"github.com/larkly/lazystack/internal/ui/fippicker"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/serverresize"
	"github.com/larkly/lazystack/internal/volume"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"charm.land/bubbletea/v2"
)

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
	case "delete_lb":
		lbClient := m.client.LoadBalancer
		id := action.ServerID
		name := action.Name
		return m, func() tea.Msg {
			err := loadbalancer.DeleteLoadBalancer(context.Background(), lbClient, id)
			if err != nil {
				return shared.ResourceActionErrMsg{Action: "Delete LB", Name: name, Err: err}
			}
			return shared.ResourceActionMsg{Action: "Deleted LB", Name: name}
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

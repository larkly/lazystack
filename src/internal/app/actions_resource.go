package app

import (
	"context"
	"fmt"

	"github.com/bosse/lazystack/internal/network"
	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/ui/modal"
	"github.com/bosse/lazystack/internal/ui/volumedetail"
	"charm.land/bubbletea/v2"
)

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

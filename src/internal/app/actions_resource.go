package app

import (
	"context"
	"fmt"

	"charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/keypaircreate"
	"github.com/larkly/lazystack/internal/ui/keypairdetail"
	"github.com/larkly/lazystack/internal/ui/lbcreate"
	"github.com/larkly/lazystack/internal/ui/lbdetail"
	"github.com/larkly/lazystack/internal/ui/lblistenercreate"
	"github.com/larkly/lazystack/internal/ui/lbmembercreate"
	"github.com/larkly/lazystack/internal/ui/lbpoolcreate"
	"github.com/larkly/lazystack/internal/ui/modal"
	"github.com/larkly/lazystack/internal/ui/networkcreate"
	"github.com/larkly/lazystack/internal/ui/routercreate"
	"github.com/larkly/lazystack/internal/ui/serverpicker"
	"github.com/larkly/lazystack/internal/ui/sgcreate"
	"github.com/larkly/lazystack/internal/ui/sgrulecreate"
	"github.com/larkly/lazystack/internal/ui/subnetcreate"
	"github.com/larkly/lazystack/internal/ui/subnetpicker"
	"github.com/larkly/lazystack/internal/ui/volumecreate"
	"github.com/larkly/lazystack/internal/ui/volumedetail"
	"github.com/larkly/lazystack/internal/ui/volumepicker"
)

func (m Model) openVolumeCreate() (Model, tea.Cmd) {
	m.volumeCreate = volumecreate.New(m.client.BlockStorage)
	m.volumeCreate.SetSize(m.width, m.height)
	m.view = viewVolumeCreate
	m.statusBar.CurrentView = "volumecreate"
	m.statusBar.Hint = m.volumeCreate.Hints()
	return m, m.volumeCreate.Init()
}

func (m Model) openVolumeDetail() (Model, tea.Cmd) {
	v := m.volumeList.SelectedVolume()
	if v == nil {
		return m, nil
	}
	m.volumeDetail = volumedetail.New(m.client.BlockStorage, m.client.Compute, v.ID)
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
	var id, name string
	switch m.view {
	case viewVolumeDetail:
		id = m.volumeDetail.SelectedVolumeID()
		name = m.volumeDetail.SelectedVolumeName()
	case viewVolumeList:
		if v := m.volumeList.SelectedVolume(); v != nil {
			id, name = v.ID, v.Name
			if name == "" {
				name = id
			}
		}
	}
	if id == "" {
		return m, nil
	}
	m.serverPicker = serverpicker.New(m.client.Compute, id, name)
	m.serverPicker.SetSize(m.width, m.height)
	return m, m.serverPicker.Init()
}

func (m Model) openVolumeDetach() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewVolumeDetail:
		id = m.volumeDetail.SelectedVolumeID()
		name = m.volumeDetail.SelectedVolumeName()
	case viewVolumeList:
		if v := m.volumeList.SelectedVolume(); v != nil {
			if v.AttachedServerID == "" {
				return m, nil
			}
			id, name = v.ID, v.Name
			if name == "" {
				name = id
			}
		}
	}
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

func (m Model) openServerVolumeAttach() (Model, tea.Cmd) {
	var serverID, serverName string
	switch m.view {
	case viewServerDetail:
		serverID = m.serverDetail.ServerID()
		serverName = m.serverDetail.ServerName()
	case viewServerList:
		if s := m.serverList.SelectedServer(); s != nil {
			serverID = s.ID
			serverName = s.Name
		}
	default:
		return m, nil
	}
	if serverID == "" {
		return m, nil
	}
	m.volumePicker = volumepicker.New(m.client.Compute, m.client.BlockStorage, serverID, serverName)
	m.volumePicker.SetSize(m.width, m.height)
	return m, m.volumePicker.Init()
}

func (m Model) openServerVolumeDetach() (Model, tea.Cmd) {
	if m.view != viewServerDetail {
		return m, nil
	}
	volID := m.serverDetail.SelectedVolumeID()
	volName := m.serverDetail.SelectedVolumeName()
	if volID == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("detach_volume", volID, volName)
	m.confirm.Title = "Detach Volume"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to detach volume %q from server %q?", volName, m.serverDetail.ServerName())
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
		shared.Debugf("[action] allocating floating IP")
		nets, err := network.ListExternalNetworks(context.Background(), networkClient)
		if err != nil {
			shared.Debugf("[action] allocate floating IP failed: %s", err)
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: err}
		}
		if len(nets) == 0 {
			shared.Debugf("[action] allocate floating IP failed: no external networks available")
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: fmt.Errorf("no external networks available")}
		}
		fip, err := network.AllocateFloatingIP(context.Background(), networkClient, nets[0].ID)
		if err != nil {
			shared.Debugf("[action] allocate floating IP failed: %s", err)
			return shared.ResourceActionErrMsg{Action: "Allocate", Name: "floating IP", Err: err}
		}
		shared.Debugf("[action] allocated floating IP %s", fip.FloatingIP)
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

func (m Model) openSGCreate() (Model, tea.Cmd) {
	m.sgCreate = sgcreate.New(m.client.Network)
	m.sgCreate.SetSize(m.width, m.height)
	return m, m.sgCreate.Init()
}

func (m Model) openSGDeleteConfirm() (Model, tea.Cmd) {
	sgID := m.secGroupView.SelectedGroupID()
	sgName := m.secGroupView.SelectedGroupName()
	if sgID == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_sg", sgID, sgName)
	m.confirm.Title = "Delete Security Group"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete security group %q?\nAll rules in this group will also be deleted.", sgName)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openSGRuleDeleteConfirm() (Model, tea.Cmd) {
	ruleID := m.secGroupView.SelectedRuleID()
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

func (m Model) openSGRuleCreate() (Model, tea.Cmd) {
	sgID := m.secGroupView.SelectedGroupID()
	sgName := m.secGroupView.SelectedGroupName()
	if sgID == "" {
		return m, nil
	}
	m.sgRuleCreate = sgrulecreate.New(m.client.Network, sgID, sgName)
	m.sgRuleCreate.SetSize(m.width, m.height)
	return m, m.sgRuleCreate.Init()
}

func (m Model) openSGRuleEdit() (Model, tea.Cmd) {
	r := m.secGroupView.SelectedRule()
	if r == nil {
		return m, nil
	}
	sgID := m.secGroupView.SelectedGroupID()
	sgName := m.secGroupView.SelectedGroupName()
	m.sgRuleCreate = sgrulecreate.NewEdit(m.client.Network, sgID, sgName, *r)
	m.sgRuleCreate.SetSize(m.width, m.height)
	return m, m.sgRuleCreate.Init()
}

func (m Model) openSGRename() (Model, tea.Cmd) {
	m.sgCreate = sgcreate.NewRename(m.client.Network, m.secGroupView.SelectedGroupID(), m.secGroupView.SelectedGroupName(), m.secGroupView.SGDescription())
	m.sgCreate.SetSize(m.width, m.height)
	return m, m.sgCreate.Init()
}

func (m Model) openSGClone() (Model, tea.Cmd) {
	m.sgCreate = sgcreate.NewClone(m.client.Network, m.secGroupView.SelectedGroupID(), m.secGroupView.SelectedGroupName(), m.secGroupView.SGDescription())
	m.sgCreate.SetSize(m.width, m.height)
	return m, m.sgCreate.Init()
}

// --- Network actions ---

func (m Model) openNetworkCreate() (Model, tea.Cmd) {
	m.networkCreate = networkcreate.New(m.client.Network)
	m.networkCreate.SetSize(m.width, m.height)
	return m, m.networkCreate.Init()
}

func (m Model) openNetworkDeleteConfirm() (Model, tea.Cmd) {
	netID := m.networkView.SelectedNetworkID()
	netName := m.networkView.SelectedNetworkName()
	if netID == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_network", netID, netName)
	m.confirm.Title = "Delete Network"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete network %q?\nAll subnets will also be deleted.", netName)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openSubnetCreate() (Model, tea.Cmd) {
	netID := m.networkView.SelectedNetworkID()
	netName := m.networkView.SelectedNetworkName()
	if netID == "" {
		return m, nil
	}
	m.subnetCreate = subnetcreate.New(m.client.Network, netID, netName)
	m.subnetCreate.SetSize(m.width, m.height)
	return m, m.subnetCreate.Init()
}

func (m Model) openSubnetDeleteConfirm() (Model, tea.Cmd) {
	subID := m.networkView.SelectedSubnetID()
	subName := m.networkView.SelectedSubnetName()
	if subID == "" {
		return m, nil
	}
	netName := m.networkView.SelectedNetworkName()
	m.confirm = modal.NewConfirm("delete_subnet", subID, subName)
	m.confirm.Title = "Delete Subnet"
	m.confirm.Body = fmt.Sprintf("Delete subnet %q from network %q?", subName, netName)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Router actions ---

func (m Model) openRouterCreate() (Model, tea.Cmd) {
	m.routerCreate = routercreate.New(m.client.Network)
	m.routerCreate.SetSize(m.width, m.height)
	return m, m.routerCreate.Init()
}

func (m Model) openRouterDeleteConfirm() (Model, tea.Cmd) {
	id := m.routerView.SelectedRouterID()
	name := m.routerView.SelectedRouterName()
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_router", id, name)
	m.confirm.Title = "Delete Router"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete router %q?\nAll interfaces will be removed.", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

func (m Model) openAddRouterInterface() (Model, tea.Cmd) {
	id := m.routerView.SelectedRouterID()
	name := m.routerView.SelectedRouterName()
	if id == "" {
		return m, nil
	}
	m.subnetPicker = subnetpicker.New(m.client.Network, id, name)
	m.subnetPicker.SetSize(m.width, m.height)
	return m, m.subnetPicker.Init()
}

func (m Model) openRemoveRouterInterfaceConfirm() (Model, tea.Cmd) {
	subnetID := m.routerView.SelectedInterfaceSubnetID()
	if subnetID == "" {
		return m, nil
	}
	routerID := m.routerView.SelectedRouterID()
	routerName := m.routerView.SelectedRouterName()
	m.confirm = modal.NewConfirm("remove_router_interface", routerID, routerName)
	m.confirm.Title = "Remove Interface"
	m.confirm.Body = fmt.Sprintf("Remove subnet interface from router %q?", routerName)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Load Balancer actions ---

func (m Model) openLBDetail() (Model, tea.Cmd) {
	lb := m.lbList.SelectedLB()
	if lb == nil {
		return m, nil
	}
	m.lbDetail = lbdetail.New(m.client.LoadBalancer, lb.ID)
	m.lbDetail.SetSize(m.width, m.height)
	m.view = viewLBDetail
	m.statusBar.CurrentView = "lbdetail"
	m.statusBar.Hint = m.lbDetail.Hints()
	return m, m.lbDetail.Init()
}

func (m Model) openLBDeleteConfirm() (Model, tea.Cmd) {
	var id, name string
	switch m.view {
	case viewLBList:
		if lb := m.lbList.SelectedLB(); lb != nil {
			id, name = lb.ID, lb.Name
			if name == "" {
				name = id
			}
		}
	case viewLBDetail:
		id = m.lbDetail.LBID()
		name = m.lbDetail.LBName()
	}
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_lb", id, name)
	m.confirm.Title = "Delete Load Balancer"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete load balancer %q?\nThis will cascade-delete all listeners, pools and members.", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Load Balancer create/edit ---

func (m Model) openLBCreate() (Model, tea.Cmd) {
	m.lbCreate = lbcreate.New(m.client.LoadBalancer, m.client.Network)
	m.lbCreate.SetSize(m.width, m.height)
	return m, m.lbCreate.Init()
}

func (m Model) openLBEdit() (Model, tea.Cmd) {
	if m.lbDetail.LBID() == "" {
		return m, nil
	}
	name := m.lbDetail.LBName()
	desc := ""
	if m.lbDetail.LB() != nil {
		desc = m.lbDetail.LB().Description
	}
	m.lbCreate = lbcreate.NewEdit(m.client.LoadBalancer, m.lbDetail.LBID(), name, desc)
	m.lbCreate.SetSize(m.width, m.height)
	return m, m.lbCreate.Init()
}

// --- Load Balancer Listener actions ---

func (m Model) openLBListenerEdit() (Model, tea.Cmd) {
	l := m.lbDetail.SelectedListener()
	if l == nil {
		return m, nil
	}
	m.lbListenerCreate = lblistenercreate.NewEdit(m.client.LoadBalancer, l.ID, l.Name, l.Description, l.ConnLimit, m.lbDetail.LBName())
	m.lbListenerCreate.SetSize(m.width, m.height)
	return m, m.lbListenerCreate.Init()
}

func (m Model) openLBListenerCreate() (Model, tea.Cmd) {
	m.lbListenerCreate = lblistenercreate.New(m.client.LoadBalancer, m.lbDetail.LBID(), m.lbDetail.LBName())
	m.lbListenerCreate.SetSize(m.width, m.height)
	return m, m.lbListenerCreate.Init()
}

func (m Model) openLBListenerDeleteConfirm() (Model, tea.Cmd) {
	id := m.lbDetail.SelectedListenerID()
	name := m.lbDetail.SelectedListenerName()
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_lb_listener", id, name)
	m.confirm.Title = "Delete Listener"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete listener %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Load Balancer Pool actions ---

func (m Model) openLBPoolEdit() (Model, tea.Cmd) {
	p := m.lbDetail.SelectedPool()
	if p == nil {
		return m, nil
	}
	m.lbPoolCreate = lbpoolcreate.NewEdit(m.client.LoadBalancer, p.ID, p.Name, p.LBMethod, m.lbDetail.LBName())
	m.lbPoolCreate.SetSize(m.width, m.height)
	return m, m.lbPoolCreate.Init()
}

func (m Model) openLBPoolCreate() (Model, tea.Cmd) {
	m.lbPoolCreate = lbpoolcreate.New(m.client.LoadBalancer, m.lbDetail.LBID(), m.lbDetail.LBName())
	m.lbPoolCreate.SetSize(m.width, m.height)
	return m, m.lbPoolCreate.Init()
}

func (m Model) openLBPoolDeleteConfirm() (Model, tea.Cmd) {
	id := m.lbDetail.SelectedPoolID()
	name := m.lbDetail.SelectedPoolName()
	if id == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_lb_pool", id, name)
	m.confirm.Title = "Delete Pool"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete pool %q?\nAll members and health monitors will also be removed.", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

// --- Load Balancer Member actions ---

func (m Model) openLBMemberEdit() (Model, tea.Cmd) {
	mem := m.lbDetail.SelectedMember()
	poolID := m.lbDetail.SelectedPoolID()
	poolName := m.lbDetail.SelectedPoolName()
	if mem == nil || poolID == "" {
		return m, nil
	}
	m.lbMemberCreate = lbmembercreate.NewEdit(
		m.client.LoadBalancer,
		poolID,
		mem.ID,
		mem.Name,
		mem.Weight,
		mem.AdminStateUp,
		mem.Backup,
		mem.MonitorAddress,
		mem.MonitorPort,
		mem.Tags,
		poolName,
	)
	m.lbMemberCreate.SetSize(m.width, m.height)
	return m, m.lbMemberCreate.Init()
}

func (m Model) openLBMemberCreate() (Model, tea.Cmd) {
	poolID := m.lbDetail.SelectedPoolID()
	poolName := m.lbDetail.SelectedPoolName()
	if poolID == "" {
		return m, nil
	}
	lbVIPAddress := ""
	if lb := m.lbDetail.LB(); lb != nil {
		lbVIPAddress = lb.VipAddress
	}
	existingMembers := m.lbDetail.SelectedPoolMembers()
	existingMemberAddrs := make([]string, 0, len(existingMembers))
	for _, member := range existingMembers {
		existingMemberAddrs = append(existingMemberAddrs, member.Address)
	}
	m.lbMemberCreate = lbmembercreate.New(m.client.LoadBalancer, m.client.Compute, poolID, poolName, lbVIPAddress, existingMemberAddrs)
	m.lbMemberCreate.SetSize(m.width, m.height)
	return m, m.lbMemberCreate.Init()
}

func (m Model) openLBMemberDeleteConfirm() (Model, tea.Cmd) {
	memberID := m.lbDetail.SelectedMemberID()
	memberName := m.lbDetail.SelectedMemberName()
	poolID := m.lbDetail.SelectedPoolID()
	if memberID == "" || poolID == "" {
		return m, nil
	}
	// Encode poolID|memberID in ServerID so it's captured at confirm time
	c := modal.NewConfirm("delete_lb_member", poolID+"|"+memberID, memberName)
	c.Title = "Delete Member"
	c.Body = fmt.Sprintf("Are you sure you want to remove member %q from the pool?", memberName)
	c.SetSize(m.width, m.height)
	m.confirm = c
	m.activeModal = modalConfirm
	return m, nil
}

// --- Key Pair actions ---

func (m Model) openKeypairDetail() (Model, tea.Cmd) {
	kp := m.keypairList.SelectedKeyPair()
	if kp == nil {
		return m, nil
	}
	m.keypairDetail = keypairdetail.New(m.client.Compute, kp.Name)
	m.keypairDetail.SetSize(m.width, m.height)
	m.view = viewKeypairDetail
	m.statusBar.CurrentView = "keypairdetail"
	m.statusBar.Hint = m.keypairDetail.Hints()
	return m, m.keypairDetail.Init()
}

func (m Model) openKeypairCreate() (Model, tea.Cmd) {
	m.keypairCreate = keypaircreate.New(m.client.Compute)
	m.keypairCreate.SetSize(m.width, m.height)
	m.view = viewKeypairCreate
	m.statusBar.CurrentView = "keypaircreate"
	m.statusBar.Hint = m.keypairCreate.Hints()
	return m, m.keypairCreate.Init()
}

func (m Model) openKeyPairDeleteConfirm() (Model, tea.Cmd) {
	var name string
	switch m.view {
	case viewKeypairList:
		if kp := m.keypairList.SelectedKeyPair(); kp != nil {
			name = kp.Name
		}
	case viewKeypairDetail:
		name = m.keypairDetail.KeyPairName()
	}
	if name == "" {
		return m, nil
	}
	m.confirm = modal.NewConfirm("delete_keypair", name, name)
	m.confirm.Title = "Delete Key Pair"
	m.confirm.Body = fmt.Sprintf("Are you sure you want to delete key pair %q?", name)
	m.confirm.SetSize(m.width, m.height)
	m.activeModal = modalConfirm
	return m, nil
}

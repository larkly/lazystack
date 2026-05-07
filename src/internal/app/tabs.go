package app

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/dnslist"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/imageview"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/lbview"
	"github.com/larkly/lazystack/internal/ui/networkview"
	"github.com/larkly/lazystack/internal/ui/routerview"
	"github.com/larkly/lazystack/internal/ui/secgroupview"
	"github.com/larkly/lazystack/internal/ui/volumelist"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TabDef describes a resource tab.
type TabDef struct {
	Name string // tab bar label (e.g., "Servers")
	Key  string // identifier (e.g., "servers", "volumes")
}

// DefaultTabs returns the default set of resource tabs.
func DefaultTabs() []TabDef {
	return []TabDef{
		{Name: "Servers", Key: "servers"},
		{Name: "Volumes", Key: "volumes"},
		{Name: "Images", Key: "images"},
		{Name: "Floating IPs", Key: "floatingips"},
		{Name: "Sec Groups", Key: "secgroups"},
		{Name: "Networks", Key: "networks"},
		{Name: "Key Pairs", Key: "keypairs"},
	}
}

func (m Model) isTopLevelView() bool {
	switch m.view {
	case viewServerList, viewVolumeList, viewFloatingIPList, viewSecGroupView, viewKeypairList, viewLBView, viewNetworkList, viewRouterView, viewImageView, viewDNSList:
		return true
	}
	return false
}

func (m Model) switchTab(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.tabs) {
		return m, nil
	}
	if idx == m.activeTab && m.isTopLevelView() {
		return m, nil
	}
	m.activeTab = idx
	td := m.tabs[idx]

	switch td.Key {
	case "servers":
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, nil

	case "volumes":
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
		if !m.tabInited[idx] {
			m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, m.refreshInterval)
			m.volumeList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.volumeList.Hints()
			return m, m.volumeList.Init()
		}
		m.statusBar.Hint = m.volumeList.Hints()
		return m, m.volumeList.ForceRefresh()

	case "floatingips":
		m.view = viewFloatingIPList
		m.statusBar.CurrentView = "floatingiplist"
		if !m.tabInited[idx] {
			m.floatingIPList = floatingiplist.New(m.client.Network, m.refreshInterval)
			m.floatingIPList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.floatingIPList.Hints()
			return m, m.floatingIPList.Init()
		}
		m.statusBar.Hint = m.floatingIPList.Hints()
		return m, m.floatingIPList.ForceRefresh()

	case "secgroups":
		m.view = viewSecGroupView
		m.statusBar.CurrentView = "secgroupview"
		if !m.tabInited[idx] {
			m.secGroupView = secgroupview.New(m.client.Network, m.refreshInterval)
			m.secGroupView.SetComputeClient(m.client.Compute)
			m.secGroupView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.secGroupView.Hints()
			return m, m.secGroupView.Init()
		}
		m.statusBar.Hint = m.secGroupView.Hints()
		return m, m.secGroupView.ForceRefresh()

	case "networks":
		m.view = viewNetworkList
		m.statusBar.CurrentView = "networkview"
		if !m.tabInited[idx] {
			m.networkView = networkview.New(m.client.Network, m.refreshInterval)
			m.networkView.SetComputeClient(m.client.Compute)
			m.networkView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.networkView.Hints()
			return m, m.networkView.Init()
		}
		m.statusBar.Hint = m.networkView.Hints()
		return m, m.networkView.ForceRefresh()

	case "loadbalancers":
		m.view = viewLBView
		m.statusBar.CurrentView = "lbview"
		if !m.tabInited[idx] {
			m.lbView = lbview.New(m.client.LoadBalancer, m.refreshInterval)
			m.lbView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.lbView.Hints()
			return m, m.lbView.Init()
		}
		m.statusBar.Hint = m.lbView.Hints()
		return m, m.lbView.ForceRefresh()

	case "routers":
		m.view = viewRouterView
		m.statusBar.CurrentView = "routerview"
		if !m.tabInited[idx] {
			shared.Debugf("[tabs] routers: first activation, calling Init()")
			m.routerView = routerview.New(m.client.Network, m.refreshInterval)
			m.routerView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.routerView.Hints()
			return m, m.routerView.Init()
		}
		shared.Debugf("[tabs] routers: re-activation, calling ForceRefresh()")
		m.statusBar.Hint = m.routerView.Hints()
		return m, m.routerView.ForceRefresh()

	case "keypairs":
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
		if !m.tabInited[idx] {
			m.keypairList = keypairlist.New(m.client.Compute, m.refreshInterval)
			m.keypairList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.keypairList.Hints()
			return m, m.keypairList.Init()
		}
		m.statusBar.Hint = m.keypairList.Hints()
		return m, m.keypairList.ForceRefresh()

	case "images":
		m.view = viewImageView
		m.statusBar.CurrentView = "imageview"
		if !m.tabInited[idx] {
			m.imageView = imageview.New(m.client.Image, m.client.Compute, m.refreshInterval)
			m.imageView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.imageView.Hints()
			return m, m.imageView.Init()
		}
		m.statusBar.Hint = m.imageView.Hints()
		return m, m.imageView.ForceRefresh()

	case "dns":
		m.view = viewDNSList
		m.statusBar.CurrentView = "dnslist"
		if !m.tabInited[idx] {
			m.dnsList = dnslist.New(m.client.DNS)
			m.dnsList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.dnsList.Hints()
			return m, m.dnsList.Init()
		}
		m.statusBar.Hint = m.dnsList.Hints()
		return m, m.dnsList.ForceRefresh()
	}
	return m, nil
}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, td := range m.tabs {
		label := fmt.Sprintf(" %d:%s ", i+1, td.Name)
		if i == m.activeTab {
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

package app

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/floatingiplist"
	"github.com/larkly/lazystack/internal/ui/imagelist"
	"github.com/larkly/lazystack/internal/ui/keypairlist"
	"github.com/larkly/lazystack/internal/ui/lblist"
	"github.com/larkly/lazystack/internal/ui/networklist"
	"github.com/larkly/lazystack/internal/ui/routerlist"
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
	case viewServerList, viewVolumeList, viewFloatingIPList, viewSecGroupView, viewKeypairList, viewLBList, viewNetworkList, viewRouterList, viewImageList:
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
		return m, nil

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
		return m, nil

	case "secgroups":
		m.view = viewSecGroupView
		m.statusBar.CurrentView = "secgroupview"
		if !m.tabInited[idx] {
			m.secGroupView = secgroupview.New(m.client.Network, m.refreshInterval)
			m.secGroupView.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.secGroupView.Hints()
			return m, m.secGroupView.Init()
		}
		m.statusBar.Hint = m.secGroupView.Hints()
		return m, nil

	case "networks":
		m.view = viewNetworkList
		m.statusBar.CurrentView = "networklist"
		if !m.tabInited[idx] {
			m.networkList = networklist.New(m.client.Network, m.refreshInterval)
			m.networkList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.networkList.Hints()
			return m, m.networkList.Init()
		}
		m.statusBar.Hint = m.networkList.Hints()
		return m, nil

	case "loadbalancers":
		m.view = viewLBList
		m.statusBar.CurrentView = "lblist"
		if !m.tabInited[idx] {
			m.lbList = lblist.New(m.client.LoadBalancer, m.refreshInterval)
			m.lbList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.lbList.Hints()
			return m, m.lbList.Init()
		}
		m.statusBar.Hint = m.lbList.Hints()
		return m, nil

	case "routers":
		m.view = viewRouterList
		m.statusBar.CurrentView = "routerlist"
		if !m.tabInited[idx] {
			m.routerList = routerlist.New(m.client.Network, m.refreshInterval)
			m.routerList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.routerList.Hints()
			return m, m.routerList.Init()
		}
		m.statusBar.Hint = m.routerList.Hints()
		return m, nil

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
		return m, nil

	case "images":
		m.view = viewImageList
		m.statusBar.CurrentView = "imagelist"
		if !m.tabInited[idx] {
			m.imageList = imagelist.New(m.client.Image, m.refreshInterval)
			m.imageList.SetSize(m.width, m.height)
			m.tabInited[idx] = true
			m.statusBar.Hint = m.imageList.Hints()
			return m, m.imageList.Init()
		}
		m.statusBar.Hint = m.imageList.Hints()
		return m, nil
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

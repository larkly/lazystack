package app

import (
	"fmt"
	"strings"

	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/ui/floatingiplist"
	"github.com/bosse/lazystack/internal/ui/keypairlist"
	"github.com/bosse/lazystack/internal/ui/secgroupview"
	"github.com/bosse/lazystack/internal/ui/volumelist"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) isTopLevelView() bool {
	switch m.view {
	case viewServerList, viewVolumeList, viewFloatingIPList, viewSecGroupView, viewKeypairList:
		return true
	}
	return false
}

func (m Model) switchTab(tab activeTab) (Model, tea.Cmd) {
	if tab == m.activeTab && m.isTopLevelView() {
		return m, nil
	}
	m.activeTab = tab

	switch tab {
	case tabServers:
		m.view = viewServerList
		m.statusBar.CurrentView = "serverlist"
		m.statusBar.Hint = m.serverList.Hints()
		return m, nil

	case tabVolumes:
		m.view = viewVolumeList
		m.statusBar.CurrentView = "volumelist"
		if !m.tabsInited[tabVolumes] {
			m.volumeList = volumelist.New(m.client.BlockStorage, m.client.Compute, m.refreshInterval)
			m.volumeList.SetSize(m.width, m.height)
			m.tabsInited[tabVolumes] = true
			m.statusBar.Hint = m.volumeList.Hints()
			return m, m.volumeList.Init()
		}
		m.statusBar.Hint = m.volumeList.Hints()
		return m, nil

	case tabFloatingIPs:
		m.view = viewFloatingIPList
		m.statusBar.CurrentView = "floatingiplist"
		if !m.tabsInited[tabFloatingIPs] {
			m.floatingIPList = floatingiplist.New(m.client.Network, m.refreshInterval)
			m.floatingIPList.SetSize(m.width, m.height)
			m.tabsInited[tabFloatingIPs] = true
			m.statusBar.Hint = m.floatingIPList.Hints()
			return m, m.floatingIPList.Init()
		}
		m.statusBar.Hint = m.floatingIPList.Hints()
		return m, nil

	case tabSecGroups:
		m.view = viewSecGroupView
		m.statusBar.CurrentView = "secgroupview"
		if !m.tabsInited[tabSecGroups] {
			m.secGroupView = secgroupview.New(m.client.Network, m.refreshInterval)
			m.secGroupView.SetSize(m.width, m.height)
			m.tabsInited[tabSecGroups] = true
			m.statusBar.Hint = m.secGroupView.Hints()
			return m, m.secGroupView.Init()
		}
		m.statusBar.Hint = m.secGroupView.Hints()
		return m, nil

	case tabKeypairs:
		m.view = viewKeypairList
		m.statusBar.CurrentView = "keypairlist"
		if !m.tabsInited[tabKeypairs] {
			m.keypairList = keypairlist.New(m.client.Compute)
			m.keypairList.SetSize(m.width, m.height)
			m.tabsInited[tabKeypairs] = true
			m.statusBar.Hint = m.keypairList.Hints()
			return m, m.keypairList.Init()
		}
		m.statusBar.Hint = m.keypairList.Hints()
		return m, nil
	}
	return m, nil
}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if activeTab(i) == m.activeTab {
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

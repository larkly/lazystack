package app

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
)

func (m Model) viewName() string {
	switch m.view {
	case viewCloudPicker:
		return "cloudpicker"
	case viewServerList:
		return "serverlist"
	case viewServerDetail:
		return "serverdetail"
	case viewServerCreate:
		return "servercreate"
	case viewConsoleLog:
		return "consolelog"
	case viewActionLog:
		return "actionlog"
	case viewVolumeList:
		return "volumelist"
	case viewVolumeDetail:
		return "volumedetail"
	case viewVolumeCreate:
		return "volumecreate"
	case viewFloatingIPList:
		return "floatingiplist"
	case viewSecGroupView:
		return "secgroupview"
	case viewKeypairList:
		return "keypairlist"
	case viewNetworkList:
		return "networkview"
	case viewKeypairCreate:
		return "keypaircreate"
	case viewKeypairDetail:
		return "keypairdetail"
	case viewRouterView:
		return "routerview"
	case viewLBView:
		return "lbview"
	case viewImageView:
		return "imageview"
	case viewHypervisorList:
		return "hypervisorlist"
	}
	return ""
}

// View renders the full UI.
func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m Model) viewContent() string {
	if m.tooSmall {
		msg := fmt.Sprintf("Terminal too small (%dx%d). Need at least %dx%d.",
			m.width, m.height, m.minWidth, m.minHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(msg))
	}

	if m.quotaView.Visible {
		return m.quotaView.Render()
	}

	if m.configView.Visible {
		return m.configView.Render()
	}

	if m.help.Visible {
		return m.help.Render()
	}

	if v, ok := m.activeModalView(); ok {
		return v
	}

	var content string
	switch m.view {
	case viewCloudPicker:
		if m.autoCloud != "" {
			msg := shared.StyleModalTitle.Render("Connecting to " + m.autoCloud + "...")
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
		}
		return m.cloudPicker.View()
	case viewServerList:
		content = m.serverList.View()
	case viewServerDetail:
		content = m.serverDetail.View()
	case viewServerCreate:
		content = m.serverCreate.View()
	case viewConsoleLog:
		content = m.consoleLog.View()
	case viewActionLog:
		content = m.actionLog.View()
	case viewVolumeList:
		content = m.volumeList.View()
	case viewVolumeDetail:
		content = m.volumeDetail.View()
	case viewVolumeCreate:
		content = m.volumeCreate.View()
	case viewFloatingIPList:
		content = m.floatingIPList.View()
	case viewSecGroupView:
		content = m.secGroupView.View()
	case viewKeypairList:
		content = m.keypairList.View()
	case viewNetworkList:
		content = m.networkView.View()
	case viewKeypairCreate:
		content = m.keypairCreate.View()
	case viewKeypairDetail:
		content = m.keypairDetail.View()
	case viewRouterView:
		content = m.routerView.View()
	case viewLBView:
		content = m.lbView.View()
	case viewImageView:
		content = m.imageView.View()
	}

	// Add tab bar for top-level views
	if m.isTopLevelView() {
		content = m.renderTabBar() + "\n" + content
	}

	// Overlay app name + version on top-right (lines 0 and 1)
	appName := lipgloss.NewStyle().
		Foreground(shared.ColorBg).
		Background(shared.ColorPrimary).
		Bold(true).
		Padding(0, 1).
		Render("LAZYSTACK")
	versionStr := ""
	if m.version != "" {
		versionStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(m.version)
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]
		firstW := lipgloss.Width(firstLine)
		nameW := lipgloss.Width(appName)
		pad := m.width - firstW - nameW
		if pad > 0 {
			lines[0] = firstLine + strings.Repeat(" ", pad) + appName
		}
	}
	if len(lines) > 1 && versionStr != "" {
		secondLine := lines[1]
		secondW := lipgloss.Width(secondLine)
		verW := lipgloss.Width(versionStr)
		pad := m.width - secondW - verW
		if pad > 0 {
			lines[1] = secondLine + strings.Repeat(" ", pad) + versionStr
		}
	}
	if len(lines) > 2 && m.latestVersion != "" {
		var indicator string
		if shared.PlainMode {
			indicator = lipgloss.NewStyle().Foreground(shared.ColorWarning).
				Render("(update: " + m.latestVersion + ")")
		} else {
			indicator = lipgloss.NewStyle().Foreground(shared.ColorWarning).
				Render("⚡ " + m.latestVersion + " available")
		}
		thirdLine := lines[2]
		thirdW := lipgloss.Width(thirdLine)
		indW := lipgloss.Width(indicator)
		pad := m.width - thirdW - indW
		if pad > 0 {
			lines[2] = thirdLine + strings.Repeat(" ", pad) + indicator
		}
	}
	content = strings.Join(lines, "\n")

	contentHeight := m.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	padded := lipgloss.NewStyle().Height(contentHeight).MaxHeight(contentHeight).Render(content)
	return padded + "\n" + m.statusBar.Render()
}

package app

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	// Overlay priority chain — overlays are mutually exclusive by design
	// (each action activates at most one), so first-match ordering is safe.
	if m.activeModal == modalConfirm {
		return m.confirm.View()
	}
	if m.activeModal == modalError {
		return m.errModal.View()
	}
	if m.fipPicker.Active {
		return m.fipPicker.View()
	}
	if m.serverPicker.Active {
		return m.serverPicker.View()
	}
	if m.volumePicker.Active {
		return m.volumePicker.View()
	}
	if m.routerCreate.Active {
		return m.routerCreate.View()
	}
	if m.subnetPicker.Active {
		return m.subnetPicker.View()
	}
	if m.networkCreate.Active {
		return m.networkCreate.View()
	}
	if m.subnetCreate.Active {
		return m.subnetCreate.View()
	}
	if m.subnetEdit.Active {
		return m.subnetEdit.View()
	}
	if m.portCreate.Active {
		return m.portCreate.View()
	}
	if m.portEdit.Active {
		return m.portEdit.View()
	}
	if m.sgCreate.Active {
		return m.sgCreate.View()
	}
	if m.sgRuleCreate.Active {
		return m.sgRuleCreate.View()
	}
	if m.imageEdit.Active {
		return m.imageEdit.View()
	}
	if m.imageCreate.Active {
		return m.imageCreate.View()
	}
	if m.imageDownload.Active {
		return m.imageDownload.View()
	}
	if m.lbCreate.Active {
		return m.lbCreate.View()
	}
	if m.lbListenerCreate.Active {
		return m.lbListenerCreate.View()
	}
	if m.lbPoolCreate.Active {
		return m.lbPoolCreate.View()
	}
	if m.lbMemberCreate.Active {
		return m.lbMemberCreate.View()
	}
	if m.lbMonitorCreate.Active {
		return m.lbMonitorCreate.View()
	}
	if m.projectPicker.Active {
		return m.projectPicker.View()
	}
	if m.serverRename.Active {
		return m.serverRename.View()
	}
	if m.serverRebuild.Active {
		return m.serverRebuild.View()
	}
	if m.serverSnapshot.Active {
		return m.serverSnapshot.View()
	}
	if m.serverResize.Active {
		return m.serverResize.View()
	}
	if m.cloneProgress.Active {
		return m.cloneProgress.View()
	}
	if m.sshPrompt.Active {
		return m.sshPrompt.View()
	}
	if m.copyPicker.Active {
		return m.copyPicker.View()
	}
	if m.consoleURL.Active {
		return m.consoleURL.View()
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

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
	case viewServiceCatalog:
		return "servicecatalog"
	case viewDNSList:
		return "dnslist"
	case viewUserManagement:
		return "usermanagement"
	case viewAuditLog:
		return "auditlog"
	}
	return ""
}

// activeViewContent renders the active resource view (without chrome).
// It reports false when the active view has no render case.
func (m Model) activeViewContent() (string, bool) {
	switch m.view {
	case viewServerList:
		return m.serverList.View(), true
	case viewServerDetail:
		return m.serverDetail.View(), true
	case viewServerCreate:
		return m.serverCreate.View(), true
	case viewConsoleLog:
		return m.consoleLog.View(), true
	case viewActionLog:
		return m.actionLog.View(), true
	case viewVolumeList:
		return m.volumeList.View(), true
	case viewVolumeDetail:
		return m.volumeDetail.View(), true
	case viewVolumeCreate:
		return m.volumeCreate.View(), true
	case viewFloatingIPList:
		return m.floatingIPList.View(), true
	case viewSecGroupView:
		return m.secGroupView.View(), true
	case viewKeypairList:
		return m.keypairList.View(), true
	case viewNetworkList:
		return m.networkView.View(), true
	case viewKeypairCreate:
		return m.keypairCreate.View(), true
	case viewKeypairDetail:
		return m.keypairDetail.View(), true
	case viewRouterView:
		return m.routerView.View(), true
	case viewLBView:
		return m.lbView.View(), true
	case viewImageView:
		return m.imageView.View(), true
	case viewHypervisorList:
		return m.hypervisorList.View(), true
	case viewServiceCatalog:
		return m.serviceCatalog.View(), true
	case viewDNSList:
		return m.dnsList.View(), true
	case viewUserManagement:
		return m.userManagement.View(), true
	case viewAuditLog:
		return m.auditLog.View(), true
	}
	return "", false
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

	if m.view == viewCloudPicker {
		if m.autoCloud != "" {
			msg := shared.StyleModalTitle.Render("Connecting to " + m.autoCloud + "...")
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
		}
		return m.cloudPicker.View()
	}
	content, _ := m.activeViewContent()

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

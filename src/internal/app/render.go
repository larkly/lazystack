package app

import (
	"fmt"
	"strings"

	"github.com/bosse/lazystack/internal/shared"
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
	case viewFloatingIPList:
		return "floatingiplist"
	case viewSecGroupView:
		return "secgroupview"
	case viewKeypairList:
		return "keypairlist"
	case viewLBList:
		return "lblist"
	case viewLBDetail:
		return "lbdetail"
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

	if m.help.Visible {
		return m.help.Render()
	}

	if m.activeModal == modalConfirm {
		return m.confirm.View()
	}
	if m.activeModal == modalError {
		return m.errModal.View()
	}
	if m.fipPicker.Active {
		return m.fipPicker.View()
	}
	if m.serverResize.Active {
		return m.serverResize.View()
	}

	var content string
	switch m.view {
	case viewCloudPicker:
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
	case viewFloatingIPList:
		content = m.floatingIPList.View()
	case viewSecGroupView:
		content = m.secGroupView.View()
	case viewKeypairList:
		content = m.keypairList.View()
	case viewLBList:
		content = m.lbList.View()
	case viewLBDetail:
		content = m.lbDetail.View()
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
	content = strings.Join(lines, "\n")

	contentHeight := m.height - 1
	if contentHeight < 0 {
		contentHeight = 0
	}

	padded := lipgloss.NewStyle().Height(contentHeight).Render(content)
	return padded + "\n" + m.statusBar.Render()
}

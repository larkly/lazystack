package sgdetail

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type focusPane int

const (
	focusInfo focusPane = iota
	focusRules
	focusServers
	focusPorts
)

const focusPaneCount = 4
const narrowThreshold = 80

type sgDetailLoadedMsg struct {
	sg         *network.SecurityGroup
	groupNames map[string]string
}
type sgDetailErrMsg struct{ err error }
type serversLoadedMsg struct {
	servers []serverRef
	ports   []network.Port
}
type serversErrMsg struct{ err error }
type detailTickMsg struct{}

type serverRef struct {
	ID     string
	Name   string
	Status string
}

// Model is the security group detail dashboard view.
type Model struct {
	networkClient *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient
	sgID          string
	sg            *network.SecurityGroup
	groupNames    map[string]string // all SG ID -> name for resolving remote group references
	loading       bool
	spinner       spinner.Model
	width         int
	height        int
	err           string
	refreshInterval time.Duration
	focus         focusPane

	// Rules pane
	ruleCursor  int
	rulesScroll int

	// Servers pane
	servers        []serverRef
	serverCursor   int
	serversScroll  int
	serversLoading bool
	serversErr     string

	// Ports pane
	ports       []network.Port
	portsCursor int
	portsScroll int
}

// New creates a security group detail model.
func New(networkClient, computeClient *gophercloud.ServiceClient, sgID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		networkClient:   networkClient,
		computeClient:   computeClient,
		sgID:            sgID,
		loading:         true,
		serversLoading:  true,
		spinner:         s,
		refreshInterval: refreshInterval,
	}
}

// Init fetches all data sources.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchSG(),
		m.fetchServers(),
		m.tickCmd(),
	)
}

// SGID returns the current security group ID.
func (m Model) SGID() string {
	return m.sgID
}

// SGName returns the current security group name.
func (m Model) SGName() string {
	if m.sg != nil {
		return m.sg.Name
	}
	return m.sgID
}

// SGDescription returns the current security group description.
func (m Model) SGDescription() string {
	if m.sg != nil {
		return m.sg.Description
	}
	return ""
}

// SelectedRuleID returns the ID of the currently selected rule, or "" if none.
func (m Model) SelectedRuleID() string {
	if m.focus != focusRules || m.sg == nil {
		return ""
	}
	rules := m.sortedRules()
	if m.ruleCursor < 0 || m.ruleCursor >= len(rules) {
		return ""
	}
	return rules[m.ruleCursor].ID
}

// SelectedServerID returns the ID of the currently selected server, or "" if none.
func (m Model) SelectedServerID() string {
	if m.focus != focusServers {
		return ""
	}
	if m.serverCursor < 0 || m.serverCursor >= len(m.servers) {
		return ""
	}
	return m.servers[m.serverCursor].ID
}

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() focusPane {
	return m.focus
}

// FocusRules is the focusRules constant for external comparison.
const FocusRules = focusRules

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sgDetailLoadedMsg:
		m.loading = false
		m.sg = msg.sg
		m.groupNames = msg.groupNames
		m.err = ""
		// Clamp cursor
		rules := m.sortedRules()
		if m.ruleCursor >= len(rules) {
			m.ruleCursor = max(0, len(rules)-1)
		}
		return m, nil

	case sgDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case serversLoadedMsg:
		m.serversLoading = false
		m.serversErr = ""
		m.servers = msg.servers
		m.ports = msg.ports
		if m.serverCursor >= len(m.servers) {
			m.serverCursor = max(0, len(m.servers)-1)
		}
		if m.portsCursor >= len(m.ports) {
			m.portsCursor = max(0, len(m.ports)-1)
		}
		return m, nil

	case serversErrMsg:
		m.serversLoading = false
		m.serversErr = msg.err.Error()
		return m, nil

	case detailTickMsg:
		return m, tea.Batch(m.fetchSG(), m.fetchServers(), m.tickCmd())

	case spinner.TickMsg:
		if m.loading || m.serversLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "secgroupview"}
		}

	case key.Matches(msg, shared.Keys.Tab):
		m.focus = (m.focus + 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + focusPaneCount - 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		m.scrollUp(1)
		return m, nil

	case key.Matches(msg, shared.Keys.Down):
		m.scrollDown(1)
		return m, nil

	case key.Matches(msg, shared.Keys.PageUp):
		m.scrollUp(10)
		return m, nil

	case key.Matches(msg, shared.Keys.PageDown):
		m.scrollDown(10)
		return m, nil
	}
	return m, nil
}

func (m *Model) scrollUp(n int) {
	switch m.focus {
	case focusInfo:
		// no scrolling for info pane
	case focusRules:
		m.ruleCursor -= n
		if m.ruleCursor < 0 {
			m.ruleCursor = 0
		}
		m.ensureRuleCursorVisible()
	case focusServers:
		m.serverCursor -= n
		if m.serverCursor < 0 {
			m.serverCursor = 0
		}
		m.ensureServerCursorVisible()
	case focusPorts:
		m.portsCursor -= n
		if m.portsCursor < 0 {
			m.portsCursor = 0
		}
		m.ensurePortCursorVisible()
	}
}

func (m *Model) scrollDown(n int) {
	switch m.focus {
	case focusInfo:
		// no scrolling for info pane
	case focusRules:
		rules := m.sortedRules()
		m.ruleCursor += n
		maxIdx := len(rules) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.ruleCursor > maxIdx {
			m.ruleCursor = maxIdx
		}
		m.ensureRuleCursorVisible()
	case focusServers:
		m.serverCursor += n
		maxIdx := len(m.servers) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.serverCursor > maxIdx {
			m.serverCursor = maxIdx
		}
		m.ensureServerCursorVisible()
	case focusPorts:
		m.portsCursor += n
		maxIdx := len(m.ports) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.portsCursor > maxIdx {
			m.portsCursor = maxIdx
		}
		m.ensurePortCursorVisible()
	}
}

func (m *Model) ensureRuleCursorVisible() {
	visibleLines := m.rulesVisibleLines()
	if m.ruleCursor < m.rulesScroll {
		m.rulesScroll = m.ruleCursor
	}
	if m.ruleCursor >= m.rulesScroll+visibleLines {
		m.rulesScroll = m.ruleCursor - visibleLines + 1
	}
}

func (m *Model) ensureServerCursorVisible() {
	visibleLines := m.serversVisibleLines()
	if m.serverCursor < m.serversScroll {
		m.serversScroll = m.serverCursor
	}
	if m.serverCursor >= m.serversScroll+visibleLines {
		m.serversScroll = m.serverCursor - visibleLines + 1
	}
}

func (m *Model) ensurePortCursorVisible() {
	visibleLines := m.portsVisibleLines()
	if m.portsCursor < m.portsScroll {
		m.portsScroll = m.portsCursor
	}
	if m.portsCursor >= m.portsScroll+visibleLines {
		m.portsScroll = m.portsCursor - visibleLines + 1
	}
}

func (m Model) rulesVisibleLines() int {
	totalH := m.panelHeight()
	topH := totalH * 55 / 100
	if topH < 6 {
		topH = 6
	}
	// Subtract 4 for border, 1 for header row
	lines := topH - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) serversVisibleLines() int {
	totalH := m.panelHeight()
	topH := totalH * 55 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := totalH - topH
	if bottomH < 4 {
		bottomH = 4
	}
	lines := bottomH - 4
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) portsVisibleLines() int {
	return m.serversVisibleLines()
}

func (m Model) panelHeight() int {
	h := m.height - 4 // title + action bar + padding
	if h < 4 {
		h = 4
	}
	return h
}

// View renders the security group detail dashboard.
func (m Model) View() string {
	var b strings.Builder

	// Title
	titleText := "Security Group"
	if m.sg != nil {
		titleText = m.sg.Name
	}
	title := shared.StyleTitle.Render(titleText)
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if m.sg == nil && m.loading {
		return b.String()
	}

	if m.width < narrowThreshold {
		b.WriteString(m.renderNarrow())
	} else {
		b.WriteString(m.renderWide())
	}

	b.WriteString(m.renderActionBar() + "\n")

	return b.String()
}

func (m Model) renderWide() string {
	totalH := m.panelHeight()

	// Top row: info (35%) | rules (65%), 1 gap
	topH := totalH * 55 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := totalH - topH
	if bottomH < 4 {
		bottomH = 4
	}

	leftW := m.width * 35 / 100
	rightW := m.width - leftW - 1

	// Top row panels
	infoContent := padContent(m.panelTitle(focusInfo), m.renderInfoContent(leftW-4))
	infoPanel := m.panelBorder(focusInfo).
		Width(leftW).
		Height(topH).
		Render(infoContent)

	rulesContent := padContent(m.panelTitle(focusRules), m.renderRulesContent(rightW-4, topH-4))
	rulesPanel := m.panelBorder(focusRules).
		Width(rightW).
		Height(topH).
		Render(rulesContent)

	// Bottom row: servers (50%) | ports (50%), 1 gap
	bottomLeftW := m.width / 2
	bottomRightW := m.width - bottomLeftW - 1

	serversContent := padContent(m.panelTitle(focusServers), m.renderServersContent(bottomLeftW-4, bottomH-4))
	serversPanel := m.panelBorder(focusServers).
		Width(bottomLeftW).
		Height(bottomH).
		Render(serversContent)

	portsContent := padContent(m.panelTitle(focusPorts), m.renderPortsContent(bottomRightW-4, bottomH-4))
	portsPanel := m.panelBorder(focusPorts).
		Width(bottomRightW).
		Height(bottomH).
		Render(portsContent)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", rulesPanel)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, serversPanel, " ", portsPanel)
	return topRow + "\n" + bottomRow + "\n"
}

func (m Model) renderNarrow() string {
	totalH := m.panelHeight()
	w := m.width - 2

	infoH := totalH * 25 / 100
	rulesH := totalH * 35 / 100
	serversH := totalH * 20 / 100
	portsH := totalH - infoH - rulesH - serversH

	if infoH < 4 {
		infoH = 4
	}
	if rulesH < 4 {
		rulesH = 4
	}
	if serversH < 3 {
		serversH = 3
	}
	if portsH < 3 {
		portsH = 3
	}

	infoContent := padContent(m.panelTitle(focusInfo), m.renderInfoContent(w-4))
	infoPanel := m.panelBorder(focusInfo).Width(w).Height(infoH).Render(infoContent)

	rulesContent := padContent(m.panelTitle(focusRules), m.renderRulesContent(w-4, rulesH-4))
	rulesPanel := m.panelBorder(focusRules).Width(w).Height(rulesH).Render(rulesContent)

	serversContent := padContent(m.panelTitle(focusServers), m.renderServersContent(w-4, serversH-4))
	serversPanel := m.panelBorder(focusServers).Width(w).Height(serversH).Render(serversContent)

	portsContent := padContent(m.panelTitle(focusPorts), m.renderPortsContent(w-4, portsH-4))
	portsPanel := m.panelBorder(focusPorts).Width(w).Height(portsH).Render(portsContent)

	return lipgloss.JoinVertical(lipgloss.Left, infoPanel, rulesPanel, serversPanel, portsPanel) + "\n"
}

// padContent adds the title, a blank line, and 1-char indent to each line of content.
func padContent(title, content string) string {
	var out []string
	out = append(out, " "+title)
	out = append(out, "") // blank line after title
	if content != "" {
		for _, l := range strings.Split(content, "\n") {
			out = append(out, " "+l)
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) panelTitle(pane focusPane) string {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}
	titleStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)

	switch pane {
	case focusInfo:
		return titleStyle.Render("Info")
	case focusRules:
		t := titleStyle.Render("Rules")
		if m.loading {
			t += " " + m.spinner.View()
		}
		return t
	case focusServers:
		t := titleStyle.Render("Servers")
		if m.serversLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusPorts:
		t := titleStyle.Render("Ports")
		if m.serversLoading {
			t += " " + m.spinner.View()
		}
		return t
	}
	return ""
}

func (m Model) panelBorder(pane focusPane) lipgloss.Style {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true)
}

func (m Model) renderInfoContent(maxWidth int) string {
	if m.sg == nil {
		return ""
	}

	sg := m.sg
	labelW := 8
	labelStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).
		Bold(true).
		Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type prop struct {
		label string
		value string
	}

	// Count ingress/egress
	var ingress, egress int
	for _, r := range sg.Rules {
		if r.Direction == "ingress" {
			ingress++
		} else {
			egress++
		}
	}
	rulesVal := fmt.Sprintf("%d", len(sg.Rules))
	if len(sg.Rules) > 0 {
		rulesVal += fmt.Sprintf(" (%d in, %d out)", ingress, egress)
	}

	allProps := []prop{
		{"Name", sg.Name},
		{"Desc", sg.Description},
		{"ID", sg.ID},
		{"Rules", rulesVal},
	}

	valW := maxWidth - labelW
	if valW < 4 {
		valW = 4
	}

	var rows []string
	for _, p := range allProps {
		if p.value == "" {
			continue
		}
		val := p.value
		if lipgloss.Width(val) > valW {
			val = val[:valW-1] + "\u2026"
		}
		rows = append(rows, labelStyle.Render(p.label)+valueStyle.Render(val))
	}

	return strings.Join(rows, "\n")
}

func (m Model) sortedRules() []network.SecurityRule {
	if m.sg == nil {
		return nil
	}
	rules := make([]network.SecurityRule, len(m.sg.Rules))
	copy(rules, m.sg.Rules)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Direction != rules[j].Direction {
			// ingress before egress
			return rules[i].Direction == "ingress"
		}
		return rules[i].Protocol < rules[j].Protocol
	})
	return rules
}

func (m Model) isDefaultEgressRule(r network.SecurityRule) bool {
	return r.Direction == "egress" &&
		r.Protocol == "" &&
		r.RemoteIPPrefix == "" &&
		r.RemoteGroupID == "" &&
		r.PortRangeMin == 0 &&
		r.PortRangeMax == 0
}

func (m Model) renderRulesContent(maxWidth, maxHeight int) string {
	rules := m.sortedRules()

	if len(rules) == 0 {
		return shared.StyleHelp.Render("No rules \u2014 Ctrl+N to add")
	}

	// Column widths: prefix(2) + dir(8) + proto(5) + ports(7) + ether(5) + spacing(4) = 31 fixed
	// Remote gets the rest
	const (
		dirW   = 8
		protoW = 5
		portsW = 7
		etherW = 4
	)
	remoteW := maxWidth - 2 - dirW - protoW - portsW - etherW - 4 // 4 for inter-column spaces
	if remoteW < 6 {
		remoteW = 6
	}

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
		dirW, "Dir", protoW, "Proto", portsW, "Ports", remoteW, "Remote", "Eth")
	headerLine := headerStyle.Render(header)

	// Visible lines for rules (subtract 1 for header)
	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	dirStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	etherStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	mutedLine := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, r := range rules {
		if i < m.rulesScroll {
			continue
		}
		if i >= m.rulesScroll+visibleLines {
			break
		}

		isDefault := m.isDefaultEgressRule(r)
		selected := m.focus == focusRules && i == m.ruleCursor

		proto := r.Protocol
		if proto == "" {
			proto = "any"
		}

		ports := ""
		if r.PortRangeMin == 0 && r.PortRangeMax == 0 {
			ports = "all"
		} else if r.PortRangeMin == r.PortRangeMax {
			ports = fmt.Sprintf("%d", r.PortRangeMin)
		} else {
			ports = fmt.Sprintf("%d-%d", r.PortRangeMin, r.PortRangeMax)
		}

		remote := r.RemoteIPPrefix
		if remote == "" && r.RemoteGroupID != "" {
			if name, ok := m.groupNames[r.RemoteGroupID]; ok {
				remote = "group:" + name
			} else {
				remote = "group:" + r.RemoteGroupID[:min(8, len(r.RemoteGroupID))] + "\u2026"
			}
		}
		if remote == "" {
			remote = "any"
		}
		if len(remote) > remoteW {
			remote = remote[:remoteW-1] + "\u2026"
		}

		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		line := fmt.Sprintf("%s%-*s %-*s %-*s %-*s %s",
			prefix,
			dirW, dirStyle.Render(r.Direction),
			protoW, proto,
			portsW, ports,
			remoteW, remote,
			etherStyle.Render(r.EtherType))

		if selected {
			line = selectedBg.Render(line)
		} else if isDefault {
			line = mutedLine.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderServersContent(maxWidth, maxHeight int) string {
	if m.serversErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.serversErr)
	}

	if len(m.servers) == 0 && !m.serversLoading {
		return shared.StyleHelp.Render("No servers using this group")
	}

	visibleLines := maxHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	// Dynamic name width: leave room for prefix(2) + space(1) + status icon+text(~10)
	nameW := maxWidth - 13
	if nameW < 10 {
		nameW = 10
	}

	var lines []string
	for i, srv := range m.servers {
		if i < m.serversScroll {
			continue
		}
		if i >= m.serversScroll+visibleLines {
			break
		}

		selected := m.focus == focusServers && i == m.serverCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := srv.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		status := shared.StatusIcon(srv.Status) + srv.Status
		line := fmt.Sprintf("%s%-*s %s", prefix, nameW, name, status)

		if selected {
			line = selectedBg.Render(line)
		}

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderPortsContent(maxWidth, maxHeight int) string {
	if m.serversErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.serversErr)
	}

	if len(m.ports) == 0 && !m.serversLoading {
		return shared.StyleHelp.Render("No ports found")
	}

	visibleLines := maxHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)
	macStyle := lipgloss.NewStyle().Foreground(shared.ColorFg).Bold(true)
	ipStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	// Each port takes 2 lines: MAC+status, then IPs
	var lines []string
	lineIdx := 0
	for i, p := range m.ports {
		if lineIdx+1 < m.portsScroll {
			lineIdx += 2
			continue
		}
		if lineIdx-m.portsScroll >= visibleLines {
			break
		}

		selected := m.focus == focusPorts && i == m.portsCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		// Line 1: MAC + status
		mac := p.MACAddress
		if len(mac) > maxWidth-14 {
			mac = mac[:maxWidth-15] + "\u2026"
		}
		line1 := fmt.Sprintf("%s%s %s", prefix, macStyle.Render(mac), p.Status)

		// Line 2: IPs (indented)
		var ips []string
		for _, ip := range p.FixedIPs {
			ips = append(ips, ip.IPAddress)
		}
		ipStr := strings.Join(ips, ", ")
		if len(ipStr) > maxWidth-6 {
			ipStr = ipStr[:maxWidth-7] + "\u2026"
		}
		line2 := "    " + ipStyle.Render(ipStr)
		if len(ips) == 0 {
			line2 = "    " + ipStyle.Render("no IPs")
		}

		if selected {
			line1 = selectedBg.Render(line1)
		}

		if lineIdx >= m.portsScroll {
			lines = append(lines, line1)
		}
		lineIdx++
		if lineIdx >= m.portsScroll && lineIdx-m.portsScroll < visibleLines {
			lines = append(lines, line2)
		}
		lineIdx++
		_ = i
	}

	return strings.Join(lines, "\n")
}

type actionButton struct {
	key   string
	label string
}

func btn(k, label string) actionButton {
	return actionButton{key: k, label: label}
}

func (m Model) renderActionBar() string {
	if m.sg == nil {
		return ""
	}

	var buttons []actionButton
	buttons = append(buttons, btn("^n", "Add Rule"))

	isDefault := m.sg.Name == "default"

	if m.focus == focusRules && m.SelectedRuleID() != "" {
		buttons = append(buttons, btn("^d", "Delete Rule"))
	} else if !isDefault {
		buttons = append(buttons, btn("^d", "Delete Group"))
	}

	if !isDefault {
		buttons = append(buttons, btn("r", "Rename"))
	}
	buttons = append(buttons, btn("c", "Clone"))

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).
		Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	var parts []string
	totalLen := 0
	maxWidth := m.width - 4

	for _, b := range buttons {
		part := keyStyle.Render("["+b.key+"]") + labelStyle.Render(b.label)
		partLen := len("["+b.key+"]") + len(b.label) + 1
		if totalLen+partLen > maxWidth && len(parts) > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("[?]More"))
			break
		}
		parts = append(parts, part)
		totalLen += partLen
	}

	return " " + strings.Join(parts, " ")
}

// ForceRefresh triggers a manual reload of all data sources.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	m.serversLoading = true
	return tea.Batch(m.spinner.Tick, m.fetchSG(), m.fetchServers())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "tab focus \u2022 \u2191\u2193 navigate \u2022 ^n add rule \u2022 ^d delete \u2022 esc back \u2022 ? help"
}

func (m Model) fetchSG() tea.Cmd {
	networkClient := m.networkClient
	sgID := m.sgID
	return func() tea.Msg {
		sg, err := network.GetSecurityGroup(context.Background(), networkClient, sgID)
		if err != nil {
			return sgDetailErrMsg{err: err}
		}
		// Fetch all SGs to build groupNames map for remote group resolution
		allSGs, err := network.ListSecurityGroups(context.Background(), networkClient)
		if err != nil {
			// Non-fatal: just use empty names
			return sgDetailLoadedMsg{sg: sg, groupNames: nil}
		}
		names := make(map[string]string, len(allSGs))
		for _, g := range allSGs {
			names[g.ID] = g.Name
		}
		return sgDetailLoadedMsg{sg: sg, groupNames: names}
	}
}

func (m Model) fetchServers() tea.Cmd {
	networkClient := m.networkClient
	computeClient := m.computeClient
	sgID := m.sgID
	return func() tea.Msg {
		// Get all ports using this security group
		fetchedPorts, err := network.ListPortsBySecurityGroup(context.Background(), networkClient, sgID)
		if err != nil {
			return serversErrMsg{err: err}
		}

		// Collect unique server device IDs
		deviceIDs := make(map[string]bool)
		for _, p := range fetchedPorts {
			if strings.HasPrefix(p.DeviceOwner, "compute:") && p.DeviceID != "" {
				deviceIDs[p.DeviceID] = true
			}
		}

		if len(deviceIDs) == 0 {
			return serversLoadedMsg{servers: nil, ports: fetchedPorts}
		}

		// Fetch all servers and filter to matching ones
		allServers, err := compute.ListServers(context.Background(), computeClient)
		if err != nil {
			return serversErrMsg{err: err}
		}

		serverMap := make(map[string]compute.Server, len(allServers))
		for _, s := range allServers {
			serverMap[s.ID] = s
		}

		var refs []serverRef
		for id := range deviceIDs {
			if s, ok := serverMap[id]; ok {
				refs = append(refs, serverRef{
					ID:     s.ID,
					Name:   s.Name,
					Status: s.Status,
				})
			}
		}

		// Sort by name
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].Name < refs[j].Name
		})

		return serversLoadedMsg{servers: refs, ports: fetchedPorts}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return detailTickMsg{}
	})
}

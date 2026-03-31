package networkview

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
	FocusSelector focusPane = iota
	focusInfo
	FocusSubnets
	focusPorts
)

const focusPaneCount = 4
const narrowThreshold = 80

type networksLoadedMsg struct {
	networks    []network.Network
	allSubnets  map[string]network.Subnet // ID → Subnet
	externalIDs map[string]bool
}
type networksErrMsg struct{ err error }
type detailLoadedMsg struct {
	netID       string
	ports       []network.Port
	serverNames map[string]string
	sgNames     map[string]string
}
type detailErrMsg struct {
	netID string
	err   error
}

// Model is the combined network selector + detail view.
type Model struct {
	networkClient *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient

	// Selector state
	networks       []network.Network
	allSubnets     map[string]network.Subnet // ID → Subnet (all subnets)
	externalIDs    map[string]bool
	cursor         int
	selectorScroll int

	// Detail state for currently selected network
	ports       []network.Port
	serverNames map[string]string // DeviceID → server name
	sgNames     map[string]string // SG ID → name
	detailErr   string

	// Pane focus and cursors
	focus         focusPane
	subnetCursor  int
	subnetsScroll int
	portsCursor   int
	portsScroll   int

	// UI state
	width           int
	height          int
	loading         bool
	detailLoading   bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
	highlightNames  map[string]bool
	lastDetailNetID string
}

// New creates a network view model.
func New(networkClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		networkClient:   networkClient,
		loading:         true,
		spinner:         s,
		allSubnets:      make(map[string]network.Subnet),
		refreshInterval: refreshInterval,
	}
}

// SetComputeClient sets the compute client for server name resolution.
func (m *Model) SetComputeClient(client *gophercloud.ServiceClient) {
	m.computeClient = client
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[networkview] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchNetworks())
}

func (m Model) selectedNetwork() *network.Network {
	if m.cursor >= 0 && m.cursor < len(m.networks) {
		return &m.networks[m.cursor]
	}
	return nil
}

// SelectedNetworkID returns the ID of the currently selected network.
func (m Model) SelectedNetworkID() string {
	if n := m.selectedNetwork(); n != nil {
		return n.ID
	}
	return ""
}

// SelectedNetworkName returns the name of the currently selected network.
func (m Model) SelectedNetworkName() string {
	if n := m.selectedNetwork(); n != nil {
		return n.Name
	}
	return ""
}

// SelectedSubnetID returns the ID of the currently selected subnet, or "".
func (m Model) SelectedSubnetID() string {
	if m.focus != FocusSubnets {
		return ""
	}
	subs := m.networkSubnets()
	if m.subnetCursor < 0 || m.subnetCursor >= len(subs) {
		return ""
	}
	return subs[m.subnetCursor].ID
}

// SelectedSubnetName returns the name of the currently selected subnet.
func (m Model) SelectedSubnetName() string {
	if m.focus != FocusSubnets {
		return ""
	}
	subs := m.networkSubnets()
	if m.subnetCursor < 0 || m.subnetCursor >= len(subs) {
		return ""
	}
	name := subs[m.subnetCursor].Name
	if name == "" {
		id := subs[m.subnetCursor].ID
		if len(id) > 8 {
			return id[:8] + "..."
		}
		return id
	}
	return name
}

// SelectedSubnet returns the full Subnet object for the currently selected subnet.
func (m Model) SelectedSubnet() *network.Subnet {
	if m.focus != FocusSubnets {
		return nil
	}
	subs := m.networkSubnets()
	if m.subnetCursor < 0 || m.subnetCursor >= len(subs) {
		return nil
	}
	return &subs[m.subnetCursor]
}

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() focusPane {
	return m.focus
}

// InSubnets returns true when the subnets pane is focused.
func (m Model) InSubnets() bool {
	return m.focus == FocusSubnets
}

// InPorts returns true when the ports pane is focused.
func (m Model) InPorts() bool {
	return m.focus == focusPorts
}

// SelectedPort returns the full Port object for the currently selected port.
func (m Model) SelectedPort() *network.Port {
	if m.focus != focusPorts {
		return nil
	}
	if m.portsCursor < 0 || m.portsCursor >= len(m.ports) {
		return nil
	}
	return &m.ports[m.portsCursor]
}

// SelectedPortID returns the ID of the currently selected port.
func (m Model) SelectedPortID() string {
	if p := m.SelectedPort(); p != nil {
		return p.ID
	}
	return ""
}

// SGNames returns the security group name map.
func (m Model) SGNames() map[string]string {
	return m.sgNames
}

// NetworkSubnets returns subnets belonging to the selected network (exported).
func (m Model) NetworkSubnets() []network.Subnet {
	return m.networkSubnets()
}

// networkSubnets returns subnets belonging to the selected network.
func (m Model) networkSubnets() []network.Subnet {
	n := m.selectedNetwork()
	if n == nil {
		return nil
	}
	var subs []network.Subnet
	for _, id := range n.SubnetIDs {
		if sub, ok := m.allSubnets[id]; ok {
			subs = append(subs, sub)
		}
	}
	return subs
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case networksLoadedMsg:
		shared.Debugf("[networkview] networksLoadedMsg: %d networks", len(msg.networks))
		var cursorID string
		if m.cursor >= 0 && m.cursor < len(m.networks) {
			cursorID = m.networks[m.cursor].ID
		}
		m.loading = false
		m.networks = msg.networks
		m.allSubnets = msg.allSubnets
		m.externalIDs = msg.externalIDs
		m.err = ""
		// Restore cursor position
		if cursorID != "" {
			for i, n := range m.networks {
				if n.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.networks) && len(m.networks) > 0 {
			m.cursor = len(m.networks) - 1
		}
		m.applyHighlightNames()
		// Fetch detail for selected network if changed
		if n := m.selectedNetwork(); n != nil && n.ID != m.lastDetailNetID {
			m.lastDetailNetID = n.ID
			m.resetDetailState()
			return m, m.fetchDetail(n.ID)
		}
		return m, nil

	case networksErrMsg:
		shared.Debugf("[networkview] networksErrMsg: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case detailLoadedMsg:
		shared.Debugf("[networkview] detailLoadedMsg: %d ports", len(msg.ports))
		if n := m.selectedNetwork(); n != nil && n.ID == msg.netID {
			m.detailLoading = false
			m.detailErr = ""
			m.ports = msg.ports
			m.serverNames = msg.serverNames
			m.sgNames = msg.sgNames
			m.clampDetailCursors()
		}
		return m, nil

	case detailErrMsg:
		shared.Debugf("[networkview] detailErrMsg: %v", msg.err)
		if n := m.selectedNetwork(); n != nil && n.ID == msg.netID {
			m.detailLoading = false
			m.detailErr = msg.err.Error()
		}
		return m, nil

	case shared.TickMsg:
		if m.loading {
			shared.Debugf("[networkview] tick skipped (loading)")
			return m, nil
		}
		shared.Debugf("[networkview] tick fetching")
		cmds := []tea.Cmd{m.fetchNetworks()}
		if n := m.selectedNetwork(); n != nil {
			cmds = append(cmds, m.fetchDetail(n.ID))
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.loading || m.detailLoading {
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

func (m *Model) resetDetailState() {
	m.detailLoading = true
	m.detailErr = ""
	m.ports = nil
	m.serverNames = nil
	m.sgNames = nil
	m.subnetCursor = 0
	m.subnetsScroll = 0
	m.portsCursor = 0
	m.portsScroll = 0
}

func (m *Model) clampDetailCursors() {
	subs := m.networkSubnets()
	if m.subnetCursor >= len(subs) {
		m.subnetCursor = max(0, len(subs)-1)
	}
	if m.portsCursor >= len(m.ports) {
		m.portsCursor = max(0, len(m.ports)-1)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Tab):
		m.focus = (m.focus + 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + focusPaneCount - 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		return m.scrollUp(1)

	case key.Matches(msg, shared.Keys.Down):
		return m.scrollDown(1)

	case key.Matches(msg, shared.Keys.PageUp):
		return m.scrollUp(10)

	case key.Matches(msg, shared.Keys.PageDown):
		return m.scrollDown(10)
	}
	return m, nil
}

func (m Model) scrollUp(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor -= n
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case focusInfo:
		// no scrolling
	case FocusSubnets:
		m.subnetCursor -= n
		if m.subnetCursor < 0 {
			m.subnetCursor = 0
		}
		m.ensureSubnetCursorVisible()
	case focusPorts:
		m.portsCursor -= n
		if m.portsCursor < 0 {
			m.portsCursor = 0
		}
		m.ensurePortCursorVisible()
	}
	return m, nil
}

func (m Model) scrollDown(n int) (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor += n
		maxIdx := len(m.networks) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
		m.ensureSelectorCursorVisible()
		return m.onSelectorChange()
	case focusInfo:
		// no scrolling
	case FocusSubnets:
		subs := m.networkSubnets()
		m.subnetCursor += n
		maxIdx := len(subs) - 1
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.subnetCursor > maxIdx {
			m.subnetCursor = maxIdx
		}
		m.ensureSubnetCursorVisible()
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
	return m, nil
}

func (m Model) onSelectorChange() (Model, tea.Cmd) {
	n := m.selectedNetwork()
	if n == nil || n.ID == m.lastDetailNetID {
		return m, nil
	}
	m.lastDetailNetID = n.ID
	m.resetDetailState()
	return m, m.fetchDetail(n.ID)
}

func (m *Model) ensureSelectorCursorVisible() {
	visH := m.selectorVisibleLines()
	if m.cursor < m.selectorScroll {
		m.selectorScroll = m.cursor
	}
	if m.cursor >= m.selectorScroll+visH {
		m.selectorScroll = m.cursor - visH + 1
	}
}

func (m *Model) ensureSubnetCursorVisible() {
	visH := m.subnetsVisibleLines()
	if m.subnetCursor < m.subnetsScroll {
		m.subnetsScroll = m.subnetCursor
	}
	if m.subnetCursor >= m.subnetsScroll+visH {
		m.subnetsScroll = m.subnetCursor - visH + 1
	}
}

func (m *Model) ensurePortCursorVisible() {
	visH := m.portsVisibleLines()
	if m.portsCursor < m.portsScroll {
		m.portsScroll = m.portsCursor
	}
	if m.portsCursor >= m.portsScroll+visH {
		m.portsScroll = m.portsCursor - visH + 1
	}
}

// --- Height calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 8
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) selectorHeight() int {
	h := len(m.networks) + 2
	if h < 3 {
		h = 3
	}
	maxH := m.totalPanelHeight() * 30 / 100
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	return h
}

func (m Model) selectorVisibleLines() int {
	lines := m.selectorHeight() - 2
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) detailHeight() int {
	return m.totalPanelHeight() - m.selectorHeight()
}

func (m Model) subnetsVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 50 / 100
	if topH < 6 {
		topH = 6
	}
	lines := topH - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) portsVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 50 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := dh - topH
	if bottomH < 4 {
		bottomH = 4
	}
	lines := bottomH - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

// --- View ---

func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Networks")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.networks))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.networks) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No networks found.") + "\n")
		return b.String()
	}

	if m.width < narrowThreshold {
		b.WriteString(m.renderNarrow())
	} else {
		b.WriteString(m.renderWide())
	}

	b.WriteString("\n" + m.renderActionBar() + "\n")

	return b.String()
}

func (m Model) renderWide() string {
	selH := m.selectorHeight()
	dh := m.detailHeight()

	// Selector: full width
	selContent := m.renderSelectorContent(m.width-4, selH-2)
	selPanel := m.panelBorder(FocusSelector).
		Width(m.width).
		Height(selH).
		Render(padContent(m.panelTitle(FocusSelector), selContent))

	// Detail top row: info (35%) | subnets (65%)
	topH := dh * 50 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := dh - topH
	if bottomH < 4 {
		bottomH = 4
	}

	leftW := m.width * 35 / 100
	rightW := m.width - leftW - 1

	infoContent := padContent(m.panelTitle(focusInfo), m.renderInfoContent(leftW-4))
	infoPanel := m.panelBorder(focusInfo).Width(leftW).Height(topH).Render(infoContent)

	subnetsContent := padContent(m.panelTitle(FocusSubnets), m.renderSubnetsContent(rightW-4, topH-4))
	subnetsPanel := m.panelBorder(FocusSubnets).Width(rightW).Height(topH).Render(subnetsContent)

	// Detail bottom row: ports (full width)
	portsContent := padContent(m.panelTitle(focusPorts), m.renderPortsContent(m.width-4, bottomH-4))
	portsPanel := m.panelBorder(focusPorts).Width(m.width).Height(bottomH).Render(portsContent)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", subnetsPanel)

	return selPanel + "\n" + topRow + "\n" + portsPanel
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	selH := m.selectorHeight()
	remaining := totalH - selH
	infoH := remaining * 25 / 100
	subnetsH := remaining * 35 / 100
	portsH := remaining - infoH - subnetsH

	if infoH < 4 {
		infoH = 4
	}
	if subnetsH < 4 {
		subnetsH = 4
	}
	if portsH < 4 {
		portsH = 4
	}

	selContent := m.renderSelectorContent(w-4, selH-2)
	selPanel := m.panelBorder(FocusSelector).Width(w).Height(selH).Render(padContent(m.panelTitle(FocusSelector), selContent))

	infoPanel := m.panelBorder(focusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(focusInfo), m.renderInfoContent(w-4)))
	subnetsPanel := m.panelBorder(FocusSubnets).Width(w).Height(subnetsH).Render(padContent(m.panelTitle(FocusSubnets), m.renderSubnetsContent(w-4, subnetsH-4)))
	portsPanel := m.panelBorder(focusPorts).Width(w).Height(portsH).Render(padContent(m.panelTitle(focusPorts), m.renderPortsContent(w-4, portsH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, selPanel, infoPanel, subnetsPanel, portsPanel)
}

// --- Panel helpers ---

func padContent(title, content string) string {
	var out []string
	out = append(out, " "+title)
	out = append(out, "")
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
	case FocusSelector:
		return titleStyle.Render("Networks")
	case focusInfo:
		return titleStyle.Render("Info")
	case FocusSubnets:
		t := titleStyle.Render("Subnets")
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusPorts:
		t := titleStyle.Render("Ports")
		if m.detailLoading {
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
		BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true)
}

// --- Selector rendering ---

func (m Model) renderSelectorContent(maxWidth, maxHeight int) string {
	if len(m.networks) == 0 {
		return ""
	}

	visibleLines := maxHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	for i, net := range m.networks {
		if i < m.selectorScroll {
			continue
		}
		if i >= m.selectorScroll+visibleLines {
			break
		}

		selected := i == m.cursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if selected {
			nameStyle = nameStyle.Foreground(shared.ColorHighlight).Bold(true)
		}

		subCount := len(net.SubnetIDs)
		subLabel := "subnets"
		if subCount == 1 {
			subLabel = "subnet"
		}

		statusStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		if net.Status != "ACTIVE" {
			statusStyle = statusStyle.Foreground(shared.ColorWarning)
		}
		statusStr := statusStyle.Render(shared.StatusIcon(net.Status) + net.Status)

		meta := fmt.Sprintf(" (%d %s)", subCount, subLabel)
		if net.Shared {
			meta += " shared"
		}
		if m.externalIDs[net.ID] {
			meta += " external"
		}

		line := prefix + nameStyle.Render(net.Name) + shared.StyleHelp.Render(meta) + "  " + statusStr
		if lipgloss.Width(line) > maxWidth+2 {
			line = line[:maxWidth+1]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Info rendering ---

func (m Model) renderInfoContent(maxWidth int) string {
	n := m.selectedNetwork()
	if n == nil {
		return ""
	}

	labelW := 10
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type prop struct{ label, value string }

	statusStr := shared.StatusIcon(n.Status) + n.Status
	sharedStr := "no"
	if n.Shared {
		sharedStr = "yes"
	}
	externalStr := "no"
	if m.externalIDs[n.ID] {
		externalStr = "yes"
	}

	allProps := []prop{
		{"Name", n.Name},
		{"ID", n.ID},
		{"Status", statusStr},
		{"Shared", sharedStr},
		{"External", externalStr},
		{"Subnets", fmt.Sprintf("%d", len(n.SubnetIDs))},
		{"Ports", fmt.Sprintf("%d", len(m.ports))},
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
		rendered := valueStyle.Render(val)
		if p.label == "Status" {
			statusColor := shared.ColorSuccess
			if n.Status != "ACTIVE" {
				statusColor = shared.ColorWarning
			}
			rendered = lipgloss.NewStyle().Foreground(statusColor).Render(val)
		}
		rows = append(rows, labelStyle.Render(p.label)+rendered)
	}
	return strings.Join(rows, "\n")
}

// --- Subnets rendering ---

func (m Model) renderSubnetsContent(maxWidth, maxHeight int) string {
	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}
	subs := m.networkSubnets()
	if len(subs) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No subnets \u2014 Ctrl+N to add")
	}

	const gap = 2

	// Calculate column widths
	nameW := len("Name")
	cidrW := len("CIDR")
	gwW := len("Gateway")
	for _, s := range subs {
		if len(s.Name) > nameW {
			nameW = len(s.Name)
		}
		if len(s.CIDR) > cidrW {
			cidrW = len(s.CIDR)
		}
		if len(s.GatewayIP) > gwW {
			gwW = len(s.GatewayIP)
		}
	}
	maxNameW := maxWidth / 3
	if maxNameW < 8 {
		maxNameW = 8
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}
	if cidrW > 20 {
		cidrW = 20
	}
	if gwW > 20 {
		gwW = 20
	}

	sep := strings.Repeat(" ", gap)
	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%-*s%sIPv%sDHCP",
		nameW, "Name", sep, cidrW, "CIDR", sep, gwW, "Gateway", sep, sep)
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, s := range subs {
		if i < m.subnetsScroll {
			continue
		}
		if i >= m.subnetsScroll+visibleLines {
			break
		}

		selected := m.focus == FocusSubnets && i == m.subnetCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		name := s.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		cidr := s.CIDR
		if len(cidr) > cidrW {
			cidr = cidr[:cidrW-1] + "\u2026"
		}

		gw := s.GatewayIP
		if gw == "" {
			gw = "\u2014"
		}
		if len(gw) > gwW {
			gw = gw[:gwW-1] + "\u2026"
		}

		dhcp := "off"
		if s.EnableDHCP {
			dhcp = "on"
		}

		line := fmt.Sprintf("%s%-*s%s%-*s%s%-*s%s%-3d%s%s",
			prefix, nameW, name, sep, cidrW, cidr, sep, gwW, gw, sep, s.IPVersion, sep, dhcp)

		if selected {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)

		// Show details for the selected subnet
		if selected {
			detailStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
			if s.IPv6AddressMode != "" {
				lines = append(lines, detailStyle.Render("      mode: "+s.IPv6AddressMode))
			}
			for _, pool := range s.AllocationPools {
				lines = append(lines, detailStyle.Render(fmt.Sprintf("      pool: %s \u2192 %s", pool.Start, pool.End)))
			}
			if len(s.DNSNameservers) > 0 {
				lines = append(lines, detailStyle.Render("      dns:  "+strings.Join(s.DNSNameservers, ", ")))
			}
			for _, r := range s.HostRoutes {
				lines = append(lines, detailStyle.Render(fmt.Sprintf("      route: %s \u2192 %s", r.DestinationCIDR, r.NextHop)))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// --- Ports rendering ---

func (m Model) renderPortsContent(maxWidth, maxHeight int) string {
	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}
	if len(m.ports) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No ports found")
	}

	const (
		statusW = 9
		macW    = 17
		gap     = 2
	)

	// Calculate dynamic column widths
	ipsW := len("IPs")
	for _, p := range m.ports {
		ipStr := m.portIPStr(p)
		if len(ipStr) > ipsW {
			ipsW = len(ipStr)
		}
	}

	deviceW := len("Device")
	for _, p := range m.ports {
		name := m.deviceName(p)
		if len(name) > deviceW {
			deviceW = len(name)
		}
	}
	if deviceW > 20 {
		deviceW = 20
	}

	ownerW := len("Owner")
	for _, p := range m.ports {
		owner := shortOwner(p.DeviceOwner)
		if len(owner) > ownerW {
			ownerW = len(owner)
		}
	}
	if ownerW > 15 {
		ownerW = 15
	}

	// Clamp IPs width to remaining space
	maxIPs := maxWidth - 2 - statusW - deviceW - ownerW - macW - gap*4
	if maxIPs < 10 {
		maxIPs = 10
	}
	if ipsW > maxIPs {
		ipsW = maxIPs
	}

	sep := strings.Repeat(" ", gap)
	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%-*s%s%-*s%s%-*s",
		statusW, "Status", sep, ipsW, "IPs", sep, deviceW, "Device", sep, ownerW, "Owner", sep, macW, "MAC")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, p := range m.ports {
		if i < m.portsScroll {
			continue
		}
		if i >= m.portsScroll+visibleLines {
			break
		}

		selected := m.focus == focusPorts && i == m.portsCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		// Build status string as plain text for alignment, color the whole line
		statusIcon := shared.StatusIcon(p.Status)
		statusPlain := fmt.Sprintf("%-*s", statusW, statusIcon+p.Status)

		ipStr := m.portIPStr(p)
		if len(ipStr) > ipsW {
			ipStr = ipStr[:ipsW-1] + "\u2026"
		}

		device := m.deviceName(p)
		if len(device) > deviceW {
			device = device[:deviceW-1] + "\u2026"
		}

		owner := shortOwner(p.DeviceOwner)
		if len(owner) > ownerW {
			owner = owner[:ownerW-1] + "\u2026"
		}

		mac := p.MACAddress

		plainLine := fmt.Sprintf("%s%-*s%s%-*s%s%-*s%s%-*s%s%-*s",
			prefix, statusW, statusPlain, sep, ipsW, ipStr, sep, deviceW, device, sep, ownerW, owner, sep, macW, mac)

		if selected {
			lines = append(lines, selectedBg.Render(plainLine))
		} else {
			// Color the status portion
			statusColor := shared.ColorSuccess
			if p.Status != "ACTIVE" {
				statusColor = shared.ColorWarning
			}
			if !p.AdminStateUp {
				statusColor = shared.ColorError
			}
			coloredStatus := lipgloss.NewStyle().Foreground(statusColor).Render(statusPlain)
			rest := fmt.Sprintf("%s%-*s%s%-*s%s%-*s%s%-*s",
				sep, ipsW, ipStr, sep, deviceW, device, sep, ownerW, owner, sep, macW, mac)
			lines = append(lines, prefix+coloredStatus+rest)
		}

		// Show extra detail for the selected port
		if selected {
			detailStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
			adminStr := "up"
			if !p.AdminStateUp {
				adminStr = lipgloss.NewStyle().Foreground(shared.ColorError).Render("down")
			}
			detailLine := fmt.Sprintf("      ID: %s  Admin: %s", p.ID[:min(8, len(p.ID))]+"\u2026", adminStr)
			if p.Name != "" {
				detailLine += "  Name: " + p.Name
			}
			lines = append(lines, detailStyle.Render(detailLine))

			// Show security groups
			if len(p.SecurityGroups) > 0 {
				var sgStrs []string
				for _, sgID := range p.SecurityGroups {
					if name, ok := m.sgNames[sgID]; ok {
						sgStrs = append(sgStrs, name)
					} else if len(sgID) > 8 {
						sgStrs = append(sgStrs, sgID[:8]+"\u2026")
					} else {
						sgStrs = append(sgStrs, sgID)
					}
				}
				sgLine := "      SGs: " + strings.Join(sgStrs, ", ")
				lines = append(lines, detailStyle.Render(sgLine))
			}

			// Show port security and allowed address pairs
			psStr := "on"
			if !p.PortSecurityEnabled {
				psStr = lipgloss.NewStyle().Foreground(shared.ColorWarning).Render("off")
			}
			psLine := "      Port Security: " + psStr
			lines = append(lines, detailStyle.Render(psLine))

			if len(p.AllowedAddressPairs) > 0 {
				var apStrs []string
				for _, ap := range p.AllowedAddressPairs {
					s := ap.IPAddress
					if ap.MACAddress != "" {
						s += " (" + ap.MACAddress + ")"
					}
					apStrs = append(apStrs, s)
				}
				apLine := "      Allowed Pairs: " + strings.Join(apStrs, ", ")
				lines = append(lines, detailStyle.Render(apLine))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) portIPStr(p network.Port) string {
	if len(p.FixedIPs) == 0 {
		return "\u2014"
	}
	var ips []string
	for _, ip := range p.FixedIPs {
		ips = append(ips, ip.IPAddress)
	}
	return strings.Join(ips, ", ")
}

func (m Model) deviceName(p network.Port) string {
	if name, ok := m.serverNames[p.DeviceID]; ok && name != "" {
		return name
	}
	if p.DeviceID != "" {
		if len(p.DeviceID) > 8 {
			return p.DeviceID[:8] + "\u2026"
		}
		return p.DeviceID
	}
	return "\u2014"
}

func shortOwner(owner string) string {
	// Simplify device_owner strings like "network:dhcp" → "dhcp", "compute:nova" → "compute"
	if strings.HasPrefix(owner, "network:") {
		return strings.TrimPrefix(owner, "network:")
	}
	if strings.HasPrefix(owner, "compute:") {
		return "compute"
	}
	if owner == "" {
		return "\u2014"
	}
	return owner
}

// --- Action bar ---

type actionButton struct {
	key   string
	label string
}

func btn(k, label string) actionButton {
	return actionButton{key: k, label: label}
}

func (m Model) renderActionBar() string {
	n := m.selectedNetwork()
	if n == nil {
		return ""
	}

	var buttons []actionButton

	switch m.focus {
	case FocusSelector, focusInfo:
		buttons = append(buttons, btn("^n", "New Network"))
		buttons = append(buttons, btn("^d", "Delete Network"))
	case FocusSubnets:
		buttons = append(buttons, btn("^n", "New Subnet"))
		if m.SelectedSubnetID() != "" {
			buttons = append(buttons, btn("enter", "Edit Subnet"))
			buttons = append(buttons, btn("^d", "Delete Subnet"))
		}
	case focusPorts:
		buttons = append(buttons, btn("^n", "New Port"))
		if m.SelectedPortID() != "" {
			buttons = append(buttons, btn("enter", "Edit Port"))
			buttons = append(buttons, btn("^d", "Delete Port"))
		}
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorHighlight).
		Background(shared.ColorSecondary).
		Bold(true).Padding(0, 0)
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	var parts []string
	totalLen := 0
	maxActionWidth := m.width - 4

	for _, b := range buttons {
		part := keyStyle.Render("["+b.key+"]") + labelStyle.Render(b.label)
		partLen := len("["+b.key+"]") + len(b.label) + 1
		if totalLen+partLen > maxActionWidth && len(parts) > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("[?]More"))
			break
		}
		parts = append(parts, part)
		totalLen += partLen
	}

	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// --- Public methods ---

// ForceRefresh triggers a manual reload.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[networkview] ForceRefresh()")
	m.loading = true
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchNetworks()}
	if n := m.selectedNetwork(); n != nil {
		m.detailLoading = true
		cmds = append(cmds, m.fetchDetail(n.ID))
	}
	return tea.Batch(cmds...)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ScrollToNames positions the cursor on the first matching network name.
func (m *Model) ScrollToNames(names []string) {
	m.highlightNames = make(map[string]bool, len(names))
	for _, n := range names {
		m.highlightNames[n] = true
	}
	m.applyHighlightNames()
}

func (m *Model) applyHighlightNames() {
	if len(m.highlightNames) == 0 {
		return
	}
	for i, net := range m.networks {
		if m.highlightNames[net.Name] {
			m.cursor = i
			m.ensureSelectorCursorVisible()
			m.highlightNames = nil
			if net.ID != m.lastDetailNetID {
				m.lastDetailNetID = net.ID
				m.resetDetailState()
			}
			return
		}
	}
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	switch m.focus {
	case FocusSubnets:
		return "\u2191\u2193 navigate \u2022 enter edit \u2022 ^n add subnet \u2022 ^d delete \u2022 tab focus \u2022 R refresh \u2022 ? help"
	case focusPorts:
		return "\u2191\u2193 navigate \u2022 enter edit \u2022 ^n create port \u2022 ^d delete \u2022 tab focus \u2022 R refresh \u2022 ? help"
	case focusInfo:
		return "tab focus \u2022 ^n new \u2022 ^d delete \u2022 R refresh \u2022 ? help"
	default:
		return "\u2191\u2193 navigate \u2022 ^n new \u2022 ^d delete \u2022 tab focus \u2022 R refresh \u2022 ? help"
	}
}

// --- Data fetching ---

func (m Model) fetchNetworks() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		shared.Debugf("[networkview] fetchNetworks start")
		nets, err := network.ListNetworks(context.Background(), client)
		if err != nil {
			shared.Debugf("[networkview] fetchNetworks error: %v", err)
			return networksErrMsg{err: err}
		}
		subs, err := network.ListSubnets(context.Background(), client)
		if err != nil {
			return networksErrMsg{err: err}
		}
		subMap := make(map[string]network.Subnet, len(subs))
		for _, s := range subs {
			subMap[s.ID] = s
		}
		// Fetch external network IDs
		extNets, err := network.ListExternalNetworks(context.Background(), client)
		extIDs := make(map[string]bool)
		if err == nil {
			for _, en := range extNets {
				extIDs[en.ID] = true
			}
		}
		shared.Debugf("[networkview] fetchNetworks done: %d networks", len(nets))
		return networksLoadedMsg{networks: nets, allSubnets: subMap, externalIDs: extIDs}
	}
}

func (m Model) fetchDetail(netID string) tea.Cmd {
	networkClient := m.networkClient
	computeClient := m.computeClient
	return func() tea.Msg {
		shared.Debugf("[networkview] fetchDetail start")
		fetchedPorts, err := network.ListPorts(context.Background(), networkClient, netID)
		if err != nil {
			shared.Debugf("[networkview] fetchDetail error: %v", err)
			return detailErrMsg{netID: netID, err: err}
		}

		// Sort ports by device owner, then MAC
		sort.Slice(fetchedPorts, func(i, j int) bool {
			if fetchedPorts[i].DeviceOwner != fetchedPorts[j].DeviceOwner {
				return fetchedPorts[i].DeviceOwner < fetchedPorts[j].DeviceOwner
			}
			return fetchedPorts[i].MACAddress < fetchedPorts[j].MACAddress
		})

		// Resolve security group names
		sgIDs := make(map[string]bool)
		for _, p := range fetchedPorts {
			for _, sgID := range p.SecurityGroups {
				sgIDs[sgID] = true
			}
		}
		sgNameMap := make(map[string]string)
		if len(sgIDs) > 0 {
			sgs, err := network.ListSecurityGroups(context.Background(), networkClient)
			if err == nil {
				for _, sg := range sgs {
					if sgIDs[sg.ID] {
						sgNameMap[sg.ID] = sg.Name
					}
				}
			}
		}

		if computeClient == nil {
			shared.Debugf("[networkview] fetchDetail done: %d ports (no compute client)", len(fetchedPorts))
			return detailLoadedMsg{netID: netID, ports: fetchedPorts, sgNames: sgNameMap}
		}

		// Collect device IDs that look like compute instances
		deviceIDs := make(map[string]bool)
		for _, p := range fetchedPorts {
			if strings.HasPrefix(p.DeviceOwner, "compute:") && p.DeviceID != "" {
				deviceIDs[p.DeviceID] = true
			}
		}

		srvNames := make(map[string]string)
		if len(deviceIDs) > 0 {
			allServers, err := compute.ListServers(context.Background(), computeClient)
			if err == nil {
				for _, s := range allServers {
					if deviceIDs[s.ID] {
						srvNames[s.ID] = s.Name
					}
				}
			}
		}

		// Also resolve router device IDs
		for _, p := range fetchedPorts {
			if strings.HasPrefix(p.DeviceOwner, "network:router_interface") && p.DeviceID != "" {
				router, err := network.GetRouter(context.Background(), networkClient, p.DeviceID)
				if err == nil && router != nil {
					srvNames[p.DeviceID] = router.Name
				}
			}
		}

		shared.Debugf("[networkview] fetchDetail done: %d ports", len(fetchedPorts))
		return detailLoadedMsg{netID: netID, ports: fetchedPorts, serverNames: srvNames, sgNames: sgNameMap}
	}
}


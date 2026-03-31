package routerview

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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
	FocusInterfaces
	focusRoutes
)

const focusPaneCount = 4
const narrowThreshold = 80

type routersLoadedMsg struct{ routers []network.Router }
type routersErrMsg struct{ err error }
type namesLoadedMsg struct {
	networkNames map[string]string
	subnetToNet  map[string]string
}
type detailLoadedMsg struct {
	routerID   string
	interfaces []network.RouterInterface
}
type detailErrMsg struct {
	routerID string
	err      error
}

// Model is the combined router selector + detail view.
type Model struct {
	networkClient *gophercloud.ServiceClient

	// Selector state
	routers        []network.Router
	cursor         int
	selectorScroll int

	// Detail state for currently selected router
	interfaces   []network.RouterInterface
	networkNames map[string]string // network ID → name
	subnetToNet  map[string]string // subnet ID → network ID
	detailErr    string
	lastDetailID string

	// Pane focus and cursors
	focus           focusPane
	interfaceCursor int
	interfaceScroll int
	routesCursor    int
	routesScroll    int

	// UI state
	width           int
	height          int
	loading         bool
	detailLoading   bool
	spinner         spinner.Model
	err             string
	refreshInterval time.Duration
	highlightNames  map[string]bool
}

// New creates a router view model.
func New(networkClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		networkClient:   networkClient,
		loading:         true,
		spinner:         s,
		networkNames:    make(map[string]string),
		subnetToNet:     make(map[string]string),
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[routerview] Init()")
	return tea.Batch(m.spinner.Tick, m.fetchRouters(), m.fetchNames())
}

func (m Model) selectedRouter() *network.Router {
	if m.cursor >= 0 && m.cursor < len(m.routers) {
		return &m.routers[m.cursor]
	}
	return nil
}

// SelectedRouterID returns the ID of the currently selected router.
func (m Model) SelectedRouterID() string {
	if r := m.selectedRouter(); r != nil {
		return r.ID
	}
	return ""
}

// SelectedRouterName returns the name of the currently selected router.
func (m Model) SelectedRouterName() string {
	if r := m.selectedRouter(); r != nil {
		if r.Name != "" {
			return r.Name
		}
		return r.ID
	}
	return ""
}

// SelectedInterfaceSubnetID returns the subnet ID of the currently selected interface.
func (m Model) SelectedInterfaceSubnetID() string {
	if m.focus != FocusInterfaces {
		return ""
	}
	if m.interfaceCursor < 0 || m.interfaceCursor >= len(m.interfaces) {
		return ""
	}
	return m.interfaces[m.interfaceCursor].SubnetID
}

// FocusedPane returns the currently focused pane.
func (m Model) FocusedPane() focusPane {
	return m.focus
}

// InInterfaces returns true when the interfaces pane is focused.
func (m Model) InInterfaces() bool {
	return m.focus == FocusInterfaces
}

func (m *Model) resetDetailState() {
	m.interfaces = nil
	m.detailErr = ""
	m.detailLoading = true
	m.interfaceCursor = 0
	m.interfaceScroll = 0
	m.routesCursor = 0
	m.routesScroll = 0
}

func (m *Model) onSelectorChange() tea.Cmd {
	r := m.selectedRouter()
	if r == nil {
		return nil
	}
	if r.ID == m.lastDetailID {
		return nil
	}
	m.lastDetailID = r.ID
	m.resetDetailState()
	return m.fetchDetail(r.ID)
}

func (m *Model) clampDetailCursors() {
	if m.interfaceCursor >= len(m.interfaces) {
		m.interfaceCursor = max(0, len(m.interfaces)-1)
	}
	r := m.selectedRouter()
	if r != nil && m.routesCursor >= len(r.Routes) {
		m.routesCursor = max(0, len(r.Routes)-1)
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case routersLoadedMsg:
		shared.Debugf("[routerview] routersLoadedMsg: %d routers", len(msg.routers))
		var cursorID string
		if m.cursor >= 0 && m.cursor < len(m.routers) {
			cursorID = m.routers[m.cursor].ID
		}
		m.loading = false
		m.routers = msg.routers
		m.err = ""
		if cursorID != "" {
			for i, r := range m.routers {
				if r.ID == cursorID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.routers) && len(m.routers) > 0 {
			m.cursor = len(m.routers) - 1
		}
		m.applyHighlightNames()
		if r := m.selectedRouter(); r != nil && r.ID != m.lastDetailID {
			shared.Debugf("[routerview] routersLoaded: new selection %s, fetching detail", r.ID[:8])
			m.lastDetailID = r.ID
			m.resetDetailState()
			return m, m.fetchDetail(r.ID)
		}
		return m, nil

	case routersErrMsg:
		shared.Debugf("[routerview] routersErrMsg: %s", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case namesLoadedMsg:
		shared.Debugf("[routerview] namesLoadedMsg: %d networks, %d subnets", len(msg.networkNames), len(msg.subnetToNet))
		m.networkNames = msg.networkNames
		m.subnetToNet = msg.subnetToNet
		return m, nil

	case detailLoadedMsg:
		shared.Debugf("[routerview] detailLoadedMsg: router=%s ifaces=%d", msg.routerID[:8], len(msg.interfaces))
		if r := m.selectedRouter(); r != nil && r.ID == msg.routerID {
			m.detailLoading = false
			m.detailErr = ""
			m.interfaces = msg.interfaces
			m.clampDetailCursors()
		}
		return m, nil

	case detailErrMsg:
		shared.Debugf("[routerview] detailErrMsg: router=%s err=%s", msg.routerID[:8], msg.err)
		if r := m.selectedRouter(); r != nil && r.ID == msg.routerID {
			m.detailLoading = false
			m.detailErr = msg.err.Error()
		}
		return m, nil

	case shared.TickMsg:
		if m.loading {
			return m, nil
		}
		shared.Debugf("[routerview] tickMsg: fetching routers")
		cmds := []tea.Cmd{m.fetchRouters()}
		if r := m.selectedRouter(); r != nil {
			cmds = append(cmds, m.fetchDetail(r.ID))
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
		switch {
		case key.Matches(msg, shared.Keys.Tab):
			m.focus = (m.focus + 1) % focusPaneCount
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focus = (m.focus + focusPaneCount - 1) % focusPaneCount
			return m, nil

		case key.Matches(msg, shared.Keys.Up):
			return m.moveUp()
		case key.Matches(msg, shared.Keys.Down):
			return m.moveDown()
		case key.Matches(msg, shared.Keys.PageDown):
			return m.pageDown()
		case key.Matches(msg, shared.Keys.PageUp):
			return m.pageUp()
		}
	}
	return m, nil
}

func (m Model) moveUp() (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		if m.cursor > 0 {
			m.cursor--
			m.ensureSelectorCursorVisible()
			return m, m.onSelectorChange()
		}
	case FocusInterfaces:
		if m.interfaceCursor > 0 {
			m.interfaceCursor--
			m.ensureInterfaceCursorVisible()
		}
	case focusRoutes:
		if m.routesCursor > 0 {
			m.routesCursor--
			m.ensureRoutesCursorVisible()
		}
	}
	return m, nil
}

func (m Model) moveDown() (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		if m.cursor < len(m.routers)-1 {
			m.cursor++
			m.ensureSelectorCursorVisible()
			return m, m.onSelectorChange()
		}
	case FocusInterfaces:
		if m.interfaceCursor < len(m.interfaces)-1 {
			m.interfaceCursor++
			m.ensureInterfaceCursorVisible()
		}
	case focusRoutes:
		r := m.selectedRouter()
		if r != nil && m.routesCursor < len(r.Routes)-1 {
			m.routesCursor++
			m.ensureRoutesCursorVisible()
		}
	}
	return m, nil
}

func (m Model) pageDown() (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor += m.selectorVisibleLines()
		if m.cursor >= len(m.routers) {
			m.cursor = len(m.routers) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureSelectorCursorVisible()
		return m, m.onSelectorChange()
	case FocusInterfaces:
		m.interfaceCursor += m.interfacesVisibleLines()
		if m.interfaceCursor >= len(m.interfaces) {
			m.interfaceCursor = len(m.interfaces) - 1
		}
		if m.interfaceCursor < 0 {
			m.interfaceCursor = 0
		}
		m.ensureInterfaceCursorVisible()
	case focusRoutes:
		r := m.selectedRouter()
		if r != nil {
			m.routesCursor += m.routesVisibleLines()
			if m.routesCursor >= len(r.Routes) {
				m.routesCursor = len(r.Routes) - 1
			}
			if m.routesCursor < 0 {
				m.routesCursor = 0
			}
			m.ensureRoutesCursorVisible()
		}
	}
	return m, nil
}

func (m Model) pageUp() (Model, tea.Cmd) {
	switch m.focus {
	case FocusSelector:
		m.cursor -= m.selectorVisibleLines()
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureSelectorCursorVisible()
		return m, m.onSelectorChange()
	case FocusInterfaces:
		m.interfaceCursor -= m.interfacesVisibleLines()
		if m.interfaceCursor < 0 {
			m.interfaceCursor = 0
		}
		m.ensureInterfaceCursorVisible()
	case focusRoutes:
		m.routesCursor -= m.routesVisibleLines()
		if m.routesCursor < 0 {
			m.routesCursor = 0
		}
		m.ensureRoutesCursorVisible()
	}
	return m, nil
}

// --- Scroll helpers ---

func (m *Model) ensureSelectorCursorVisible() {
	vis := m.selectorVisibleLines()
	if m.cursor < m.selectorScroll {
		m.selectorScroll = m.cursor
	}
	if m.cursor >= m.selectorScroll+vis {
		m.selectorScroll = m.cursor - vis + 1
	}
}

func (m *Model) ensureInterfaceCursorVisible() {
	vis := m.interfacesVisibleLines()
	if m.interfaceCursor < m.interfaceScroll {
		m.interfaceScroll = m.interfaceCursor
	}
	if m.interfaceCursor >= m.interfaceScroll+vis {
		m.interfaceScroll = m.interfaceCursor - vis + 1
	}
}

func (m *Model) ensureRoutesCursorVisible() {
	vis := m.routesVisibleLines()
	if m.routesCursor < m.routesScroll {
		m.routesScroll = m.routesCursor
	}
	if m.routesCursor >= m.routesScroll+vis {
		m.routesScroll = m.routesCursor - vis + 1
	}
}

// --- Layout calculations ---

func (m Model) totalPanelHeight() int {
	h := m.height - 6
	if h < 10 {
		h = 10
	}
	return h
}

func (m Model) selectorHeight() int {
	h := m.totalPanelHeight() * 30 / 100
	if h > 7 {
		h = 7
	}
	if h < 5 {
		h = 5
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

func (m Model) interfacesVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 55 / 100
	if topH < 6 {
		topH = 6
	}
	lines := topH - 5
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m Model) routesVisibleLines() int {
	dh := m.detailHeight()
	topH := dh * 55 / 100
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

	title := shared.StyleTitle.Render("Routers")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.routers))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.routers) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No routers found.") + "\n")
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

	// Detail top row: info (35%) | interfaces (65%)
	topH := dh * 55 / 100
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

	ifacesContent := padContent(m.panelTitle(FocusInterfaces), m.renderInterfacesContent(rightW-4, topH-4))
	ifacesPanel := m.panelBorder(FocusInterfaces).Width(rightW).Height(topH).Render(ifacesContent)

	// Detail bottom row: routes (full width)
	routesContent := padContent(m.panelTitle(focusRoutes), m.renderRoutesContent(m.width-4, bottomH-4))
	routesPanel := m.panelBorder(focusRoutes).Width(m.width).Height(bottomH).Render(routesContent)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", ifacesPanel)

	return selPanel + "\n" + topRow + "\n" + routesPanel
}

func (m Model) renderNarrow() string {
	w := m.width - 2
	totalH := m.totalPanelHeight()

	selH := m.selectorHeight()
	remaining := totalH - selH
	infoH := remaining * 25 / 100
	ifacesH := remaining * 40 / 100
	routesH := remaining - infoH - ifacesH

	if infoH < 4 {
		infoH = 4
	}
	if ifacesH < 4 {
		ifacesH = 4
	}
	if routesH < 4 {
		routesH = 4
	}

	selContent := m.renderSelectorContent(w-4, selH-2)
	selPanel := m.panelBorder(FocusSelector).Width(w).Height(selH).Render(padContent(m.panelTitle(FocusSelector), selContent))

	infoPanel := m.panelBorder(focusInfo).Width(w).Height(infoH).Render(padContent(m.panelTitle(focusInfo), m.renderInfoContent(w-4)))
	ifacesPanel := m.panelBorder(FocusInterfaces).Width(w).Height(ifacesH).Render(padContent(m.panelTitle(FocusInterfaces), m.renderInterfacesContent(w-4, ifacesH-4)))
	routesPanel := m.panelBorder(focusRoutes).Width(w).Height(routesH).Render(padContent(m.panelTitle(focusRoutes), m.renderRoutesContent(w-4, routesH-4)))

	return lipgloss.JoinVertical(lipgloss.Left, selPanel, infoPanel, ifacesPanel, routesPanel)
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
		return titleStyle.Render("Routers")
	case focusInfo:
		return titleStyle.Render("Info")
	case FocusInterfaces:
		t := titleStyle.Render("Interfaces")
		if m.detailLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusRoutes:
		t := titleStyle.Render("Routes")
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
	if len(m.routers) == 0 {
		return ""
	}

	visibleLines := maxHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	var lines []string
	for i, r := range m.routers {
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

		name := r.Name
		if name == "" && len(r.ID) > 8 {
			name = r.ID[:8] + "..."
		}

		statusStyle := lipgloss.NewStyle().Foreground(shared.ColorSuccess)
		if r.Status != "ACTIVE" {
			statusStyle = statusStyle.Foreground(shared.ColorWarning)
		}
		statusStr := statusStyle.Render(shared.StatusIcon(r.Status) + r.Status)

		meta := ""
		if r.ExternalGatewayNetworkID != "" {
			netName := m.networkNames[r.ExternalGatewayNetworkID]
			if netName == "" {
				netName = "external"
			}
			meta = " (" + netName + ")"
		}

		adminStr := ""
		if !r.AdminStateUp {
			adminStr = " admin:down"
		}

		line := prefix + nameStyle.Render(name) + shared.StyleHelp.Render(meta+adminStr) + "  " + statusStr
		if lipgloss.Width(line) > maxWidth+2 {
			line = line[:maxWidth+1]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// --- Info rendering ---

func (m Model) renderInfoContent(maxWidth int) string {
	r := m.selectedRouter()
	if r == nil {
		return ""
	}

	labelW := 14
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)

	type prop struct{ label, value string }

	statusStr := shared.StatusIcon(r.Status) + r.Status
	adminStr := "Down"
	if r.AdminStateUp {
		adminStr = "Up"
	}

	extNet := "none"
	if r.ExternalGatewayNetworkID != "" {
		extNet = m.networkNames[r.ExternalGatewayNetworkID]
		if extNet == "" {
			extNet = r.ExternalGatewayNetworkID
		}
	}

	gw4 := r.ExternalGatewayIPv4
	if gw4 == "" {
		gw4 = "\u2014"
	}
	gw6 := r.ExternalGatewayIPv6
	if gw6 == "" {
		gw6 = "\u2014"
	}

	allProps := []prop{
		{"Name", r.Name},
		{"ID", r.ID},
		{"Status", statusStr},
		{"Admin State", adminStr},
	}
	if r.Description != "" {
		allProps = append(allProps, prop{"Description", r.Description})
	}
	allProps = append(allProps,
		prop{"Ext. Network", extNet},
		prop{"IPv6 Gateway", gw6},
		prop{"IPv4 Gateway", gw4},
		prop{"Interfaces", fmt.Sprintf("%d", len(m.interfaces))},
		prop{"Routes", fmt.Sprintf("%d", len(r.Routes))},
	)

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
			if r.Status != "ACTIVE" {
				statusColor = shared.ColorWarning
			}
			rendered = lipgloss.NewStyle().Foreground(statusColor).Render(val)
		}
		if p.label == "Admin State" {
			stateColor := shared.ColorSuccess
			if adminStr == "Down" {
				stateColor = shared.ColorError
			}
			rendered = lipgloss.NewStyle().Foreground(stateColor).Render(val)
		}
		rows = append(rows, labelStyle.Render(p.label)+rendered)
	}
	return strings.Join(rows, "\n")
}

// --- Interfaces rendering ---

func (m Model) renderInterfacesContent(maxWidth, maxHeight int) string {
	if m.detailErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.detailErr)
	}
	if len(m.interfaces) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No interfaces \u2014 Ctrl+A to add")
	}

	const gap = 2

	// Calculate column widths
	netW := len("Network")
	ipW := len("IP")
	for _, iface := range m.interfaces {
		netName := m.resolveInterfaceNetwork(iface)
		if len(netName) > netW {
			netW = len(netName)
		}
		if len(iface.IPAddress) > ipW {
			ipW = len(iface.IPAddress)
		}
	}
	maxNetW := maxWidth / 3
	if maxNetW < 8 {
		maxNetW = 8
	}
	if netW > maxNetW {
		netW = maxNetW
	}
	if ipW > 42 {
		ipW = 42
	}

	sep := strings.Repeat(" ", gap)
	portW := 8

	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%-*s%s%s",
		netW, "Network", sep, ipW, "IP", sep, "Port")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, iface := range m.interfaces {
		if i < m.interfaceScroll {
			continue
		}
		if i >= m.interfaceScroll+visibleLines {
			break
		}

		selected := m.focus == FocusInterfaces && i == m.interfaceCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		netName := m.resolveInterfaceNetwork(iface)
		if len(netName) > netW {
			netName = netName[:netW-1] + "\u2026"
		}

		ip := iface.IPAddress
		if len(ip) > ipW {
			ip = ip[:ipW-1] + "\u2026"
		}

		portShort := iface.PortID
		if len(portShort) > portW {
			portShort = portShort[:portW]
		}

		line := fmt.Sprintf("%s%-*s%s%-*s%s%s",
			prefix, netW, netName, sep, ipW, ip, sep, portShort)

		if selected {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) resolveInterfaceNetwork(iface network.RouterInterface) string {
	if netID, ok := m.subnetToNet[iface.SubnetID]; ok {
		if name, ok := m.networkNames[netID]; ok && name != "" {
			return name
		}
	}
	// Fall back to subnet ID
	if len(iface.SubnetID) > 8 {
		return iface.SubnetID[:8] + "\u2026"
	}
	return iface.SubnetID
}

// --- Routes rendering ---

func (m Model) renderRoutesContent(maxWidth, maxHeight int) string {
	r := m.selectedRouter()
	if r == nil {
		return ""
	}

	if len(r.Routes) == 0 && !m.detailLoading {
		return shared.StyleHelp.Render("No static routes")
	}

	const gap = 2

	destW := len("Destination")
	hopW := len("Next Hop")
	for _, route := range r.Routes {
		if len(route.DestinationCIDR) > destW {
			destW = len(route.DestinationCIDR)
		}
		if len(route.NextHop) > hopW {
			hopW = len(route.NextHop)
		}
	}
	if destW > 45 {
		destW = 45
	}
	if hopW > 45 {
		hopW = 45
	}

	sep := strings.Repeat(" ", gap)
	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	header := fmt.Sprintf("  %-*s%s%s", destW, "Destination", sep, "Next Hop")
	headerLine := headerStyle.Render(header)

	visibleLines := maxHeight - 1
	if visibleLines < 1 {
		visibleLines = 1
	}

	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	var lines []string
	lines = append(lines, headerLine)

	for i, route := range r.Routes {
		if i < m.routesScroll {
			continue
		}
		if i >= m.routesScroll+visibleLines {
			break
		}

		selected := m.focus == focusRoutes && i == m.routesCursor
		prefix := "  "
		if selected {
			prefix = "\u25b8 "
		}

		dest := route.DestinationCIDR
		if len(dest) > destW {
			dest = dest[:destW-1] + "\u2026"
		}
		hop := route.NextHop
		if len(hop) > hopW {
			hop = hop[:hopW-1] + "\u2026"
		}

		line := fmt.Sprintf("%s%-*s%s%s", prefix, destW, dest, sep, hop)

		if selected {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
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
	r := m.selectedRouter()
	if r == nil {
		return ""
	}

	var buttons []actionButton

	switch m.focus {
	case FocusSelector, focusInfo:
		buttons = append(buttons, btn("^n", "New Router"))
		buttons = append(buttons, btn("^d", "Delete Router"))
	case FocusInterfaces:
		buttons = append(buttons, btn("^a", "Add Interface"))
		if m.SelectedInterfaceSubnetID() != "" {
			buttons = append(buttons, btn("^t", "Remove Interface"))
		}
	case focusRoutes:
		// read-only for now
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
	shared.Debugf("[routerview] ForceRefresh()")
	m.loading = true
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchRouters(), m.fetchNames()}
	if r := m.selectedRouter(); r != nil {
		m.detailLoading = true
		cmds = append(cmds, m.fetchDetail(r.ID))
	}
	return tea.Batch(cmds...)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ScrollToNames positions the cursor on the first matching router name.
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
	for i, r := range m.routers {
		if m.highlightNames[r.Name] {
			m.cursor = i
			m.ensureSelectorCursorVisible()
			m.highlightNames = nil
			if r.ID != m.lastDetailID {
				m.lastDetailID = r.ID
				m.resetDetailState()
			}
			return
		}
	}
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	switch m.focus {
	case FocusInterfaces:
		return "\u2191\u2193 navigate \u2022 ^a add interface \u2022 ^t remove \u2022 tab/shift+tab focus \u2022 R refresh \u2022 ? help"
	case focusRoutes:
		return "\u2191\u2193 navigate \u2022 tab/shift+tab focus \u2022 R refresh \u2022 ? help"
	case focusInfo:
		return "tab/shift+tab focus \u2022 ^n new \u2022 ^d delete \u2022 R refresh \u2022 ? help"
	default:
		return "\u2191\u2193 navigate \u2022 ^n new \u2022 ^d delete \u2022 tab/shift+tab focus \u2022 R refresh \u2022 ? help"
	}
}

// --- Data fetching ---

func (m Model) fetchRouters() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		shared.Debugf("[routerview] fetchRouters: start ListRouters")
		routers, err := network.ListRouters(context.Background(), client)
		if err != nil {
			shared.Debugf("[routerview] fetchRouters: error: %s", err)
			return routersErrMsg{err: err}
		}
		sort.Slice(routers, func(i, j int) bool {
			return strings.ToLower(routers[i].Name) < strings.ToLower(routers[j].Name)
		})
		shared.Debugf("[routerview] fetchRouters: done, %d routers", len(routers))
		return routersLoadedMsg{routers: routers}
	}
}

func (m Model) fetchNames() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		ctx := context.Background()
		shared.Debugf("[routerview] fetchNames: start ListNetworks")
		nets, err := network.ListNetworks(ctx, client)
		if err != nil {
			shared.Debugf("[routerview] fetchNames: ListNetworks error: %s", err)
			return namesLoadedMsg{}
		}
		networkNames := make(map[string]string, len(nets))
		for _, n := range nets {
			networkNames[n.ID] = n.Name
		}
		shared.Debugf("[routerview] fetchNames: start ListSubnets")
		subs, err := network.ListSubnets(ctx, client)
		if err != nil {
			shared.Debugf("[routerview] fetchNames: ListSubnets error: %s", err)
			return namesLoadedMsg{networkNames: networkNames}
		}
		subnetToNet := make(map[string]string, len(subs))
		for _, s := range subs {
			subnetToNet[s.ID] = s.NetworkID
		}
		shared.Debugf("[routerview] fetchNames: done, %d nets %d subs", len(networkNames), len(subnetToNet))
		return namesLoadedMsg{networkNames: networkNames, subnetToNet: subnetToNet}
	}
}

func (m Model) fetchDetail(routerID string) tea.Cmd {
	client := m.networkClient
	short := routerID
	if len(short) > 8 {
		short = short[:8]
	}
	return func() tea.Msg {
		shared.Debugf("[routerview] fetchDetail: start ListRouterInterfaces router=%s", short)
		ifaces, err := network.ListRouterInterfaces(context.Background(), client, routerID)
		if err != nil {
			shared.Debugf("[routerview] fetchDetail: error: %s", err)
			return detailErrMsg{routerID: routerID, err: err}
		}
		shared.Debugf("[routerview] fetchDetail: done, %d interfaces", len(ifaces))
		return detailLoadedMsg{routerID: routerID, interfaces: ifaces}
	}
}

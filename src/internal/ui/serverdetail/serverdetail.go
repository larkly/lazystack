package serverdetail

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/compute"
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
	focusConsole
	focusActions
)

const (
	narrowThreshold = 80
	maxConsoleLines = 500
)

type serverDetailLoadedMsg struct {
	server *compute.Server
}

type serverDetailErrMsg struct {
	err error
}

type consoleLoadedMsg struct {
	output string
}

type consoleErrMsg struct {
	err error
}

type actionsLoadedMsg struct {
	actions []compute.Action
}

type actionsErrMsg struct {
	err error
}

type detailTickMsg struct{}

// Model is the server detail dashboard view.
type Model struct {
	client          *gophercloud.ServiceClient
	serverID        string
	server          *compute.Server
	loading         bool
	spinner         spinner.Model
	width           int
	height          int
	scroll          int // info panel scroll
	err             string
	refreshInterval time.Duration
	pendingAction   string

	consoleLines   []string
	consoleScroll  int
	consoleLoading bool
	consoleErr     string

	actions        []compute.Action
	actionsScroll  int
	actionsLoading bool
	actionsErr     string

	focus focusPane
}

// New creates a server detail model.
func New(client *gophercloud.ServiceClient, serverID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		client:          client,
		serverID:        serverID,
		loading:         true,
		consoleLoading:  true,
		actionsLoading:  true,
		spinner:         s,
		refreshInterval: refreshInterval,
	}
}

// Init fetches all data sources.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchServer(),
		m.fetchConsole(),
		m.fetchActions(),
		m.tickCmd(),
	)
}

// ServerID returns the current server ID.
func (m Model) ServerID() string {
	return m.serverID
}

// ServerName returns the current server name.
func (m Model) ServerName() string {
	if m.server != nil {
		return m.server.Name
	}
	return m.serverID
}

// ServerStatus returns the current server status.
func (m Model) ServerStatus() string {
	if m.server != nil {
		return m.server.Status
	}
	return ""
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case serverDetailLoadedMsg:
		m.loading = false
		if m.pendingAction != "" && msg.server != nil {
			if msg.server.Status != "VERIFY_RESIZE" {
				m.pendingAction = ""
			}
		}
		m.server = msg.server
		m.err = ""
		return m, nil

	case serverDetailErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case consoleLoadedMsg:
		m.consoleLoading = false
		m.consoleErr = ""
		m.consoleLines = strings.Split(msg.output, "\n")
		// Auto-scroll to bottom on first load
		if m.consoleScroll == 0 {
			m.consoleScroll = m.consoleMaxScroll()
		}
		return m, nil

	case consoleErrMsg:
		m.consoleLoading = false
		m.consoleErr = msg.err.Error()
		return m, nil

	case actionsLoadedMsg:
		m.actionsLoading = false
		m.actionsErr = ""
		m.actions = msg.actions
		return m, nil

	case actionsErrMsg:
		m.actionsLoading = false
		m.actionsErr = msg.err.Error()
		return m, nil

	case detailTickMsg:
		return m, tea.Batch(m.fetchServer(), m.fetchConsole(), m.fetchActions(), m.tickCmd())

	case spinner.TickMsg:
		if m.loading || m.consoleLoading || m.actionsLoading {
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
			return shared.ViewChangeMsg{View: "serverlist"}
		}

	case key.Matches(msg, shared.Keys.Tab):
		m.focus = (m.focus + 1) % 3
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + 2) % 3
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		m.scrollUp(1)
		return m, nil

	case key.Matches(msg, shared.Keys.Down):
		m.scrollDown(1)
		return m, nil

	case key.Matches(msg, shared.Keys.PageUp):
		m.scrollUp(m.panelHeight() - 2)
		return m, nil

	case key.Matches(msg, shared.Keys.PageDown):
		m.scrollDown(m.panelHeight() - 2)
		return m, nil

	case key.Matches(msg, shared.Keys.JumpVolumes):
		if m.server != nil && len(m.server.VolAttach) > 0 {
			ids := m.server.VolAttach
			return m, func() tea.Msg {
				return shared.NavigateToResourceMsg{Tab: "volumes", Highlight: ids}
			}
		}

	case key.Matches(msg, shared.Keys.JumpSecGroups):
		if m.server != nil && len(m.server.SecGroups) > 0 {
			names := m.server.SecGroups
			return m, func() tea.Msg {
				return shared.NavigateToResourceMsg{Tab: "secgroups", Highlight: names}
			}
		}

	case key.Matches(msg, shared.Keys.JumpNetworks):
		if m.server != nil && len(m.server.Networks) > 0 {
			names := make([]string, 0, len(m.server.Networks))
			for name := range m.server.Networks {
				names = append(names, name)
			}
			return m, func() tea.Msg {
				return shared.NavigateToResourceMsg{Tab: "networks", Highlight: names}
			}
		}

	// Console-specific: g/G for top/bottom when console focused
	case m.focus == focusConsole && msg.String() == "g":
		m.consoleScroll = 0
		return m, nil
	case m.focus == focusConsole && msg.String() == "G":
		m.consoleScroll = m.consoleMaxScroll()
		return m, nil
	}

	return m, nil
}

func (m *Model) scrollUp(n int) {
	switch m.focus {
	case focusInfo:
		m.scroll -= n
		if m.scroll < 0 {
			m.scroll = 0
		}
	case focusConsole:
		m.consoleScroll -= n
		if m.consoleScroll < 0 {
			m.consoleScroll = 0
		}
	case focusActions:
		m.actionsScroll -= n
		if m.actionsScroll < 0 {
			m.actionsScroll = 0
		}
	}
}

func (m *Model) scrollDown(n int) {
	switch m.focus {
	case focusInfo:
		m.scroll += n
	case focusConsole:
		m.consoleScroll += n
		if max := m.consoleMaxScroll(); m.consoleScroll > max {
			m.consoleScroll = max
		}
	case focusActions:
		m.actionsScroll += n
		if max := m.actionsMaxScroll(); m.actionsScroll > max {
			m.actionsScroll = max
		}
	}
}

func (m Model) panelHeight() int {
	h := m.height - 4 // title + action bar + padding
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) consoleMaxScroll() int {
	_, consoleH, _ := m.rightPanelHeights()
	max := len(m.consoleLines) - consoleH + 2 // border padding
	if max < 0 {
		return 0
	}
	return max
}

func (m Model) actionsMaxScroll() int {
	_, _, actionsH := m.rightPanelHeights()
	max := len(m.actions) - actionsH + 2
	if max < 0 {
		return 0
	}
	return max
}

func (m Model) rightPanelHeights() (totalH, consoleH, actionsH int) {
	totalH = m.panelHeight()
	consoleH = totalH * 60 / 100
	if consoleH < 4 {
		consoleH = 4
	}
	actionsH = totalH - consoleH
	if actionsH < 3 {
		actionsH = 3
	}
	return
}

// View renders the server detail dashboard.
func (m Model) View() string {
	var b strings.Builder

	// Title
	titleText := "Server Dashboard"
	if m.server != nil {
		titleText = m.server.Name
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

	if m.server == nil && m.loading {
		return b.String()
	}

	// Banners
	if m.server != nil {
		if m.pendingAction != "" {
			banner := lipgloss.NewStyle().
				Foreground(shared.ColorSuccess).
				Bold(true).
				Render(fmt.Sprintf(" \u2713 %s \u2014 waiting for server...", m.pendingAction))
			b.WriteString(banner + "\n")
		} else if m.server.Status == "VERIFY_RESIZE" {
			banner := lipgloss.NewStyle().
				Foreground(shared.ColorWarning).
				Bold(true).
				Render(" \u26a0 Resize pending \u2014 ^y confirm \u2022 ^x revert")
			b.WriteString(banner + "\n")
		}
	}

	if m.width < narrowThreshold {
		b.WriteString(m.renderNarrow())
	} else {
		b.WriteString(m.renderWide())
	}

	// Action bar
	b.WriteString(m.renderActionBar() + "\n")

	return b.String()
}

func (m Model) renderWide() string {
	totalH := m.panelHeight()
	leftWidth := m.width * 45 / 100
	if leftWidth < 32 {
		leftWidth = 32
	}
	rightWidth := m.width - leftWidth - 1

	_, consoleH, actionsH := m.rightPanelHeights()

	// Left panel: info (border=2, content padding=1 each side)
	infoContent := padContent(m.renderInfoContent(leftWidth - 6), leftWidth-4)
	infoBorder := m.panelBorder(focusInfo)
	infoPanel := infoBorder.
		Width(leftWidth - 2).
		Height(totalH - 2).
		Render(infoContent)

	// Right panel: console + actions stacked
	consoleContent := padContent(m.renderConsoleContent(rightWidth-6, consoleH-4), rightWidth-4)
	consoleBorder := m.panelBorder(focusConsole)
	consolePanel := consoleBorder.
		Width(rightWidth - 2).
		Height(consoleH - 2).
		Render(consoleContent)

	actionsContent := padContent(m.renderActionsContent(rightWidth-6, actionsH-4), rightWidth-4)
	actionsBorder := m.panelBorder(focusActions)
	actionsPanel := actionsBorder.
		Width(rightWidth - 2).
		Height(actionsH - 2).
		Render(actionsContent)

	rightPanel := lipgloss.JoinVertical(lipgloss.Left, consolePanel, actionsPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", rightPanel) + "\n"
}

func (m Model) renderNarrow() string {
	totalH := m.panelHeight()
	w := m.width - 2
	infoH := totalH * 40 / 100
	consoleH := totalH * 35 / 100
	actionsH := totalH - infoH - consoleH

	if infoH < 4 {
		infoH = 4
	}
	if consoleH < 3 {
		consoleH = 3
	}
	if actionsH < 3 {
		actionsH = 3
	}

	infoContent := padContent(m.renderInfoContent(w-6), w-4)
	infoPanel := m.panelBorder(focusInfo).Width(w).Height(infoH-2).Render(infoContent)

	consoleContent := padContent(m.renderConsoleContent(w-6, consoleH-4), w-4)
	consolePanel := m.panelBorder(focusConsole).Width(w).Height(consoleH-2).Render(consoleContent)

	actionsContent := padContent(m.renderActionsContent(w-6, actionsH-4), w-4)
	actionsPanel := m.panelBorder(focusActions).Width(w).Height(actionsH-2).Render(actionsContent)

	return lipgloss.JoinVertical(lipgloss.Left, infoPanel, consolePanel, actionsPanel) + "\n"
}

// padContent adds a blank line above and 1-char indent to each line of content.
func padContent(content string, _ int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	var out []string
	out = append(out, "") // blank line after header
	for _, l := range lines {
		out = append(out, " "+l)
	}
	return strings.Join(out, "\n")
}

func (m Model) panelBorder(pane focusPane) lipgloss.Style {
	borderColor := shared.ColorMuted
	if m.focus == pane {
		borderColor = shared.ColorPrimary
	}

	titleStr := ""
	switch pane {
	case focusInfo:
		titleStr = " Info "
	case focusConsole:
		titleStr = " Console Log "
		if m.consoleLoading {
			titleStr += m.spinner.View() + " "
		}
	case focusActions:
		titleStr = " Action History "
		if m.actionsLoading {
			titleStr += m.spinner.View() + " "
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		SetString(titleStr)
}

func (m Model) renderInfoContent(maxWidth int) string {
	if m.server == nil {
		return ""
	}

	s := m.server
	locked := "No"
	if s.Locked {
		locked = "Yes"
	}

	type prop struct {
		label string
		value string
	}

	allProps := []prop{
		{"Status", shared.StatusIcon(s.Status) + s.Status},
		{"Flavor", s.FlavorName},
		{"Power", s.PowerState},
		{"Image", s.ImageName},
		{"IPv6", strings.Join(s.IPv6, ", ")},
		{"Key", s.KeyName},
		{"IPv4", strings.Join(s.IPv4, ", ")},
		{"AZ", s.AZ},
		{"FloatIP", strings.Join(s.FloatingIP, ", ")},
		{"Age", formatAge(s.Created)},
		{"Locked", locked},
		{"ID", s.ID},
	}

	labelW := 8
	labelStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).
		Bold(true).
		Width(labelW)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	jumpStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	// Two-column grid when wide enough, single column otherwise
	twoColThreshold := 88
	useTwoCols := maxWidth >= twoColThreshold

	var grid string
	if useTwoCols {
		leftColW := (maxWidth - 2) / 2
		rightColW := maxWidth - leftColW - 2

		renderCol := func(props []prop, colW int) string {
			valW := colW - labelW
			if valW < 4 {
				valW = 4
			}
			var rows []string
			for _, p := range props {
				if p.value == "" {
					continue
				}
				val := p.value
				if lipgloss.Width(val) > valW {
					val = val[:valW-1] + "\u2026"
				}
				if p.label == "Status" {
					val = StatusStyle(s.Status).Render(val)
				} else {
					val = valueStyle.Render(val)
				}
				rows = append(rows, labelStyle.Render(p.label)+val)
			}
			return lipgloss.NewStyle().Width(colW).Render(strings.Join(rows, "\n"))
		}

		// Split into left/right by alternating (allProps is interleaved)
		var left, right []prop
		for i, p := range allProps {
			if i%2 == 0 {
				left = append(left, p)
			} else {
				right = append(right, p)
			}
		}
		grid = lipgloss.JoinHorizontal(lipgloss.Top,
			renderCol(left, leftColW), "  ", renderCol(right, rightColW))
	} else {
		// Single column — full width, no truncation needed for most values
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
			if p.label == "Status" {
				val = StatusStyle(s.Status).Render(val)
			} else {
				val = valueStyle.Render(val)
			}
			rows = append(rows, labelStyle.Render(p.label)+val)
		}
		grid = strings.Join(rows, "\n")
	}

	lines := strings.Split(grid, "\n")

	// Resources section
	if len(s.VolAttach) > 0 || len(s.SecGroups) > 0 || len(s.Networks) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("Resources"))

		if len(s.VolAttach) > 0 {
			val := strings.Join(s.VolAttach, ", ")
			if len(val) > maxWidth-14 && maxWidth > 18 {
				val = fmt.Sprintf("%d attached", len(s.VolAttach))
			}
			lines = append(lines, labelStyle.Render("Volumes")+valueStyle.Render(val)+" "+jumpStyle.Render("[v]"))
		}
		if len(s.SecGroups) > 0 {
			val := strings.Join(s.SecGroups, ", ")
			if len(val) > maxWidth-14 && maxWidth > 18 {
				val = fmt.Sprintf("%d groups", len(s.SecGroups))
			}
			lines = append(lines, labelStyle.Render("SecGrps")+valueStyle.Render(val)+" "+jumpStyle.Render("[g]"))
		}
		if len(s.Networks) > 0 {
			netNames := make([]string, 0, len(s.Networks))
			for name := range s.Networks {
				netNames = append(netNames, name)
			}
			sort.Strings(netNames)
			lines = append(lines, labelStyle.Render("Nets")+" "+jumpStyle.Render("[N]"))
			for _, name := range netNames {
				ips := s.Networks[name]
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(shared.ColorCyan).Render(name)+" "+valueStyle.Render(strings.Join(ips, ", ")))
			}
		}
	}

	// Apply scroll
	viewH := m.panelHeight() - 2
	if m.scroll > len(lines)-viewH {
		// Clamp — use a local copy so we don't mutate
	}
	start := m.scroll
	if start > len(lines) {
		start = len(lines)
	}
	end := start + viewH
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		if len(lines) > 0 {
			start = max(0, len(lines)-viewH)
			end = len(lines)
		} else {
			return ""
		}
	}

	return strings.Join(lines[start:end], "\n")
}

func (m Model) renderConsoleContent(maxWidth, maxHeight int) string {
	if m.consoleErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.consoleErr)
	}

	if len(m.consoleLines) == 0 {
		if m.consoleLoading {
			return ""
		}
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No console output available.")
	}

	start := m.consoleScroll
	if start < 0 {
		start = 0
	}
	end := start + maxHeight
	if end > len(m.consoleLines) {
		end = len(m.consoleLines)
	}
	if start >= end {
		start = max(0, end-maxHeight)
	}

	var lines []string
	for i := start; i < end; i++ {
		line := m.consoleLines[i]
		if len(line) > maxWidth {
			line = line[:maxWidth]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderActionsContent(maxWidth, maxHeight int) string {
	if m.actionsErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.actionsErr)
	}

	if len(m.actions) == 0 {
		if m.actionsLoading {
			return ""
		}
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No actions recorded.")
	}

	start := m.actionsScroll
	if start < 0 {
		start = 0
	}
	end := start + maxHeight
	if end > len(m.actions) {
		end = len(m.actions)
	}
	if start >= end {
		start = max(0, end-maxHeight)
	}

	style := lipgloss.NewStyle().Foreground(shared.ColorFg)
	errStyle := lipgloss.NewStyle().Foreground(shared.ColorError)

	var lines []string
	for i := start; i < end; i++ {
		a := m.actions[i]
		age := formatAge(a.StartTime)
		icon := shared.StatusIcon("ACTIVE") // default green dot
		if a.Message != "" {
			icon = shared.StatusIcon("ERROR")
		}

		line := fmt.Sprintf("%s%-14s %s", icon, a.Action, age)
		if len(line) > maxWidth {
			line = line[:maxWidth]
		}
		if a.Message != "" {
			lines = append(lines, errStyle.Render(line))
		} else {
			lines = append(lines, style.Render(line))
		}
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
	if m.server == nil {
		return ""
	}

	s := m.server
	var buttons []actionButton

	// Always available (read-only)
	buttons = append(buttons, btn("x", "SSH"), btn("y", "CopySSH"))

	// Power state dependent
	switch {
	case s.Status == "VERIFY_RESIZE":
		buttons = append(buttons, btn("^y", "Confirm"), btn("^x", "Revert"))
	case s.Status == "ACTIVE":
		buttons = append(buttons, btn("o", "Stop"), btn("^o", "Reboot"))
	case s.Status == "SHUTOFF":
		buttons = append(buttons, btn("o", "Start"))
	case s.Status == "PAUSED":
		buttons = append(buttons, btn("p", "Unpause"))
	case s.Status == "SUSPENDED":
		buttons = append(buttons, btn("^z", "Resume"))
	case s.Status == "SHELVED", s.Status == "SHELVED_OFFLOADED":
		buttons = append(buttons, btn("^e", "Unshelve"))
	case s.Status == "RESCUE":
		buttons = append(buttons, btn("^w", "Unrescue"))
	}

	// Mutating actions (hidden when locked)
	if !s.Locked {
		buttons = append(buttons,
			btn("c", "Clone"),
			btn("^d", "Delete"),
			btn("^f", "Resize"),
			btn("r", "Rename"),
			btn("^g", "Rebuild"),
			btn("^s", "Snapshot"),
		)
	}

	// Lock toggle
	if s.Locked {
		buttons = append(buttons, btn("^l", "Unlock"))
	} else {
		buttons = append(buttons, btn("^l", "Lock"))
	}

	// Console/noVNC
	buttons = append(buttons, btn("V", "noVNC"))

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
		partLen := len("["+b.key+"]") + len(b.label) + 1 // +1 for space
		if totalLen+partLen > maxWidth && len(parts) > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("[?]More"))
			break
		}
		parts = append(parts, part)
		totalLen += partLen
	}

	return " " + strings.Join(parts, " ")
}

// StatusStyle returns the style for a server status.
func StatusStyle(status string) lipgloss.Style {
	color, ok := shared.StatusColors[status]
	if !ok {
		color = shared.ColorFg
	}
	return lipgloss.NewStyle().Foreground(color)
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
}

func (m Model) fetchServer() tea.Cmd {
	client := m.client
	id := m.serverID
	return func() tea.Msg {
		srv, err := compute.GetServer(context.Background(), client, id)
		if err != nil {
			return serverDetailErrMsg{err: err}
		}
		return serverDetailLoadedMsg{server: srv}
	}
}

func (m Model) fetchConsole() tea.Cmd {
	client := m.client
	id := m.serverID
	return func() tea.Msg {
		output, err := compute.GetConsoleOutput(context.Background(), client, id, maxConsoleLines)
		if err != nil {
			return consoleErrMsg{err: err}
		}
		return consoleLoadedMsg{output: output}
	}
}

func (m Model) fetchActions() tea.Cmd {
	client := m.client
	id := m.serverID
	return func() tea.Msg {
		actions, err := compute.ListActions(context.Background(), client, id)
		if err != nil {
			return actionsErrMsg{err: err}
		}
		return actionsLoadedMsg{actions: actions}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return detailTickMsg{}
	})
}

// ForceRefresh triggers a manual reload of all data sources.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	m.consoleLoading = true
	m.actionsLoading = true
	return tea.Batch(m.spinner.Tick, m.fetchServer(), m.fetchConsole(), m.fetchActions())
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ServerFlavor returns the current server flavor name.
func (m Model) ServerFlavor() string {
	if m.server != nil {
		return m.server.FlavorName
	}
	return ""
}

// ServerImageID returns the current server's image ID.
func (m Model) ServerImageID() string {
	if m.server != nil {
		return m.server.ImageID
	}
	return ""
}

// Server returns the full server object, or nil if not loaded.
func (m Model) Server() *compute.Server {
	return m.server
}

// ServerLocked returns whether the server is locked.
func (m Model) ServerLocked() bool {
	if m.server != nil {
		return m.server.Locked
	}
	return false
}

// ServerKeyName returns the server's key pair name.
func (m Model) ServerKeyName() string {
	if m.server != nil {
		return m.server.KeyName
	}
	return ""
}

// ServerFloatingIPs returns the server's floating IPs.
func (m Model) ServerFloatingIPs() []string {
	if m.server != nil {
		return m.server.FloatingIP
	}
	return nil
}

// ServerIPv6 returns the server's IPv6 addresses.
func (m Model) ServerIPv6() []string {
	if m.server != nil {
		return m.server.IPv6
	}
	return nil
}

// ServerIPv4 returns the server's IPv4 addresses.
func (m Model) ServerIPv4() []string {
	if m.server != nil {
		return m.server.IPv4
	}
	return nil
}

// SetServer updates the server data directly.
func (m *Model) SetServer(s *compute.Server) {
	if m.pendingAction != "" && s != nil && s.Status != "VERIFY_RESIZE" {
		m.pendingAction = ""
	}
	m.server = s
	m.loading = false
	m.err = ""
}

// SetPendingAction marks an action as in-progress.
func (m *Model) SetPendingAction(action string) {
	m.pendingAction = action
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	base := "tab focus \u2022 esc back \u2022 ? help"
	if m.pendingAction != "" {
		return base
	}
	if m.server != nil && m.server.Status == "VERIFY_RESIZE" {
		return "^y confirm resize \u2022 ^x revert \u2022 " + base
	}
	switch m.focus {
	case focusInfo:
		return "\u2191\u2193 scroll info \u2022 v/g/N resources \u2022 " + base
	case focusConsole:
		return "\u2191\u2193 scroll log \u2022 g top \u2022 G bottom \u2022 " + base
	case focusActions:
		return "\u2191\u2193 scroll actions \u2022 " + base
	}
	return base
}

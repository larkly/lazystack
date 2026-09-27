package serverdetail

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/copypicker"
	"github.com/larkly/lazystack/internal/volume"
)

type focusPane int

const (
	focusInfo focusPane = iota
	focusInterfaces
	focusVolumes
	focusConsole
	focusActions
)

const focusPaneCount = 5

const (
	narrowThreshold = 80
	maxConsoleLines = 500
)

func shouldPollDetailAPIs(status string) bool {
	switch status {
	case "SHUTOFF", "SHELVED", "SHELVED_OFFLOADED":
		return false
	default:
		return true
	}
}

// Every data reply carries the instance ID of the model that requested it.
// A new model is created each time a server detail is opened (including
// the same server after a cloud/project switch), so a late reply for an
// earlier server can never be applied to the one now on screen.

type serverDetailLoadedMsg struct {
	inst   uint64
	server *compute.Server
}

type serverDetailErrMsg struct {
	inst uint64
	err  error
}

type consoleLoadedMsg struct {
	inst   uint64
	output string
}

type consoleErrMsg struct {
	inst uint64
	err  error
}

type actionsLoadedMsg struct {
	inst    uint64
	actions []compute.Action
}

type actionsErrMsg struct {
	inst uint64
	err  error
}

type interfacesLoadedMsg struct {
	inst  uint64
	ports []network.Port
}

type interfacesErrMsg struct {
	inst uint64
	err  error
}

type volumeInfoLoadedMsg struct {
	inst    uint64
	volumes map[string]*volume.Volume
}

// lastInstance hands out a unique ID to every detail model.
var lastInstance atomic.Uint64

// replyInstance returns the requesting model instance of a data reply.
func replyInstance(msg tea.Msg) (uint64, bool) {
	switch msg := msg.(type) {
	case serverDetailLoadedMsg:
		return msg.inst, true
	case serverDetailErrMsg:
		return msg.inst, true
	case consoleLoadedMsg:
		return msg.inst, true
	case consoleErrMsg:
		return msg.inst, true
	case actionsLoadedMsg:
		return msg.inst, true
	case actionsErrMsg:
		return msg.inst, true
	case interfacesLoadedMsg:
		return msg.inst, true
	case interfacesErrMsg:
		return msg.inst, true
	case volumeInfoLoadedMsg:
		return msg.inst, true
	}
	return 0, false
}

// Model is the server detail dashboard view.
type Model struct {
	inst            uint64 // unique per model; tags every data request
	client          *gophercloud.ServiceClient
	networkClient   *gophercloud.ServiceClient
	blockClient     *gophercloud.ServiceClient
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

	interfaces        []network.Port
	interfacesScroll  int
	interfacesLoading bool
	interfacesErr     string

	volumeInfo   map[string]*volume.Volume // volume ID → full volume data
	volumeScroll int
	volumeCursor int

	focus focusPane
}

// New creates a server detail model.
func New(client, networkClient, blockClient *gophercloud.ServiceClient, serverID string, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		inst:              lastInstance.Add(1),
		client:            client,
		networkClient:     networkClient,
		blockClient:       blockClient,
		serverID:          serverID,
		loading:           true,
		consoleLoading:    true,
		actionsLoading:    true,
		interfacesLoading: true,
		spinner:           s,
		refreshInterval:   refreshInterval,
		volumeInfo:        make(map[string]*volume.Volume),
	}
}

// Init fetches all data sources.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[serverdetail] Init()")
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.fetchServer(),
		m.fetchConsole(),
		m.fetchActions(),
	}
	if m.networkClient != nil {
		cmds = append(cmds, m.fetchInterfaces())
	}
	return tea.Batch(cmds...)
}

func (m Model) canPollDetailAPIs() bool {
	if m.server == nil {
		return true
	}
	return shouldPollDetailAPIs(m.server.Status)
}

// Instance returns the unique ID of this model instance. It differs for
// every opened detail view, even of the same server.
func (m Model) Instance() uint64 {
	return m.inst
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

// SelectedVolumeID returns the volume ID under the cursor in the volumes pane.
func (m Model) SelectedVolumeID() string {
	if m.server == nil || len(m.server.VolAttach) == 0 {
		return ""
	}
	if m.volumeCursor >= 0 && m.volumeCursor < len(m.server.VolAttach) {
		return m.server.VolAttach[m.volumeCursor].ID
	}
	return ""
}

// SelectedVolumeName returns the name of the volume under the cursor.
func (m Model) SelectedVolumeName() string {
	id := m.SelectedVolumeID()
	if id == "" {
		return ""
	}
	if vol, ok := m.volumeInfo[id]; ok && vol.Name != "" {
		return vol.Name
	}
	return id
}

// FocusedOnVolumes returns true when the volumes pane has focus.
func (m Model) FocusedOnVolumes() bool {
	return m.focus == focusVolumes
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
	if inst, ok := replyInstance(msg); ok && inst != m.inst {
		shared.Debugf("[serverdetail] dropping %T for another detail view", msg)
		return m, nil
	}
	switch msg := msg.(type) {
	case serverDetailLoadedMsg:
		if msg.server != nil && msg.server.ID != m.serverID {
			return m, nil
		}
		shared.Debugf("[serverdetail] serverDetailLoadedMsg")
		m.loading = false
		if m.pendingAction != "" && msg.server != nil {
			if msg.server.Status != "VERIFY_RESIZE" {
				m.pendingAction = ""
			}
		}
		m.server = msg.server
		m.err = ""
		m.clampScrolls()
		// Fetch volume names if we have attachments and haven't yet
		if msg.server != nil && len(msg.server.VolAttach) > 0 && m.blockClient != nil {
			needFetch := false
			for _, va := range msg.server.VolAttach {
				if _, ok := m.volumeInfo[va.ID]; !ok {
					needFetch = true
					break
				}
			}
			if needFetch {
				return m, m.fetchVolumeInfo(msg.server.VolAttach)
			}
		}
		return m, nil

	case serverDetailErrMsg:
		shared.Debugf("[serverdetail] serverDetailErrMsg: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case consoleLoadedMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] consoleLoadedMsg: %d chars", len(msg.output))
		m.consoleLoading = false
		m.consoleErr = ""
		m.consoleLines = strings.Split(msg.output, "\n")
		// Auto-scroll to bottom on first load
		if m.consoleScroll == 0 {
			m.consoleScroll = m.consoleMaxScroll()
		}
		m.clampScrolls()
		return m, nil

	case consoleErrMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] consoleErrMsg: %v", msg.err)
		m.consoleLoading = false
		m.consoleErr = msg.err.Error()
		return m, nil

	case actionsLoadedMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] actionsLoadedMsg: %d actions", len(msg.actions))
		m.actionsLoading = false
		m.actionsErr = ""
		m.actions = msg.actions
		m.clampScrolls()
		return m, nil

	case actionsErrMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] actionsErrMsg: %v", msg.err)
		m.actionsLoading = false
		m.actionsErr = msg.err.Error()
		return m, nil

	case interfacesLoadedMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] interfacesLoadedMsg: %d ports", len(msg.ports))
		m.interfacesLoading = false
		m.interfacesErr = ""
		m.interfaces = msg.ports
		m.clampScrolls()
		return m, nil

	case interfacesErrMsg:
		if !m.canPollDetailAPIs() {
			return m, nil
		}
		shared.Debugf("[serverdetail] interfacesErrMsg: %v", msg.err)
		m.interfacesLoading = false
		m.interfacesErr = msg.err.Error()
		return m, nil

	case volumeInfoLoadedMsg:
		for id, vol := range msg.volumes {
			m.volumeInfo[id] = vol
		}
		return m, nil

	case shared.TickMsg:
		if m.loading {
			shared.Debugf("[serverdetail] tick skipped (loading)")
			return m, nil
		}
		cmds := []tea.Cmd{m.fetchServer()}
		if m.canPollDetailAPIs() {
			shared.Debugf("[serverdetail] tick fetching full detail")
			cmds = append(cmds, m.fetchConsole(), m.fetchActions())
			if m.networkClient != nil {
				cmds = append(cmds, m.fetchInterfaces())
			}
		} else {
			shared.Debugf("[serverdetail] tick idling detail APIs for status %s", m.server.Status)
			m.consoleLoading = false
			m.actionsLoading = false
			m.interfacesLoading = false
			m.consoleErr = ""
			m.actionsErr = ""
			m.interfacesErr = ""
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.loading || m.consoleLoading || m.actionsLoading || m.interfacesLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
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
		m.focus = (m.focus + 1) % focusPaneCount
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab):
		m.focus = (m.focus + focusPaneCount - 1) % focusPaneCount
		return m, nil

	// Console-specific: g/G for top/bottom when console focused. These must
	// precede the resource jumps so g is not taken by JumpSecGroups.
	case m.focus == focusConsole && msg.String() == "g":
		m.consoleScroll = 0
		return m, nil
	case m.focus == focusConsole && msg.String() == "G":
		m.consoleScroll = m.consoleMaxScroll()
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		// Enter on volumes pane navigates to selected volume detail
		if m.focus == focusVolumes && m.server != nil && len(m.server.VolAttach) > 0 {
			idx := m.volumeCursor
			if idx >= 0 && idx < len(m.server.VolAttach) {
				volID := m.server.VolAttach[idx].ID
				return m, func() tea.Msg {
					return shared.NavigateToDetailMsg{Resource: "volume", ID: volID}
				}
			}
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		m.scrollUp(1)
		return m, nil

	case key.Matches(msg, shared.Keys.Down):
		m.scrollDown(1)
		return m, nil

	case key.Matches(msg, shared.Keys.PageUp):
		m.scrollUp(m.pageSize())
		return m, nil

	case key.Matches(msg, shared.Keys.PageDown):
		m.scrollDown(m.pageSize())
		return m, nil

	case key.Matches(msg, shared.Keys.JumpVolumes):
		if m.server != nil && len(m.server.VolAttach) > 0 {
			ids := compute.VolumeAttachmentIDs(m.server.VolAttach)
			if len(ids) == 1 {
				// Single volume: go directly to detail
				return m, func() tea.Msg {
					return shared.NavigateToDetailMsg{Resource: "volume", ID: ids[0]}
				}
			}
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
	case focusInterfaces:
		m.interfacesScroll -= n
		if m.interfacesScroll < 0 {
			m.interfacesScroll = 0
		}
	case focusVolumes:
		if m.server != nil {
			m.volumeCursor -= n
			m.clampVolumes()
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
		if max := m.infoMaxScroll(); m.scroll > max {
			m.scroll = max
		}
	case focusInterfaces:
		m.interfacesScroll += n
		if max := m.interfacesMaxScroll(); m.interfacesScroll > max {
			m.interfacesScroll = max
		}
	case focusVolumes:
		if m.server != nil {
			m.volumeCursor += n
			m.clampVolumes()
		}
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

// paneRect is the outer size of a dashboard pane, including its border.
type paneRect struct{ w, h int }

// contentWidth is the body width: border (2) + padContent indent (1) + margin (1).
func (r paneRect) contentWidth() int { return max(0, r.w-4) }

// contentHeight is the number of body rows: border (2) + title + blank line.
func (r paneRect) contentHeight() int { return max(0, r.h-4) }

// detailLayout holds the pane rectangles. Rendering, scroll limits and
// cursor visibility all derive from it so they can never disagree.
type detailLayout struct {
	narrow                                      bool
	info, console, interfaces, volumes, actions paneRect
}

// bannerLines is the number of rows used by the pending-action/resize banner.
func (m Model) bannerLines() int {
	if m.server != nil && (m.pendingAction != "" || m.server.Status == "VERIFY_RESIZE") {
		return 1
	}
	return 0
}

// panelHeight is the height available to the pane grid.
func (m Model) panelHeight() int {
	h := m.height - 4 - m.bannerLines() // title + blank + action bar + status bar
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) layout() detailLayout {
	totalH := m.panelHeight()

	if m.width < narrowThreshold {
		w := m.width - 2
		hs := []int{totalH * 25 / 100, totalH * 15 / 100, totalH * 15 / 100, totalH * 25 / 100, 0}
		hs[4] = totalH - hs[0] - hs[1] - hs[2] - hs[3]
		// Each pane needs border (2) + title + blank + at least one row.
		fitHeights(hs, []int{5, 5, 5, 5, 5}, totalH)
		return detailLayout{
			narrow:     true,
			info:       paneRect{w, hs[0]},
			interfaces: paneRect{w, hs[1]},
			volumes:    paneRect{w, hs[2]},
			console:    paneRect{w, hs[3]},
			actions:    paneRect{w, hs[4]},
		}
	}

	// Shared row heights
	topH := totalH * 65 / 100
	if topH < 6 {
		topH = 6
	}
	bottomH := totalH - topH
	if bottomH < 4 {
		bottomH = 4
	}

	// Top row: 50/50 split, 1 gap
	leftW := m.width / 2
	rightW := m.width - leftW - 1

	// Bottom row: equal thirds, 2 gaps
	bottomContentW := m.width - 2
	col1W := bottomContentW / 3
	col2W := bottomContentW / 3
	col3W := bottomContentW - col1W - col2W

	return detailLayout{
		info:       paneRect{leftW, topH},
		console:    paneRect{rightW, topH},
		interfaces: paneRect{col1W, bottomH},
		volumes:    paneRect{col2W, bottomH},
		actions:    paneRect{col3W, bottomH},
	}
}

// fitHeights raises each height to its minimum, then takes rows back from
// the panes with the most slack until the total fits (when it can).
func fitHeights(hs, mins []int, total int) {
	sum := 0
	for i := range hs {
		hs[i] = max(hs[i], mins[i])
		sum += hs[i]
	}
	for sum > total {
		best := -1
		for i := range hs {
			if hs[i] > mins[i] && (best < 0 || hs[i]-mins[i] > hs[best]-mins[best]) {
				best = i
			}
		}
		if best < 0 {
			return
		}
		hs[best]--
		sum--
	}
}

// pageSize is the PgUp/PgDn step for the focused pane.
func (m Model) pageSize() int {
	l := m.layout()
	var r paneRect
	switch m.focus {
	case focusInfo:
		r = l.info
	case focusConsole:
		r = l.console
	case focusInterfaces:
		r = l.interfaces
	case focusVolumes:
		r = l.volumes
	case focusActions:
		r = l.actions
	}
	return max(1, r.contentHeight())
}

func (m Model) consoleMaxScroll() int {
	return max(0, len(m.consoleLines)-m.layout().console.contentHeight())
}

func (m Model) actionsMaxScroll() int {
	return max(0, len(m.actions)-m.layout().actions.contentHeight())
}

func (m Model) interfacesMaxScroll() int {
	r := m.layout().interfaces
	return max(0, len(m.interfaceLines(r.contentWidth()))-r.contentHeight())
}

// infoMaxScroll returns the highest valid scroll offset for the info pane,
// computed the same way renderInfoContent slices its lines.
func (m Model) infoMaxScroll() int {
	r := m.layout().info
	return max(0, len(m.infoLines(r.contentWidth()))-r.contentHeight())
}

func (m Model) volumeCount() int {
	if m.server == nil {
		return 0
	}
	return len(m.server.VolAttach)
}

func (m Model) volumeMaxScroll() int {
	return max(0, m.volumeCount()-m.layout().volumes.contentHeight())
}

// clampVolumes keeps the volume cursor on an attachment and scrolls the
// volumes pane so the cursor row is visible.
func (m *Model) clampVolumes() {
	m.volumeCursor = max(0, min(m.volumeCursor, m.volumeCount()-1))
	visible := max(1, m.layout().volumes.contentHeight())
	if m.volumeCursor < m.volumeScroll {
		m.volumeScroll = m.volumeCursor
	}
	if m.volumeCursor >= m.volumeScroll+visible {
		m.volumeScroll = m.volumeCursor - visible + 1
	}
	m.volumeScroll = max(0, min(m.volumeScroll, m.volumeMaxScroll()))
}

// clampScrolls brings every pane's offset back into range after the data
// or the terminal size changed.
func (m *Model) clampScrolls() {
	m.scroll = max(0, min(m.scroll, m.infoMaxScroll()))
	m.consoleScroll = max(0, min(m.consoleScroll, m.consoleMaxScroll()))
	m.actionsScroll = max(0, min(m.actionsScroll, m.actionsMaxScroll()))
	m.interfacesScroll = max(0, min(m.interfacesScroll, m.interfacesMaxScroll()))
	m.clampVolumes()
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
	l := m.layout()

	infoPanel := m.renderPane(focusInfo, l.info, m.renderInfoContent(l.info.contentWidth(), l.info.contentHeight()))
	consolePanel := m.renderPane(focusConsole, l.console, m.renderConsoleContent(l.console.contentWidth(), l.console.contentHeight()))
	ifacePanel := m.renderPane(focusInterfaces, l.interfaces, m.renderInterfacesContent(l.interfaces.contentWidth(), l.interfaces.contentHeight()))
	volPanel := m.renderPane(focusVolumes, l.volumes, m.renderVolumesContent(l.volumes.contentWidth(), l.volumes.contentHeight()))
	actionsPanel := m.renderPane(focusActions, l.actions, m.renderActionsContent(l.actions.contentWidth(), l.actions.contentHeight()))

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, infoPanel, " ", consolePanel)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, ifacePanel, " ", volPanel, " ", actionsPanel)

	return topRow + "\n" + bottomRow + "\n"
}

func (m Model) renderNarrow() string {
	l := m.layout()

	infoPanel := m.renderPane(focusInfo, l.info, m.renderInfoContent(l.info.contentWidth(), l.info.contentHeight()))
	ifacePanel := m.renderPane(focusInterfaces, l.interfaces, m.renderInterfacesContent(l.interfaces.contentWidth(), l.interfaces.contentHeight()))
	volPanel := m.renderPane(focusVolumes, l.volumes, m.renderVolumesContent(l.volumes.contentWidth(), l.volumes.contentHeight()))
	consolePanel := m.renderPane(focusConsole, l.console, m.renderConsoleContent(l.console.contentWidth(), l.console.contentHeight()))
	actionsPanel := m.renderPane(focusActions, l.actions, m.renderActionsContent(l.actions.contentWidth(), l.actions.contentHeight()))

	return lipgloss.JoinVertical(lipgloss.Left, infoPanel, ifacePanel, volPanel, consolePanel, actionsPanel) + "\n"
}

// renderPane renders a bordered pane of exactly r.w x r.h cells (Width and
// Height include the border in lipgloss v2). Lines are truncated rather than
// wrapped and surplus rows are dropped, because lipgloss treats Height as a
// minimum and an overflowing pane would push the rest of the dashboard out
// of the viewport.
func (m Model) renderPane(pane focusPane, r paneRect, content string) string {
	innerW, innerH := max(0, r.w-2), max(0, r.h-2)
	lines := strings.Split(padContent(m.panelTitle(pane), content), "\n")
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, innerW, "")
	}
	return m.panelBorder(pane).Width(r.w).Height(r.h).Render(strings.Join(lines, "\n"))
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
	case focusInterfaces:
		t := titleStyle.Render("Interfaces")
		if m.interfacesLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusVolumes:
		return titleStyle.Render("Volumes")
	case focusConsole:
		t := titleStyle.Render("Console Log (L)")
		if m.consoleLoading {
			t += " " + m.spinner.View()
		}
		return t
	case focusActions:
		t := titleStyle.Render("Action History")
		if m.actionsLoading {
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

// infoLines builds the full (unscrolled) info-pane content lines.
func (m Model) infoLines(maxWidth int) []string {
	if m.server == nil {
		return nil
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

	flavorVal := s.FlavorName
	if s.FlavorVCPUs > 0 {
		flavorVal += fmt.Sprintf(" (%dv/%dM/%dG)", s.FlavorVCPUs, s.FlavorRAM, s.FlavorDisk)
	}

	allProps := []prop{
		{"Status", shared.StatusIcon(s.Status) + s.Status},
		{"Flavor", flavorVal},
		{"Power", s.PowerState},
		{"Image", s.ImageName},
		{"IPv6", strings.Join(s.IPv6, ", ")},
		{"Key", s.KeyName},
		{"IPv4", strings.Join(s.IPv4, ", ")},
		{"AZ", s.AZ},
		{"FloatIP", strings.Join(s.FloatingIP, ", ")},
		{"Age", formatAge(s.Created)},
		{"Locked", locked},
		{"SSHUser", s.Metadata["lazystack_ssh_user"]},
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
	if len(s.SecGroups) > 0 || len(s.Networks) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary).Render("Resources"))

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

	return lines
}

func (m Model) renderInfoContent(maxWidth, viewH int) string {
	lines := m.infoLines(maxWidth)
	if len(lines) == 0 || viewH <= 0 {
		return ""
	}

	// Apply scroll
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
	// Show friendly message for inactive servers instead of API error
	if m.server != nil {
		switch m.server.Status {
		case "SHUTOFF", "SUSPENDED", "SHELVED", "SHELVED_OFFLOADED":
			return lipgloss.NewStyle().Foreground(shared.ColorMuted).
				Render(shared.StatusIcon(m.server.Status) + "Server " + m.server.Status + " \u2014 console unavailable")
		}
	}
	if m.consoleErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("Console unavailable")
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

func (m Model) renderVolumesContent(maxWidth, maxHeight int) string {
	if m.server == nil || len(m.server.VolAttach) == 0 {
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No volumes attached.")
	}

	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	selectedStyle := lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	cursorStr := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render("\u25b8 ")

	var lines []string
	for i, va := range m.server.VolAttach {
		selected := m.focus == focusVolumes && i == m.volumeCursor
		prefix := "  "
		style := valueStyle
		if selected {
			prefix = cursorStr
			style = selectedStyle
		}

		vol, hasInfo := m.volumeInfo[va.ID]

		// Line 1: name (or ID) — always shown
		name := va.ID
		if hasInfo && vol.Name != "" {
			name = vol.Name
		}

		// Build detail parts by priority
		// Priority: name > size > device > type > AZ
		var parts []string
		if hasInfo {
			parts = append(parts, fmt.Sprintf("%dGB", vol.Size))
			if va.Device != "" {
				parts = append(parts, va.Device)
			}
			if maxWidth > 30 && vol.VolumeType != "" {
				parts = append(parts, vol.VolumeType)
			}
			if maxWidth > 45 && vol.AZ != "" {
				parts = append(parts, vol.AZ)
			}
		} else if va.Device != "" {
			parts = append(parts, va.Device)
		}

		detail := ""
		if len(parts) > 0 {
			detail = " " + mutedStyle.Render("\u2022 "+strings.Join(parts, " \u2022 "))
		}

		lines = append(lines, prefix+style.Render(name)+detail)
	}

	start := m.volumeScroll
	if start < 0 {
		start = 0
	}
	end := start + maxHeight
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		start = max(0, end-maxHeight)
	}

	return strings.Join(lines[start:end], "\n")
}

func (m Model) renderInterfacesContent(maxWidth, maxHeight int) string {
	if m.interfacesErr != "" {
		return lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.interfacesErr)
	}

	if len(m.interfaces) == 0 {
		if m.interfacesLoading {
			return ""
		}
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("No interfaces found.")
	}

	lines := m.interfaceLines(maxWidth)
	start := m.interfacesScroll
	if start < 0 {
		start = 0
	}
	end := start + maxHeight
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		start = max(0, end-maxHeight)
	}

	return strings.Join(lines[start:end], "\n")
}

// interfaceLines builds the full (unscrolled) interfaces-pane content lines.
func (m Model) interfaceLines(maxWidth int) []string {
	if m.interfacesErr != "" || len(m.interfaces) == 0 {
		return nil
	}

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	labelW := 6
	lbl := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)

	var lines []string
	for i, p := range m.interfaces {
		if i > 0 {
			lines = append(lines, "")
		}
		// Port header
		lines = append(lines, labelStyle.Render(p.MACAddress)+" "+mutedStyle.Render(p.Status))

		// Fixed IPs — one per line with label
		for j, ip := range p.FixedIPs {
			ipLabel := ""
			if j == 0 {
				ipLabel = "IPs"
			}
			lines = append(lines, lbl.Render(ipLabel)+valueStyle.Render(ip.IPAddress))
		}

		// Network and port IDs (truncated for space)
		if maxWidth > 40 {
			netID := p.NetworkID
			if len(netID) > maxWidth-10 {
				netID = netID[:maxWidth-13] + "\u2026"
			}
			lines = append(lines, lbl.Render("Net")+mutedStyle.Render(netID))
		}
	}

	return lines
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

	// Read-only actions (always available)
	buttons = append(buttons, btn("x", "SSH"))

	if !s.Locked {
		// Power state dependent (mutating, hidden when locked)
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

		// Other mutating actions
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

	const more = "[?]More"
	maxWidth := m.width - 4
	fullLen := 0
	for _, b := range buttons {
		fullLen += len("["+b.key+"]") + len(b.label) + 1 // +1 for space
	}
	// When not everything fits, reserve room for the "[?]More" marker so the
	// bar never exceeds the terminal width.
	limit := maxWidth
	if fullLen > maxWidth {
		limit = maxWidth - len(more) - 1
	}

	var parts []string
	totalLen := 0
	for _, b := range buttons {
		part := keyStyle.Render("["+b.key+"]") + labelStyle.Render(b.label)
		partLen := len("["+b.key+"]") + len(b.label) + 1 // +1 for space
		if totalLen+partLen > limit && len(parts) > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(more))
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
	inst := m.inst
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[serverdetail] fetchServer start")
		srv, err := compute.GetServer(ctx, client, id)
		if err != nil {
			shared.Debugf("[serverdetail] fetchServer error: %v", err)
			return serverDetailErrMsg{inst: inst, err: err}
		}
		shared.Debugf("[serverdetail] fetchServer done")
		return serverDetailLoadedMsg{inst: inst, server: srv}
	}
}

func (m Model) fetchConsole() tea.Cmd {
	client := m.client
	id := m.serverID
	inst := m.inst
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[serverdetail] fetchConsole start")
		output, err := compute.GetConsoleOutput(ctx, client, id, maxConsoleLines)
		if err != nil {
			shared.Debugf("[serverdetail] fetchConsole error: %v", err)
			return consoleErrMsg{inst: inst, err: err}
		}
		shared.Debugf("[serverdetail] fetchConsole done: %d chars", len(output))
		return consoleLoadedMsg{inst: inst, output: output}
	}
}

func (m Model) fetchActions() tea.Cmd {
	client := m.client
	id := m.serverID
	inst := m.inst
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[serverdetail] fetchActions start")
		actions, err := compute.ListActions(ctx, client, id)
		if err != nil {
			shared.Debugf("[serverdetail] fetchActions error: %v", err)
			return actionsErrMsg{inst: inst, err: err}
		}
		shared.Debugf("[serverdetail] fetchActions done: %d actions", len(actions))
		return actionsLoadedMsg{inst: inst, actions: actions}
	}
}

func (m Model) fetchInterfaces() tea.Cmd {
	client := m.networkClient
	id := m.serverID
	inst := m.inst
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[serverdetail] fetchInterfaces start")
		ports, err := network.ListPortsByDevice(ctx, client, id)
		if err != nil {
			shared.Debugf("[serverdetail] fetchInterfaces error: %v", err)
			return interfacesErrMsg{inst: inst, err: err}
		}
		shared.Debugf("[serverdetail] fetchInterfaces done: %d ports", len(ports))
		return interfacesLoadedMsg{inst: inst, ports: ports}
	}
}

func (m Model) fetchVolumeInfo(attachments []compute.VolumeAttachment) tea.Cmd {
	client := m.blockClient
	inst := m.inst
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		vols := make(map[string]*volume.Volume)
		for _, va := range attachments {
			v, err := volume.GetVolume(ctx, client, va.ID)
			if err == nil {
				vols[va.ID] = v
			}
		}
		return volumeInfoLoadedMsg{inst: inst, volumes: vols}
	}
}

// ForceRefresh triggers a manual reload of all data sources.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[serverdetail] ForceRefresh()")
	m.loading = true
	cmds := []tea.Cmd{m.spinner.Tick, m.fetchServer()}
	if m.canPollDetailAPIs() {
		m.consoleLoading = true
		m.actionsLoading = true
		m.interfacesLoading = m.networkClient != nil
		cmds = append(cmds, m.fetchConsole(), m.fetchActions())
		if m.networkClient != nil {
			cmds = append(cmds, m.fetchInterfaces())
		}
	} else {
		m.consoleLoading = false
		m.actionsLoading = false
		m.interfacesLoading = false
		m.consoleErr = ""
		m.actionsErr = ""
		m.interfacesErr = ""
	}
	return tea.Batch(cmds...)
}

// SetSize updates the dimensions and reclamps every pane's scroll offset.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.clampScrolls()
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

// CopyEntries returns the title and copyable fields for the loaded server.
func (m Model) CopyEntries() (string, []copypicker.Entry) {
	s := m.server
	if s == nil {
		return "", nil
	}
	b := copypicker.Builder{}
	b.Add("ID", s.ID).
		Add("Name", s.Name).
		AddEach("IPv4", s.IPv4).
		AddEach("IPv6", s.IPv6).
		AddEach("Floating IP", s.FloatingIP)
	return "Copy — server " + s.Name, b.Entries()
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

// SetServer updates the server data directly. Data for any other server
// than the one this view shows is ignored.
func (m *Model) SetServer(s *compute.Server) {
	if s != nil && s.ID != m.serverID {
		return
	}
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
	case focusInterfaces:
		return "\u2191\u2193 scroll interfaces \u2022 " + base
	case focusVolumes:
		if m.blockClient == nil {
			return "\u2191\u2193 select \u2022 enter detail \u2022 " + shared.Keys.AssignFIP.Help().Key + " assign FIP \u2022 " + base
		}
		return "\u2191\u2193 select \u2022 enter detail \u2022 " + shared.Keys.Attach.Help().Key + " attach volume \u2022 ^t detach \u2022 " +
			shared.Keys.AssignFIP.Help().Key + " assign FIP \u2022 " + base
	case focusConsole:
		return "\u2191\u2193 scroll log \u2022 g top \u2022 G bottom \u2022 " + base
	case focusActions:
		return "\u2191\u2193 scroll actions \u2022 " + base
	}
	return base
}

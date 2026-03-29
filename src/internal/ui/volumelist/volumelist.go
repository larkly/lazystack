package volumelist

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type volumesLoadedMsg struct{ volumes []volume.Volume }
type volumesErrMsg struct{ err error }
type serverNamesMsg map[string]string
type tickMsg struct{}
type sortClearMsg struct{}

// Column defines a volume list column.
type Column struct {
	Title    string
	MinWidth int
	Flex     int
	Priority int
	Key      string
	width    int
	hidden   bool
}

func defaultColumns() []Column {
	return []Column{
		{Title: "Name", MinWidth: 10, Flex: 3, Priority: 0, Key: "name"},
		{Title: "ID", MinWidth: 10, Flex: 0, Priority: 2, Key: "id"},
		{Title: "Status", MinWidth: 14, Flex: 0, Priority: 0, Key: "status"},
		{Title: "Size", MinWidth: 6, Flex: 0, Priority: 0, Key: "size"},
		{Title: "Type", MinWidth: 8, Flex: 1, Priority: 1, Key: "type"},
		{Title: "Attached To", MinWidth: 10, Flex: 2, Priority: 1, Key: "attached"},
		{Title: "Device", MinWidth: 8, Flex: 0, Priority: 2, Key: "device"},
		{Title: "Bootable", MinWidth: 8, Flex: 0, Priority: 3, Key: "bootable"},
	}
}

func computeWidths(columns []Column, totalWidth int) []Column {
	for i := range columns {
		columns[i].hidden = false
		columns[i].width = columns[i].MinWidth
	}

	maxPrio := 0
	for _, c := range columns {
		if c.Priority > maxPrio {
			maxPrio = c.Priority
		}
	}

	for prio := maxPrio; prio >= 0; prio-- {
		if fitsWidth(columns, totalWidth) {
			break
		}
		for i := range columns {
			if columns[i].Priority == prio {
				columns[i].hidden = true
			}
		}
	}

	gaps := -1
	totalMin := 0
	totalFlex := 0
	for _, c := range columns {
		if c.hidden {
			continue
		}
		gaps++
		totalMin += c.MinWidth
		totalFlex += c.Flex
	}
	if gaps < 0 {
		gaps = 0
	}

	available := totalWidth - 2 - gaps
	if available < 0 {
		available = 0
	}

	remaining := available - totalMin
	if remaining > 0 && totalFlex > 0 {
		for i := range columns {
			if columns[i].hidden || columns[i].Flex == 0 {
				continue
			}
			extra := remaining * columns[i].Flex / totalFlex
			columns[i].width += extra
		}
	}

	return columns
}

func fitsWidth(columns []Column, totalWidth int) bool {
	needed := 2
	first := true
	for _, c := range columns {
		if c.hidden {
			continue
		}
		if !first {
			needed++
		}
		first = false
		needed += c.MinWidth
	}
	return needed <= totalWidth
}

// Model is the volume list view.
type Model struct {
	client          *gophercloud.ServiceClient
	computeClient   *gophercloud.ServiceClient
	volumes         []volume.Volume
	columns         []Column
	cursor          int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	err             string
	scrollOff       int
	refreshInterval time.Duration
	serverNames     map[string]string
	sortCol         int
	sortAsc         bool
	sortHighlight   bool
	sortClearAt     time.Time
	highlight       map[string]bool // volume IDs to highlight (from cross-resource navigation)
}

// New creates a volume list model.
func New(client, computeClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:          client,
		computeClient:   computeClient,
		columns:         defaultColumns(),
		loading:         true,
		spinner:         s,
		refreshInterval: refreshInterval,
		serverNames:     make(map[string]string),
		sortAsc:         true,
	}
}

// Init starts the initial fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchVolumes(), m.tickCmd())
}

// SelectedVolume returns the volume under the cursor.
func (m Model) SelectedVolume() *volume.Volume {
	if m.cursor >= 0 && m.cursor < len(m.volumes) {
		v := m.volumes[m.cursor]
		return &v
	}
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volumesLoadedMsg:
		m.loading = false
		m.volumes = msg.volumes
		m.err = ""
		m.sortVolumes()
		m.applyHighlight()
		return m, m.fetchMissingServerNames()

	case volumesErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case serverNamesMsg:
		for id, name := range msg {
			m.serverNames[id] = name
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchVolumes(), m.tickCmd())

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.columns = computeWidths(m.columns, m.width)
		return m, nil

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Sort):
			visibleCount := m.visibleColCount()
			if visibleCount > 0 {
				m.sortCol = (m.sortCol + 1) % visibleCount
				m.sortAsc = true
				m.sortHighlight = true
				m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
				m.sortVolumes()
				return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
					return sortClearMsg{}
				})
			}
		case key.Matches(msg, shared.Keys.ReverseSort):
			if m.visibleColCount() > 0 {
				m.sortAsc = !m.sortAsc
				m.sortHighlight = true
				m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
				m.sortVolumes()
				return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
					return sortClearMsg{}
				})
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.volumes)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.tableHeight()
			if m.cursor >= len(m.volumes) {
				m.cursor = len(m.volumes) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.tableHeight()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		}
	}
	return m, nil
}

func (m *Model) ensureVisible() {
	th := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+th {
		m.scrollOff = m.cursor - th + 1
	}
}

func (m Model) tableHeight() int {
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) visibleColCount() int {
	n := 0
	for _, col := range m.columns {
		if !col.hidden {
			n++
		}
	}
	return n
}

func (m Model) visibleColKey(idx int) string {
	n := 0
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		if n == idx {
			return col.Key
		}
		n++
	}
	return ""
}

func (m *Model) sortVolumes() {
	if len(m.volumes) == 0 {
		return
	}
	colKey := m.visibleColKey(m.sortCol)
	if colKey == "" {
		return
	}
	asc := m.sortAsc
	sort.SliceStable(m.volumes, func(i, j int) bool {
		a, b := m.volumes[i], m.volumes[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "id":
			less = a.ID < b.ID
		case "status":
			less = a.Status < b.Status
		case "size":
			less = a.Size < b.Size
		case "type":
			less = strings.ToLower(a.VolumeType) < strings.ToLower(b.VolumeType)
		case "attached":
			an := m.serverName(a.AttachedServerID)
			bn := m.serverName(b.AttachedServerID)
			less = strings.ToLower(an) < strings.ToLower(bn)
		case "device":
			less = a.AttachedDevice < b.AttachedDevice
		case "bootable":
			less = a.Bootable < b.Bootable
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

func (m Model) serverName(id string) string {
	if name, ok := m.serverNames[id]; ok {
		return name
	}
	if len(id) > 8 {
		return id[:8] + "…"
	}
	return id
}

// View renders the volume list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Volumes")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.volumes))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.volumes) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No volumes found.") + "\n")
		return b.String()
	}

	// Header
	visIdx := 0
	header := m.renderRow(func(col Column) string {
		title := col.Title
		idx := visIdx
		visIdx++
		indicator := ""
		if idx == m.sortCol {
			if m.sortAsc {
				indicator = " ▲"
			} else {
				indicator = " ▼"
			}
		}
		if idx == m.sortCol && m.sortHighlight {
			return lipgloss.NewStyle().
				Width(col.width).
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(title + indicator)
		}
		return shared.StyleHeader.Width(col.width).Render(title + indicator)
	})
	b.WriteString(header + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width)) + "\n")

	th := m.tableHeight()
	end := m.scrollOff + th
	if end > len(m.volumes) {
		end = len(m.volumes)
	}

	for i := m.scrollOff; i < end; i++ {
		v := m.volumes[i]
		cursor := i == m.cursor

		name := v.Name
		if name == "" && len(v.ID) > 8 {
			name = v.ID[:8] + "…"
		}

		attached := ""
		device := ""
		if v.AttachedServerID != "" {
			attached = m.serverName(v.AttachedServerID)
			device = v.AttachedDevice
		}

		shortID := v.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		values := map[string]string{
			"name":     name,
			"id":       shortID,
			"status":   shared.StatusIcon(v.Status) + v.Status,
			"size":     fmt.Sprintf("%dGB", v.Size),
			"type":     v.VolumeType,
			"attached": attached,
			"device":   device,
			"bootable": v.Bootable,
		}

		row := m.renderDataRow(values, v.Status, cursor)
		b.WriteString(row + "\n")
	}

	return b.String()
}

func (m Model) renderRow(render func(Column) string) string {
	var parts []string
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		parts = append(parts, render(col))
	}
	return "  " + strings.Join(parts, " ")
}

func (m Model) renderDataRow(values map[string]string, status string, cursor bool) string {
	var rowBg color.Color
	hasBg := false
	if cursor {
		rowBg = lipgloss.Color("#073642")
		hasBg = true
	}

	var parts []string
	for _, col := range m.columns {
		if col.hidden {
			continue
		}
		val := values[col.Key]
		w := col.width
		if len(val) > w && w > 1 {
			val = val[:w-1] + "…"
		}

		style := lipgloss.NewStyle().Width(w)
		if col.Key == "status" {
			style = volumeStatusStyle(status).Width(w)
		}
		if cursor {
			style = style.Bold(true)
		}
		if hasBg {
			style = style.Background(rowBg)
		}

		parts = append(parts, style.Render(val))
	}

	prefix := "  "
	prefixStyle := lipgloss.NewStyle()
	gapStyle := lipgloss.NewStyle()
	if hasBg {
		prefixStyle = prefixStyle.Background(rowBg)
		gapStyle = gapStyle.Background(rowBg)
	}

	gap := gapStyle.Render(" ")
	row := prefixStyle.Render(prefix) + strings.Join(parts, gap)

	if hasBg {
		rowW := lipgloss.Width(row)
		if rowW < m.width {
			row += gapStyle.Render(strings.Repeat(" ", m.width-rowW))
		}
	}

	return row
}

func volumeStatusStyle(status string) lipgloss.Style {
	var fg color.Color = shared.ColorFg
	switch status {
	case "available":
		fg = shared.ColorSuccess
	case "in-use":
		fg = shared.ColorCyan
	case "creating", "downloading", "uploading", "extending":
		fg = shared.ColorWarning
	case "error", "error_deleting", "error_restoring":
		fg = shared.ColorError
	case "deleting":
		fg = shared.ColorMuted
	}
	return lipgloss.NewStyle().Foreground(fg)
}

func (m Model) fetchVolumes() tea.Cmd {
	client := m.client
	if client == nil {
		return func() tea.Msg {
			return volumesErrMsg{err: fmt.Errorf("block storage service not available")}
		}
	}
	return func() tea.Msg {
		vols, err := volume.ListVolumes(context.Background(), client)
		if err != nil {
			return volumesErrMsg{err: err}
		}
		return volumesLoadedMsg{volumes: vols}
	}
}

func (m Model) fetchMissingServerNames() tea.Cmd {
	if m.computeClient == nil {
		return nil
	}
	missing := make(map[string]bool)
	for _, v := range m.volumes {
		if v.AttachedServerID != "" {
			if _, ok := m.serverNames[v.AttachedServerID]; !ok {
				missing[v.AttachedServerID] = true
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	client := m.computeClient
	return func() tea.Msg {
		result := make(serverNamesMsg)
		for id := range missing {
			srv, err := compute.GetServer(context.Background(), client, id)
			if err == nil && srv != nil {
				result[id] = srv.Name
			}
		}
		return result
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ForceRefresh triggers a manual reload of the volume list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchVolumes())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.columns = computeWidths(m.columns, w)
}

// SetHighlight marks volume IDs for cursor positioning (cross-resource navigation).
func (m *Model) SetHighlight(ids []string) {
	m.highlight = make(map[string]bool, len(ids))
	for _, id := range ids {
		m.highlight[id] = true
	}
	m.applyHighlight()
}

func (m *Model) applyHighlight() {
	if len(m.highlight) == 0 {
		return
	}
	for i, v := range m.volumes {
		if m.highlight[v.ID] {
			m.cursor = i
			m.ensureVisible()
			m.highlight = nil
			return
		}
	}
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ navigate • enter detail • ^n create • ^d delete • R refresh • 1-5/←→ switch tab • ? help"
}

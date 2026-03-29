package serverlist

import (
	"context"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type serversLoadedMsg struct {
	servers []compute.Server
}

type serversErrMsg struct {
	err error
}

type sortClearMsg struct{}

type imageNamesMsg map[string]string // image ID → name

// Model is the server list view.
type Model struct {
	client          *gophercloud.ServiceClient
	imageClient     *gophercloud.ServiceClient
	servers         []compute.Server
	filtered        []compute.Server
	columns         []Column
	cursor          int
	width           int
	height          int
	loading         bool
	spinner         spinner.Model
	filter          textinput.Model
	filtering       bool
	err             string
	scrollOff       int
	refreshInterval time.Duration
	imageNames      map[string]string // cache of image ID → name
	selected        map[string]bool   // selected server IDs for bulk actions
	sortCol         int
	sortAsc         bool
	sortHighlight   bool
	sortClearAt     time.Time
}

// New creates a new server list model.
func New(client, imageClient *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	fi := textinput.New()
	fi.Prompt = ""
	fi.Placeholder = "filter..."
	fi.CharLimit = 64
	fi.SetVirtualCursor(false)

	return Model{
		client:          client,
		imageClient:     imageClient,
		columns:         DefaultColumns(),
		loading:         true,
		spinner:         s,
		imageNames:      make(map[string]string),
		selected:        make(map[string]bool),
		filter:          fi,
		refreshInterval: refreshInterval,
		sortAsc:         true,
	}
}

// Init starts the initial server fetch and auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchServers(),
	)
}

// SelectedServer returns the currently selected server, if any.
func (m Model) SelectedServer() *compute.Server {
	if len(m.filtered) == 0 {
		return nil
	}
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		s := m.filtered[m.cursor]
		return &s
	}
	return nil
}

// Update handles messages for the server list.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case serversLoadedMsg:
		m.loading = false
		m.servers = msg.servers
		// Apply cached image names
		for i := range m.servers {
			if m.servers[i].ImageName == "" && m.servers[i].ImageID != "" {
				if name, ok := m.imageNames[m.servers[i].ImageID]; ok {
					m.servers[i].ImageName = name
				}
			}
		}
		m.err = ""
		m.applyFilter()
		// Fetch any unknown image names
		cmd := m.fetchMissingImageNames()
		if cmd != nil {
			return m, tea.Batch(cmd, m.tickCmd())
		}
		return m, m.tickCmd()

	case imageNamesMsg:
		for id, name := range msg {
			m.imageNames[id] = name
		}
		for i := range m.servers {
			if m.servers[i].ImageName == "" && m.servers[i].ImageID != "" {
				if name, ok := m.imageNames[m.servers[i].ImageID]; ok {
					m.servers[i].ImageName = name
				}
			}
		}
		m.applyFilter()
		return m, nil

	case serversErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, m.tickCmd()

	case shared.TickMsg:
		return m, m.fetchServers()

	case shared.RefreshServersMsg:
		m.loading = true
		return m, tea.Batch(m.spinner.Tick, m.fetchServers())

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
		m.columns = ComputeWidths(m.columns, m.width)
		return m, nil

	case sortClearMsg:
		m.sortHighlight = false
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateNormal(msg)
	}

	return m, nil
}

func (m Model) updateNormal(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Sort):
		visibleCount := m.visibleColCount()
		if visibleCount > 0 {
			var cursorID string
			if m.cursor >= 0 && m.cursor < len(m.filtered) {
				cursorID = m.filtered[m.cursor].ID
			}
			m.sortCol = (m.sortCol + 1) % visibleCount
			m.sortAsc = true
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortServers()
			if cursorID != "" {
				for i, s := range m.filtered {
					if s.ID == cursorID {
						m.cursor = i
						break
					}
				}
			}
			return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return sortClearMsg{}
			})
		}
	case key.Matches(msg, shared.Keys.ReverseSort):
		if m.visibleColCount() > 0 {
			var cursorID string
			if m.cursor >= 0 && m.cursor < len(m.filtered) {
				cursorID = m.filtered[m.cursor].ID
			}
			m.sortAsc = !m.sortAsc
			m.sortHighlight = true
			m.sortClearAt = time.Now().Add(1500 * time.Millisecond)
			m.sortServers()
			if cursorID != "" {
				for i, s := range m.filtered {
					if s.ID == cursorID {
						m.cursor = i
						break
					}
				}
			}
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
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case key.Matches(msg, shared.Keys.PageDown):
		m.cursor += m.tableHeight()
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
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
	case key.Matches(msg, shared.Keys.Filter):
		m.filtering = true
		m.filter.Focus()
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if s := m.SelectedServer(); s != nil {
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverdetail"}
			}
		}
	case key.Matches(msg, shared.Keys.Create):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "servercreate"}
		}
	case key.Matches(msg, shared.Keys.Delete):
		// Handled by root model (modal confirmation)
	case key.Matches(msg, shared.Keys.Reboot):
		// Handled by root model (modal confirmation)
	case key.Matches(msg, shared.Keys.Select):
		if s := m.SelectedServer(); s != nil {
			if m.selected[s.ID] {
				delete(m.selected, s.ID)
			} else {
				m.selected[s.ID] = true
			}
			// Move cursor down after selection
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.ensureVisible()
			}
		}
	}
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter.SetValue("")
		m.filter.Blur()
		m.applyFilter()
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m *Model) applyFilter() {
	var cursorID string
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		cursorID = m.filtered[m.cursor].ID
	}
	query := strings.ToLower(m.filter.Value())
	if query == "" {
		m.filtered = m.servers
	} else {
		m.filtered = nil
		for _, s := range m.servers {
			allIPs := strings.ToLower(strings.Join(s.IPv4, " ") + " " + strings.Join(s.IPv6, " ") + " " + strings.Join(s.FloatingIP, " "))
			if strings.Contains(strings.ToLower(s.Name), query) ||
				strings.Contains(strings.ToLower(s.ID), query) ||
				strings.Contains(strings.ToLower(s.Status), query) ||
				strings.Contains(allIPs, query) ||
				strings.Contains(strings.ToLower(s.FlavorName), query) ||
				strings.Contains(strings.ToLower(s.ImageName), query) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	m.sortServers()
	if cursorID != "" {
		for i, s := range m.filtered {
			if s.ID == cursorID {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.scrollOff = 0
}

func (m Model) visibleColCount() int {
	n := 0
	for _, col := range m.columns {
		if !col.Hidden() {
			n++
		}
	}
	return n
}

func (m Model) visibleColKey(idx int) string {
	n := 0
	for _, col := range m.columns {
		if col.Hidden() {
			continue
		}
		if n == idx {
			return col.Key
		}
		n++
	}
	return ""
}

func (m *Model) sortServers() {
	if len(m.filtered) == 0 {
		return
	}
	colKey := m.visibleColKey(m.sortCol)
	if colKey == "" {
		return
	}
	asc := m.sortAsc
	sort.SliceStable(m.filtered, func(i, j int) bool {
		a, b := m.filtered[i], m.filtered[j]
		var less bool
		switch colKey {
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case "status":
			less = a.Status < b.Status
		case "ipv4":
			ai, bi := "", ""
			if len(a.IPv4) > 0 {
				ai = a.IPv4[0]
			}
			if len(b.IPv4) > 0 {
				bi = b.IPv4[0]
			}
			less = ai < bi
		case "ipv6":
			ai, bi := "", ""
			if len(a.IPv6) > 0 {
				ai = a.IPv6[0]
			}
			if len(b.IPv6) > 0 {
				bi = b.IPv6[0]
			}
			less = ai < bi
		case "floating":
			ai, bi := "", ""
			if len(a.FloatingIP) > 0 {
				ai = a.FloatingIP[0]
			}
			if len(b.FloatingIP) > 0 {
				bi = b.FloatingIP[0]
			}
			less = ai < bi
		case "flavor":
			less = strings.ToLower(a.FlavorName) < strings.ToLower(b.FlavorName)
		case "image":
			less = strings.ToLower(a.ImageName) < strings.ToLower(b.ImageName)
		case "age":
			less = a.Created.Before(b.Created)
		case "key":
			less = strings.ToLower(a.KeyName) < strings.ToLower(b.KeyName)
		case "id":
			less = a.ID < b.ID
		default:
			less = false
		}
		if !asc {
			return !less
		}
		return less
	})
}

func (m *Model) ensureVisible() {
	tableHeight := m.tableHeight()
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+tableHeight {
		m.scrollOff = m.cursor - tableHeight + 1
	}
}

func (m Model) tableHeight() int {
	// title + filter + header + separator + status bar
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the server list.
func (m Model) View() string {
	var b strings.Builder

	// Title
	title := shared.StyleTitle.Render("Servers")
	if m.loading {
		title += " " + m.spinner.View()
	}
	count := fmt.Sprintf(" (%d)", len(m.filtered))
	b.WriteString(title + shared.StyleHelp.Render(count) + "\n")

	// Filter bar
	if m.filtering {
		b.WriteString("  / " + m.filter.View() + "\n")
	} else if m.filter.Value() != "" {
		b.WriteString(shared.StyleHelp.Render(fmt.Sprintf("  filter: %s (/ to edit, esc to clear)", m.filter.Value())) + "\n")
	} else {
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.filtered) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No servers found. Press [c] to create one.") + "\n")
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
				Width(col.Width()).
				Foreground(shared.ColorHighlight).
				Bold(true).
				Render(title + indicator)
		}
		return shared.StyleHeader.Width(col.Width()).Render(title + indicator)
	})
	b.WriteString(header + "\n")
	sep := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(strings.Repeat("─", m.width))
	b.WriteString(sep + "\n")

	// Rows
	tableH := m.tableHeight()
	end := m.scrollOff + tableH
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.scrollOff; i < end; i++ {
		s := m.filtered[i]
		selected := i == m.cursor
		row := m.renderServerRow(s, selected)
		b.WriteString(row + "\n")
	}

	return b.String()
}

func (m Model) renderRow(render func(Column) string) string {
	var parts []string
	for _, col := range m.columns {
		if col.Hidden() {
			continue
		}
		parts = append(parts, render(col))
	}
	return "  " + strings.Join(parts, " ")
}

func (m Model) renderServerRow(s compute.Server, cursor bool) string {
	// Build combined status: "ACTIVE/RUNNING"
	statusVal := shared.StatusIcon(s.Status) + s.Status + "/" + s.PowerState

	// Selection and lock indicators on name
	nameVal := s.Name
	if s.Locked {
		nameVal = "🔒 " + nameVal
	}

	// Row prefix: selection marker
	prefix := "  "
	if m.selected[s.ID] {
		prefix = "● "
	}

	values := map[string]string{
		"name":     nameVal,
		"status":   statusVal,
		"ipv4":     strings.Join(s.IPv4, ", "),
		"ipv6":     strings.Join(s.IPv6, ", "),
		"floating": strings.Join(s.FloatingIP, ", "),
		"flavor":   s.FlavorName,
		"image":    s.ImageName,
		"age":      formatAge(s.Created),
		"key":      s.KeyName,
		"id":       s.ID,
	}

	isSelected := m.selected[s.ID]

	// Determine row background
	var rowBg color.Color
	hasBg := false
	if cursor {
		rowBg = lipgloss.Color("#073642")
		hasBg = true
	} else if isSelected {
		rowBg = lipgloss.Color("#1a1a2e")
		hasBg = true
	}

	var parts []string
	for _, col := range m.columns {
		if col.Hidden() {
			continue
		}
		val := values[col.Key]
		w := col.Width()
		if len(val) > w && w > 1 {
			val = val[:w-1] + "…"
		}

		style := lipgloss.NewStyle().Width(w)
		if col.Key == "status" {
			style = m.statusColumnStyle(s, w)
		}
		if isSelected {
			style = style.Foreground(shared.ColorPrimary)
		}
		if cursor {
			style = style.Bold(true)
		}
		if hasBg {
			style = style.Background(rowBg)
		}

		parts = append(parts, style.Render(val))
	}

	// Style the prefix and gaps with same background
	prefixStyle := lipgloss.NewStyle()
	gapStyle := lipgloss.NewStyle()
	if hasBg {
		prefixStyle = prefixStyle.Background(rowBg)
		gapStyle = gapStyle.Background(rowBg)
	}
	if isSelected {
		prefixStyle = prefixStyle.Foreground(shared.ColorPrimary)
	}

	gap := gapStyle.Render(" ")
	row := prefixStyle.Render(prefix) + strings.Join(parts, gap)

	// Pad to full width
	if hasBg {
		rowW := lipgloss.Width(row)
		if rowW < m.width {
			row += gapStyle.Render(strings.Repeat(" ", m.width-rowW))
		}
	}

	return row
}

func (m Model) statusColumnStyle(s compute.Server, w int) lipgloss.Style {
	// Use the more severe color between status and power state
	statusColor, ok := shared.StatusColors[s.Status]
	if !ok {
		statusColor = shared.ColorFg
	}
	// If status is ACTIVE but power is not RUNNING, use power color
	if s.Status == "ACTIVE" && s.PowerState != "RUNNING" {
		if pc, ok := shared.PowerColors[s.PowerState]; ok {
			statusColor = pc
		}
	}
	// If status indicates a problem, always use status color
	if s.Status == "ERROR" || s.Status == "SHUTOFF" {
		statusColor = shared.StatusColors[s.Status]
	}
	return lipgloss.NewStyle().Width(w).Foreground(statusColor)
}

func formatAge(created time.Time) string {
	if created.IsZero() {
		return ""
	}
	d := time.Since(created)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days > 365 {
			return fmt.Sprintf("%dy", days/365)
		}
		return fmt.Sprintf("%dd", days)
	}
}

func (m Model) fetchServers() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		servers, err := compute.ListServers(context.Background(), client)
		if err != nil {
			return serversErrMsg{err: err}
		}
		return serversLoadedMsg{servers: servers}
	}
}

func (m Model) fetchMissingImageNames() tea.Cmd {
	if m.imageClient == nil {
		return nil
	}
	// Collect image IDs we don't have names for
	missing := make(map[string]bool)
	for _, s := range m.servers {
		if s.ImageID != "" && s.ImageName == "" {
			if _, ok := m.imageNames[s.ImageID]; !ok {
				missing[s.ImageID] = true
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}

	client := m.imageClient
	return func() tea.Msg {
		result := make(imageNamesMsg)
		images, err := img.ListImages(context.Background(), client)
		if err != nil {
			return result // silently fail, names are optional
		}
		for _, image := range images {
			if missing[image.ID] {
				result[image.ID] = image.Name
			}
		}
		return result
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return shared.TickMsg{}
	})
}

// Hints returns context-sensitive key hints for the status bar.
func (m Model) Hints() string {
	if m.filtering {
		return "enter confirm • esc clear"
	}
	if len(m.selected) > 0 {
		return fmt.Sprintf("(%d selected) space toggle • ^d delete • ^o reboot • esc clear • ? help", len(m.selected))
	}
	return "↑↓ navigate • space select • enter detail • ^n create • ^d delete • ^o reboot • / filter • ? help"
}

// SelectedServers returns all selected servers, or the cursor server if none selected.
func (m Model) SelectedServers() []compute.Server {
	if len(m.selected) > 0 {
		var result []compute.Server
		for _, s := range m.filtered {
			if m.selected[s.ID] {
				result = append(result, s)
			}
		}
		return result
	}
	if s := m.SelectedServer(); s != nil {
		return []compute.Server{*s}
	}
	return nil
}

// ClearSelection clears all selected servers.
func (m *Model) ClearSelection() {
	m.selected = make(map[string]bool)
}

// SelectionCount returns the number of selected servers.
func (m Model) SelectionCount() int {
	return len(m.selected)
}

// ServerNames returns a set of all server names in the current list.
func (m Model) ServerNames() map[string]bool {
	names := make(map[string]bool, len(m.servers))
	for _, s := range m.servers {
		names[s.Name] = true
	}
	return names
}

// ForceRefresh triggers a manual reload of the server list.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchServers())
}

// SetClient updates the compute client.
func (m *Model) SetClient(client *gophercloud.ServiceClient) {
	m.client = client
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.columns = ComputeWidths(m.columns, w)
}

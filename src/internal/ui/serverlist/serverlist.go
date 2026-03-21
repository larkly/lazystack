package serverlist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bosse/lazystack/internal/shared"
	"github.com/bosse/lazystack/internal/compute"
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

// Model is the server list view.
type Model struct {
	client          *gophercloud.ServiceClient
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
}

// New creates a new server list model.
func New(client *gophercloud.ServiceClient, refreshInterval time.Duration) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	fi := textinput.New()
	fi.Placeholder = "filter..."
	fi.CharLimit = 64

	return Model{
		client:          client,
		columns:         DefaultColumns(),
		loading:         true,
		spinner:         s,
		filter:          fi,
		refreshInterval: refreshInterval,
	}
}

// Init starts the initial server fetch and auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchServers(),
		m.tickCmd(),
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
		m.err = ""
		m.applyFilter()
		return m, nil

	case serversErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case shared.TickMsg:
		return m, tea.Batch(m.fetchServers(), m.tickCmd())

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
	case key.Matches(msg, shared.Keys.HardReboot):
		if msg.String() == "R" {
			m.loading = true
			return m, tea.Batch(m.spinner.Tick, m.fetchServers())
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
	query := strings.ToLower(m.filter.Value())
	if query == "" {
		m.filtered = m.servers
	} else {
		m.filtered = nil
		for _, s := range m.servers {
			if strings.Contains(strings.ToLower(s.Name), query) ||
				strings.Contains(strings.ToLower(s.ID), query) ||
				strings.Contains(strings.ToLower(s.Status), query) ||
				strings.Contains(strings.ToLower(s.IP), query) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.scrollOff = 0
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
	header := m.renderRow(func(col Column) string {
		return shared.StyleHeader.Width(col.Width).Render(col.Title)
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
		parts = append(parts, render(col))
	}
	return "  " + strings.Join(parts, " ")
}

func (m Model) renderServerRow(s compute.Server, selected bool) string {
	values := map[string]string{
		"name":   s.Name,
		"status": s.Status,
		"ip":     s.IP,
		"flavor": s.FlavorID,
		"key":    s.KeyName,
		"id":     s.ID,
	}

	var parts []string
	for _, col := range m.columns {
		val := values[col.Key]
		if len(val) > col.Width {
			val = val[:col.Width-1] + "…"
		}

		style := lipgloss.NewStyle().Width(col.Width)
		if col.Key == "status" {
			style = StatusStyle(s.Status).Width(col.Width)
		}
		if selected {
			style = style.Background(lipgloss.Color("#073642")).Bold(true)
		}

		parts = append(parts, style.Render(val))
	}

	return "  " + strings.Join(parts, " ")
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
	return "↑↓ navigate • enter detail • c create • d delete • r reboot • / filter • ? help"
}

// SetClient updates the compute client.
func (m *Model) SetClient(client *gophercloud.ServiceClient) {
	m.client = client
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

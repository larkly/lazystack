package servicecatalog

import (
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type catalogLoadedMsg struct {
	entries []compute.ServiceEntry
}

// Model is the service catalog browser.
type Model struct {
	client       *gophercloud.ProviderClient
	endpointOpts gophercloud.EndpointOpts
	entries      []compute.ServiceEntry
	cursor       int
	scroll       int
	width        int
	height       int
	loading      bool
	spinner      spinner.Model
	err          string
}

// New creates a service catalog model.
func New(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:       pc,
		endpointOpts: eo,
		loading:      true,
		spinner:      s,
	}
}

// Init fetches the catalog.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetch())
}

func (m Model) fetch() tea.Cmd {
	return func() tea.Msg {
		entries := compute.FetchServiceCatalog(m.client, m.endpointOpts)
		return catalogLoadedMsg{entries: entries}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogLoadedMsg:
		m.loading = false
		m.entries = msg.entries
		m.err = ""
		return m, nil

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
		switch {
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scroll {
					m.scroll = m.cursor
				}
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				visibleRows := m.height - 6
				if visibleRows > 0 && m.cursor >= m.scroll+visibleRows {
					m.scroll = m.cursor - visibleRows + 1
				}
			}
			return m, nil
		case key.Matches(msg, shared.Keys.PageUp):
			visibleRows := m.height - 6
			if visibleRows > 0 {
				m.cursor = max(0, m.cursor-visibleRows)
				if m.cursor < m.scroll {
					m.scroll = m.cursor
				}
			}
			return m, nil
		case key.Matches(msg, shared.Keys.PageDown):
			visibleRows := m.height - 6
			if visibleRows > 0 {
				m.cursor = min(len(m.entries)-1, m.cursor+visibleRows)
				if m.cursor >= m.scroll+visibleRows {
					m.scroll = max(0, m.cursor-visibleRows+1)
				}
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		}
	}

	return m, nil
}

// ForceRefresh triggers a reload.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetch())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ select • Esc back • ? help"
}

// View renders the service catalog.
func (m Model) View() string {
	if m.loading {
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.spinner.View()+" Loading service catalog...",
			lipgloss.WithWhitespaceChars("."),
		)
	}

	if m.err != "" {
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: "+m.err),
		)
	}

	var b strings.Builder

	// Header
	b.WriteString(shared.StyleTitle.Render(" Service Catalog "))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("Services available in the current OpenStack cloud"))
	b.WriteString("\n\n")

	// Column headers
	headerStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	availableCount := 0
	for _, e := range m.entries {
		if e.Available {
			availableCount++
		}
	}
	header := fmt.Sprintf("%-4s %-8s %-30s %-50s",
		"", "Status", "Service", "Public Endpoint")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", min(m.width, 100)))
	b.WriteString("\n")

	visibleRows := m.height - 9
	if visibleRows <= 0 {
		visibleRows = 10
	}
	endIdx := min(len(m.entries), m.scroll+visibleRows)

	serviceFg := lipgloss.NewStyle().Foreground(shared.ColorFg)
	availableSt := lipgloss.NewStyle().Foreground(shared.ColorHighlight)
	unavailableSt := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	mutedSt := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	highlightBg := lipgloss.NewStyle().Background(lipgloss.Color("#2a2a3a"))

	for i := m.scroll; i < endIdx; i++ {
		e := m.entries[i]
		indicator := " "
		if i == m.cursor {
			indicator = ">"
		}

		status := "    —    "
		statusStyle := unavailableSt
		endpoint := "—"
		if e.Available {
			status = " ✓ ready "
			statusStyle = availableSt
			if len(e.Endpoints) > 0 {
				endpoint = e.Endpoints[0].URL
			}
		}

		line := fmt.Sprintf("%-2s %-15s %-30s %-50s",
			indicator,
			statusStyle.Render(status),
			serviceFg.Render(e.Name),
			mutedSt.Render(truncateStr(endpoint, 48)),
		)

		if i == m.cursor {
			line = highlightBg.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	footer := fmt.Sprintf("%d/%d available — use ↑/↓ to navigate, Esc to go back",
		availableCount, len(m.entries))
	b.WriteString(shared.StyleHelp.Render(footer))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Left, lipgloss.Top,
		b.String(),
	)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

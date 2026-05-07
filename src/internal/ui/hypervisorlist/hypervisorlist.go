package hypervisorlist

import (
	"context"
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

type hypervisorsLoadedMsg struct {
	items []compute.Hypervisor
}

type hypervisorsErrMsg struct {
	err error
}

// Model is the hypervisor list viewer.
type Model struct {
	client  *gophercloud.ServiceClient
	items   []compute.Hypervisor
	cursor  int
	scroll  int
	width   int
	height  int
	loading bool
	spinner spinner.Model
	err     string
}

// New creates a hypervisor list model.
func New(client *gophercloud.ServiceClient) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:  client,
		loading: true,
		spinner: s,
	}
}

// Init fetches hypervisors.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetch())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hypervisorsLoadedMsg:
		m.loading = false
		m.items = msg.items
		m.err = ""
		return m, nil

	case hypervisorsErrMsg:
		m.loading = false
		m.err = msg.err.Error()
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
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scroll {
					m.scroll = m.cursor
				}
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.scroll+visible {
					m.scroll = m.cursor - visible + 1
				}
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.cursor -= m.visibleRows()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.scroll = m.cursor
		case key.Matches(msg, shared.Keys.PageDown):
			m.cursor += m.visibleRows()
			if m.cursor >= len(m.items) {
				m.cursor = len(m.items) - 1
			}
			visible := m.visibleRows()
			if m.cursor >= m.scroll+visible {
				m.scroll = m.cursor - visible + 1
			}
		}
	}
	return m, nil
}

func (m Model) visibleRows() int {
	h := m.height - 4 // title + status bar + header
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the hypervisor list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Hypervisors")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.items) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No hypervisors found.") + "\n")
		return b.String()
	}

	// Header row
	header := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
	headerRow := fmt.Sprintf("  %-30s %-12s %-10s %-10s %-12s",
		"Hostname", "Status", "VCPUs", "RAM (GB)", "Running VMs")
	b.WriteString(header.Render(headerRow) + "\n")

	visible := m.visibleRows()
	end := m.scroll + visible
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.scroll; i < end; i++ {
		h := m.items[i]
		prefix := "  "
		if i == m.cursor {
			prefix = "▶ "
		}

		ramUsedGB := h.MemoryMBUsed / 1024
		ramTotalGB := h.MemoryMB / 1024

		row := fmt.Sprintf("%s%-30s %-12s %s/%s %s/%s %s%s",
			prefix,
			truncate(h.Name, 29),
			h.Status,
			formatInt(h.VCPUsUsed), formatInt(h.VCPUs),
			formatInt(ramUsedGB), formatInt(ramTotalGB),
			formatInt(h.RunningVMs),
			"",
		)

		style := lipgloss.NewStyle()
		if i == m.cursor {
			style = style.Background(shared.ColorHighlight).Foreground(shared.ColorFg)
		} else if h.Status != "enabled" {
			style = style.Foreground(shared.ColorMuted)
		}
		// Truncate to width
		rowStr := style.Render(row)
		if len(rowStr) > m.width {
			rowStr = rowStr[:m.width-1]
		}
		b.WriteString(rowStr + "\n")
	}

	return b.String()
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ select • esc back • R refresh • ? help"
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

func (m Model) fetch() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		items, err := compute.ListHypervisors(context.Background(), client)
		if err != nil {
			return hypervisorsErrMsg{err: err}
		}
		return hypervisorsLoadedMsg{items: items}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func formatInt(n int) string {
	return fmt.Sprintf("%d", n)
}

package usermanagement

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

type usersLoadedMsg struct {
	items []compute.User
}

type usersErrMsg struct {
	err error
}

// Model is the user management viewer.
type Model struct {
	providerClient    *gophercloud.ProviderClient
	endpointOpts      gophercloud.EndpointOpts
	items             []compute.User
	cursor            int
	scroll            int
	width             int
	height            int
	loading           bool
	spinner           spinner.Model
	err               string
	confirmingDelete  string // user ID pending delete confirmation
	toggling          string // user ID being toggled
}

// New creates a user management model.
func New(pc *gophercloud.ProviderClient, eo gophercloud.EndpointOpts) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		providerClient: pc,
		endpointOpts:   eo,
		loading:        true,
		spinner:        s,
	}
}

// Init fetches users.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetch())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case usersLoadedMsg:
		m.loading = false
		m.items = msg.items
		m.err = ""
		m.confirmingDelete = ""
		m.toggling = ""
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		return m, nil

	case usersErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		m.confirmingDelete = ""
		m.toggling = ""
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
		// If confirming a delete
		if m.confirmingDelete != "" {
			switch {
			case msg.String() == "y":
				uid := m.confirmingDelete
				m.confirmingDelete = ""
				return m, m.doDelete(uid)
			case msg.String() == "n" || key.Matches(msg, shared.Keys.Back):
				m.confirmingDelete = ""
				return m, nil
			}
			return m, nil
		}

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

		case key.Matches(msg, shared.Keys.Enter):
			if m.cursor >= 0 && m.cursor < len(m.items) {
				return m, m.doToggle(m.items[m.cursor])
			}

		case msg.String() == "d":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				m.confirmingDelete = m.items[m.cursor].ID
			}
		}
	}

	return m, nil
}

func (m Model) visibleRows() int {
	h := m.height - 6 // title + header + footer + status bar
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the user management list.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("User Management")
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n")

	if m.confirmingDelete != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(
			fmt.Sprintf("  Really delete user? (y/n) ")) + "\n\n")
	}

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.items) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No users found.") + "\n")
		return b.String()
	}

	// Header row
	header := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorMuted)
	headerRow := fmt.Sprintf("  %-30s %-35s %-8s %-20s %-30s",
		"Name", "Email", "Enabled", "Domain", "Description")
	b.WriteString(header.Render(headerRow) + "\n")

	visible := m.visibleRows()
	end := m.scroll + visible
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.scroll; i < end; i++ {
		u := m.items[i]
		prefix := "  "
		if i == m.cursor {
			prefix = "▶ "
		}

		enabledStr := "No"
		if u.Enabled {
			enabledStr = "Yes"
		}

		row := fmt.Sprintf("%s%-30s %-35s %-8s %-20s %-30s",
			prefix,
			truncate(u.Name, 29),
			truncate(u.Email, 34),
			enabledStr,
			truncate(u.DomainID, 19),
			truncate(u.Description, 29),
		)

		style := lipgloss.NewStyle()
		if i == m.cursor {
			style = style.Background(shared.ColorHighlight).Foreground(shared.ColorFg)
		} else if !u.Enabled {
			style = style.Foreground(shared.ColorMuted)
		}
		rowStr := style.Render(row)
		if len(rowStr) > m.width {
			rowStr = rowStr[:m.width-1]
		}
		b.WriteString(rowStr + "\n")
	}

	// Footer
	b.WriteString("\n")
	footer := fmt.Sprintf("%d users — enter toggle • d delete • esc back • R refresh",
		len(m.items))
	b.WriteString(shared.StyleHelp.Render(footer))

	return b.String()
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ select • enter toggle • d delete • esc back • R refresh • ? help"
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
	pc := m.providerClient
	eo := m.endpointOpts
	return func() tea.Msg {
		items, err := compute.ListUsers(context.Background(), pc, eo)
		if err != nil {
			return usersErrMsg{err: err}
		}
		return usersLoadedMsg{items: items}
	}
}

func (m Model) doToggle(u compute.User) tea.Cmd {
	pc := m.providerClient
	eo := m.endpointOpts
	uid := u.ID
	wantEnabled := !u.Enabled
	return func() tea.Msg {
		err := compute.SetUserEnabled(context.Background(), pc, eo, uid, wantEnabled)
		if err != nil {
			return usersErrMsg{err: err}
		}
		// Re-fetch to get updated state
		items, err := compute.ListUsers(context.Background(), pc, eo)
		if err != nil {
			return usersErrMsg{err: err}
		}
		return usersLoadedMsg{items: items}
	}
}

func (m Model) doDelete(uid string) tea.Cmd {
	pc := m.providerClient
	eo := m.endpointOpts
	return func() tea.Msg {
		err := compute.DeleteUser(context.Background(), pc, eo, uid)
		if err != nil {
			return usersErrMsg{err: err}
		}
		items, err := compute.ListUsers(context.Background(), pc, eo)
		if err != nil {
			return usersErrMsg{err: err}
		}
		return usersLoadedMsg{items: items}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	return s[:n-1] + "…"
}

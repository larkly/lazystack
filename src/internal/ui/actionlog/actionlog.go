package actionlog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bosse/lazystack/internal/compute"
	"github.com/bosse/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type actionsLoadedMsg struct {
	actions []compute.Action
}

type actionsErrMsg struct {
	err error
}

// Model is the action history viewer.
type Model struct {
	client     *gophercloud.ServiceClient
	serverID   string
	serverName string
	actions    []compute.Action
	scroll     int
	width      int
	height     int
	loading    bool
	spinner    spinner.Model
	err        string
}

// New creates an action log viewer.
func New(client *gophercloud.ServiceClient, serverID, serverName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		client:     client,
		serverID:   serverID,
		serverName: serverName,
		loading:    true,
		spinner:    s,
	}
}

// Init fetches actions.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchActions())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case actionsLoadedMsg:
		m.loading = false
		m.actions = msg.actions
		m.err = ""
		return m, nil

	case actionsErrMsg:
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
				return shared.ViewChangeMsg{View: "serverdetail"}
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, shared.Keys.Down):
			max := len(m.actions) - m.viewHeight()
			if max < 0 {
				max = 0
			}
			if m.scroll < max {
				m.scroll++
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.scroll += m.viewHeight()
			max := len(m.actions) - m.viewHeight()
			if max < 0 {
				max = 0
			}
			if m.scroll > max {
				m.scroll = max
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.viewHeight()
			if m.scroll < 0 {
				m.scroll = 0
			}
		}
	}
	return m, nil
}

func (m Model) viewHeight() int {
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the action log.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Action History: " + m.serverName)
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.actions) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No actions recorded.") + "\n")
		return b.String()
	}

	// Header
	header := fmt.Sprintf("  %-16s %-20s %s", "Action", "Time", "Request ID")
	b.WriteString(shared.StyleHeader.Render(header) + "\n")

	vh := m.viewHeight()
	end := m.scroll + vh
	if end > len(m.actions) {
		end = len(m.actions)
	}

	for i := m.scroll; i < end; i++ {
		a := m.actions[i]
		age := formatAge(a.StartTime)
		ts := a.StartTime.Format("2006-01-02 15:04:05")
		reqID := a.RequestID
		if len(reqID) > 20 {
			reqID = reqID[:20] + "…"
		}

		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if a.Message != "" {
			style = style.Foreground(shared.ColorError)
		}

		line := fmt.Sprintf("  %-16s %-20s %s", a.Action, ts+" ("+age+")", reqID)
		b.WriteString(style.Render(line) + "\n")
	}

	return b.String()
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
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

// ForceRefresh triggers a manual reload of the action history.
func (m *Model) ForceRefresh() tea.Cmd {
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchActions())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "↑↓ scroll • R refresh • esc back • ? help"
}

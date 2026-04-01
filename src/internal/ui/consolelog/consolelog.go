package consolelog

import (
	"context"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const maxLines = 500

type consoleLoadedMsg struct {
	output string
}

type consoleErrMsg struct {
	err error
}

// Model is the console log viewer.
type Model struct {
	client     *gophercloud.ServiceClient
	serverID   string
	serverName string
	lines      []string
	scroll     int
	width      int
	height     int
	loading    bool
	spinner    spinner.Model
	err        string
}

// New creates a console log viewer.
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

// Init fetches console output.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[consolelog] Init() serverID=%s serverName=%q", m.serverID, m.serverName)
	return tea.Batch(m.spinner.Tick, m.fetchConsole())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case consoleLoadedMsg:
		m.loading = false
		m.lines = strings.Split(msg.output, "\n")
		m.err = ""
		// Scroll to bottom
		m.scroll = m.maxScroll()
		shared.Debugf("[consolelog] output loaded %d lines", len(m.lines))
		return m, nil

	case consoleErrMsg:
		m.loading = false
		m.err = msg.err.Error()
		shared.Debugf("[consolelog] error: %v", msg.err)
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
			max := m.maxScroll()
			if m.scroll < max {
				m.scroll++
			}
		case key.Matches(msg, shared.Keys.PageDown):
			m.scroll += m.viewHeight()
			max := m.maxScroll()
			if m.scroll > max {
				m.scroll = max
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.viewHeight()
			if m.scroll < 0 {
				m.scroll = 0
			}
		case msg.String() == "g":
			m.scroll = 0
		case msg.String() == "G":
			m.scroll = m.maxScroll()
		}
	}
	return m, nil
}

func (m Model) maxScroll() int {
	max := len(m.lines) - m.viewHeight()
	if max < 0 {
		return 0
	}
	return max
}

func (m Model) viewHeight() int {
	h := m.height - 3 // title + status bar
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the console log.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Console: " + m.serverName)
	if m.loading {
		title += " " + m.spinner.View()
	}
	b.WriteString(title + "\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  Error: "+m.err) + "\n")
		return b.String()
	}

	if len(m.lines) == 0 && !m.loading {
		b.WriteString(shared.StyleHelp.Render("  No console output available.") + "\n")
		return b.String()
	}

	vh := m.viewHeight()
	end := m.scroll + vh
	if end > len(m.lines) {
		end = len(m.lines)
	}

	for i := m.scroll; i < end; i++ {
		line := m.lines[i]
		if len(line) > m.width-2 {
			line = line[:m.width-2]
		}
		b.WriteString("  " + line + "\n")
	}

	return b.String()
}

func (m Model) fetchConsole() tea.Cmd {
	client := m.client
	id := m.serverID
	return func() tea.Msg {
		output, err := compute.GetConsoleOutput(context.Background(), client, id, maxLines)
		if err != nil {
			return consoleErrMsg{err: err}
		}
		return consoleLoadedMsg{output: output}
	}
}

// ForceRefresh triggers a manual reload of the console output.
func (m *Model) ForceRefresh() tea.Cmd {
	shared.Debugf("[consolelog] ForceRefresh()")
	m.loading = true
	return tea.Batch(m.spinner.Tick, m.fetchConsole())
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	return "↑↓ scroll • g top • G bottom • R refresh • esc back • ? help"
}

package serveradminact

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
)

type adminAction struct {
	label     string
	apiAction string
	needsHost bool
}

var adminActions = []adminAction{
	{"Cold Migrate", "Migrate", false},
	{"Live Migrate", "Live Migrate", true},
	{"Evacuate", "Evacuate", true},
	{"Force Delete", "Force Delete", false},
	{"Reset State", "Reset State", false},
}

var serverStates = []string{"active", "error"}

type actionDoneMsg struct {
	action string
	name   string
}

type actionErrMsg struct {
	action string
	name   string
	err    error
}

// Model is the admin server actions modal.
type Model struct {
	Active       bool
	client       *gophercloud.ServiceClient
	serverID     string
	serverName   string
	width        int
	height       int
	cursor       int
	promptStage  string // "" = picking, "host" = entering host, "state" = picking state, "confirm" = confirming
	stateCursor  int
	hostInput    textinput.Model
	submitting   bool
	err          string
	spinner      spinner.Model
}

// New creates an admin actions modal.
func New(client *gophercloud.ServiceClient, serverID, serverName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	hi := textinput.New()
	hi.Prompt = "Host: "
	hi.Placeholder = "compute-host-01"
	hi.CharLimit = 255

	return Model{
		Active:     true,
		client:     client,
		serverID:   serverID,
		serverName: serverName,
		hostInput:  hi,
		spinner:    s,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case actionDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: msg.action, Name: msg.name}
		}

	case actionErrMsg:
		m.submitting = false
		m.err = fmt.Sprintf("%s %s: %v", msg.action, msg.name, msg.err)
		return m, nil

	case spinner.TickMsg:
		if m.submitting {
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
	if m.submitting {
		return m, nil
	}

	if key.Matches(msg, shared.Keys.Back) {
		if m.promptStage != "" {
			m.promptStage = ""
			m.err = ""
			return m, nil
		}
		m.Active = false
		return m, nil
	}

	switch m.promptStage {
	case "":
		switch {
		case key.Matches(msg, shared.Keys.Up):
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(adminActions) - 1
			}
		case key.Matches(msg, shared.Keys.Down):
			m.cursor++
			if m.cursor >= len(adminActions) {
				m.cursor = 0
			}
		case key.Matches(msg, shared.Keys.Enter):
			return m, m.enterAction()
		}

	case "host":
		switch {
		case key.Matches(msg, shared.Keys.Enter):
			host := strings.TrimSpace(m.hostInput.Value())
			if host == "" {
				m.err = "Host name is required"
				return m, nil
			}
			m.submitting = true
			m.err = ""
			return m, m.executeWithHost(host)
		default:
			var cmd tea.Cmd
			m.hostInput, cmd = m.hostInput.Update(msg)
			return m, cmd
		}

	case "state":
		switch {
		case key.Matches(msg, shared.Keys.Up):
			m.stateCursor--
			if m.stateCursor < 0 {
				m.stateCursor = len(serverStates) - 1
			}
		case key.Matches(msg, shared.Keys.Down):
			m.stateCursor++
			if m.stateCursor >= len(serverStates) {
				m.stateCursor = 0
			}
		case key.Matches(msg, shared.Keys.Enter):
			state := serverStates[m.stateCursor]
			m.submitting = true
			m.err = ""
			a := adminActions[m.cursor]
			return m, m.executeAction(a, state)
		}

	case "confirm":
		switch {
		case key.Matches(msg, shared.Keys.Confirm), msg.String() == "y":
			m.submitting = true
			m.err = ""
			a := adminActions[m.cursor]
			return m, m.executeAction(a, "")
		case key.Matches(msg, shared.Keys.Deny), msg.String() == "n":
			m.promptStage = ""
			return m, nil
		}
	}
	return m, nil
}

func (m Model) enterAction() tea.Cmd {
	a := adminActions[m.cursor]
	switch {
	case a.needsHost:
		m.promptStage = "host"
		m.hostInput.SetValue("")
		m.hostInput.Focus()
		m.err = ""
		return textinput.Blink
	case a.apiAction == "Force Delete":
		m.promptStage = "confirm"
		m.err = ""
		return nil
	case a.apiAction == "Reset State":
		m.promptStage = "state"
		m.stateCursor = 0
		m.err = ""
		return nil
	default:
		// Cold Migrate: execute directly
		m.submitting = true
		m.err = ""
		return m.executeAction(a, "")
	}
}

func (m Model) executeWithHost(host string) tea.Cmd {
	a := adminActions[m.cursor]
	return m.executeAction(a, host)
}

func (m Model) executeAction(a adminAction, arg string) tea.Cmd {
	client := m.client
	id := m.serverID
	name := m.serverName
	action := a.apiAction

	return func() tea.Msg {
		shared.Debugf("[serveradminact] executing %s on server %s", action, id)
		var err error

		switch action {
		case "Migrate":
			err = compute.MigrateServer(context.Background(), client, id)
		case "Live Migrate":
			err = compute.LiveMigrateServer(context.Background(), client, id, arg)
		case "Evacuate":
			_, err = compute.EvacuateServer(context.Background(), client, id, arg, false)
		case "Force Delete":
			err = compute.ForceDeleteServer(context.Background(), client, id)
		case "Reset State":
			err = compute.ResetServerState(context.Background(), client, id, arg)
		}

		if err != nil {
			shared.Debugf("[serveradminact] %s failed: %v", action, err)
			return actionErrMsg{action: action, name: name, err: err}
		}
		shared.Debugf("[serveradminact] %s succeeded for %s", action, name)
		return actionDoneMsg{action: action, name: name}
	}
}

// View renders the admin actions modal.
func (m Model) View() string {
	if !m.Active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(shared.ColorPrimary).
		Bold(true).
		Padding(0, 1)
	title := titleStyle.Render(fmt.Sprintf("Admin Actions — %s", m.serverName))

	var body string
	switch m.promptStage {
	case "":
		body = m.renderActionList()
	case "host":
		body = m.renderHostPrompt()
	case "state":
		body = m.renderStatePrompt()
	case "confirm":
		body = m.renderConfirmPrompt()
	}

	if m.err != "" {
		body += "\n\n" + lipgloss.NewStyle().
			Foreground(shared.ColorError).
			Render("  Error: "+m.err)
	}

	if m.submitting {
		body += "\n\n  " + m.spinner.View() + " Executing..."
	}

	width := 60
	if m.width < 64 {
		width = m.width - 4
	}

	footer := lipgloss.NewStyle().
		Foreground(shared.ColorMuted).
		Render("  esc back • ↑↓ navigate • enter select")

	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(shared.ColorPrimary).
		Padding(1, 2).
		Render(title + "\n\n" + body + "\n\n" + footer)
}

func (m Model) renderActionList() string {
	var lines []string
	cursorStyle := lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	cursorMark := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render("▸ ")

	for i, a := range adminActions {
		prefix := "  "
		style := normalStyle
		if i == m.cursor {
			prefix = cursorMark
			style = cursorStyle
		}
		lines = append(lines, prefix+style.Render(a.label))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHostPrompt() string {
	labelStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).
		Bold(true)
	hintStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted)
	a := adminActions[m.cursor]
	return labelStyle.Render(fmt.Sprintf("  %s — Target Host:", a.label)) +
		"\n\n  " + m.hostInput.View() +
		"\n\n  " + hintStyle.Render("enter to execute • esc to cancel")
}

func (m Model) renderStatePrompt() string {
	var lines []string
	cursorStyle := lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	cursorMark := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render("▸ ")

	labelStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).Bold(true)
	lines = append(lines, labelStyle.Render("  Reset State — New State:")+"\n")

	for i, state := range serverStates {
		prefix := "  "
		style := normalStyle
		if i == m.stateCursor {
			prefix = cursorMark
			style = cursorStyle
		}
		lines = append(lines, prefix+style.Render(state))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderConfirmPrompt() string {
	warnStyle := lipgloss.NewStyle().
		Foreground(shared.ColorWarning).
		Bold(true)
	mutedStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted)

	return warnStyle.Render(fmt.Sprintf("  Force delete %s?", m.serverName)) +
		"\n\n  " + mutedStyle.Render("This cannot be undone. Are you sure?") +
		"\n\n  " + lipgloss.NewStyle().Foreground(shared.ColorFg).Render("[y] Yes  [n] No")
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

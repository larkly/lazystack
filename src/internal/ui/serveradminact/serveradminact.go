package serveradminact

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// ActionRequestMsg asks the root model to run an admin action. The modal
// only collects input; execution (deadline, in-flight lock, audit and
// result reporting) happens in the application so the outcome stays
// visible after the modal has closed.
type ActionRequestMsg struct {
	Action     string // "Migrate", "Live Migrate", "Evacuate", "Force Delete", "Reset State"
	ServerID   string
	ServerName string
	Arg        string // target host (Live Migrate, Evacuate) or state (Reset State)
}

// Model is the admin server actions modal.
type Model struct {
	Active      bool
	serverID    string
	serverName  string
	width       int
	height      int
	cursor      int
	promptStage string // "" = picking, "host" = entering host, "state" = picking state, "confirm" = confirming
	// confirmFrom is the stage the confirmation returns to on n/esc, and
	// confirmArg the host or state it will submit.
	confirmFrom string
	confirmArg  string
	stateCursor int
	hostInput   textinput.Model
	err         string
}

// New creates an admin actions modal.
func New(serverID, serverName string) Model {
	hi := textinput.New()
	hi.Prompt = "Host: "
	hi.Placeholder = "compute-host-01"
	hi.CharLimit = 255

	return Model{
		Active:     true,
		serverID:   serverID,
		serverName: serverName,
		hostInput:  hi,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
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
	if m.promptStage == "confirm" {
		switch {
		case key.Matches(msg, shared.Keys.Confirm):
			return m.submit(m.confirmArg)
		case key.Matches(msg, shared.Keys.Deny), key.Matches(msg, shared.Keys.Back):
			m.promptStage = m.confirmFrom
			if m.promptStage == "host" {
				return m, m.hostInput.Focus()
			}
		}
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
			return m.enterAction()
		}

	case "host":
		switch {
		case key.Matches(msg, shared.Keys.Enter):
			host := strings.TrimSpace(m.hostInput.Value())
			if host == "" {
				m.err = "Host name is required"
				return m, nil
			}
			m.hostInput.Blur()
			return m.confirm("host", host)
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
			return m.confirm("state", serverStates[m.stateCursor])
		}
	}
	return m, nil
}

// confirm asks for a y before submitting arg; n/esc return to stage from.
func (m Model) confirm(from, arg string) (Model, tea.Cmd) {
	m.promptStage = "confirm"
	m.confirmFrom = from
	m.confirmArg = arg
	m.err = ""
	return m, nil
}

// enterAction opens the prompt the selected action needs, or submits it.
// It returns the updated model: the prompt state must survive the call.
func (m Model) enterAction() (Model, tea.Cmd) {
	a := adminActions[m.cursor]
	m.err = ""
	switch {
	case a.needsHost:
		m.promptStage = "host"
		m.hostInput.SetValue("")
		return m, m.hostInput.Focus()
	case a.apiAction == "Reset State":
		m.promptStage = "state"
		m.stateCursor = 0
		return m, nil
	default:
		// Cold Migrate and Force Delete need no further input.
		return m.confirm("", "")
	}
}

// submit closes the modal and hands the request to the application. The
// modal is inactive afterwards, so a repeated key cannot submit twice.
func (m Model) submit(arg string) (Model, tea.Cmd) {
	req := ActionRequestMsg{
		Action:     adminActions[m.cursor].apiAction,
		ServerID:   m.serverID,
		ServerName: m.serverName,
		Arg:        arg,
	}
	shared.Debugf("[serveradminact] requesting %s on server %s", req.Action, req.ServerID)
	m.Active = false
	m.promptStage = ""
	m.confirmFrom, m.confirmArg = "", ""
	return m, func() tea.Msg { return req }
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

// confirmQuestion names the server and the target of the pending action.
func (m Model) confirmQuestion() string {
	switch adminActions[m.cursor].apiAction {
	case "Migrate":
		return fmt.Sprintf("Cold migrate %s?", m.serverName)
	case "Live Migrate":
		return fmt.Sprintf("Live migrate %s to host %s?", m.serverName, m.confirmArg)
	case "Evacuate":
		return fmt.Sprintf("Evacuate %s to host %s?", m.serverName, m.confirmArg)
	case "Reset State":
		return fmt.Sprintf("Reset state of %s to %s?", m.serverName, m.confirmArg)
	case "Force Delete":
		return fmt.Sprintf("Force delete %s?", m.serverName)
	}
	return fmt.Sprintf("%s %s?", adminActions[m.cursor].label, m.serverName)
}

func (m Model) renderConfirmPrompt() string {
	warnStyle := lipgloss.NewStyle().
		Foreground(shared.ColorWarning).
		Bold(true)
	mutedStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted)

	out := warnStyle.Render("  " + m.confirmQuestion())
	if adminActions[m.cursor].apiAction == "Force Delete" {
		out += "\n\n  " + mutedStyle.Render("This cannot be undone. Are you sure?")
	}
	return out + "\n\n  " + lipgloss.NewStyle().Foreground(shared.ColorFg).Render("[y] Yes  [n] No")
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns the key hints shown in the status bar.
func (m Model) Hints() string {
	switch m.promptStage {
	case "host":
		return "enter execute • esc back"
	case "state":
		return shared.Glyph("↑↓", "up/down") + " navigate • enter set state • esc back"
	case "confirm":
		return "y confirm • n/esc back"
	}
	return shared.Glyph("↑↓", "up/down") + " navigate • enter select • esc close"
}

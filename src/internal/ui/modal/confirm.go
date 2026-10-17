package modal

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
)

// ServerRef identifies a server for an action.
type ServerRef struct {
	ID   string
	Name string
	// Action is optional per-server concrete action for bulk operations.
	Action string
}

// ConfirmAction is the result of a confirmation dialog.
type ConfirmAction struct {
	Action        string
	ServerID      string
	Name          string
	Servers       []ServerRef // for bulk actions
	Confirm       bool
	DeleteVolumes bool     // delete attached volumes too
	VolumeIDs     []string // volume IDs to delete
}

// ConfirmModel is a confirmation dialog.
type ConfirmModel struct {
	Action   string
	ServerID string
	Name     string
	Servers  []ServerRef // for bulk actions
	Body     string      // custom body text (overrides default)
	Title    string      // custom title (overrides default)
	Width    int
	Height   int
	focused  int // 0 = confirm, 1 = cancel

	// Volume deletion checkbox
	VolumeIDs     []string // attached volume IDs
	deleteVolumes bool     // checkbox state
}

// NewConfirm creates a confirmation dialog for a single server.
func NewConfirm(action, serverID, name string) ConfirmModel {
	return ConfirmModel{
		Action:   action,
		ServerID: serverID,
		Name:     name,
		focused:  1, // default to cancel for safety
	}
}

// NewBulkConfirm creates a confirmation dialog for multiple servers. Its
// body names the servers, e.g. "Delete 3 servers: a, b, c".
func NewBulkConfirm(action string, servers []ServerRef) ConfirmModel {
	verb := action
	if verb != "" {
		verb = strings.ToUpper(verb[:1]) + verb[1:]
	}
	return ConfirmModel{
		Action:  action,
		Servers: servers,
		Name:    fmt.Sprintf("%d servers", len(servers)),
		Body:    fmt.Sprintf("%s %d servers: %s", verb, len(servers), ServerNames(servers)),
		focused: 1,
	}
}

// maxListedServers is how many names a bulk confirmation lists before
// summarizing the rest.
const maxListedServers = 5

// ServerNames lists up to five server names (the ID when a server has no
// name), followed by "and N more" for the rest.
func ServerNames(servers []ServerRef) string {
	var names []string
	for i, s := range servers {
		if i == maxListedServers {
			break
		}
		name := s.Name
		if name == "" {
			name = s.ID
		}
		names = append(names, name)
	}
	out := strings.Join(names, ", ")
	if rest := len(servers) - len(names); rest > 0 {
		out += fmt.Sprintf(" and %d more", rest)
	}
	return out
}

func (m ConfirmModel) confirmMsg() tea.Cmd {
	shared.Debugf("[confirm] confirmed action=%s name=%q", m.Action, m.Name)
	return func() tea.Msg {
		return ConfirmAction{
			Action:        m.Action,
			ServerID:      m.ServerID,
			Name:          m.Name,
			Servers:       m.Servers,
			Confirm:       true,
			DeleteVolumes: m.deleteVolumes,
			VolumeIDs:     m.VolumeIDs,
		}
	}
}

func (m ConfirmModel) cancelMsg() tea.Cmd {
	shared.Debugf("[confirm] cancelled action=%s name=%q", m.Action, m.Name)
	return func() tea.Msg {
		return ConfirmAction{
			Action:   m.Action,
			ServerID: m.ServerID,
			Name:     m.Name,
			Servers:  m.Servers,
			Confirm:  false,
		}
	}
}

// Update handles input for the confirmation dialog.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Confirm):
			return m, m.confirmMsg()
		case key.Matches(msg, shared.Keys.Deny):
			return m, m.cancelMsg()
		case key.Matches(msg, shared.Keys.Back):
			return m, m.cancelMsg()
		case key.Matches(msg, shared.Keys.Tab),
			key.Matches(msg, shared.Keys.ShiftTab),
			key.Matches(msg, shared.Keys.Left),
			key.Matches(msg, shared.Keys.Right),
			key.Matches(msg, shared.Keys.Up),
			key.Matches(msg, shared.Keys.Down):
			m.focused = 1 - m.focused
		case key.Matches(msg, shared.Keys.Select):
			if len(m.VolumeIDs) > 0 {
				m.deleteVolumes = !m.deleteVolumes
			}
		case key.Matches(msg, shared.Keys.Enter):
			if m.focused == 0 {
				return m, m.confirmMsg()
			}
			return m, m.cancelMsg()
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

// View renders the confirmation dialog.
func (m ConfirmModel) View() string {
	titleText := m.Title
	if titleText == "" {
		titleText = fmt.Sprintf("Confirm %s", m.Action)
	}
	title := shared.StyleModalTitle.Render(titleText)

	body := m.Body
	if body == "" {
		body = fmt.Sprintf("Are you sure you want to %s server %q?",
			m.Action, m.Name)
	}

	// Buttons
	confirmStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focused == 0 {
		confirmStyle = shared.StyleButtonSubmit
	} else {
		cancelStyle = shared.StyleButtonCancel
	}
	buttons := confirmStyle.Render("[y] Confirm") + "  " + cancelStyle.Render("[n] Cancel")

	// Volume deletion checkbox
	volCheckbox := ""
	if len(m.VolumeIDs) > 0 {
		check := "[ ]"
		if m.deleteVolumes {
			check = "[x]"
		}
		volCheckbox = fmt.Sprintf("\n\n%s delete %d attached volume(s)  (space to toggle)", check, len(m.VolumeIDs))
	}

	content := title + "\n\n" + body + volCheckbox + "\n\n" + buttons
	modalWidth := 50
	if len(m.VolumeIDs) > 0 {
		modalWidth = 70
	}
	box := shared.StyleModal.Width(shared.ModalWidth(modalWidth, m.Width)).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *ConfirmModel) SetSize(w, h int) {
	m.Width = w
	m.Height = h
}

// Hints returns the key hints shown in the status bar.
func (m ConfirmModel) Hints() string {
	if len(m.VolumeIDs) > 0 {
		return "y confirm • n/esc cancel • space toggle volumes • tab switch"
	}
	return "y confirm • n/esc cancel • tab switch"
}

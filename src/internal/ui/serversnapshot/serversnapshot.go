package serversnapshot

import (
	"context"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

type snapshotSuccessMsg struct{ name string }
type snapshotErrMsg struct{ err error }

// Model is the server snapshot overlay modal.
type Model struct {
	Active     bool
	client     *gophercloud.ServiceClient
	serverID   string
	serverName string
	nameInput  textinput.Model
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a snapshot modal for the given server.
func New(client *gophercloud.ServiceClient, id, serverName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "snapshot name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.SetValue(serverName + "-snapshot")
	ni.Focus()
	ni.CursorEnd()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:     true,
		client:     client,
		serverID:   id,
		serverName: serverName,
		nameInput:  ni,
		spinner:    s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotSuccessMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: "Snapshot created for", Name: msg.name}
		}
	case snapshotErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
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
		if m.submitting {
			return m, nil
		}
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		return m.submit()
	default:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Snapshot name cannot be empty"
		return m, nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	id := m.serverID
	serverName := m.serverName
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[serversnapshot] creating snapshot %q for server %s", name, id)
		err := compute.CreateSnapshot(context.Background(), client, id, name)
		if err != nil {
			shared.Debugf("[serversnapshot] error creating snapshot %q: %v", name, err)
			return snapshotErrMsg{err: err}
		}
		shared.Debugf("[serversnapshot] created snapshot %q for server %s", name, serverName)
		return snapshotSuccessMsg{name: serverName}
	})
}

// View renders the snapshot overlay.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Create Snapshot")

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Creating snapshot...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Render("Name")
	body.WriteString("  " + label + "  " + m.nameInput.View() + "\n\n")
	body.WriteString(shared.StyleHelp.Render("  enter: create  esc: cancel"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderModal(content string) string {
	modalWidth := 55
	if m.width > 0 && m.width < 65 {
		modalWidth = m.width - 6
	}
	box := shared.StyleModal.Width(modalWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

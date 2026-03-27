package serverrename

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

type renameSuccessMsg struct{ newName string }
type renameErrMsg struct{ err error }

// Model is the server rename overlay modal.
type Model struct {
	Active     bool
	client     *gophercloud.ServiceClient
	serverID   string
	origName   string
	nameInput  textinput.Model
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a rename modal for the given server.
func New(client *gophercloud.ServiceClient, id, name string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "server name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.SetValue(name)
	ni.Focus()
	ni.CursorEnd()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:   true,
		client:   client,
		serverID: id,
		origName: name,
		nameInput: ni,
		spinner:  s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case renameSuccessMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: "Renamed", Name: msg.newName}
		}
	case renameErrMsg:
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

	// Route textinput blink messages
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
	newName := strings.TrimSpace(m.nameInput.Value())
	if newName == "" {
		m.err = "Name cannot be empty"
		return m, nil
	}
	if newName == m.origName {
		m.Active = false
		return m, nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	id := m.serverID
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := compute.RenameServer(context.Background(), client, id, newName)
		if err != nil {
			return renameErrMsg{err: err}
		}
		return renameSuccessMsg{newName: newName}
	})
}

// View renders the rename overlay.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Rename Server")

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Renaming...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Render("Name")
	body.WriteString("  " + label + "  " + m.nameInput.View() + "\n\n")
	body.WriteString(shared.StyleHelp.Render("  enter: confirm  esc: cancel"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderModal(content string) string {
	modalWidth := 50
	if m.width > 0 && m.width < 60 {
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

package sgcreate

import (
	"context"
	"fmt"
	"strings"

	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

// Mode determines the modal's behavior.
type Mode int

const (
	ModeCreate Mode = iota
	ModeRename
	ModeClone
)

const (
	fieldName   = 0
	fieldDesc   = 1
	fieldSubmit = 2
	fieldCancel = 3
	numFields   = 4
)

type sgCreatedMsg struct{}
type sgCreateErrMsg struct{ err error }

// Model is the security group create/rename/clone modal.
type Model struct {
	Active    bool
	client    *gophercloud.ServiceClient
	nameInput textinput.Model
	descInput textinput.Model
	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
	mode       Mode
	sgID       string // target SG ID (for rename)
	srcSGID    string // source SG ID (for clone)
}

// New creates a security group create modal.
func New(client *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "security group name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "optional description"
	di.CharLimit = 255
	di.SetWidth(40)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:    true,
		client:    client,
		nameInput: ni,
		descInput: di,
		spinner:   s,
	}
}

// NewRename creates a rename modal pre-filled with the current name and description.
func NewRename(client *gophercloud.ServiceClient, sgID, currentName, currentDesc string) Model {
	m := New(client)
	m.mode = ModeRename
	m.sgID = sgID
	m.nameInput.SetValue(currentName)
	m.descInput.SetValue(currentDesc)
	return m
}

// NewClone creates a clone modal pre-filled with "Copy of <name>" and the same description.
func NewClone(client *gophercloud.ServiceClient, srcSGID, currentName, currentDesc string) Model {
	m := New(client)
	m.mode = ModeClone
	m.srcSGID = srcSGID
	m.nameInput.SetValue("Copy of " + currentName)
	m.descInput.SetValue(currentDesc)
	return m
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sgCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created security group"
		switch m.mode {
		case ModeRename:
			action = "Renamed security group"
		case ModeClone:
			action = "Cloned security group"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.nameInput.Value()}
		}
	case sgCreateErrMsg:
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
	return m, nil
}

func (m Model) isTextInput() bool {
	return m.focusField == fieldName || m.focusField == fieldDesc
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Route to text input first — only intercept navigation keys
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.focusField = (m.focusField - 1 + numFields) % numFields
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.focusField++
			m.updateFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			switch m.focusField {
			case fieldName:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			case fieldDesc:
				var cmd tea.Cmd
				m.descInput, cmd = m.descInput.Update(msg)
				return m, cmd
			}
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil

	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.focusField = (m.focusField + 1) % numFields
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.focusField = (m.focusField - 1 + numFields) % numFields
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.Right) && (m.focusField == fieldSubmit || m.focusField == fieldCancel):
		if m.focusField == fieldSubmit {
			m.focusField = fieldCancel
		} else {
			m.focusField = fieldSubmit
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Left) && (m.focusField == fieldSubmit || m.focusField == fieldCancel):
		if m.focusField == fieldCancel {
			m.focusField = fieldSubmit
		} else {
			m.focusField = fieldCancel
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldName, fieldDesc:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		}
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	switch m.focusField {
	case fieldName:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	case fieldDesc:
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldDesc {
		m.descInput.Focus()
	} else {
		m.descInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Security group name is required"
		return m, nil
	}

	desc := strings.TrimSpace(m.descInput.Value())
	m.submitting = true
	m.err = ""
	client := m.client

	switch m.mode {
	case ModeRename:
		sgID := m.sgID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			shared.Debugf("[sgcreate] renaming security group %s to %q", sgID, name)
			_, err := network.UpdateSecurityGroup(context.Background(), client, sgID, name, &desc)
			if err != nil {
				shared.Debugf("[sgcreate] error renaming security group %s: %v", sgID, err)
				return sgCreateErrMsg{err: err}
			}
			shared.Debugf("[sgcreate] renamed security group %s to %q", sgID, name)
			return sgCreatedMsg{}
		})
	case ModeClone:
		srcID := m.srcSGID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			shared.Debugf("[sgcreate] cloning security group %s as %q", srcID, name)
			_, err := network.CloneSecurityGroup(context.Background(), client, srcID, name, desc)
			if err != nil {
				shared.Debugf("[sgcreate] error cloning security group %s: %v", srcID, err)
				return sgCreateErrMsg{err: err}
			}
			shared.Debugf("[sgcreate] cloned security group %s as %q", srcID, name)
			return sgCreatedMsg{}
		})
	default:
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			shared.Debugf("[sgcreate] creating security group %q", name)
			_, err := network.CreateSecurityGroup(context.Background(), client, name, desc)
			if err != nil {
				shared.Debugf("[sgcreate] error creating security group %q: %v", name, err)
				return sgCreateErrMsg{err: err}
			}
			shared.Debugf("[sgcreate] created security group %q", name)
			return sgCreatedMsg{}
		})
	}
}

// View renders the modal.
func (m Model) View() string {
	titleText := "Create Security Group"
	switch m.mode {
	case ModeRename:
		titleText = "Rename Security Group"
	case ModeClone:
		titleText = "Clone Security Group"
	}
	title := shared.StyleModalTitle.Render(titleText)

	var body strings.Builder

	if m.submitting {
		body.WriteString(m.spinner.View() + " Creating...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	fields := []struct {
		label   string
		value   string
		focused bool
	}{
		{"Name", m.nameInput.View(), m.focusField == fieldName},
		{"Description", m.descInput.View(), m.focusField == fieldDesc},
	}

	for _, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := lipgloss.NewStyle().Width(14).Foreground(shared.ColorSecondary).Render(f.label)
		body.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, f.value))
	}

	body.WriteString("\n")
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
	}
	body.WriteString("  " + submitStyle.Render("[ctrl+s] Submit") + "  " + cancelStyle.Render("[esc] Cancel") + "\n")
	body.WriteString("\n")
	body.WriteString(shared.StyleHelp.Render("  tab/↑↓ fields • ctrl+s submit • esc cancel"))

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

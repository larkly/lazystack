package lbmembercreate

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const (
	fieldName   = 0
	fieldAddr   = 1
	fieldPort   = 2
	fieldWeight = 3
	fieldSubmit = 4
	fieldCancel = 5
	numFields   = 6
)

type memberCreatedMsg struct{}
type memberCreateErrMsg struct{ err error }

// Model is the member create form modal.
type Model struct {
	Active   bool
	client   *gophercloud.ServiceClient
	poolID   string
	poolName string

	nameInput   textinput.Model
	addrInput   textinput.Model
	portInput   textinput.Model
	weightInput textinput.Model

	// Edit mode
	editMode bool
	memberID string

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a member create form.
func New(client *gophercloud.ServiceClient, poolID, poolName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "member name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.Focus()

	ai := textinput.New()
	ai.Prompt = ""
	ai.Placeholder = "e.g. 10.0.0.5"
	ai.CharLimit = 45
	ai.SetWidth(20)

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "e.g. 8080"
	pi.CharLimit = 5
	pi.SetWidth(10)

	wi := textinput.New()
	wi.Prompt = ""
	wi.Placeholder = "1"
	wi.CharLimit = 4
	wi.SetWidth(6)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:      true,
		client:      client,
		poolID:      poolID,
		poolName:    poolName,
		nameInput:   ni,
		addrInput:   ai,
		portInput:   pi,
		weightInput: wi,
		spinner:     s,
	}
}

// NewEdit creates an edit form for an existing member (name + weight only).
func NewEdit(client *gophercloud.ServiceClient, poolID, memberID, currentName string, currentWeight int, poolName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "member name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.SetValue(currentName)
	ni.Focus()

	ai := textinput.New()
	ai.Prompt = ""
	ai.CharLimit = 45
	ai.SetWidth(20)

	pi := textinput.New()
	pi.Prompt = ""
	pi.CharLimit = 5
	pi.SetWidth(10)

	wi := textinput.New()
	wi.Prompt = ""
	wi.Placeholder = "1"
	wi.CharLimit = 4
	wi.SetWidth(6)
	wi.SetValue(strconv.Itoa(currentWeight))

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:      true,
		client:      client,
		poolID:      poolID,
		poolName:    poolName,
		editMode:    true,
		memberID:    memberID,
		nameInput:   ni,
		addrInput:   ai,
		portInput:   pi,
		weightInput: wi,
		spinner:     s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case memberCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Added member to"
		if m.editMode {
			action = "Updated member in"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.poolName}
		}
	case memberCreateErrMsg:
		m.submitting = false
		m.err = shared.SanitizeAPIError(msg.err)
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

func (m *Model) advanceFocus(dir int) {
	for {
		m.focusField = (m.focusField + dir + numFields) % numFields
		// In edit mode, skip address and port
		if m.editMode && (m.focusField == fieldAddr || m.focusField == fieldPort) {
			continue
		}
		break
	}
	m.updateFocus()
}

func (m Model) isTextInput() bool {
	if m.editMode {
		return m.focusField == fieldName || m.focusField == fieldWeight
	}
	return m.focusField <= fieldWeight
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
			m.advanceFocus(1)
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
			m.advanceFocus(-1)
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.advanceFocus(1)
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			return m.updateTextInput(msg)
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.advanceFocus(1)
		return m, nil
	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.advanceFocus(-1)
		return m, nil
	case key.Matches(msg, shared.Keys.Right):
		if m.focusField == fieldSubmit {
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		if m.focusField == fieldCancel {
			m.focusField = fieldSubmit
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		}
		return m, nil
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	return m, nil
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.addrInput.Blur()
	m.portInput.Blur()
	m.weightInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldAddr:
		m.addrInput.Focus()
	case fieldPort:
		m.portInput.Focus()
	case fieldWeight:
		m.weightInput.Focus()
	}
}

func (m Model) updateTextInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldAddr:
		m.addrInput, cmd = m.addrInput.Update(msg)
	case fieldPort:
		m.portInput, cmd = m.portInput.Update(msg)
	case fieldWeight:
		m.weightInput, cmd = m.weightInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())

	weight := 1
	if w := strings.TrimSpace(m.weightInput.Value()); w != "" {
		var err error
		weight, err = strconv.Atoi(w)
		if err != nil || weight < 0 || weight > 256 {
			m.err = "Weight must be a number between 0 and 256"
			return m, nil
		}
	}

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		poolID := m.poolID
		memberID := m.memberID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			err := loadbalancer.UpdateMember(context.Background(), client, poolID, memberID, &name, &weight, nil)
			if err != nil {
				return memberCreateErrMsg{err: err}
			}
			return memberCreatedMsg{}
		})
	}

	addr := strings.TrimSpace(m.addrInput.Value())
	if addr == "" {
		m.err = "Address is required"
		return m, nil
	}
	if net.ParseIP(addr) == nil {
		m.err = "Address must be a valid IPv4 or IPv6 address"
		return m, nil
	}

	port, err := strconv.Atoi(strings.TrimSpace(m.portInput.Value()))
	if err != nil || port < 1 || port > 65535 {
		m.err = "Port must be a number between 1 and 65535"
		return m, nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	poolID := m.poolID

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_, err := loadbalancer.CreateMember(context.Background(), client, poolID, name, addr, port, weight)
		if err != nil {
			return memberCreateErrMsg{err: err}
		}
		return memberCreatedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "tab/↑↓ navigate • ctrl+s submit • esc cancel"
}

// View renders the form.
func (m Model) View() string {
	titleText := "Add Member to " + m.poolName
	if m.editMode {
		titleText = "Edit Member"
	}
	title := shared.StyleModalTitle.Render(titleText)

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(12)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(12)

	label := func(name string, field int) string {
		if m.focusField == field {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	var rows []string

	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	if !m.editMode {
		rows = append(rows, label("Address", fieldAddr)+m.addrInput.View())
		rows = append(rows, label("Port", fieldPort)+m.portInput.View())
	}
	rows = append(rows, label("Weight", fieldWeight)+m.weightInput.View())

	if m.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err))
	}

	rows = append(rows, "")
	submitStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	if m.focusField == fieldSubmit {
		submitStyle = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	}
	if m.focusField == fieldCancel {
		cancelStyle = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	}

	if m.submitting {
		action := "Adding"
		if m.editMode {
			action = "Updating"
		}
		rows = append(rows, m.spinner.View()+" "+action+" member...")
	} else {
		rows = append(rows, submitStyle.Render("[ Submit ]")+"  "+cancelStyle.Render("[ Cancel ]"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(50).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

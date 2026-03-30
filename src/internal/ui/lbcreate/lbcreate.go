package lbcreate

import (
	"context"
	"strings"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/network"
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
	fieldDesc   = 1
	fieldSubnet = 2
	fieldSubmit = 3
	fieldCancel = 4
	numFields   = 5

	// Edit mode uses fewer fields
	editNumFields = 4 // name, desc, submit, cancel
)

type lbCreatedMsg struct{}
type lbCreateErrMsg struct{ err error }
type subnetsLoadedMsg struct{ subnets []network.Subnet }
type subnetsFetchErrMsg struct{ err error }

// Model is the load balancer create/edit form modal.
type Model struct {
	Active        bool
	client        *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient

	nameInput textinput.Model
	descInput textinput.Model

	// Subnet picker
	subnets        []network.Subnet
	selectedSubnet int
	pickerOpen     bool
	pickerCursor   int
	subnetsLoading bool

	// Edit mode
	editMode bool
	lbID     string

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a load balancer create form.
func New(lbClient, networkClient *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "load balancer name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.Focus()

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "description (optional)"
	di.CharLimit = 255
	di.SetWidth(40)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         lbClient,
		networkClient:  networkClient,
		nameInput:      ni,
		descInput:      di,
		spinner:        s,
		subnetsLoading: true,
	}
}

// NewEdit creates an edit form for an existing load balancer (name + description only).
func NewEdit(client *gophercloud.ServiceClient, lbID, currentName, currentDesc string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "load balancer name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.SetValue(currentName)
	ni.Focus()

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "description (optional)"
	di.CharLimit = 255
	di.SetWidth(40)
	di.SetValue(currentDesc)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:    true,
		client:    client,
		editMode:  true,
		lbID:      lbID,
		nameInput: ni,
		descInput: di,
		spinner:   s,
	}
}

// Init fetches subnets for the picker.
func (m Model) Init() tea.Cmd {
	if m.editMode {
		return nil
	}
	return tea.Batch(m.spinner.Tick, m.fetchSubnets())
}

func (m Model) fetchSubnets() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		subnets, err := network.ListSubnets(context.Background(), client)
		if err != nil {
			return subnetsFetchErrMsg{err: err}
		}
		return subnetsLoadedMsg{subnets: subnets}
	}
}

func (m Model) fieldCount() int {
	if m.editMode {
		return editNumFields
	}
	return numFields
}

func (m Model) mapField(f int) int {
	if !m.editMode {
		return f
	}
	// Edit mode: name(0), desc(1), submit(2), cancel(3) → mapped to fieldName, fieldDesc, fieldSubmit, fieldCancel
	switch f {
	case 0:
		return fieldName
	case 1:
		return fieldDesc
	case 2:
		return fieldSubmit
	case 3:
		return fieldCancel
	}
	return fieldName
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subnetsLoadedMsg:
		m.subnetsLoading = false
		m.subnets = msg.subnets
		return m, nil
	case subnetsFetchErrMsg:
		m.subnetsLoading = false
		m.err = shared.SanitizeAPIError(msg.err)
		return m, nil
	case lbCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created LB"
		if m.editMode {
			action = "Updated LB"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: strings.TrimSpace(m.nameInput.Value())}
		}
	case lbCreateErrMsg:
		m.submitting = false
		m.err = shared.SanitizeAPIError(msg.err)
		return m, nil
	case spinner.TickMsg:
		if m.submitting || m.subnetsLoading {
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
		if m.pickerOpen {
			return m.handlePickerKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) currentField() int {
	return m.mapField(m.focusField)
}

func (m Model) isTextInput() bool {
	f := m.currentField()
	return f == fieldName || f == fieldDesc
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
			m.focusField = (m.focusField + 1) % m.fieldCount()
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
			m.focusField = (m.focusField - 1 + m.fieldCount()) % m.fieldCount()
			m.updateFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.focusField = (m.focusField + 1) % m.fieldCount()
			m.updateFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			var cmd tea.Cmd
			switch m.currentField() {
			case fieldName:
				m.nameInput, cmd = m.nameInput.Update(msg)
			case fieldDesc:
				m.descInput, cmd = m.descInput.Update(msg)
			}
			return m, cmd
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.focusField = (m.focusField + 1) % m.fieldCount()
		m.updateFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.focusField = (m.focusField - 1 + m.fieldCount()) % m.fieldCount()
		m.updateFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		switch m.currentField() {
		case fieldSubnet:
			m.pickerOpen = true
			m.pickerCursor = m.selectedSubnet
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		default:
			m.focusField = (m.focusField + 1) % m.fieldCount()
			m.updateFocus()
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Right):
		if m.currentField() == fieldSubmit {
			m.focusField++
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		if m.currentField() == fieldCancel {
			m.focusField--
		}
		return m, nil
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	return m, nil
}

func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.pickerOpen = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Down):
		if m.pickerCursor < len(m.subnets)-1 {
			m.pickerCursor++
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		m.selectedSubnet = m.pickerCursor
		m.pickerOpen = false
		m.focusField = (m.focusField + 1) % m.fieldCount()
		m.updateFocus()
		return m, nil
	}
	return m, nil
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.descInput.Blur()
	switch m.currentField() {
	case fieldName:
		m.nameInput.Focus()
	case fieldDesc:
		m.descInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Name is required"
		return m, nil
	}

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		id := m.lbID
		desc := strings.TrimSpace(m.descInput.Value())
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			err := loadbalancer.UpdateLoadBalancer(context.Background(), client, id, &name, &desc, nil)
			if err != nil {
				return lbCreateErrMsg{err: err}
			}
			return lbCreatedMsg{}
		})
	}

	if len(m.subnets) == 0 {
		m.err = "No subnets available"
		return m, nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	desc := strings.TrimSpace(m.descInput.Value())
	subnetID := m.subnets[m.selectedSubnet].ID

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_, err := loadbalancer.CreateLoadBalancer(context.Background(), client, name, desc, subnetID)
		if err != nil {
			return lbCreateErrMsg{err: err}
		}
		return lbCreatedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	if m.pickerOpen {
		return "↑↓ navigate • enter select • esc cancel"
	}
	return "tab/↑↓ navigate • enter open picker • ctrl+s submit • esc cancel"
}

// View renders the form.
func (m Model) View() string {
	titleText := "Create Load Balancer"
	if m.editMode {
		titleText = "Edit Load Balancer"
	}
	title := shared.StyleModalTitle.Render(titleText)

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(12)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(12)

	label := func(name string, mappedField int) string {
		if m.currentField() == mappedField {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	var rows []string

	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	rows = append(rows, label("Description", fieldDesc)+m.descInput.View())

	// Subnet picker (create mode only)
	if !m.editMode {
		subnetDisplay := lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("(loading...)")
		if !m.subnetsLoading && len(m.subnets) > 0 {
			s := m.subnets[m.selectedSubnet]
			subnetDisplay = lipgloss.NewStyle().Foreground(shared.ColorFg).Render(s.Name + " (" + s.CIDR + ")")
		} else if !m.subnetsLoading && len(m.subnets) == 0 {
			subnetDisplay = lipgloss.NewStyle().Foreground(shared.ColorError).Render("no subnets available")
		}
		if m.subnetsLoading {
			subnetDisplay = m.spinner.View() + " loading subnets..."
		}
		enterHint := ""
		if m.currentField() == fieldSubnet {
			enterHint = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("  [enter to pick]")
		}
		rows = append(rows, label("Subnet", fieldSubnet)+subnetDisplay+enterHint)
	}

	if m.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err))
	}

	rows = append(rows, "")
	submitStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	if m.currentField() == fieldSubmit {
		submitStyle = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	}
	if m.currentField() == fieldCancel {
		cancelStyle = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
	}

	if m.submitting {
		action := "Creating"
		if m.editMode {
			action = "Updating"
		}
		rows = append(rows, m.spinner.View()+" "+action+" load balancer...")
	} else {
		rows = append(rows, submitStyle.Render("[ Submit ]")+"  "+cancelStyle.Render("[ Cancel ]"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")

	// Overlay subnet picker if open
	if m.pickerOpen {
		content += "\n\n" + m.renderPicker()
	}

	box := shared.StyleModal.Width(55).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderPicker() string {
	title := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render("Select Subnet")
	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Bold(true)

	maxVisible := 8
	start := 0
	if m.pickerCursor >= maxVisible {
		start = m.pickerCursor - maxVisible + 1
	}

	var lines []string
	lines = append(lines, title)
	for i, s := range m.subnets {
		if i < start {
			continue
		}
		if i >= start+maxVisible {
			break
		}
		prefix := "  "
		if i == m.pickerCursor {
			prefix = "\u25b8 "
		}
		line := prefix + s.Name + " (" + s.CIDR + ")"
		if i == m.pickerCursor {
			line = selectedBg.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

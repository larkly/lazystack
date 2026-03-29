package lbpoolcreate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const (
	fieldName       = 0
	fieldProtocol   = 1
	fieldLBMethod   = 2
	fieldMonType    = 3
	fieldMonURL     = 4
	fieldMonCodes   = 5
	fieldMonDelay   = 6
	fieldMonTimeout = 7
	fieldMonRetries = 8
	fieldSubmit     = 9
	fieldCancel     = 10
	numFields       = 11
)

var (
	protocolOpts = []string{"TCP", "HTTP", "HTTPS", "UDP", "PROXY"}
	lbMethodOpts = []string{"ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP"}
	monTypeOpts  = []string{"NONE", "HTTP", "HTTPS", "TCP", "PING"}
)

type poolCreatedMsg struct{}
type poolCreateErrMsg struct{ err error }

// Model is the pool create form modal.
type Model struct {
	Active bool
	client *gophercloud.ServiceClient
	lbID   string
	lbName string

	nameInput        textinput.Model
	selectedProtocol int
	selectedLBMethod int
	selectedMonType  int
	monURLInput      textinput.Model
	monCodesInput    textinput.Model
	monDelayInput    textinput.Model
	monTimeoutInput  textinput.Model
	monRetriesInput  textinput.Model

	// Edit mode
	editMode bool
	poolID   string

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// NewEdit creates an edit form for an existing pool (name + LB method only).
func NewEdit(client *gophercloud.ServiceClient, poolID, currentName, currentLBMethod, lbName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "pool name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.SetValue(currentName)
	ni.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		Active:    true,
		client:    client,
		lbName:    lbName,
		editMode:  true,
		poolID:    poolID,
		nameInput: ni,
		spinner:   s,
	}

	// Pre-fill LB method
	for i, method := range lbMethodOpts {
		if method == currentLBMethod {
			m.selectedLBMethod = i
			break
		}
	}

	// Create empty text inputs to avoid nil panics
	for _, ti := range []*textinput.Model{&m.monURLInput, &m.monCodesInput, &m.monDelayInput, &m.monTimeoutInput, &m.monRetriesInput} {
		*ti = textinput.New()
		ti.Prompt = ""
	}

	return m
}

// New creates a pool create form.
func New(client *gophercloud.ServiceClient, lbID, lbName string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "pool name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.Focus()

	murl := textinput.New()
	murl.Prompt = ""
	murl.Placeholder = "/health"
	murl.CharLimit = 128
	murl.SetWidth(30)

	mcodes := textinput.New()
	mcodes.Prompt = ""
	mcodes.Placeholder = "200"
	mcodes.CharLimit = 20
	mcodes.SetWidth(15)

	mdelay := textinput.New()
	mdelay.Prompt = ""
	mdelay.Placeholder = "5"
	mdelay.CharLimit = 4
	mdelay.SetWidth(6)

	mtimeout := textinput.New()
	mtimeout.Prompt = ""
	mtimeout.Placeholder = "3"
	mtimeout.CharLimit = 4
	mtimeout.SetWidth(6)

	mretries := textinput.New()
	mretries.Prompt = ""
	mretries.Placeholder = "3"
	mretries.CharLimit = 2
	mretries.SetWidth(4)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:          true,
		client:          client,
		lbID:            lbID,
		lbName:          lbName,
		nameInput:       ni,
		monURLInput:     murl,
		monCodesInput:   mcodes,
		monDelayInput:   mdelay,
		monTimeoutInput: mtimeout,
		monRetriesInput: mretries,
		spinner:         s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) hasHTTPMonitor() bool {
	t := monTypeOpts[m.selectedMonType]
	return t == "HTTP" || t == "HTTPS"
}

func (m Model) hasMonitor() bool {
	return monTypeOpts[m.selectedMonType] != "NONE"
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case poolCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created pool on"
		if m.editMode {
			action = "Updated pool on"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.lbName}
		}
	case poolCreateErrMsg:
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
	switch m.focusField {
	case fieldName, fieldMonURL, fieldMonCodes, fieldMonDelay, fieldMonTimeout, fieldMonRetries:
		return true
	}
	return false
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
		switch m.focusField {
		case fieldProtocol:
			m.selectedProtocol = (m.selectedProtocol + 1) % len(protocolOpts)
		case fieldLBMethod:
			m.selectedLBMethod = (m.selectedLBMethod + 1) % len(lbMethodOpts)
		case fieldMonType:
			m.selectedMonType = (m.selectedMonType + 1) % len(monTypeOpts)
		case fieldSubmit:
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldProtocol:
			m.selectedProtocol = (m.selectedProtocol - 1 + len(protocolOpts)) % len(protocolOpts)
		case fieldLBMethod:
			m.selectedLBMethod = (m.selectedLBMethod - 1 + len(lbMethodOpts)) % len(lbMethodOpts)
		case fieldMonType:
			m.selectedMonType = (m.selectedMonType - 1 + len(monTypeOpts)) % len(monTypeOpts)
		case fieldCancel:
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
		default:
			m.advanceFocus(1)
		}
		return m, nil
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	return m, nil
}

func (m *Model) advanceFocus(dir int) {
	for {
		m.focusField = (m.focusField + dir + numFields) % numFields
		// In edit mode, only allow name, LB method, submit, cancel
		if m.editMode && m.focusField != fieldName && m.focusField != fieldLBMethod &&
			m.focusField != fieldSubmit && m.focusField != fieldCancel {
			continue
		}
		// Skip monitor fields when monitor is NONE
		if !m.editMode && !m.hasMonitor() && m.focusField >= fieldMonURL && m.focusField <= fieldMonRetries {
			continue
		}
		// Skip HTTP-only fields for non-HTTP monitors
		if !m.editMode && !m.hasHTTPMonitor() && (m.focusField == fieldMonURL || m.focusField == fieldMonCodes) {
			continue
		}
		break
	}
	m.updateFocusInputs()
}

func (m *Model) updateFocusInputs() {
	m.nameInput.Blur()
	m.monURLInput.Blur()
	m.monCodesInput.Blur()
	m.monDelayInput.Blur()
	m.monTimeoutInput.Blur()
	m.monRetriesInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldMonURL:
		m.monURLInput.Focus()
	case fieldMonCodes:
		m.monCodesInput.Focus()
	case fieldMonDelay:
		m.monDelayInput.Focus()
	case fieldMonTimeout:
		m.monTimeoutInput.Focus()
	case fieldMonRetries:
		m.monRetriesInput.Focus()
	}
}

func (m Model) updateTextInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focusField {
	case fieldName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case fieldMonURL:
		m.monURLInput, cmd = m.monURLInput.Update(msg)
	case fieldMonCodes:
		m.monCodesInput, cmd = m.monCodesInput.Update(msg)
	case fieldMonDelay:
		m.monDelayInput, cmd = m.monDelayInput.Update(msg)
	case fieldMonTimeout:
		m.monTimeoutInput, cmd = m.monTimeoutInput.Update(msg)
	case fieldMonRetries:
		m.monRetriesInput, cmd = m.monRetriesInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	lbMethod := lbMethodOpts[m.selectedLBMethod]

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		id := m.poolID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			err := loadbalancer.UpdatePool(context.Background(), client, id, &name, lbMethod)
			if err != nil {
				return poolCreateErrMsg{err: err}
			}
			return poolCreatedMsg{}
		})
	}

	protocol := protocolOpts[m.selectedProtocol]

	var monOpts *monitors.CreateOpts
	if m.hasMonitor() {
		monType := monTypeOpts[m.selectedMonType]

		delay, err := strconv.Atoi(strings.TrimSpace(m.monDelayInput.Value()))
		if err != nil || delay < 1 {
			delay = 5
		}
		timeout, err := strconv.Atoi(strings.TrimSpace(m.monTimeoutInput.Value()))
		if err != nil || timeout < 1 {
			timeout = 3
		}
		retries, err := strconv.Atoi(strings.TrimSpace(m.monRetriesInput.Value()))
		if err != nil || retries < 1 || retries > 10 {
			retries = 3
		}

		monOpts = &monitors.CreateOpts{
			Type:       monType,
			Delay:      delay,
			Timeout:    timeout,
			MaxRetries: retries,
		}

		if m.hasHTTPMonitor() {
			urlPath := strings.TrimSpace(m.monURLInput.Value())
			if urlPath == "" {
				urlPath = "/"
			}
			monOpts.URLPath = urlPath
			monOpts.HTTPMethod = "GET"
			codes := strings.TrimSpace(m.monCodesInput.Value())
			if codes == "" {
				codes = "200"
			}
			monOpts.ExpectedCodes = codes
		}
	}

	m.submitting = true
	m.err = ""
	client := m.client
	lbID := m.lbID

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_, err := loadbalancer.CreatePool(context.Background(), client, lbID, name, protocol, lbMethod, monOpts)
		if err != nil {
			return poolCreateErrMsg{err: err}
		}
		return poolCreatedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "tab/↑↓ navigate • ←→ pick option • ctrl+s submit • esc cancel"
}

func renderPicker(opts []string, selected int, focused bool) string {
	var parts []string
	for i, o := range opts {
		if i == selected {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render("["+o+"]"))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" "+o+" "))
		}
	}
	return strings.Join(parts, " ")
}

// View renders the form.
func (m Model) View() string {
	titleText := "Add Pool to " + m.lbName
	if m.editMode {
		titleText = "Edit Pool"
	}
	title := shared.StyleModalTitle.Render(titleText)

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(14)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(14)
	sectionStyle := lipgloss.NewStyle().Foreground(shared.ColorCyan).Bold(true)

	label := func(name string, field int) string {
		if m.focusField == field {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	var rows []string

	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	if !m.editMode {
		rows = append(rows, label("Protocol", fieldProtocol)+renderPicker(protocolOpts, m.selectedProtocol, m.focusField == fieldProtocol))
	}
	rows = append(rows, label("LB Method", fieldLBMethod)+renderPicker(lbMethodOpts, m.selectedLBMethod, m.focusField == fieldLBMethod))

	if !m.editMode {
		// Health monitor section
		rows = append(rows, "")
		rows = append(rows, sectionStyle.Render("\u2665 Health Monitor"))
		rows = append(rows, label("Monitor Type", fieldMonType)+renderPicker(monTypeOpts, m.selectedMonType, m.focusField == fieldMonType))

		if m.hasMonitor() {
			if m.hasHTTPMonitor() {
				rows = append(rows, label("URL Path", fieldMonURL)+m.monURLInput.View())
				rows = append(rows, label("Expect Codes", fieldMonCodes)+m.monCodesInput.View())
			}
			rows = append(rows, label("Delay (s)", fieldMonDelay)+m.monDelayInput.View())
			rows = append(rows, label("Timeout (s)", fieldMonTimeout)+m.monTimeoutInput.View())
			rows = append(rows, label("Max Retries", fieldMonRetries)+m.monRetriesInput.View())
		}
	}

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
		rows = append(rows, fmt.Sprintf("%s Creating pool...", m.spinner.View()))
	} else {
		rows = append(rows, submitStyle.Render("[ Submit ]")+"  "+cancelStyle.Render("[ Cancel ]"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(60).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

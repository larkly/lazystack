package lbmonitorcreate

import (
	"context"
	"fmt"
	"regexp"
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
	fieldType       = 0
	fieldURLPath    = 1
	fieldCodes      = 2
	fieldHTTPMethod = 3
	fieldDelay      = 4
	fieldTimeout    = 5
	fieldRetries    = 6
	fieldSubmit     = 7
	fieldCancel     = 8
	numFields       = 9
)

var (
	typeOpts       = []string{"HTTP", "HTTPS", "TCP", "PING"}
	httpMethodOpts = []string{"GET", "HEAD", "POST"}
)

type monitorCreatedMsg struct{}
type monitorCreateErrMsg struct{ err error }

// Model is the health monitor create/edit form modal.
type Model struct {
	Active   bool
	client   *gophercloud.ServiceClient
	poolID   string
	poolName string

	selectedType       int
	urlPathInput       textinput.Model
	codesInput         textinput.Model
	selectedHTTPMethod int
	delayInput         textinput.Model
	timeoutInput       textinput.Model
	retriesInput       textinput.Model

	editMode  bool
	monitorID string

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a health monitor create form.
func New(client *gophercloud.ServiceClient, poolID, poolName string) Model {
	urlPath := textinput.New()
	urlPath.Prompt = ""
	urlPath.Placeholder = "/health"
	urlPath.CharLimit = 128
	urlPath.SetWidth(30)

	codes := textinput.New()
	codes.Prompt = ""
	codes.Placeholder = "200"
	codes.CharLimit = 20
	codes.SetWidth(15)

	delay := textinput.New()
	delay.Prompt = ""
	delay.Placeholder = "5"
	delay.CharLimit = 4
	delay.SetWidth(6)

	timeout := textinput.New()
	timeout.Prompt = ""
	timeout.Placeholder = "3"
	timeout.CharLimit = 4
	timeout.SetWidth(6)

	retries := textinput.New()
	retries.Prompt = ""
	retries.Placeholder = "3"
	retries.CharLimit = 2
	retries.SetWidth(4)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:       true,
		client:       client,
		poolID:       poolID,
		poolName:     poolName,
		urlPathInput: urlPath,
		codesInput:   codes,
		delayInput:   delay,
		timeoutInput: timeout,
		retriesInput: retries,
		spinner:      s,
	}
}

// NewEdit creates an edit form for an existing health monitor.
func NewEdit(client *gophercloud.ServiceClient, monitorID string, current *loadbalancer.HealthMonitor, poolName string) Model {
	m := New(client, "", poolName)
	m.editMode = true
	m.monitorID = monitorID

	for i, t := range typeOpts {
		if t == current.Type {
			m.selectedType = i
			break
		}
	}
	if current.URLPath != "" {
		m.urlPathInput.SetValue(current.URLPath)
	}
	if current.ExpectedCodes != "" {
		m.codesInput.SetValue(current.ExpectedCodes)
	}
	for i, method := range httpMethodOpts {
		if method == current.HTTPMethod {
			m.selectedHTTPMethod = i
			break
		}
	}
	if current.Delay > 0 {
		m.delayInput.SetValue(strconv.Itoa(current.Delay))
	}
	if current.Timeout > 0 {
		m.timeoutInput.SetValue(strconv.Itoa(current.Timeout))
	}
	if current.MaxRetries > 0 {
		m.retriesInput.SetValue(strconv.Itoa(current.MaxRetries))
	}

	// Start focus on first editable field (skip type in edit mode)
	monType := current.Type
	if monType == "HTTP" || monType == "HTTPS" {
		m.focusField = fieldURLPath
	} else {
		m.focusField = fieldDelay
	}
	m.updateFocusInputs()

	return m
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) isHTTPType() bool {
	t := typeOpts[m.selectedType]
	return t == "HTTP" || t == "HTTPS"
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case monitorCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created monitor on"
		if m.editMode {
			action = "Updated monitor on"
		}
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.poolName}
		}
	case monitorCreateErrMsg:
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

func (m Model) isTextInput() bool {
	switch m.focusField {
	case fieldURLPath, fieldCodes, fieldDelay, fieldTimeout, fieldRetries:
		return true
	}
	return false
}

func (m *Model) advanceFocus(dir int) {
	for {
		m.focusField = (m.focusField + dir + numFields) % numFields
		if m.editMode && m.focusField == fieldType {
			continue
		}
		if !m.isHTTPType() && (m.focusField == fieldURLPath || m.focusField == fieldCodes || m.focusField == fieldHTTPMethod) {
			continue
		}
		break
	}
	m.updateFocusInputs()
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.Active = false
			return m, nil
		case key.Matches(msg, shared.Keys.Tab):
			m.advanceFocus(1)
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.advanceFocus(-1)
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.advanceFocus(1)
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			var cmd tea.Cmd
			switch m.focusField {
			case fieldURLPath:
				m.urlPathInput, cmd = m.urlPathInput.Update(msg)
			case fieldCodes:
				m.codesInput, cmd = m.codesInput.Update(msg)
			case fieldDelay:
				m.delayInput, cmd = m.delayInput.Update(msg)
			case fieldTimeout:
				m.timeoutInput, cmd = m.timeoutInput.Update(msg)
			case fieldRetries:
				m.retriesInput, cmd = m.retriesInput.Update(msg)
			}
			return m, cmd
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
		case fieldType:
			m.selectedType = (m.selectedType + 1) % len(typeOpts)
		case fieldHTTPMethod:
			m.selectedHTTPMethod = (m.selectedHTTPMethod + 1) % len(httpMethodOpts)
		case fieldSubmit:
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldType:
			m.selectedType = (m.selectedType - 1 + len(typeOpts)) % len(typeOpts)
		case fieldHTTPMethod:
			m.selectedHTTPMethod = (m.selectedHTTPMethod - 1 + len(httpMethodOpts)) % len(httpMethodOpts)
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

func (m *Model) updateFocusInputs() {
	m.urlPathInput.Blur()
	m.codesInput.Blur()
	m.delayInput.Blur()
	m.timeoutInput.Blur()
	m.retriesInput.Blur()
	switch m.focusField {
	case fieldURLPath:
		m.urlPathInput.Focus()
	case fieldCodes:
		m.codesInput.Focus()
	case fieldDelay:
		m.delayInput.Focus()
	case fieldTimeout:
		m.timeoutInput.Focus()
	case fieldRetries:
		m.retriesInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	delayStr := strings.TrimSpace(m.delayInput.Value())
	if delayStr == "" {
		delayStr = "5"
	}
	delay, err := strconv.Atoi(delayStr)
	if err != nil || delay < 1 {
		m.err = "Delay must be a positive number (seconds)"
		return m, nil
	}
	timeoutStr := strings.TrimSpace(m.timeoutInput.Value())
	if timeoutStr == "" {
		timeoutStr = "3"
	}
	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil || timeout < 1 {
		m.err = "Timeout must be a positive number (seconds)"
		return m, nil
	}
	retriesStr := strings.TrimSpace(m.retriesInput.Value())
	if retriesStr == "" {
		retriesStr = "3"
	}
	retries, err := strconv.Atoi(retriesStr)
	if err != nil || retries < 1 || retries > 10 {
		m.err = "Max retries must be a number between 1 and 10"
		return m, nil
	}

	urlPath := strings.TrimSpace(m.urlPathInput.Value())
	codes := strings.TrimSpace(m.codesInput.Value())
	httpMethod := httpMethodOpts[m.selectedHTTPMethod]

	if m.isHTTPType() && codes != "" {
		if !validExpectedCodes(codes) {
			m.err = "Expected codes: single (200), list (200,201), or range (200-299)"
			return m, nil
		}
	}

	m.submitting = true
	m.err = ""
	client := m.client

	if m.editMode {
		id := m.monitorID
		// Reviewer fix #1: only send HTTP-specific fields for HTTP/HTTPS monitors
		var urlPathPtr, codesPtr, httpMethodPtr *string
		if m.isHTTPType() {
			urlPathPtr = &urlPath
			codesPtr = &codes
			httpMethodPtr = &httpMethod
		}
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			err := loadbalancer.UpdateHealthMonitor(context.Background(), client, id, &delay, &timeout, &retries, urlPathPtr, codesPtr, httpMethodPtr)
			if err != nil {
				return monitorCreateErrMsg{err: err}
			}
			return monitorCreatedMsg{}
		})
	}

	poolID := m.poolID
	monType := typeOpts[m.selectedType]
	if m.isHTTPType() {
		if urlPath == "" {
			urlPath = "/"
		}
		if codes == "" {
			codes = "200"
		}
	} else {
		urlPath = ""
		codes = ""
		httpMethod = ""
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_, err := loadbalancer.CreateHealthMonitor(context.Background(), client, poolID, monType, delay, timeout, retries, urlPath, codes, httpMethod)
		if err != nil {
			return monitorCreateErrMsg{err: err}
		}
		return monitorCreatedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "tab/\u2191\u2193 navigate \u2022 \u2190\u2192 pick type \u2022 ctrl+s submit \u2022 esc cancel"
}

func renderPicker(opts []string, selected int) string {
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
	titleText := "Add Health Monitor to " + m.poolName
	if m.editMode {
		titleText = "Edit Health Monitor"
	}
	title := shared.StyleModalTitle.Render(titleText)

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(14)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(14)

	label := func(name string, field int) string {
		if m.focusField == field {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	var rows []string

	if !m.editMode {
		rows = append(rows, label("Type", fieldType)+renderPicker(typeOpts, m.selectedType))
	} else {
		rows = append(rows, labelStyle.Render("Type")+lipgloss.NewStyle().Foreground(shared.ColorFg).Render(typeOpts[m.selectedType]))
	}

	if m.isHTTPType() {
		rows = append(rows, label("URL Path", fieldURLPath)+m.urlPathInput.View())
		rows = append(rows, label("Expect Codes", fieldCodes)+m.codesInput.View())
		rows = append(rows, label("HTTP Method", fieldHTTPMethod)+renderPicker(httpMethodOpts, m.selectedHTTPMethod))
	}

	rows = append(rows, label("Delay (s)", fieldDelay)+m.delayInput.View())
	rows = append(rows, label("Timeout (s)", fieldTimeout)+m.timeoutInput.View())
	rows = append(rows, label("Max Retries", fieldRetries)+m.retriesInput.View())

	if m.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err))
	}

	rows = append(rows, "")
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
	}

	if m.submitting {
		action := "Creating"
		if m.editMode {
			action = "Updating"
		}
		rows = append(rows, fmt.Sprintf("%s %s health monitor...", m.spinner.View(), action))
	} else {
		rows = append(rows, submitStyle.Render("[ctrl+s] Submit")+"  "+cancelStyle.Render("[esc] Cancel"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(55).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

var expectedCodesRe = regexp.MustCompile(`^[1-5][0-9]{2}([,-][1-5][0-9]{2})*$`)

func validExpectedCodes(s string) bool {
	return expectedCodesRe.MatchString(s)
}

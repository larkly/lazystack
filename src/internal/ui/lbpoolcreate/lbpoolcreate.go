package lbpoolcreate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
)

const (
	fieldName       = 0
	fieldProtocol   = 1
	fieldLBMethod   = 2
	fieldListener   = 3
	fieldMonType    = 4
	fieldMonURL     = 5
	fieldMonCodes   = 6
	fieldMonDelay   = 7
	fieldMonTimeout = 8
	fieldMonRetries = 9
	fieldSubmit     = 10
	fieldCancel     = 11
	numFields       = 12
)

var (
	protocolOpts = []string{"TCP", "HTTP", "HTTPS", "UDP", "PROXY"}
	lbMethodOpts = []string{"ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP", "SOURCE_IP_PORT"}
	monTypeOpts  = []string{"NONE", "HTTP", "HTTPS", "TCP", "PING"}
)

type poolCreatedMsg struct{ listener string } // listener the pool was bound to, if any
type poolCreateErrMsg struct {
	err     error
	partial string // what was already created when a later step failed
}

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

	// Listener binding: listeners without a default pool that the new pool
	// can become the default pool of (0 = none, i = listeners[i-1]).
	listeners        []loadbalancer.Listener
	selectedListener int
	// Edit mode: listeners currently using this pool as default pool.
	boundListeners []string

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
// listeners are the load balancer's listeners, used to show which of them
// route to this pool.
func NewEdit(client *gophercloud.ServiceClient, poolID, currentName, currentLBMethod, lbName string, listeners []loadbalancer.Listener) Model {
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

	for _, l := range listeners {
		if l.DefaultPoolID == poolID {
			m.boundListeners = append(m.boundListeners, listenerLabel(l))
		}
	}

	// Create empty text inputs to avoid nil panics
	for _, ti := range []*textinput.Model{&m.monURLInput, &m.monCodesInput, &m.monDelayInput, &m.monTimeoutInput, &m.monRetriesInput} {
		*ti = textinput.New()
		ti.Prompt = ""
	}

	return m
}

// New creates a pool create form. listeners are the load balancer's
// listeners; those without a default pool are offered for binding.
func New(client *gophercloud.ServiceClient, lbID, lbName string, listeners []loadbalancer.Listener) Model {
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
		listeners:       unboundListeners(listeners),
		spinner:         s,
	}
}

// unboundListeners returns the listeners that have no default pool yet.
func unboundListeners(all []loadbalancer.Listener) []loadbalancer.Listener {
	var out []loadbalancer.Listener
	for _, l := range all {
		if l.DefaultPoolID == "" {
			out = append(out, l)
		}
	}
	return out
}

func listenerLabel(l loadbalancer.Listener) string {
	name := l.Name
	if name == "" {
		name = l.ID
	}
	return fmt.Sprintf("%s (%s:%d)", name, l.Protocol, l.ProtocolPort)
}

// chosenListener returns the listener to bind the new pool to, or nil.
func (m Model) chosenListener() *loadbalancer.Listener {
	if m.selectedListener > 0 && m.selectedListener <= len(m.listeners) {
		return &m.listeners[m.selectedListener-1]
	}
	return nil
}

func (m Model) listenerView() string {
	if l := m.chosenListener(); l != nil {
		return "← " + listenerLabel(*l) + " →"
	}
	if len(m.listeners) == 0 {
		return "none (no listener without a default pool)"
	}
	return "← none →"
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[lbpoolcreate] Init() lbName=%q editMode=%v", m.lbName, m.editMode)
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
		name := m.lbName
		if m.editMode {
			action = "Updated pool on"
		}
		if msg.listener != "" {
			name += " as default pool of " + msg.listener
		}
		shared.Debugf("[lbpoolcreate] success lbName=%q listener=%q", m.lbName, msg.listener)
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: name}
		}
	case poolCreateErrMsg:
		m.submitting = false
		m.err = shared.SanitizeAPIError(msg.err)
		var cleanupErr *loadbalancer.PoolCleanupError
		if errors.As(msg.err, &cleanupErr) && !strings.Contains(m.err, "may remain") {
			m.err += fmt.Sprintf(" Rollback failed: pool %s may remain and need manual deletion.", cleanupErr.PoolID)
		}
		if msg.partial != "" {
			// The pool exists but the binding does not: say so explicitly.
			m.err = msg.partial + ": " + m.err
		}
		shared.Debugf("[lbpoolcreate] error: %v", msg.err)
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
		case fieldListener:
			m.selectedListener = (m.selectedListener + 1) % (len(m.listeners) + 1)
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
		case fieldListener:
			n := len(m.listeners) + 1
			m.selectedListener = (m.selectedListener - 1 + n) % n
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
	if name == "" {
		m.err = "Name is required"
		return m, nil
	}
	lbMethod := lbMethodOpts[m.selectedLBMethod]

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		id := m.poolID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			ctx, cancel := shared.RequestCtx()
			defer cancel()
			err := loadbalancer.UpdatePool(ctx, client, id, &name, lbMethod, nil)
			if err != nil {
				return poolCreateErrMsg{err: err}
			}
			return poolCreatedMsg{}
		})
	}

	protocol := protocolOpts[m.selectedProtocol]

	// Reject a listener that cannot route to this pool's protocol before
	// anything is created.
	var listenerID, listenerName string
	if l := m.chosenListener(); l != nil {
		if !loadbalancer.CompatiblePoolProtocol(l.Protocol, protocol) {
			m.err = fmt.Sprintf("A %s pool cannot be the default pool of %s listener %s", protocol, l.Protocol, listenerLabel(*l))
			return m, nil
		}
		listenerID, listenerName = l.ID, listenerLabel(*l)
	}

	var monOpts *monitors.CreateOpts
	if m.hasMonitor() {
		monType := monTypeOpts[m.selectedMonType]

		// Validate everything before the pool POST so a bad monitor
		// setting cannot leave a pool behind without its monitor.
		timing, err := loadbalancer.ParseMonitorTiming(m.monDelayInput.Value(), m.monTimeoutInput.Value(), m.monRetriesInput.Value())
		if err != nil {
			m.err = err.Error()
			return m, nil
		}

		monOpts = &monitors.CreateOpts{
			Type:       monType,
			Delay:      timing.Delay,
			Timeout:    timing.Timeout,
			MaxRetries: timing.MaxRetries,
		}

		if m.hasHTTPMonitor() {
			codes, err := loadbalancer.NormalizeExpectedCodes(m.monCodesInput.Value())
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
			urlPath := strings.TrimSpace(m.monURLInput.Value())
			if urlPath == "" {
				urlPath = "/"
			}
			monOpts.URLPath = urlPath
			monOpts.HTTPMethod = "GET"
			monOpts.ExpectedCodes = codes
		}
	}

	m.submitting = true
	m.err = ""
	shared.Debugf("[lbpoolcreate] submit protocol=%s method=%s", protocol, lbMethod)
	client := m.client
	lbID := m.lbID

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		// CreatePool waits for the pool to become ACTIVE (~60s) before the
		// optional health-monitor create, so allow a generous deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		pool, err := loadbalancer.CreatePool(ctx, client, lbID, name, protocol, lbMethod, monOpts)
		if err != nil {
			return poolCreateErrMsg{err: err}
		}
		if listenerID == "" {
			return poolCreatedMsg{}
		}
		// Octavia rejects changes while the load balancer is PENDING_*, so
		// wait for it before binding; success is only reported once the
		// listener confirms the new default pool.
		err = loadbalancer.WaitForActive(ctx, client, lbID, 2*time.Minute)
		if err == nil {
			err = loadbalancer.SetListenerDefaultPool(ctx, client, listenerID, pool.ID)
		}
		if err != nil {
			shared.Debugf("[lbpoolcreate] binding pool %s to listener %s failed: %v", pool.ID, listenerID, err)
			return poolCreateErrMsg{
				err:     err,
				partial: fmt.Sprintf("Pool %s was created but is not attached to listener %s", pool.ID, listenerName),
			}
		}
		return poolCreatedMsg{listener: listenerName}
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

func renderPicker(opts []string, selected int, _ bool) string {
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
	if m.editMode {
		bound := "none"
		if len(m.boundListeners) > 0 {
			bound = strings.Join(m.boundListeners, ", ")
		}
		rows = append(rows, labelStyle.Render("Default for")+lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(bound))
	} else {
		rows = append(rows, label("Listener", fieldListener)+lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render(m.listenerView()))
	}

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
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
	}

	if m.submitting {
		rows = append(rows, fmt.Sprintf("%s Creating pool...", m.spinner.View()))
	} else {
		rows = append(rows, submitStyle.Render("[ctrl+s] Submit")+"  "+cancelStyle.Render("[esc] Cancel"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(60).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

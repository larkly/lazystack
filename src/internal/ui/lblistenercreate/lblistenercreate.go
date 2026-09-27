package lblistenercreate

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
)

const (
	fieldName        = 0
	fieldProtocol    = 1
	fieldPort        = 2
	fieldDefaultPool = 3
	fieldDesc        = 4
	fieldConnLimit   = 5
	fieldSubmit      = 6
	fieldCancel      = 7
	numFields        = 8
)

var protocolOpts = []string{"TCP", "HTTP", "HTTPS", "UDP"}

type listenerCreatedMsg struct{}
type listenerCreateErrMsg struct{ err error }

// Model is the listener create form modal.
type Model struct {
	Active bool
	client *gophercloud.ServiceClient
	lbID   string
	lbName string

	nameInput        textinput.Model
	selectedProtocol int
	portInput        textinput.Model
	descInput        textinput.Model
	connLimitInput   textinput.Model

	// Default pool binding: pools of this LB, and the chosen one
	// (0 = none, i = pools[i-1]).
	pools        []loadbalancer.Pool
	selectedPool int
	poolTouched  bool // the user changed the default pool choice

	// Edit mode
	editMode      bool
	listenerID    string
	protocol      string // existing listener's protocol (edit mode)
	initialPoolID string // existing default pool (edit mode)

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int
}

// New creates a listener create form. pools are the load balancer's pools
// offered as the listener's default pool.
func New(client *gophercloud.ServiceClient, lbID, lbName string, pools []loadbalancer.Pool) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "listener name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.Focus()

	pi := textinput.New()
	pi.Prompt = ""
	pi.Placeholder = "e.g. 80"
	pi.CharLimit = 5
	pi.SetWidth(10)

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "description (optional)"
	di.CharLimit = 255
	di.SetWidth(30)

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "unlimited"
	ci.CharLimit = 7
	ci.SetWidth(10)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		lbID:           lbID,
		lbName:         lbName,
		nameInput:      ni,
		portInput:      pi,
		descInput:      di,
		connLimitInput: ci,
		pools:          pools,
		spinner:        s,
	}
}

// NewEdit creates an edit form for an existing listener. protocol and
// currentPoolID describe the listener; pools are the load balancer's pools.
func NewEdit(client *gophercloud.ServiceClient, listenerID, currentName, currentDesc string, currentConnLimit int, lbName, protocol, currentPoolID string, pools []loadbalancer.Pool) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "listener name"
	ni.CharLimit = 64
	ni.SetWidth(30)
	ni.SetValue(currentName)
	ni.Focus()

	pi := textinput.New()
	pi.Prompt = ""
	pi.CharLimit = 5
	pi.SetWidth(10)

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "description (optional)"
	di.CharLimit = 255
	di.SetWidth(30)
	di.SetValue(currentDesc)

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "unlimited"
	ci.CharLimit = 7
	ci.SetWidth(10)
	if currentConnLimit > 0 {
		ci.SetValue(strconv.Itoa(currentConnLimit))
	}

	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		Active:         true,
		client:         client,
		lbName:         lbName,
		editMode:       true,
		listenerID:     listenerID,
		protocol:       protocol,
		initialPoolID:  currentPoolID,
		nameInput:      ni,
		portInput:      pi,
		descInput:      di,
		connLimitInput: ci,
		pools:          pools,
		spinner:        s,
	}
	for i, p := range pools {
		if p.ID == currentPoolID {
			m.selectedPool = i + 1
		}
	}
	return m
}

// chosenPool returns the selected default pool, or nil for none.
func (m Model) chosenPool() *loadbalancer.Pool {
	if m.selectedPool > 0 && m.selectedPool <= len(m.pools) {
		return &m.pools[m.selectedPool-1]
	}
	return nil
}

func (m Model) listenerProtocol() string {
	if m.editMode {
		return m.protocol
	}
	return protocolOpts[m.selectedProtocol]
}

func (m Model) poolLabel(p loadbalancer.Pool) string {
	name := p.Name
	if name == "" {
		name = p.ID
	}
	return name + " (" + p.Protocol + ")"
}

// defaultPoolView renders the default-pool choice, including a current
// binding to a pool that is not in the list.
func (m Model) defaultPoolView() string {
	if p := m.chosenPool(); p != nil {
		return m.poolLabel(*p)
	}
	if m.editMode && m.initialPoolID != "" && m.selectedPool == 0 && !m.hasPool(m.initialPoolID) {
		return m.initialPoolID + " (current)"
	}
	if len(m.pools) == 0 {
		return "none (no pools on this load balancer)"
	}
	return "none"
}

func (m Model) hasPool(id string) bool {
	for _, p := range m.pools {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[lblistenercreate] Init() lbName=%q editMode=%v", m.lbName, m.editMode)
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listenerCreatedMsg:
		m.submitting = false
		m.Active = false
		action := "Created listener on"
		if m.editMode {
			action = "Updated listener on"
		}
		shared.Debugf("[lblistenercreate] success lbName=%q", m.lbName)
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: action, Name: m.lbName}
		}
	case listenerCreateErrMsg:
		m.submitting = false
		m.err = shared.SanitizeAPIError(msg.err)
		shared.Debugf("[lblistenercreate] error: %v", msg.err)
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
	case fieldName, fieldDesc, fieldConnLimit:
		return true
	case fieldPort:
		return !m.editMode
	}
	return false
}

func (m *Model) advanceFocus(dir int) {
	for {
		m.focusField = (m.focusField + dir + numFields) % numFields
		// In edit mode, skip protocol and port (not updatable)
		if m.editMode && (m.focusField == fieldProtocol || m.focusField == fieldPort) {
			continue
		}
		break
	}
	m.updateFocus()
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
			case fieldName:
				m.nameInput, cmd = m.nameInput.Update(msg)
			case fieldPort:
				m.portInput, cmd = m.portInput.Update(msg)
			case fieldDesc:
				m.descInput, cmd = m.descInput.Update(msg)
			case fieldConnLimit:
				m.connLimitInput, cmd = m.connLimitInput.Update(msg)
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
		if m.focusField == fieldProtocol {
			m.selectedProtocol = (m.selectedProtocol + 1) % len(protocolOpts)
		}
		if m.focusField == fieldDefaultPool {
			m.selectedPool = (m.selectedPool + 1) % (len(m.pools) + 1)
			m.poolTouched = true
		}
		if m.focusField == fieldSubmit {
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		if m.focusField == fieldProtocol {
			m.selectedProtocol = (m.selectedProtocol - 1 + len(protocolOpts)) % len(protocolOpts)
		}
		if m.focusField == fieldDefaultPool {
			n := len(m.pools) + 1
			m.selectedPool = (m.selectedPool - 1 + n) % n
			m.poolTouched = true
		}
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

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.portInput.Blur()
	m.descInput.Blur()
	m.connLimitInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldPort:
		m.portInput.Focus()
	case fieldDesc:
		m.descInput.Focus()
	case fieldConnLimit:
		m.connLimitInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Name is required"
		return m, nil
	}

	desc := strings.TrimSpace(m.descInput.Value())
	var connLimit *int
	if cl := strings.TrimSpace(m.connLimitInput.Value()); cl != "" {
		v, err := strconv.Atoi(cl)
		if err != nil || v < 1 {
			m.err = "Connection limit must be a positive number"
			return m, nil
		}
		connLimit = &v
	}

	// Reject a pool the listener protocol cannot route to before any request.
	pool := m.chosenPool()
	if pool != nil && (!m.editMode || m.poolTouched) && !loadbalancer.CompatiblePoolProtocol(m.listenerProtocol(), pool.Protocol) {
		m.err = "A " + m.listenerProtocol() + " listener cannot use " + pool.Protocol + " pool " + m.poolLabel(*pool)
		return m, nil
	}
	poolID := ""
	if pool != nil {
		poolID = pool.ID
	}

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		id := m.listenerID
		update := loadbalancer.ListenerUpdate{Name: &name, Description: &desc, ConnLimit: connLimit}
		if m.poolTouched && poolID != m.initialPoolID {
			update.DefaultPoolID = &poolID // "" removes the binding
		}
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			ctx, cancel := shared.RequestCtx()
			defer cancel()
			err := loadbalancer.UpdateListenerWithPool(ctx, client, id, update)
			if err != nil {
				return listenerCreateErrMsg{err: err}
			}
			return listenerCreatedMsg{}
		})
	}

	port, err := strconv.Atoi(strings.TrimSpace(m.portInput.Value()))
	if err != nil || port < 1 || port > 65535 {
		m.err = "Port must be a number between 1 and 65535"
		return m, nil
	}

	m.submitting = true
	m.err = ""
	shared.Debugf("[lblistenercreate] submit protocol=%s port=%d", protocolOpts[m.selectedProtocol], port)
	client := m.client
	lbID := m.lbID
	protocol := protocolOpts[m.selectedProtocol]

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		_, err := loadbalancer.CreateListener(ctx, client, lbID, name, protocol, port, poolID)
		if err != nil {
			return listenerCreateErrMsg{err: err}
		}
		return listenerCreatedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "tab/↑↓ navigate • ←→ pick protocol/pool • ctrl+s submit • esc cancel"
}

// View renders the form.
func (m Model) View() string {
	titleText := "Add Listener to " + m.lbName
	if m.editMode {
		titleText = "Edit Listener"
	}
	title := shared.StyleModalTitle.Render(titleText)

	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(12)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary)

	var rows []string

	// Name
	label := labelStyle.Render("Name")
	if m.focusField == fieldName {
		label = focusStyle.Bold(true).Width(12).Render("Name")
	}
	rows = append(rows, label+m.nameInput.View())

	if !m.editMode {
		// Protocol
		label = labelStyle.Render("Protocol")
		if m.focusField == fieldProtocol {
			label = focusStyle.Bold(true).Width(12).Render("Protocol")
		}
		var protoDisplay []string
		for i, p := range protocolOpts {
			if i == m.selectedProtocol {
				protoDisplay = append(protoDisplay, lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render("["+p+"]"))
			} else {
				protoDisplay = append(protoDisplay, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(" "+p+" "))
			}
		}
		rows = append(rows, label+strings.Join(protoDisplay, " "))

		// Port
		label = labelStyle.Render("Port")
		if m.focusField == fieldPort {
			label = focusStyle.Bold(true).Width(12).Render("Port")
		}
		rows = append(rows, label+m.portInput.View())
	}

	// Default pool
	label = labelStyle.Render("Default Pool")
	if m.focusField == fieldDefaultPool {
		label = focusStyle.Bold(true).Width(12).Render("Default Pool")
	}
	poolText := m.defaultPoolView()
	if len(m.pools) > 0 {
		poolText = "← " + poolText + " →"
	}
	rows = append(rows, label+lipgloss.NewStyle().Foreground(shared.ColorHighlight).Render(poolText))

	// Description
	label = labelStyle.Render("Description")
	if m.focusField == fieldDesc {
		label = focusStyle.Bold(true).Width(12).Render("Description")
	}
	rows = append(rows, label+m.descInput.View())

	// Connection Limit
	label = labelStyle.Render("Conn Limit")
	if m.focusField == fieldConnLimit {
		label = focusStyle.Bold(true).Width(12).Render("Conn Limit")
	}
	rows = append(rows, label+m.connLimitInput.View())

	// Error
	if m.err != "" {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().Foreground(shared.ColorError).Render(m.err))
	}

	// Buttons
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
		rows = append(rows, m.spinner.View()+" "+action+" listener...")
	} else {
		rows = append(rows, submitStyle.Render("[ctrl+s] Submit")+"  "+cancelStyle.Render("[esc] Cancel"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(50).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

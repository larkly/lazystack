package lbmembercreate

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/larkly/lazystack/internal/compute"
	"github.com/larkly/lazystack/internal/loadbalancer"
	"github.com/larkly/lazystack/internal/shared"
)

const (
	fieldAddrSource = iota
	fieldAddr
	fieldServer
	fieldName
	fieldPort
	fieldWeight
	fieldAdminState
	fieldBackup
	fieldMonitorAddr
	fieldMonitorPort
	fieldTags
	fieldSubmit
	fieldCancel
	numFields
)

const (
	addressSourceIP = iota
	addressSourceServer
)

var addressSourceOpts = []string{"IP", "Server"}
var enabledDisabledOpts = []string{"Enabled", "Disabled"}
var yesNoOpts = []string{"No", "Yes"}

type memberCreatedMsg struct{}
type memberCreateErrMsg struct{ err error }
type memberServersLoadedMsg struct{ servers []memberServerOption }
type memberServersErrMsg struct{ err error }

type memberServerOption struct {
	id      string
	name    string
	address string
	status  string
}

// Model is the member create form modal.
type Model struct {
	Active        bool
	client        *gophercloud.ServiceClient
	computeClient *gophercloud.ServiceClient
	poolID        string
	poolName      string
	excludedAddrs map[string]struct{}

	nameInput   textinput.Model
	addrInput   textinput.Model
	portInput   textinput.Model
	weightInput textinput.Model
	monitorAddr textinput.Model
	monitorPort textinput.Model
	tagsInput   textinput.Model

	adminStateUp bool
	backup       bool

	addressSource    int
	serverOptions    []memberServerOption
	filteredServers  []memberServerOption
	selectedServerID string
	serversLoading   bool
	preferredIPVer   int

	serverPickerOpen bool
	serverFilter     string
	serverFiltering  bool
	pickerCursor     int
	pickerScroll     int

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
func New(client, computeClient *gophercloud.ServiceClient, poolID, poolName, lbVIPAddress string, existingMemberAddrs []string) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "member name"
	ni.CharLimit = 64
	ni.SetWidth(30)

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

	mai := textinput.New()
	mai.Prompt = ""
	mai.Placeholder = "optional health-check IP"
	mai.CharLimit = 45
	mai.SetWidth(28)

	mpi := textinput.New()
	mpi.Prompt = ""
	mpi.Placeholder = "optional health-check port"
	mpi.CharLimit = 5
	mpi.SetWidth(18)

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "comma,separated,labels"
	ti.CharLimit = 256
	ti.SetWidth(36)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		computeClient:  computeClient,
		poolID:         poolID,
		poolName:       poolName,
		excludedAddrs:  makeAddressSet(existingMemberAddrs),
		nameInput:      ni,
		addrInput:      ai,
		portInput:      pi,
		weightInput:    wi,
		monitorAddr:    mai,
		monitorPort:    mpi,
		tagsInput:      ti,
		adminStateUp:   true,
		spinner:        s,
		serversLoading: computeClient != nil,
		preferredIPVer: ipVersion(lbVIPAddress),
		focusField:     fieldAddrSource,
	}
}

// NewEdit creates an edit form for an existing member.
func NewEdit(client *gophercloud.ServiceClient, poolID, memberID, currentName string, currentWeight int, currentAdminStateUp, currentBackup bool, currentMonitorAddress string, currentMonitorPort int, currentTags []string, poolName string) Model {
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

	mai := textinput.New()
	mai.Prompt = ""
	mai.Placeholder = "optional health-check IP"
	mai.CharLimit = 45
	mai.SetWidth(28)
	mai.SetValue(currentMonitorAddress)

	mpi := textinput.New()
	mpi.Prompt = ""
	mpi.Placeholder = "optional health-check port"
	mpi.CharLimit = 5
	mpi.SetWidth(18)
	if currentMonitorPort > 0 {
		mpi.SetValue(strconv.Itoa(currentMonitorPort))
	}

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "comma,separated,labels"
	ti.CharLimit = 256
	ti.SetWidth(36)
	ti.SetValue(strings.Join(currentTags, ", "))

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:       true,
		client:       client,
		poolID:       poolID,
		poolName:     poolName,
		editMode:     true,
		memberID:     memberID,
		nameInput:    ni,
		addrInput:    ai,
		portInput:    pi,
		weightInput:  wi,
		monitorAddr:  mai,
		monitorPort:  mpi,
		tagsInput:    ti,
		adminStateUp: currentAdminStateUp,
		backup:       currentBackup,
		spinner:      s,
		focusField:   fieldName,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	if m.editMode || m.computeClient == nil {
		return nil
	}
	return tea.Batch(m.spinner.Tick, m.fetchServers())
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
		m.err = msg.err.Error()
		return m, nil

	case memberServersLoadedMsg:
		m.serversLoading = false
		m.serverOptions = msg.servers
		if m.selectedServerID == "" && len(m.serverOptions) > 0 {
			m.selectedServerID = m.serverOptions[0].id
		}
		m.applyServerFilter()
		return m, nil

	case memberServersErrMsg:
		m.serversLoading = false
		m.err = msg.err.Error()
		m.applyServerFilter()
		return m, nil

	case spinner.TickMsg:
		if m.submitting || m.serversLoading {
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
		if m.serverPickerOpen {
			return m.handleServerPickerKey(msg)
		}
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) usesManualIP() bool {
	return !m.editMode && m.addressSource == addressSourceIP
}

func (m Model) usesServerSelection() bool {
	return !m.editMode && m.addressSource == addressSourceServer
}

func (m *Model) advanceFocus(dir int) {
	for {
		m.focusField = (m.focusField + dir + numFields) % numFields
		if m.editMode && (m.focusField == fieldAddrSource || m.focusField == fieldAddr || m.focusField == fieldServer || m.focusField == fieldPort) {
			continue
		}
		if m.usesManualIP() && m.focusField == fieldServer {
			continue
		}
		if m.usesServerSelection() && m.focusField == fieldAddr {
			continue
		}
		break
	}
	m.updateFocus()
}

func (m Model) isTextInput() bool {
	if m.editMode {
		switch m.focusField {
		case fieldName, fieldWeight, fieldMonitorAddr, fieldMonitorPort, fieldTags:
			return true
		}
		return false
	}
	switch m.focusField {
	case fieldName, fieldPort, fieldWeight, fieldMonitorAddr, fieldMonitorPort, fieldTags:
		return true
	case fieldAddr:
		return m.usesManualIP()
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
		case fieldAddrSource:
			m.addressSource = (m.addressSource + 1) % len(addressSourceOpts)
			m.err = ""
		case fieldAdminState:
			m.adminStateUp = !m.adminStateUp
		case fieldBackup:
			m.backup = !m.backup
		case fieldSubmit:
			m.focusField = fieldCancel
			m.updateFocus()
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldAddrSource:
			m.addressSource = (m.addressSource - 1 + len(addressSourceOpts)) % len(addressSourceOpts)
			m.err = ""
		case fieldAdminState:
			m.adminStateUp = !m.adminStateUp
		case fieldBackup:
			m.backup = !m.backup
		case fieldCancel:
			m.focusField = fieldSubmit
			m.updateFocus()
		}
		return m, nil

	case keyText(msg) == "/":
		if m.focusField == fieldServer && m.usesServerSelection() {
			m.openServerPicker()
			m.serverFiltering = true
			m.serverFilter = ""
			m.applyServerFilter()
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldServer:
			m.openServerPicker()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			m.Active = false
			return m, nil
		default:
			m.advanceFocus(1)
			return m, nil
		}
	}

	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	return m, nil
}

func (m Model) handleServerPickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.serverFiltering {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			m.serverFiltering = false
			m.serverFilter = ""
			m.applyServerFilter()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.serverFiltering = false
			return m, nil
		default:
			s := keyText(msg)
			switch s {
			case "backspace":
				if len(m.serverFilter) > 0 {
					m.serverFilter = m.serverFilter[:len(m.serverFilter)-1]
					m.applyServerFilter()
				}
				return m, nil
			case "esc":
				m.serverFiltering = false
				m.serverFilter = ""
				m.applyServerFilter()
				return m, nil
			}
			if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				m.serverFilter += s
				m.applyServerFilter()
				return m, nil
			}
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.serverPickerOpen = false
		return m, nil

	case key.Matches(msg, shared.Keys.Up):
		if m.pickerCursor > 0 {
			m.pickerCursor--
			m.ensurePickerVisible()
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Down):
		if m.pickerCursor < len(m.filteredServers)-1 {
			m.pickerCursor++
			m.ensurePickerVisible()
		}
		return m, nil

	case keyText(msg) == "/":
		m.serverFiltering = true
		if m.serverFilter == "" {
			m.applyServerFilter()
		}
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		if len(m.filteredServers) == 0 {
			return m, nil
		}
		m.selectServer(m.filteredServers[m.pickerCursor])
		m.serverPickerOpen = false
		m.serverFiltering = false
		m.focusField = fieldName
		m.updateFocus()
		return m, nil
	}

	return m, nil
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.addrInput.Blur()
	m.portInput.Blur()
	m.weightInput.Blur()
	m.monitorAddr.Blur()
	m.monitorPort.Blur()
	m.tagsInput.Blur()
	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldAddr:
		m.addrInput.Focus()
	case fieldPort:
		m.portInput.Focus()
	case fieldWeight:
		m.weightInput.Focus()
	case fieldMonitorAddr:
		m.monitorAddr.Focus()
	case fieldMonitorPort:
		m.monitorPort.Focus()
	case fieldTags:
		m.tagsInput.Focus()
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
	case fieldMonitorAddr:
		m.monitorAddr, cmd = m.monitorAddr.Update(msg)
	case fieldMonitorPort:
		m.monitorPort, cmd = m.monitorPort.Update(msg)
	case fieldTags:
		m.tagsInput, cmd = m.tagsInput.Update(msg)
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

	monitorAddressRaw := strings.TrimSpace(m.monitorAddr.Value())
	if monitorAddressRaw != "" && net.ParseIP(monitorAddressRaw) == nil {
		m.err = "Monitor address must be a valid IPv4 or IPv6 address"
		return m, nil
	}

	monitorPortRaw := strings.TrimSpace(m.monitorPort.Value())
	var monitorPortValue int
	var monitorPort *int
	if monitorPortRaw != "" {
		var err error
		monitorPortValue, err = strconv.Atoi(monitorPortRaw)
		if err != nil || monitorPortValue < 1 || monitorPortValue > 65535 {
			m.err = "Monitor port must be a number between 1 and 65535"
			return m, nil
		}
		monitorPort = &monitorPortValue
	}

	tags := parseTags(m.tagsInput.Value())
	adminStateUp := m.adminStateUp
	backup := m.backup

	if m.editMode {
		m.submitting = true
		m.err = ""
		client := m.client
		poolID := m.poolID
		memberID := m.memberID
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			opts := loadbalancer.MemberUpdateOpts{
				Name:              &name,
				Weight:            &weight,
				AdminStateUp:      &adminStateUp,
				Backup:            &backup,
				MonitorAddressSet: true,
				MonitorPortSet:    true,
				Tags:              &tags,
			}
			if monitorAddressRaw != "" {
				opts.MonitorAddress = &monitorAddressRaw
			}
			if monitorPort != nil {
				opts.MonitorPort = monitorPort
			}
			err := loadbalancer.UpdateMember(context.Background(), client, poolID, memberID, opts)
			if err != nil {
				return memberCreateErrMsg{err: err}
			}
			return memberCreatedMsg{}
		})
	}

	var addr string
	if m.usesManualIP() {
		addr = strings.TrimSpace(m.addrInput.Value())
		if addr == "" {
			m.err = "Address is required"
			return m, nil
		}
		if net.ParseIP(addr) == nil {
			m.err = "Address must be a valid IPv4 or IPv6 address"
			return m, nil
		}
	} else {
		selected, ok := m.selectedServer()
		if !ok {
			m.err = "Select a server first"
			return m, nil
		}
		addr = selected.address
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
		_, err := loadbalancer.CreateMember(context.Background(), client, poolID, loadbalancer.MemberCreateOpts{
			Name:           name,
			Address:        addr,
			ProtocolPort:   port,
			Weight:         weight,
			AdminStateUp:   adminStateUp,
			Backup:         backup,
			MonitorAddress: monitorAddressRaw,
			MonitorPort:    monitorPort,
			Tags:           tags,
		})
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
	if m.serverPickerOpen {
		if m.serverFiltering {
			return "type to filter • backspace delete • enter done • esc clear"
		}
		return "↑↓ navigate • enter select • / filter • esc close"
	}
	return "tab/↑↓ navigate • ←→ toggle • enter open picker • ctrl+s submit • esc cancel"
}

// View renders the form.
func (m Model) View() string {
	if m.serverPickerOpen {
		return m.serverPickerView()
	}

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

	if !m.editMode {
		rows = append(rows, label("Source", fieldAddrSource)+renderPicker(addressSourceOpts, m.addressSource))
		if m.usesManualIP() {
			rows = append(rows, label("Address", fieldAddr)+m.addrInput.View())
		} else {
			rows = append(rows, label("Server", fieldServer)+m.selectedServerView())
		}
	}
	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	if !m.editMode {
		rows = append(rows, label("Port", fieldPort)+m.portInput.View())
	}
	rows = append(rows, label("Weight", fieldWeight)+m.weightInput.View())
	rows = append(rows, label("State", fieldAdminState)+renderPicker(enabledDisabledOpts, enabledIndex(m.adminStateUp)))
	rows = append(rows, label("Backup", fieldBackup)+renderPicker(yesNoOpts, yesNoIndex(m.backup)))
	rows = append(rows, label("Mon Addr", fieldMonitorAddr)+m.monitorAddr.View())
	rows = append(rows, label("Mon Port", fieldMonitorPort)+m.monitorPort.View())
	rows = append(rows, label("Labels", fieldTags)+m.tagsInput.View())

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
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) serverPickerView() string {
	title := shared.StyleModalTitle.Render("Select Member Server")
	pickerWidth := m.pickerWidth()
	contentWidth := pickerWidth - 4

	var body string
	switch {
	case m.serversLoading:
		body = m.spinner.View() + " Loading matching servers..."
	case m.err != "" && len(m.serverOptions) == 0:
		body = lipgloss.NewStyle().Foreground(shared.ColorError).Render("Error: " + m.err)
	case len(m.filteredServers) == 0:
		body = shared.StyleHelp.Render("No matching servers found")
	default:
		var lines []string
		start := m.pickerScroll
		end := start + m.pickerListHeight()
		if end > len(m.filteredServers) {
			end = len(m.filteredServers)
		}
		for i := start; i < end; i++ {
			srv := m.filteredServers[i]
			cursor := "  "
			if i == m.pickerCursor {
				cursor = "▸ "
			}
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if i == m.pickerCursor {
				style = style.Foreground(shared.ColorHighlight).Bold(true)
			}
			statusStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
			if srv.status == "ACTIVE" {
				statusStyle = statusStyle.Foreground(shared.ColorSuccess)
			}
			statusText := shared.StatusIcon(srv.status) + srv.status
			statusWidth := minInt(14, maxInt(10, contentWidth/6))
			addressWidth := minInt(44, maxInt(20, contentWidth/2))
			nameWidth := maxInt(12, contentWidth-lipgloss.Width(cursor)-statusWidth-addressWidth-6)
			line := fmt.Sprintf(
				"%s%-*s  %-*s  %s",
				cursor,
				nameWidth,
				truncateString(srv.name, nameWidth),
				addressWidth,
				truncateString(srv.address, addressWidth),
				statusStyle.Render(truncateString(statusText, statusWidth)),
			)
			lines = append(lines, style.Render(line))
		}
		body = strings.Join(lines, "\n")
	}

	filterLine := "Filter: "
	if m.serverFilter != "" {
		filterLine += m.serverFilter
	}
	if m.serverFiltering {
		filterLine = shared.StyleHelp.Render(filterLine)
	}

	content := title + "\n\n" + body + "\n\n" + truncateString(filterLine, contentWidth) + "\n\n" + shared.StyleHelp.Render("↑↓ navigate • enter select • / filter • esc close")
	box := shared.StyleModal.Width(pickerWidth).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
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

func (m Model) selectedServerView() string {
	if m.serversLoading {
		return m.spinner.View() + " Loading matching servers..."
	}
	selected, ok := m.selectedServer()
	if !ok {
		return lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("Press enter to choose server")
	}
	return truncateString(fmt.Sprintf("%s (%s)", selected.name, selected.address), maxInt(24, m.formWidth()-18))
}

func (m Model) selectedServer() (memberServerOption, bool) {
	for _, srv := range m.serverOptions {
		if srv.id == m.selectedServerID {
			return srv, true
		}
	}
	return memberServerOption{}, false
}

func (m *Model) openServerPicker() {
	m.serverPickerOpen = true
	m.serverFiltering = false
	m.applyServerFilter()
}

func (m *Model) selectServer(srv memberServerOption) {
	m.selectedServerID = srv.id
	if strings.TrimSpace(m.nameInput.Value()) == "" {
		m.nameInput.SetValue(srv.name)
	}
}

func (m *Model) applyServerFilter() {
	m.filteredServers = nil
	query := strings.ToLower(strings.TrimSpace(m.serverFilter))
	for _, srv := range m.serverOptions {
		if query == "" ||
			strings.Contains(strings.ToLower(srv.name), query) ||
			strings.Contains(strings.ToLower(srv.id), query) ||
			strings.Contains(strings.ToLower(srv.address), query) {
			m.filteredServers = append(m.filteredServers, srv)
		}
	}

	m.pickerCursor = 0
	m.pickerScroll = 0
	for i, srv := range m.filteredServers {
		if srv.id == m.selectedServerID {
			m.pickerCursor = i
			m.ensurePickerVisible()
			return
		}
	}
}

func (m *Model) ensurePickerVisible() {
	height := m.pickerListHeight()
	if m.pickerCursor < m.pickerScroll {
		m.pickerScroll = m.pickerCursor
	}
	if m.pickerCursor >= m.pickerScroll+height {
		m.pickerScroll = m.pickerCursor - height + 1
	}
}

func (m Model) pickerListHeight() int {
	h := m.height - 12
	if h < 4 {
		h = 4
	}
	return h
}

func (m Model) fetchServers() tea.Cmd {
	client := m.computeClient
	preferredVersion := m.preferredIPVer
	return func() tea.Msg {
		servers, err := compute.ListServers(context.Background(), client)
		if err != nil {
			return memberServersErrMsg{err: err}
		}
		var options []memberServerOption
		for _, srv := range servers {
			if srv.Status != "ACTIVE" && srv.Status != "SHUTOFF" {
				continue
			}
			addr := preferredMemberAddress(srv, preferredVersion)
			if addr == "" {
				continue
			}
			if _, exists := m.excludedAddrs[addr]; exists {
				continue
			}
			name := strings.TrimSpace(srv.Name)
			if name == "" {
				name = srv.ID
			}
			options = append(options, memberServerOption{
				id:      srv.ID,
				name:    name,
				address: addr,
				status:  srv.Status,
			})
		}
		return memberServersLoadedMsg{servers: options}
	}
}

func preferredMemberAddress(srv compute.Server, preferredVersion int) string {
	switch preferredVersion {
	case 4:
		if len(srv.IPv4) > 0 {
			return srv.IPv4[0]
		}
		return ""
	case 6:
		if len(srv.IPv6) > 0 {
			return srv.IPv6[0]
		}
		return ""
	default:
		if len(srv.IPv4) > 0 {
			return srv.IPv4[0]
		}
		if len(srv.IPv6) > 0 {
			return srv.IPv6[0]
		}
		return ""
	}
}

func ipVersion(addr string) int {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return 0
	}
	if ip.To4() != nil {
		return 4
	}
	return 6
}

func makeAddressSet(addrs []string) map[string]struct{} {
	set := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		set[addr] = struct{}{}
	}
	return set
}

func parseTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func enabledIndex(v bool) int {
	if v {
		return 0
	}
	return 1
}

func yesNoIndex(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (m Model) formWidth() int {
	if m.width <= 0 {
		return 60
	}
	return maxInt(48, minInt(72, m.width-6))
}

func (m Model) pickerWidth() int {
	if m.width <= 0 {
		return 96
	}
	return maxInt(56, minInt(110, m.width-4))
}

func truncateString(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return s[:width-1] + "…"
}

func keyText(msg tea.KeyMsg) string {
	return msg.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

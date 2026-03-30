package routercreate

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

const (
	fieldName       = 0
	fieldExtNet     = 1
	fieldAdminState = 2
	fieldSubmit     = 3
	fieldCancel     = 4
	numFields       = 5
)

var adminStates = []string{"Up", "Down"}

type extNetsLoadedMsg struct{ nets []network.Network }
type fetchErrMsg struct{ err error }
type routerCreatedMsg struct{}
type routerCreateErrMsg struct{ err error }

// Model is the router create modal.
type Model struct {
	Active         bool
	client         *gophercloud.ServiceClient
	nameInput      textinput.Model
	extNetworks    []network.Network
	selectedExtNet int // -1 = none
	adminState     int // 0=Up, 1=Down
	pickerOpen     bool
	pickerCursor   int
	pickerFilter   textinput.Model
	focusField     int
	loading        int
	submitting     bool
	spinner        spinner.Model
	err            string
	width          int
	height         int
}

// New creates a router create modal.
func New(client *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "router name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	pf := textinput.New()
	pf.Prompt = "/ "
	pf.Placeholder = "filter..."
	pf.CharLimit = 64
	pf.SetVirtualCursor(false)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		nameInput:      ni,
		pickerFilter:   pf,
		spinner:        s,
		loading:        1,
		selectedExtNet: -1,
	}
}

// Init starts fetching external networks.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchExtNetworks())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case extNetsLoadedMsg:
		m.loading--
		m.extNetworks = msg.nets
		return m, nil
	case fetchErrMsg:
		m.loading--
		m.err = msg.err.Error()
		return m, nil

	case routerCreatedMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Created router", Name: m.nameInput.Value()}
		}
	case routerCreateErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		if m.loading > 0 || m.submitting {
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
		if m.loading > 0 || m.submitting {
			return m, nil
		}
		if m.pickerOpen {
			return m.updatePicker(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	return m.focusField == fieldName
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
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
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

	case key.Matches(msg, shared.Keys.Right):
		switch m.focusField {
		case fieldAdminState:
			m.adminState = (m.adminState + 1) % len(adminStates)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldAdminState:
			m.adminState = (m.adminState - 1 + len(adminStates)) % len(adminStates)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldName, fieldAdminState:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldExtNet:
			m.pickerOpen = true
			m.pickerCursor = 0
			m.pickerFilter.SetValue("")
			m.pickerFilter.Focus()
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

	if m.focusField == fieldName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updatePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	items := m.pickerItems()
	filtered := m.filteredPickerItems(items)

	switch msg.String() {
	case "esc":
		m.pickerOpen = false
		m.pickerFilter.Blur()
		return m, nil
	case "enter":
		if len(filtered) > 0 && m.pickerCursor < len(filtered) {
			m.selectedExtNet = filtered[m.pickerCursor].id
		}
		m.pickerOpen = false
		m.pickerFilter.Blur()
		m.focusField = (m.focusField + 1) % numFields
		m.updateFocus()
		return m, nil
	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case "down", "j":
		if m.pickerCursor < len(filtered)-1 {
			m.pickerCursor++
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.pickerFilter, cmd = m.pickerFilter.Update(msg)
	m.pickerCursor = 0
	return m, cmd
}

type pickerItem struct {
	id   int
	name string
	desc string
}

func (m Model) pickerItems() []pickerItem {
	items := make([]pickerItem, len(m.extNetworks))
	for i, n := range m.extNetworks {
		desc := n.ID
		if len(desc) > 8 {
			desc = desc[:8]
		}
		items[i] = pickerItem{id: i, name: n.Name, desc: desc}
	}
	return items
}

func (m Model) filteredPickerItems(items []pickerItem) []pickerItem {
	q := strings.ToLower(m.pickerFilter.Value())
	if q == "" {
		return items
	}
	var filtered []pickerItem
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.name), q) ||
			strings.Contains(strings.ToLower(item.desc), q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Router name is required"
		return m, nil
	}

	adminUp := m.adminState == 0
	extNetworkID := ""
	if m.selectedExtNet >= 0 && m.selectedExtNet < len(m.extNetworks) {
		extNetworkID = m.extNetworks[m.selectedExtNet].ID
	}

	m.submitting = true
	m.err = ""
	client := m.client

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[routercreate] creating router %q", name)
		_, err := network.CreateRouter(context.Background(), client, name, extNetworkID, adminUp)
		if err != nil {
			shared.Debugf("[routercreate] error creating router %q: %v", name, err)
			return routerCreateErrMsg{err: err}
		}
		shared.Debugf("[routercreate] created router %q", name)
		return routerCreatedMsg{}
	})
}

// View renders the modal.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Create Router")

	var body strings.Builder

	if m.loading > 0 {
		body.WriteString(m.spinner.View() + " Loading external networks...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.submitting {
		body.WriteString(m.spinner.View() + " Creating...")
		content := title + "\n\n" + body.String()
		return m.renderModal(content)
	}

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("⚠ "+m.err) + "\n\n")
	}

	type field struct {
		label   string
		value   string
		focused bool
	}
	fields := []field{
		{"Name", m.nameInput.View(), m.focusField == fieldName},
		{"External Net", m.extNetDisplay(), m.focusField == fieldExtNet},
		{"Admin State", cycleDisplay(adminStates, m.adminState), m.focusField == fieldAdminState},
	}

	for i, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := lipgloss.NewStyle().Width(14).Foreground(shared.ColorSecondary).Render(f.label)
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if f.focused {
			style = style.Foreground(shared.ColorHighlight)
		}
		body.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, style.Render(f.value)))

		// Show inline picker if open for ext net field
		if m.pickerOpen && i == 1 {
			body.WriteString(m.renderPicker())
		}
	}

	body.WriteString("\n")
	submitStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	if m.focusField == fieldSubmit {
		submitStyle = submitStyle.Background(shared.ColorSuccess).Foreground(shared.ColorBg).Bold(true)
	}
	if m.focusField == fieldCancel {
		cancelStyle = cancelStyle.Background(shared.ColorError).Foreground(shared.ColorBg).Bold(true)
	}
	body.WriteString("  " + submitStyle.Render("[ctrl+s] Submit") + "  " + cancelStyle.Render("[esc] Cancel") + "\n")
	body.WriteString("\n")
	body.WriteString(shared.StyleHelp.Render("  tab/↑↓ fields • ←→ cycle • enter picker • ctrl+s submit • esc cancel"))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) extNetDisplay() string {
	if m.selectedExtNet >= 0 && m.selectedExtNet < len(m.extNetworks) {
		n := m.extNetworks[m.selectedExtNet]
		return n.Name
	}
	return "<press enter to select, optional>"
}

func (m Model) renderPicker() string {
	var b strings.Builder

	items := m.pickerItems()
	filtered := m.filteredPickerItems(items)

	b.WriteString("      " + m.pickerFilter.View() + "\n")

	maxShow := 8
	if len(filtered) < maxShow {
		maxShow = len(filtered)
	}

	start := 0
	if m.pickerCursor >= maxShow {
		start = m.pickerCursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		item := filtered[i]
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.pickerCursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		desc := ""
		if item.desc != "" {
			desc = shared.StyleHelp.Render(" " + item.desc)
		}
		b.WriteString(fmt.Sprintf("      %s%s%s\n", cursor, style.Render(item.name), desc))
	}

	return b.String()
}

func cycleDisplay(options []string, selected int) string {
	var parts []string
	for i, opt := range options {
		if i == selected {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(shared.ColorHighlight).Render("● "+opt))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("○ "+opt))
		}
	}
	return strings.Join(parts, "  ")
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

func (m Model) fetchExtNetworks() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		nets, err := network.ListExternalNetworks(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return extNetsLoadedMsg{nets: nets}
	}
}

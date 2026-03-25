package servercreate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/network"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
)

// Field indices.
const (
	fieldName     = 0
	fieldImage    = 1
	fieldFlavor   = 2
	fieldNetwork  = 3
	fieldKeypair  = 4
	fieldSecGroup = 5
	fieldCount    = 6
	fieldSubmit   = 7
	fieldCancel   = 8
	numFields     = 9
)

type imagesLoadedMsg struct{ images []img.Image }
type flavorsLoadedMsg struct{ flavors []compute.Flavor }
type networksLoadedMsg struct{ networks []network.Network }
type keypairsLoadedMsg struct{ keypairs []compute.KeyPair }
type secGroupsLoadedMsg struct{ secGroups []network.SecurityGroup }
type fetchErrMsg struct{ err error }

type serverCreatedMsg struct{ server *compute.Server }
type serverCreateErrMsg struct{ err error }

// Model is the server create form.
type Model struct {
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient

	nameInput  textinput.Model
	countInput textinput.Model

	images     []img.Image
	flavors    []compute.Flavor
	networks   []network.Network
	keypairs   []compute.KeyPair
	secGroups  []network.SecurityGroup

	selectedImage     int
	selectedFlavor    int
	selectedNetwork   int
	selectedKeypair   int
	selectedSecGroups map[int]bool

	// Inline picker state
	pickerOpen   bool
	pickerField  int
	pickerCursor int
	pickerFilter textinput.Model

	focusField int
	loading    int // number of pending fetches
	spinner    spinner.Model
	submitting bool
	err        string
	width      int
	height     int
}

// New creates a server create form.
func New(computeClient, imageClient, networkClient *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "server name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "1"
	ci.CharLimit = 4
	ci.SetWidth(10)

	pf := textinput.New()
	pf.Prompt = "/ "
	pf.Placeholder = "filter..."
	pf.CharLimit = 64
	pf.SetVirtualCursor(false)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		computeClient:     computeClient,
		imageClient:       imageClient,
		networkClient:     networkClient,
		nameInput:         ni,
		countInput:        ci,
		pickerFilter:      pf,
		spinner:           s,
		loading:           5,
		selectedImage:     -1,
		selectedFlavor:    -1,
		selectedNetwork:   -1,
		selectedKeypair:   -1,
		selectedSecGroups: make(map[int]bool),
	}
}

// Init starts parallel fetches.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchImages(),
		m.fetchFlavors(),
		m.fetchNetworks(),
		m.fetchKeypairs(),
		m.fetchSecGroups(),
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imagesLoadedMsg:
		m.images = msg.images
		m.loading--
		return m, nil
	case flavorsLoadedMsg:
		m.flavors = msg.flavors
		m.loading--
		return m, nil
	case networksLoadedMsg:
		m.networks = msg.networks
		m.loading--
		return m, nil
	case keypairsLoadedMsg:
		m.keypairs = msg.keypairs
		m.loading--
		return m, nil
	case secGroupsLoadedMsg:
		m.secGroups = msg.secGroups
		m.loading--
		return m, nil
	case fetchErrMsg:
		m.loading--
		m.err = msg.err.Error()
		return m, nil

	case serverCreatedMsg:
		m.submitting = false
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "serverlist"}
		}
	case serverCreateErrMsg:
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
		if m.pickerOpen {
			return m.updatePicker(msg)
		}
		return m.updateForm(msg)
	}
	return m, nil
}

func (m Model) updateForm(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "serverlist"}
		}

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
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.Left) && (m.focusField == fieldSubmit || m.focusField == fieldCancel):
		if m.focusField == fieldCancel {
			m.focusField = fieldSubmit
		} else {
			m.focusField = fieldCancel
		}
		m.updateFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldName, fieldCount:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		default:
			// Open picker for selection fields
			m.pickerOpen = true
			m.pickerField = m.focusField
			m.pickerCursor = 0
			m.pickerFilter.SetValue("")
			m.pickerFilter.Focus()
			return m, nil
		}
	}

	// Handle ctrl+s to submit
	if msg.String() == "ctrl+s" {
		return m.submit()
	}

	// Text field input
	if m.focusField == fieldName {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
	if m.focusField == fieldCount {
		var cmd tea.Cmd
		m.countInput, cmd = m.countInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updatePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	items := m.pickerItems()
	isMultiSelect := m.pickerField == fieldSecGroup

	switch msg.String() {
	case "esc":
		m.pickerOpen = false
		m.pickerFilter.Blur()
		return m, nil
	case "space":
		if isMultiSelect {
			filtered := m.filteredPickerItems(items)
			if len(filtered) > 0 && m.pickerCursor < len(filtered) {
				idx := filtered[m.pickerCursor].id
				if m.selectedSecGroups[idx] {
					delete(m.selectedSecGroups, idx)
				} else {
					m.selectedSecGroups[idx] = true
				}
			}
			return m, nil
		}
	case "enter":
		if !isMultiSelect {
			filtered := m.filteredPickerItems(items)
			if len(filtered) > 0 && m.pickerCursor < len(filtered) {
				m.setPickerSelection(filtered[m.pickerCursor].id)
			}
		}
		m.pickerOpen = false
		m.pickerFilter.Blur()
		// Advance to next field
		m.focusField = (m.focusField + 1) % numFields
		m.updateFocus()
		return m, nil
	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case "down", "j":
		filtered := m.filteredPickerItems(items)
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
	switch m.pickerField {
	case fieldImage:
		items := make([]pickerItem, len(m.images))
		for i, img := range m.images {
			items[i] = pickerItem{id: i, name: img.Name, desc: img.ID[:8]}
		}
		return items
	case fieldFlavor:
		items := make([]pickerItem, len(m.flavors))
		for i, f := range m.flavors {
			items[i] = pickerItem{
				id:   i,
				name: f.Name,
				desc: fmt.Sprintf("%d vCPU, %d MB RAM, %d GB disk", f.VCPUs, f.RAM, f.Disk),
			}
		}
		return items
	case fieldNetwork:
		items := make([]pickerItem, len(m.networks))
		for i, n := range m.networks {
			shared := ""
			if n.Shared {
				shared = " (shared)"
			}
			items[i] = pickerItem{id: i, name: n.Name + shared, desc: n.ID[:8]}
		}
		return items
	case fieldKeypair:
		items := make([]pickerItem, len(m.keypairs))
		for i, kp := range m.keypairs {
			items[i] = pickerItem{id: i, name: kp.Name, desc: kp.Type}
		}
		return items
	case fieldSecGroup:
		items := make([]pickerItem, len(m.secGroups))
		for i, sg := range m.secGroups {
			items[i] = pickerItem{id: i, name: sg.Name, desc: sg.Description}
		}
		return items
	}
	return nil
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

func (m *Model) setPickerSelection(idx int) {
	switch m.pickerField {
	case fieldImage:
		m.selectedImage = idx
	case fieldFlavor:
		m.selectedFlavor = idx
	case fieldNetwork:
		m.selectedNetwork = idx
	case fieldKeypair:
		m.selectedKeypair = idx
	}
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldCount {
		m.countInput.Focus()
	} else {
		m.countInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Server name is required"
		return m, nil
	}
	if m.selectedImage < 0 {
		m.err = "Image is required"
		return m, nil
	}
	if m.selectedFlavor < 0 {
		m.err = "Flavor is required"
		return m, nil
	}

	count := 1
	countStr := strings.TrimSpace(m.countInput.Value())
	if countStr != "" {
		n, err := strconv.Atoi(countStr)
		if err != nil || n < 1 {
			m.err = "Count must be a positive number"
			return m, nil
		}
		if n > 100 {
			m.err = "Count must be 100 or less"
			return m, nil
		}
		count = n
	}

	opts := servers.CreateOpts{
		Name:      name,
		ImageRef:  m.images[m.selectedImage].ID,
		FlavorRef: m.flavors[m.selectedFlavor].ID,
	}

	if count > 1 {
		opts.Min = count
		opts.Max = count
	}

	if m.selectedNetwork >= 0 {
		opts.Networks = []servers.Network{
			{UUID: m.networks[m.selectedNetwork].ID},
		}
	}

	if len(m.selectedSecGroups) > 0 {
		var sgNames []string
		for idx := range m.selectedSecGroups {
			if idx < len(m.secGroups) {
				sgNames = append(sgNames, m.secGroups[idx].Name)
			}
		}
		opts.SecurityGroups = sgNames
	}

	var createOpts servers.CreateOptsBuilder = opts
	if m.selectedKeypair >= 0 {
		createOpts = keypairs.CreateOptsExt{
			CreateOptsBuilder: opts,
			KeyName:           m.keypairs[m.selectedKeypair].Name,
		}
	}

	m.submitting = true
	m.err = ""
	client := m.computeClient
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		srv, err := compute.CreateServerWithOpts(context.Background(), client, createOpts)
		if err != nil {
			return serverCreateErrMsg{err: err}
		}
		return serverCreatedMsg{server: srv}
	})
}

// View renders the create form.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Create Server")
	if m.loading > 0 {
		title += " " + m.spinner.View() + shared.StyleHelp.Render(" loading resources...")
	}
	if m.submitting {
		title += " " + m.spinner.View() + shared.StyleHelp.Render(" creating...")
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  ⚠ "+m.err) + "\n\n")
	}

	fields := []struct {
		label    string
		value    string
		focused  bool
		isInput  bool
	}{
		{"Server Name", m.nameInput.View(), m.focusField == fieldName, true},
		{"Image", m.selectionDisplay(fieldImage), m.focusField == fieldImage, false},
		{"Flavor", m.selectionDisplay(fieldFlavor), m.focusField == fieldFlavor, false},
		{"Network", m.selectionDisplay(fieldNetwork), m.focusField == fieldNetwork, false},
		{"Key Pair", m.selectionDisplay(fieldKeypair), m.focusField == fieldKeypair, false},
		{"Security Groups", m.selectionDisplay(fieldSecGroup), m.focusField == fieldSecGroup, false},
		{"Count", m.countInput.View(), m.focusField == fieldCount, true},
	}

	for i, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := shared.StyleLabel.Render(f.label)

		if f.isInput {
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, f.value))
		} else {
			style := lipgloss.NewStyle().Foreground(shared.ColorFg)
			if f.focused {
				style = style.Foreground(shared.ColorHighlight)
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, style.Render(f.value)))
		}

		// Show inline picker if open for this field
		if m.pickerOpen && m.pickerField == i {
			b.WriteString(m.renderPicker())
		}
	}

	b.WriteString("\n")

	// Buttons
	submitStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	cancelStyle := lipgloss.NewStyle().Padding(0, 2).Background(shared.ColorMuted).Foreground(shared.ColorFg)
	if m.focusField == fieldSubmit {
		submitStyle = submitStyle.Background(shared.ColorSuccess).Foreground(shared.ColorBg).Bold(true)
	}
	if m.focusField == fieldCancel {
		cancelStyle = cancelStyle.Background(shared.ColorError).Foreground(shared.ColorBg).Bold(true)
	}
	b.WriteString("  " + submitStyle.Render("[ctrl+s] Submit") + "  " + cancelStyle.Render("[esc] Cancel") + "\n")
	b.WriteString("\n")
	b.WriteString(shared.StyleHelp.Render("  tab/↑↓ navigate • enter select • ctrl+s submit • esc cancel") + "\n")

	return b.String()
}

func (m Model) selectionDisplay(field int) string {
	switch field {
	case fieldImage:
		if m.selectedImage >= 0 && m.selectedImage < len(m.images) {
			return m.images[m.selectedImage].Name
		}
	case fieldFlavor:
		if m.selectedFlavor >= 0 && m.selectedFlavor < len(m.flavors) {
			f := m.flavors[m.selectedFlavor]
			return fmt.Sprintf("%s (%d vCPU, %d MB RAM)", f.Name, f.VCPUs, f.RAM)
		}
	case fieldNetwork:
		if m.selectedNetwork >= 0 && m.selectedNetwork < len(m.networks) {
			return m.networks[m.selectedNetwork].Name
		}
	case fieldKeypair:
		if m.selectedKeypair >= 0 && m.selectedKeypair < len(m.keypairs) {
			return m.keypairs[m.selectedKeypair].Name
		}
	case fieldSecGroup:
		if len(m.selectedSecGroups) > 0 {
			var names []string
			for idx := range m.selectedSecGroups {
				if idx < len(m.secGroups) {
					names = append(names, m.secGroups[idx].Name)
				}
			}
			return strings.Join(names, ", ")
		}
		return "<enter to select, optional>"
	}
	return "<press enter to select>"
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

	isMultiSelect := m.pickerField == fieldSecGroup
	for i := start; i < end; i++ {
		item := filtered[i]
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.pickerCursor {
			cursor = "▸ "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		check := ""
		if isMultiSelect && m.selectedSecGroups[item.id] {
			check = "● "
		} else if isMultiSelect {
			check = "○ "
		}
		desc := ""
		if item.desc != "" {
			desc = shared.StyleHelp.Render(" " + item.desc)
		}
		b.WriteString(fmt.Sprintf("      %s%s%s%s\n", cursor, check, style.Render(item.name), desc))
	}

	return b.String()
}

func (m Model) fetchImages() tea.Cmd {
	client := m.imageClient
	return func() tea.Msg {
		images, err := img.ListImages(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return imagesLoadedMsg{images: images}
	}
}

func (m Model) fetchFlavors() tea.Cmd {
	client := m.computeClient
	return func() tea.Msg {
		flavors, err := compute.ListFlavors(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return flavorsLoadedMsg{flavors: flavors}
	}
}

func (m Model) fetchNetworks() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		nets, err := network.ListNetworks(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return networksLoadedMsg{networks: nets}
	}
}

func (m Model) fetchKeypairs() tea.Cmd {
	client := m.computeClient
	return func() tea.Msg {
		kps, err := compute.ListKeyPairs(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return keypairsLoadedMsg{keypairs: kps}
	}
}

func (m Model) fetchSecGroups() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		sgs, err := network.ListSecurityGroups(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return secGroupsLoadedMsg{secGroups: sgs}
	}
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.pickerOpen && m.pickerField == fieldSecGroup {
		return "↑↓ navigate • space toggle • enter confirm • esc close • type to filter"
	}
	if m.pickerOpen {
		return "↑↓ navigate • enter select • esc close • type to filter"
	}
	return "tab/shift+tab fields • enter open picker • ctrl+s submit • esc cancel"
}

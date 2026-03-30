package volumecreate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/volume"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
)

const (
	fieldName   = 0
	fieldSize   = 1
	fieldType   = 2
	fieldAZ     = 3
	fieldDesc   = 4
	fieldSubmit = 5
	fieldCancel = 6
	numFields   = 7
)

type volumeTypesLoadedMsg struct{ types []volume.VolumeType }
type fetchErrMsg struct{ err error }
type volumeCreatedMsg struct{ vol *volume.Volume }
type volumeCreateErrMsg struct{ err error }

// Model is the volume create form.
type Model struct {
	bsClient *gophercloud.ServiceClient

	nameInput textinput.Model
	sizeInput textinput.Model
	descInput textinput.Model

	volumeTypes []volume.VolumeType

	selectedType int
	selectedAZ   int

	// Inline picker state
	pickerOpen   bool
	pickerField  int
	pickerCursor int
	pickerFilter textinput.Model

	focusField int
	loading    int
	spinner    spinner.Model
	submitting bool
	err        string
	width      int
	height     int
}

// New creates a volume create form.
func New(bsClient *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "volume name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "size in GB"
	si.CharLimit = 10
	si.SetWidth(15)

	di := textinput.New()
	di.Prompt = ""
	di.Placeholder = "optional description"
	di.CharLimit = 255
	di.SetWidth(40)

	pf := textinput.New()
	pf.Prompt = "/ "
	pf.Placeholder = "filter..."
	pf.CharLimit = 64
	pf.SetVirtualCursor(false)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		bsClient:     bsClient,
		nameInput:    ni,
		sizeInput:    si,
		descInput:    di,
		pickerFilter: pf,
		spinner:      s,
		loading:      1,
		selectedType: -1,
		selectedAZ:   -1,
	}
}

// Init starts parallel fetches.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchVolumeTypes(),
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volumeTypesLoadedMsg:
		m.volumeTypes = msg.types
		m.loading--
		return m, nil
	case fetchErrMsg:
		m.loading--
		m.err = msg.err.Error()
		return m, nil

	case volumeCreatedMsg:
		m.submitting = false
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "volumelist"}
		}
	case volumeCreateErrMsg:
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

func (m Model) isTextInput() bool {
	return m.focusField == fieldName || m.focusField == fieldSize || m.focusField == fieldDesc
}

func (m Model) updateForm(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Route to text input first — only intercept navigation keys
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "volumelist"}
			}
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
			case fieldSize:
				var cmd tea.Cmd
				m.sizeInput, cmd = m.sizeInput.Update(msg)
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
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "volumelist"}
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
		case fieldName, fieldSize, fieldDesc:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "volumelist"}
			}
		default:
			m.pickerOpen = true
			m.pickerField = m.focusField
			m.pickerCursor = 0
			m.pickerFilter.SetValue("")
			m.pickerFilter.Focus()
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
	case fieldSize:
		var cmd tea.Cmd
		m.sizeInput, cmd = m.sizeInput.Update(msg)
		return m, cmd
	case fieldDesc:
		var cmd tea.Cmd
		m.descInput, cmd = m.descInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updatePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	items := m.pickerItems()

	switch msg.String() {
	case "esc":
		m.pickerOpen = false
		m.pickerFilter.Blur()
		return m, nil
	case "enter":
		filtered := m.filteredPickerItems(items)
		if len(filtered) > 0 && m.pickerCursor < len(filtered) {
			m.setPickerSelection(filtered[m.pickerCursor].id)
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
	case fieldType:
		items := make([]pickerItem, len(m.volumeTypes))
		for i, vt := range m.volumeTypes {
			items[i] = pickerItem{id: i, name: vt.Name, desc: vt.ID[:8]}
		}
		return items
	case fieldAZ:
		azs := []string{"nova", "az1", "az2"}
		items := make([]pickerItem, len(azs))
		for i, az := range azs {
			items[i] = pickerItem{id: i, name: az}
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
	case fieldType:
		m.selectedType = idx
	case fieldAZ:
		m.selectedAZ = idx
	}
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldSize {
		m.sizeInput.Focus()
	} else {
		m.sizeInput.Blur()
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
		m.err = "Volume name is required"
		return m, nil
	}

	sizeStr := strings.TrimSpace(m.sizeInput.Value())
	if sizeStr == "" {
		m.err = "Size is required"
		return m, nil
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 1 {
		m.err = "Size must be a positive integer (GB)"
		return m, nil
	}

	opts := volumes.CreateOpts{
		Name:        name,
		Size:        size,
		Description: strings.TrimSpace(m.descInput.Value()),
	}

	if m.selectedType >= 0 && m.selectedType < len(m.volumeTypes) {
		opts.VolumeType = m.volumeTypes[m.selectedType].Name
	}

	m.submitting = true
	m.err = ""
	client := m.bsClient
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[volumecreate] creating volume %q (size=%dGB)", name, size)
		vol, err := volume.CreateVolume(context.Background(), client, opts)
		if err != nil {
			shared.Debugf("[volumecreate] error creating volume %q: %v", name, err)
			return volumeCreateErrMsg{err: err}
		}
		shared.Debugf("[volumecreate] created volume %q (id=%s)", name, vol.ID)
		return volumeCreatedMsg{vol: vol}
	})
}

// View renders the create form.
func (m Model) View() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Create Volume")
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
		label   string
		value   string
		focused bool
		isInput bool
	}{
		{"Name", m.nameInput.View(), m.focusField == fieldName, true},
		{"Size (GB)", m.sizeInput.View(), m.focusField == fieldSize, true},
		{"Type", m.selectionDisplay(fieldType), m.focusField == fieldType, false},
		{"AZ", m.selectionDisplay(fieldAZ), m.focusField == fieldAZ, false},
		{"Description", m.descInput.View(), m.focusField == fieldDesc, true},
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

		// pickerField maps to the field's index in the fields slice
		pickerFieldIdx := -1
		switch m.pickerField {
		case fieldType:
			pickerFieldIdx = 2
		case fieldAZ:
			pickerFieldIdx = 3
		}
		if m.pickerOpen && pickerFieldIdx == i {
			b.WriteString(m.renderPicker())
		}
	}

	b.WriteString("\n")

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
	case fieldType:
		if m.selectedType >= 0 && m.selectedType < len(m.volumeTypes) {
			return m.volumeTypes[m.selectedType].Name
		}
	case fieldAZ:
		if m.selectedAZ >= 0 {
			azs := []string{"nova", "az1", "az2"}
			if m.selectedAZ < len(azs) {
				return azs[m.selectedAZ]
			}
		}
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

func (m Model) fetchVolumeTypes() tea.Cmd {
	client := m.bsClient
	return func() tea.Msg {
		types, err := volume.ListVolumeTypes(context.Background(), client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return volumeTypesLoadedMsg{types: types}
	}
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.pickerOpen {
		return "↑↓ navigate • enter select • esc close • type to filter"
	}
	return "tab/shift+tab fields • enter open picker • ctrl+s submit • esc cancel"
}

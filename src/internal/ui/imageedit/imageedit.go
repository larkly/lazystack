package imageedit

import (
	"context"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
)

const (
	fieldName      = 0
	fieldVisibility = 1
	fieldMinDisk   = 2
	fieldMinRAM    = 3
	fieldTags      = 4
	fieldProtected = 5
	fieldSubmit    = 6
	fieldCancel    = 7
	numFields      = 8
)

var (
	visibilityOpts = []string{"public", "private", "shared", "community"}
	protectedOpts  = []string{"no", "yes"}
)

type imageEditedMsg struct{}
type imageEditErrMsg struct{ err error }

// Model is the image edit modal.
type Model struct {
	Active   bool
	client   *gophercloud.ServiceClient
	imageID  string

	nameInput    textinput.Model
	visibility   int
	minDiskInput textinput.Model
	minRAMInput  textinput.Model
	tagsInput    textinput.Model
	protected    int

	// Initial values for change detection
	initName       string
	initVisibility int
	initMinDisk    string
	initMinRAM     string
	initTags       string
	initProtected  int

	focusField int
	submitting bool
	spinner    spinner.Model
	width      int
	height     int
	err        string
}

// New creates an image edit modal with pre-filled values.
func New(client *gophercloud.ServiceClient, imageID, name, visibility string,
	minDisk, minRAM int, tags []string, protected bool) Model {

	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "image name"
	ni.SetValue(name)
	ni.CharLimit = 128
	ni.SetWidth(40)
	ni.Focus()

	mdi := textinput.New()
	mdi.Prompt = ""
	mdi.Placeholder = "0"
	mdi.SetValue(strconv.Itoa(minDisk))
	mdi.CharLimit = 6
	mdi.SetWidth(8)

	mri := textinput.New()
	mri.Prompt = ""
	mri.Placeholder = "0"
	mri.SetValue(strconv.Itoa(minRAM))
	mri.CharLimit = 8
	mri.SetWidth(8)

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "comma,separated,tags"
	ti.SetValue(strings.Join(tags, ", "))
	ti.CharLimit = 256
	ti.SetWidth(40)

	visIdx := 0
	for i, v := range visibilityOpts {
		if v == visibility {
			visIdx = i
			break
		}
	}

	protIdx := 0
	if protected {
		protIdx = 1
	}

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		Active:         true,
		client:         client,
		imageID:        imageID,
		nameInput:      ni,
		visibility:     visIdx,
		minDiskInput:   mdi,
		minRAMInput:    mri,
		tagsInput:      ti,
		protected:      protIdx,
		initName:       name,
		initVisibility: visIdx,
		initMinDisk:    strconv.Itoa(minDisk),
		initMinRAM:     strconv.Itoa(minRAM),
		initTags:       strings.Join(tags, ", "),
		initProtected:  protIdx,
		spinner:        s,
	}
}

// Init returns initial commands.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[imageedit] Init() imageID=%s", m.imageID)
	return m.nameInput.Focus()
}

func (m Model) isTextInput() bool {
	switch m.focusField {
	case fieldName, fieldMinDisk, fieldMinRAM, fieldTags:
		return true
	}
	return false
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case imageEditedMsg:
		m.Active = false
		m.submitting = false
		shared.Debugf("[imageedit] update success imageID=%s name=%q", m.imageID, m.nameInput.Value())
		return m, func() tea.Msg {
			return shared.ResourceActionMsg{Action: "Updated image", Name: m.nameInput.Value()}
		}

	case imageEditErrMsg:
		m.submitting = false
		m.err = msg.err.Error()
		shared.Debugf("[imageedit] error: %v", msg.err)
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

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
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
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			var cmd tea.Cmd
			switch m.focusField {
			case fieldName:
				m.nameInput, cmd = m.nameInput.Update(msg)
			case fieldMinDisk:
				m.minDiskInput, cmd = m.minDiskInput.Update(msg)
			case fieldMinRAM:
				m.minRAMInput, cmd = m.minRAMInput.Update(msg)
			case fieldTags:
				m.tagsInput, cmd = m.tagsInput.Update(msg)
			}
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
		case fieldVisibility:
			m.visibility = (m.visibility + 1) % len(visibilityOpts)
		case fieldProtected:
			m.protected = (m.protected + 1) % len(protectedOpts)
		case fieldSubmit:
			m.focusField = fieldCancel
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldVisibility:
			m.visibility = (m.visibility - 1 + len(visibilityOpts)) % len(visibilityOpts)
		case fieldProtected:
			m.protected = (m.protected - 1 + len(protectedOpts)) % len(protectedOpts)
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
			m.focusField = (m.focusField + 1) % numFields
			m.updateFocus()
		}
		return m, nil
	case msg.String() == "ctrl+s":
		return m.submit()
	}
	return m, nil
}

func (m *Model) updateFocus() {
	m.nameInput.Blur()
	m.minDiskInput.Blur()
	m.minRAMInput.Blur()
	m.tagsInput.Blur()

	switch m.focusField {
	case fieldName:
		m.nameInput.Focus()
	case fieldMinDisk:
		m.minDiskInput.Focus()
	case fieldMinRAM:
		m.minRAMInput.Focus()
	case fieldTags:
		m.tagsInput.Focus()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	opts := image.UpdateImageOpts{}

	name := m.nameInput.Value()
	if name != m.initName {
		opts.Name = &name
	}

	if m.visibility != m.initVisibility {
		vis := visibilityOpts[m.visibility]
		opts.Visibility = &vis
	}

	diskStr := strings.TrimSpace(m.minDiskInput.Value())
	if diskStr != m.initMinDisk {
		disk, err := strconv.Atoi(diskStr)
		if err != nil && diskStr != "" {
			m.err = "Min Disk must be a number"
			return m, nil
		}
		if diskStr == "" {
			disk = 0
		}
		opts.MinDisk = &disk
	}

	ramStr := strings.TrimSpace(m.minRAMInput.Value())
	if ramStr != m.initMinRAM {
		ram, err := strconv.Atoi(ramStr)
		if err != nil && ramStr != "" {
			m.err = "Min RAM must be a number"
			return m, nil
		}
		if ramStr == "" {
			ram = 0
		}
		opts.MinRAM = &ram
	}

	tagsStr := m.tagsInput.Value()
	if tagsStr != m.initTags {
		var tags []string
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		opts.Tags = &tags
	}

	if m.protected != m.initProtected {
		p := m.protected == 1
		opts.Protected = &p
	}

	m.submitting = true
	m.err = ""
	shared.Debugf("[imageedit] update submit imageID=%s", m.imageID)
	client := m.client
	imageID := m.imageID
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := image.UpdateImage(context.Background(), client, imageID, opts)
		if err != nil {
			return imageEditErrMsg{err: err}
		}
		return imageEditedMsg{}
	})
}

// SetSize updates the dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints.
func (m Model) Hints() string {
	return "tab/↑↓ navigate • ←→ cycle • ctrl+s submit • esc cancel"
}

// View renders the form.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("Edit Image")

	labelW := 12
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorSecondary).Bold(true).Width(labelW)
	focusStyle := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Width(labelW)

	label := func(name string, field int) string {
		if m.focusField == field {
			return focusStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	cycleDisplay := func(opts []string, idx int) string {
		var parts []string
		for i, o := range opts {
			if i == idx {
				parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true).Render(o))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(o))
			}
		}
		return strings.Join(parts, " / ")
	}

	var rows []string
	rows = append(rows, label("Name", fieldName)+m.nameInput.View())
	rows = append(rows, label("Visibility", fieldVisibility)+cycleDisplay(visibilityOpts, m.visibility))
	rows = append(rows, label("Min Disk", fieldMinDisk)+m.minDiskInput.View()+" GB")
	rows = append(rows, label("Min RAM", fieldMinRAM)+m.minRAMInput.View()+" MB")
	rows = append(rows, label("Tags", fieldTags)+m.tagsInput.View())
	rows = append(rows, label("Protected", fieldProtected)+cycleDisplay(protectedOpts, m.protected))

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
		rows = append(rows, m.spinner.View()+" Updating image...")
	} else {
		rows = append(rows, submitStyle.Render("[ctrl+s] Submit")+"  "+cancelStyle.Render("[esc] Cancel"))
	}

	content := title + "\n\n" + strings.Join(rows, "\n")
	box := shared.StyleModal.Width(m.formWidth()).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) formWidth() int {
	if m.width <= 0 {
		return 60
	}
	w := m.width - 6
	if w > 72 {
		w = 72
	}
	if w < 48 {
		w = 48
	}
	return w
}

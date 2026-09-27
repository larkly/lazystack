package servercreate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/audit"
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/shared"
)

// Field indices.
const (
	fieldName         = 0
	fieldNameTemplate = 1
	fieldImage        = 2
	fieldFlavor       = 3
	fieldNetwork      = 4
	fieldKeypair      = 5
	fieldSecGroup     = 6
	fieldUserData     = 7
	fieldCount        = 8
	fieldSubmit       = 9
	fieldCancel       = 10
	numFields         = 11
)

var udFileExtensions = map[string]bool{
	".yaml": true, ".yml": true, ".sh": true,
	".cfg": true, ".txt": true, ".conf": true,
	"": true, // extensionless files
}

type udPickerEntry struct {
	name  string
	path  string
	isDir bool
	size  int64
}

type imagesLoadedMsg struct{ images []img.Image }
type flavorsLoadedMsg struct{ flavors []compute.Flavor }
type networksLoadedMsg struct{ networks []network.Network }
type keypairsLoadedMsg struct{ keypairs []compute.KeyPair }
type secGroupsLoadedMsg struct{ secGroups []network.SecurityGroup }
type fetchErrMsg struct{ err error }

type serverCreatedMsg struct {
	shared.Audit
	server *compute.Server
}
type serverCreateErrMsg struct {
	shared.Audit
	err error
}

// ServerCloneCreatedMsg is sent when a server is created in clone mode with volume cloning enabled.
type ServerCloneCreatedMsg struct {
	Server    *compute.Server
	VolumeIDs []string // source volume IDs to clone
}

// CloneConfig holds pre-fill data for clone mode.
type CloneConfig struct {
	SourceName    string
	ImageID       string
	FlavorID      string
	FlavorName    string
	KeyName       string
	SecGroupNames []string
	NetworkNames  map[string][]string // from Server.Networks
	VolumeIDs     []string            // from Server.VolAttach
}

// Model is the server create form.
type Model struct {
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient

	nameInput     textinput.Model
	templateInput textinput.Model
	countInput    textinput.Model

	images    []img.Image
	flavors   []compute.Flavor
	networks  []network.Network
	keypairs  []compute.KeyPair
	secGroups []network.SecurityGroup

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

	// User data (cloud-init)
	userData     []byte
	userDataFile string // display filename

	// User data file picker
	udPickerOpen    bool
	udPickerDir     string
	udPickerEntries []udPickerEntry
	udPickerCursor  int

	// Clone mode
	cloneMode    bool
	cloneConfig  *CloneConfig
	cloneVolumes bool // checkbox state for volume cloning
}

// New creates a server create form.
func New(computeClient, imageClient, networkClient *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "server name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "web-{n:02d}"
	ti.CharLimit = 255
	ti.SetWidth(40)

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
		templateInput:     ti,
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

// NewClone creates a server create form in clone mode with pre-filled data.
func NewClone(computeClient, imageClient, networkClient *gophercloud.ServiceClient, cfg CloneConfig) Model {
	m := New(computeClient, imageClient, networkClient)
	m.cloneMode = true
	m.cloneConfig = &cfg
	m.nameInput.SetValue(cfg.SourceName)
	m.nameInput.CursorEnd()
	m.templateInput.SetValue("")
	return m
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
		if m.loading == 0 && m.cloneMode {
			m.applyClonePreFill()
		}
		return m, nil
	case flavorsLoadedMsg:
		m.flavors = msg.flavors
		m.loading--
		if m.loading == 0 && m.cloneMode {
			m.applyClonePreFill()
		}
		return m, nil
	case networksLoadedMsg:
		m.networks = msg.networks
		m.loading--
		if m.loading == 0 && m.cloneMode {
			m.applyClonePreFill()
		}
		return m, nil
	case keypairsLoadedMsg:
		m.keypairs = msg.keypairs
		m.loading--
		if m.loading == 0 && m.cloneMode {
			m.applyClonePreFill()
		}
		return m, nil
	case secGroupsLoadedMsg:
		m.secGroups = msg.secGroups
		m.loading--
		if m.loading == 0 && m.cloneMode {
			m.applyClonePreFill()
		}
		return m, nil
	case fetchErrMsg:
		m.loading--
		m.err = msg.err.Error()
		return m, nil

	case serverCreatedMsg:
		m.submitting = false
		if m.cloneMode && m.cloneVolumes && m.hasCloneVolumes() {
			volIDs := m.cloneConfig.VolumeIDs
			srv := msg.server
			return m, func() tea.Msg {
				return ServerCloneCreatedMsg{Server: srv, VolumeIDs: volIDs}
			}
		}
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
		if m.udPickerOpen {
			return m.updateUDPicker(msg)
		}
		if m.pickerOpen {
			return m.updatePicker(msg)
		}
		return m.updateForm(msg)
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	if m.focusField == fieldName {
		return true
	}
	if m.focusField == fieldNameTemplate && !m.cloneMode {
		return true
	}
	if m.focusField == fieldCount && !m.cloneMode {
		return true
	}
	return false
}

func (m Model) updateForm(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Route to text input first — only intercept navigation keys
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "serverlist"}
			}
		case key.Matches(msg, shared.Keys.Tab):
			m.advanceFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.ShiftTab):
			m.retreatFocus()
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			m.advanceFocus()
			return m, nil
		case msg.String() == "ctrl+s":
			return m.submit()
		default:
			switch m.focusField {
			case fieldName:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			case fieldNameTemplate:
				var cmd tea.Cmd
				m.templateInput, cmd = m.templateInput.Update(msg)
				return m, cmd
			case fieldCount:
				var cmd tea.Cmd
				m.countInput, cmd = m.countInput.Update(msg)
				return m, cmd
			}
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "serverlist"}
		}

	case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
		m.advanceFocus()
		return m, nil

	case key.Matches(msg, shared.Keys.ShiftTab), key.Matches(msg, shared.Keys.Up):
		m.retreatFocus()
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
		case fieldName:
			m.advanceFocus()
			return m, nil
		case fieldNameTemplate:
			m.advanceFocus()
			return m, nil
		case fieldUserData:
			return m.openUDPicker()
		case fieldCount:
			if m.cloneMode && m.hasCloneVolumes() {
				m.cloneVolumes = !m.cloneVolumes
				return m, nil
			}
			m.advanceFocus()
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

	// Handle backspace on user data to clear selection
	if m.focusField == fieldUserData && (msg.String() == "backspace" || msg.String() == "delete") {
		m.userData = nil
		m.userDataFile = ""
		return m, nil
	}

	// Handle space to toggle clone volumes checkbox
	if m.cloneMode && m.focusField == fieldCount && m.hasCloneVolumes() && key.Matches(msg, shared.Keys.Select) {
		m.cloneVolumes = !m.cloneVolumes
		return m, nil
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
		m.advanceFocus()
		return m, nil
	// Only arrows navigate: j/k must reach the filter as text.
	case "up":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case "down":
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
			items[i] = pickerItem{id: i, name: img.Name, desc: shortID(img.ID)}
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
			items[i] = pickerItem{id: i, name: n.Name + shared, desc: shortID(n.ID)}
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

// shortID abbreviates an ID for display to at most 8 runes; short or empty
// IDs are returned unchanged.
func shortID(id string) string {
	r := []rune(id)
	if len(r) > 8 {
		return string(r[:8])
	}
	return id
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

func (m Model) sortedSecGroupIndices() []int {
	indices := make([]int, 0, len(m.selectedSecGroups))
	for idx := range m.selectedSecGroups {
		if idx < len(m.secGroups) {
			indices = append(indices, idx)
		}
	}
	sort.Ints(indices)
	return indices
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldNameTemplate && !m.cloneMode {
		m.templateInput.Focus()
	} else {
		m.templateInput.Blur()
	}
	if m.focusField == fieldCount && !m.cloneMode {
		m.countInput.Focus()
	} else {
		m.countInput.Blur()
	}
}

// advanceFocus moves focus forward by 1, skipping fieldCount in clone mode without volumes.
func (m *Model) advanceFocus() {
	m.focusField = (m.focusField + 1) % numFields
	if m.focusField == fieldNameTemplate && m.cloneMode {
		m.focusField = (m.focusField + 1) % numFields
	}
	if m.focusField == fieldCount && m.cloneMode && !m.hasCloneVolumes() {
		m.focusField = (m.focusField + 1) % numFields
	}
	m.updateFocus()
}

// retreatFocus moves focus backward by 1, skipping fieldCount in clone mode without volumes.
func (m *Model) retreatFocus() {
	m.focusField = (m.focusField - 1 + numFields) % numFields
	if m.focusField == fieldNameTemplate && m.cloneMode {
		m.focusField = (m.focusField - 1 + numFields) % numFields
	}
	if m.focusField == fieldCount && m.cloneMode && !m.hasCloneVolumes() {
		m.focusField = (m.focusField - 1 + numFields) % numFields
	}
	m.updateFocus()
}

func (m Model) userDataDisplay() string {
	if m.userDataFile != "" {
		return fmt.Sprintf("%s (%s)", m.userDataFile, shared.FormatSize(int64(len(m.userData))))
	}
	return "none"
}

// --- User data file picker ---

func (m Model) openUDPicker() (Model, tea.Cmd) {
	dir, _ := os.UserHomeDir()
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return m.loadUDDir(dir)
}

func (m Model) loadUDDir(dir string) (Model, tea.Cmd) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.err = "Cannot read directory: " + err.Error()
		return m, nil
	}

	var dirs, files []udPickerEntry

	if dir != "/" {
		dirs = append(dirs, udPickerEntry{name: "..", path: filepath.Dir(dir), isDir: true})
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			dirs = append(dirs, udPickerEntry{name: e.Name() + "/", path: full, isDir: true})
		} else {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if udFileExtensions[ext] {
				info, _ := e.Info()
				var size int64
				if info != nil {
					size = info.Size()
				}
				files = append(files, udPickerEntry{name: e.Name(), path: full, size: size})
			}
		}
	}

	sort.Slice(dirs[1:], func(i, j int) bool {
		return strings.ToLower(dirs[i+1].name) < strings.ToLower(dirs[j+1].name)
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].name) < strings.ToLower(files[j].name)
	})

	m.udPickerEntries = append(dirs, files...)
	m.udPickerDir = dir
	m.udPickerOpen = true
	m.udPickerCursor = 0
	m.err = ""
	return m, nil
}

func (m Model) updateUDPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.udPickerOpen = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.udPickerCursor > 0 {
			m.udPickerCursor--
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Down):
		if m.udPickerCursor < len(m.udPickerEntries)-1 {
			m.udPickerCursor++
		}
		return m, nil
	case key.Matches(msg, shared.Keys.PageUp):
		m.udPickerCursor -= 10
		if m.udPickerCursor < 0 {
			m.udPickerCursor = 0
		}
		return m, nil
	case key.Matches(msg, shared.Keys.PageDown):
		m.udPickerCursor += 10
		if m.udPickerCursor >= len(m.udPickerEntries) {
			m.udPickerCursor = len(m.udPickerEntries) - 1
		}
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if m.udPickerCursor < 0 || m.udPickerCursor >= len(m.udPickerEntries) {
			return m, nil
		}
		entry := m.udPickerEntries[m.udPickerCursor]
		if entry.isDir {
			return m.loadUDDir(entry.path)
		}
		// Read file content
		data, err := os.ReadFile(entry.path)
		if err != nil {
			m.err = "Cannot read file: " + err.Error()
			m.udPickerOpen = false
			return m, nil
		}
		m.userData = data
		m.userDataFile = entry.name
		m.udPickerOpen = false
		m.advanceFocus()
		return m, nil
	}
	return m, nil
}

func (m Model) renderUDPicker() string {
	var b strings.Builder

	titleText := "Create Server"
	if m.cloneMode {
		titleText = "Clone Server"
	}
	title := shared.StyleTitle.Render(titleText + " \u2014 Select User Data File")
	b.WriteString(title + "\n\n")

	dirStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	b.WriteString("  " + dirStyle.Render("Browsing: "+m.udPickerDir) + "\n")
	b.WriteString("  " + dirStyle.Render("Showing: .yaml .yml .sh .cfg .txt .conf") + "\n\n")

	maxShow := m.height - 10
	if maxShow < 5 {
		maxShow = 5
	}
	start := 0
	if m.udPickerCursor >= maxShow {
		start = m.udPickerCursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(m.udPickerEntries) {
		end = len(m.udPickerEntries)
	}

	nameW := 40
	if m.width > 80 {
		nameW = m.width - 40
	}

	for i := start; i < end; i++ {
		e := m.udPickerEntries[i]
		cursor := "  "
		if i == m.udPickerCursor {
			cursor = "\u25b8 "
		}

		name := e.name
		if len(name) > nameW {
			name = name[:nameW-1] + "\u2026"
		}

		var sizeStr string
		if e.isDir {
			sizeStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("<dir>")
		} else {
			sizeStr = lipgloss.NewStyle().Foreground(shared.ColorMuted).Render(shared.FormatSize(e.size))
		}

		nameStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if e.isDir {
			nameStyle = nameStyle.Foreground(shared.ColorCyan)
		}
		if i == m.udPickerCursor {
			nameStyle = nameStyle.Bold(true).Foreground(shared.ColorHighlight)
		}

		b.WriteString("  " + cursor + nameStyle.Width(nameW).Render(name) + "  " + sizeStr + "\n")
	}

	if len(m.udPickerEntries) == 0 {
		b.WriteString("  " + shared.StyleHelp.Render("No matching files found") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(shared.StyleHelp.Render("  \u2191\u2193 navigate \u2022 enter select \u2022 esc back") + "\n")

	return b.String()
}

func (m Model) hasCloneVolumes() bool {
	return m.cloneConfig != nil && len(m.cloneConfig.VolumeIDs) > 0
}

// submit validates the form and creates the server(s).
//
// Naming: a populated Name Template takes precedence over Server Name. Each
// instance is then created with its own request named from the template
// (index 0..count-1), exactly as previewed, and the typed name is ignored.
// Without a template, Server Name is required and count > 1 uses a single
// Nova multi-create request (min_count = max_count = count).
func (m Model) submit() (Model, tea.Cmd) {
	if m.submitting {
		return m, nil
	}
	name := strings.TrimSpace(m.nameInput.Value())
	tmpl := strings.TrimSpace(m.templateInput.Value())
	if name == "" && tmpl == "" {
		m.err = "Server name is required"
		return m, nil
	}
	if m.selectedImage < 0 || m.selectedImage >= len(m.images) {
		m.err = "Image is required"
		return m, nil
	}
	if m.selectedFlavor < 0 || m.selectedFlavor >= len(m.flavors) {
		m.err = "Flavor is required"
		return m, nil
	}

	count := 1
	if !m.cloneMode {
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
	}

	base := servers.CreateOpts{
		ImageRef:  m.images[m.selectedImage].ID,
		FlavorRef: m.flavors[m.selectedFlavor].ID,
		UserData:  m.userData,
	}

	if m.selectedNetwork >= 0 && m.selectedNetwork < len(m.networks) {
		base.Networks = []servers.Network{
			{UUID: m.networks[m.selectedNetwork].ID},
		}
	}

	if len(m.selectedSecGroups) > 0 {
		var sgNames []string
		for _, idx := range m.sortedSecGroupIndices() {
			sgNames = append(sgNames, m.secGroups[idx].Name)
		}
		base.SecurityGroups = sgNames
	}

	keyName := ""
	if m.selectedKeypair >= 0 && m.selectedKeypair < len(m.keypairs) {
		keyName = m.keypairs[m.selectedKeypair].Name
	}
	build := func(name string, multi int) servers.CreateOptsBuilder {
		opts := base
		opts.Name = name
		if multi > 1 {
			opts.Min = multi
			opts.Max = multi
		}
		if keyName != "" {
			return keypairs.CreateOptsExt{CreateOptsBuilder: opts, KeyName: keyName}
		}
		return opts
	}

	m.submitting = true
	m.err = ""
	client := m.computeClient
	action := audit.ActionCreate
	if m.cloneMode {
		action = audit.ActionClone
	}

	if tmpl != "" {
		names := make([]string, count)
		for i := range names {
			names[i] = expandNameTemplate(tmpl, i)
		}
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return createNamed(client, names, build, action)
		})
	}

	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		shared.Debugf("[servercreate] creating server %q (count %d)", name, count)
		srv, err := compute.CreateServerWithOpts(ctx, client, build(name, count))
		id := ""
		if srv != nil {
			id = srv.ID
		}
		rec := shared.NewAudit(action, "server", id, name, err).
			WithDetails(map[string]string{"count": strconv.Itoa(count)})
		if err != nil {
			shared.Debugf("[servercreate] error creating server %q: %v", name, err)
			return serverCreateErrMsg{Audit: rec, err: err}
		}
		shared.Debugf("[servercreate] created server %q (id=%s)", name, srv.ID)
		return serverCreatedMsg{Audit: rec, server: srv}
	})
}

// createNamed creates one server per name, stopping at the first failure.
// A failure after some servers were created reports the created names and
// IDs so the user can see (and clean up) what already exists.
func createNamed(client *gophercloud.ServiceClient, names []string, build func(string, int) servers.CreateOptsBuilder, action audit.ActionType) tea.Msg {
	var first *compute.Server
	var created []string
	var recs []shared.AuditRecord
	for _, n := range names {
		shared.Debugf("[servercreate] creating server %q", n)
		ctx, cancel := shared.RequestCtx()
		srv, err := compute.CreateServerWithOpts(ctx, client, build(n, 1))
		cancel()
		rec := shared.AuditRecord{Action: action, ResourceType: "server", ResourceName: n, Err: err}
		if srv != nil {
			rec.ResourceID = srv.ID
		}
		recs = append(recs, rec)
		if err != nil {
			shared.Debugf("[servercreate] error creating server %q: %v", n, err)
			if len(created) == 0 {
				return serverCreateErrMsg{Audit: shared.Audits(recs...), err: err}
			}
			return serverCreateErrMsg{Audit: shared.Audits(recs...), err: fmt.Errorf("created %d of %d servers (%s); %s failed: %w",
				len(created), len(names), strings.Join(created, ", "), n, err)}
		}
		shared.Debugf("[servercreate] created server %q (id=%s)", n, srv.ID)
		if first == nil {
			first = srv
		}
		created = append(created, fmt.Sprintf("%s (%s)", n, srv.ID))
	}
	return serverCreatedMsg{Audit: shared.Audits(recs...), server: first}
}

// View renders the create form.
func (m Model) View() string {
	if m.udPickerOpen {
		return m.renderUDPicker()
	}

	var b strings.Builder

	titleText := "Create Server"
	if m.cloneMode {
		titleText = "Clone Server"
	}
	title := shared.StyleTitle.Render(titleText)
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

	type fieldDef struct {
		label   string
		value   string
		focused bool
		isInput bool
	}
	fields := []fieldDef{
		{"Server Name", m.nameInput.View(), m.focusField == fieldName, true},
		{"Name Template", m.templateInput.View(), m.focusField == fieldNameTemplate, true},
		{"Image", m.selectionDisplay(fieldImage), m.focusField == fieldImage, false},
		{"Flavor", m.selectionDisplay(fieldFlavor), m.focusField == fieldFlavor, false},
		{"Network", m.selectionDisplay(fieldNetwork), m.focusField == fieldNetwork, false},
		{"Key Pair", m.selectionDisplay(fieldKeypair), m.focusField == fieldKeypair, false},
		{"Security Groups", m.selectionDisplay(fieldSecGroup), m.focusField == fieldSecGroup, false},
		{"User Data", m.userDataDisplay(), m.focusField == fieldUserData, false},
	}
	if m.cloneMode && m.hasCloneVolumes() {
		check := "[ ]"
		if m.cloneVolumes {
			check = "[x]"
		}
		fields = append(fields, fieldDef{"Clone Volumes", fmt.Sprintf("%s clone all %d attached volumes", check, len(m.cloneConfig.VolumeIDs)), m.focusField == fieldCount, false})
	} else if !m.cloneMode {
		fields = append(fields, fieldDef{"Count", m.countInput.View(), m.focusField == fieldCount, true})
	}

	for i, f := range fields {
		if m.cloneMode && f.label == "Name Template" {
			continue
		}
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

		// Show inline picker if open for this field (field indices map 1:1 with field constants)
		if m.pickerOpen && m.pickerField == i {
			b.WriteString(m.renderPicker())
		}
	}

	b.WriteString("\n")

	// Preview generated names when count > 1 and template is set
	if !m.cloneMode {
		countStr := strings.TrimSpace(m.countInput.Value())
		tmpl := strings.TrimSpace(m.templateInput.Value())
		count := 1
		if countStr != "" {
			c, err := strconv.Atoi(countStr)
			if err == nil && c > 1 && c <= 100 {
				count = c
			}
		}
		if tmpl != "" {
			names := previewNames(tmpl, count, 5)
			if count <= 1 {
				names = []string{expandNameTemplate(tmpl, 0)}
			}
			line := "  Preview: " + strings.Join(names, ", ")
			if count > len(names) {
				line += ", …"
			}
			if strings.TrimSpace(m.nameInput.Value()) != "" {
				line += " (template overrides Server Name)"
			}
			b.WriteString(shared.StyleHelp.Render(line) + "\n")
		}
	}

	// Buttons
	submitStyle := shared.StyleButton
	cancelStyle := shared.StyleButton
	if m.focusField == fieldSubmit {
		submitStyle = shared.StyleButtonSubmit
	}
	if m.focusField == fieldCancel {
		cancelStyle = shared.StyleButtonCancel
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
			for _, idx := range m.sortedSecGroupIndices() {
				names = append(names, m.secGroups[idx].Name)
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

func (m *Model) applyClonePreFill() {
	cfg := m.cloneConfig

	// Match image by ID
	for i, img := range m.images {
		if img.ID == cfg.ImageID {
			m.selectedImage = i
			break
		}
	}

	// Match flavor by ID, fallback to name
	for i, f := range m.flavors {
		if f.ID == cfg.FlavorID {
			m.selectedFlavor = i
			break
		}
		if f.Name == cfg.FlavorName {
			m.selectedFlavor = i
		}
	}

	// The form takes one network. Server.Networks is a map with no
	// meaningful order, so pick the first of the source server's networks,
	// in name order, that still exists; the choice is stable across runs.
	netNames := make([]string, 0, len(cfg.NetworkNames))
	for name := range cfg.NetworkNames {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)
matchNetwork:
	for _, netName := range netNames {
		for i, n := range m.networks {
			if n.Name == netName {
				m.selectedNetwork = i
				break matchNetwork
			}
		}
	}

	// Match keypair by name
	for i, kp := range m.keypairs {
		if kp.Name == cfg.KeyName {
			m.selectedKeypair = i
			break
		}
	}

	// Match security groups by name
	for i, sg := range m.secGroups {
		for _, name := range cfg.SecGroupNames {
			if sg.Name == name {
				m.selectedSecGroups[i] = true
			}
		}
	}
}

func (m Model) fetchImages() tea.Cmd {
	client := m.imageClient
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		images, err := img.ListImages(ctx, client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return imagesLoadedMsg{images: images}
	}
}

func (m Model) fetchFlavors() tea.Cmd {
	client := m.computeClient
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		flavors, err := compute.ListFlavors(ctx, client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return flavorsLoadedMsg{flavors: flavors}
	}
}

func (m Model) fetchNetworks() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		nets, err := network.ListNetworks(ctx, client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return networksLoadedMsg{networks: nets}
	}
}

func (m Model) fetchKeypairs() tea.Cmd {
	client := m.computeClient
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		kps, err := compute.ListKeyPairs(ctx, client)
		if err != nil {
			return fetchErrMsg{err: err}
		}
		return keypairsLoadedMsg{keypairs: kps}
	}
}

func (m Model) fetchSecGroups() tea.Cmd {
	client := m.networkClient
	return func() tea.Msg {
		ctx, cancel := shared.RequestCtx()
		defer cancel()
		sgs, err := network.ListSecurityGroups(ctx, client)
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

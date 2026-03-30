package keypaircreate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/larkly/lazystack/internal/compute"
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
	fieldType      = 1
	fieldPublicKey = 2
	fieldBrowse    = 3
	fieldSubmit    = 4
	fieldCancel    = 5
	numFields      = 6
)

type keyTypeOption struct {
	label     string
	algorithm string
	keySize   int
}

var keyTypes = []keyTypeOption{
	{label: "RSA 2048", algorithm: "rsa", keySize: 2048},
	{label: "RSA 4096", algorithm: "rsa", keySize: 4096},
	{label: "ED25519", algorithm: "ed25519"},
}

type keypairCreatedMsg struct{ kp *compute.KeyPairFull }
type keypairCreateErrMsg struct{ err error }

// pubKeyFile is a discovered public key file.
type pubKeyFile struct {
	path string
	name string
}

// Model is the keypair create/import form.
type Model struct {
	computeClient *gophercloud.ServiceClient

	nameInput      textinput.Model
	publicKeyInput textinput.Model
	selectedType   int

	// File picker
	filePicker     bool
	pubKeyFiles    []pubKeyFile
	filePickerIdx  int

	focusField int
	submitting bool
	spinner    spinner.Model
	err        string
	width      int
	height     int

	// After generation, show private key
	showPrivateKey bool
	privateKey     string
	publicKey      string
	keypairName    string
	privateScroll  int
	savedPath      string
	saveErr        string

	// Save path input
	showSaveInput bool
	savePathInput textinput.Model
}

// New creates a keypair create form.
func New(computeClient *gophercloud.ServiceClient) Model {
	ni := textinput.New()
	ni.Prompt = ""
	ni.Placeholder = "keypair name"
	ni.CharLimit = 255
	ni.SetWidth(40)
	ni.Focus()

	pk := textinput.New()
	pk.Prompt = ""
	pk.Placeholder = "paste public key or press enter on [Browse] below"
	pk.CharLimit = 4096
	pk.SetWidth(60)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		computeClient:  computeClient,
		nameInput:      ni,
		publicKeyInput: pk,
		spinner:        s,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case keypairCreatedMsg:
		m.submitting = false
		if msg.kp.PrivateKey != "" {
			m.showPrivateKey = true
			m.privateKey = msg.kp.PrivateKey
			m.publicKey = msg.kp.PublicKey
			m.keypairName = msg.kp.Name
			return m, nil
		}
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "keypairlist"}
		}
	case keypairCreateErrMsg:
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
		if m.showPrivateKey {
			return m.handlePrivateKeyView(msg)
		}
		if m.submitting {
			return m, nil
		}
		if m.filePicker {
			return m.handleFilePicker(msg)
		}
		return m.updateForm(msg)
	}
	return m, nil
}

func (m Model) handlePrivateKeyView(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.showSaveInput {
		return m.handleSaveInput(msg)
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		if m.savedPath != "" {
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "keypairlist"}
			}
		}
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "keypairlist"}
		}
	case key.Matches(msg, shared.Keys.Enter):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "keypairlist"}
		}
	case key.Matches(msg, shared.Keys.Down):
		m.privateScroll++
		lines := strings.Count(m.privateKey, "\n") + 1
		maxScroll := lines - (m.height - 10)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.privateScroll > maxScroll {
			m.privateScroll = maxScroll
		}
	case key.Matches(msg, shared.Keys.Up):
		if m.privateScroll > 0 {
			m.privateScroll--
		}
	default:
		if msg.String() == "s" && m.savedPath == "" {
			m.showSaveInput = true
			home, _ := os.UserHomeDir()
			defaultPath := filepath.Join(home, ".ssh", m.keypairName)
			m.savePathInput = textinput.New()
			m.savePathInput.Prompt = "Save to: "
			m.savePathInput.SetValue(defaultPath)
			m.savePathInput.CharLimit = 256
			m.savePathInput.SetWidth(60)
			m.savePathInput.Focus()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleSaveInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.showSaveInput = false
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		path := m.savePathInput.Value()
		if path == "" {
			m.showSaveInput = false
			return m, nil
		}
		// Expand ~ if present
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}
		// Save private key with 0600 permissions
		if err := os.WriteFile(path, []byte(m.privateKey), 0600); err != nil {
			m.saveErr = err.Error()
			m.showSaveInput = false
			return m, nil
		}
		// Save public key alongside
		if m.publicKey != "" {
			_ = os.WriteFile(path+".pub", []byte(m.publicKey), 0644)
		}
		m.savedPath = path
		m.showSaveInput = false
		return m, nil
	default:
		var cmd tea.Cmd
		m.savePathInput, cmd = m.savePathInput.Update(msg)
		return m, cmd
	}
}

func (m Model) handleFilePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.filePicker = false
		return m, nil
	case key.Matches(msg, shared.Keys.Up):
		if m.filePickerIdx > 0 {
			m.filePickerIdx--
		}
	case key.Matches(msg, shared.Keys.Down):
		if m.filePickerIdx < len(m.pubKeyFiles)-1 {
			m.filePickerIdx++
		}
	case key.Matches(msg, shared.Keys.Enter):
		if len(m.pubKeyFiles) > 0 && m.filePickerIdx < len(m.pubKeyFiles) {
			f := m.pubKeyFiles[m.filePickerIdx]
			data, err := os.ReadFile(f.path)
			if err != nil {
				m.err = fmt.Sprintf("reading %s: %v", f.path, err)
				m.filePicker = false
				return m, nil
			}
			m.publicKeyInput.SetValue(strings.TrimSpace(string(data)))
			m.filePicker = false
			m.focusField = fieldSubmit
			m.updateFocus()
		}
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	return m.focusField == fieldName || m.focusField == fieldPublicKey
}

func (m Model) updateForm(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Route to text input first — only intercept navigation keys
	if m.isTextInput() {
		switch {
		case key.Matches(msg, shared.Keys.Back):
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "keypairlist"}
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
			case fieldPublicKey:
				var cmd tea.Cmd
				m.publicKeyInput, cmd = m.publicKeyInput.Update(msg)
				return m, cmd
			}
		}
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		return m, func() tea.Msg {
			return shared.ViewChangeMsg{View: "keypairlist"}
		}

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
		case fieldType:
			m.selectedType = (m.selectedType + 1) % len(keyTypes)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			m.updateFocus()
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			m.updateFocus()
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Left):
		switch m.focusField {
		case fieldType:
			m.selectedType = (m.selectedType - 1 + len(keyTypes)) % len(keyTypes)
			return m, nil
		case fieldSubmit:
			m.focusField = fieldCancel
			m.updateFocus()
			return m, nil
		case fieldCancel:
			m.focusField = fieldSubmit
			m.updateFocus()
			return m, nil
		}

	case key.Matches(msg, shared.Keys.Enter):
		switch m.focusField {
		case fieldName, fieldType, fieldPublicKey:
			m.focusField++
			m.updateFocus()
			return m, nil
		case fieldBrowse:
			m.openFilePicker()
			return m, nil
		case fieldSubmit:
			return m.submit()
		case fieldCancel:
			return m, func() tea.Msg {
				return shared.ViewChangeMsg{View: "keypairlist"}
			}
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
	case fieldPublicKey:
		var cmd tea.Cmd
		m.publicKeyInput, cmd = m.publicKeyInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) openFilePicker() {
	m.pubKeyFiles = nil
	m.filePickerIdx = 0

	home, err := os.UserHomeDir()
	if err != nil {
		m.err = "cannot determine home directory"
		return
	}

	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		m.err = fmt.Sprintf("cannot read %s: %v", sshDir, err)
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".pub") {
			m.pubKeyFiles = append(m.pubKeyFiles, pubKeyFile{
				path: filepath.Join(sshDir, name),
				name: name,
			})
		}
	}

	if len(m.pubKeyFiles) == 0 {
		m.err = "no .pub files found in " + sshDir
		return
	}

	m.filePicker = true
}

func (m *Model) updateFocus() {
	if m.focusField == fieldName {
		m.nameInput.Focus()
	} else {
		m.nameInput.Blur()
	}
	if m.focusField == fieldPublicKey {
		m.publicKeyInput.Focus()
	} else {
		m.publicKeyInput.Blur()
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = "Key pair name is required"
		return m, nil
	}

	publicKey := strings.TrimSpace(m.publicKeyInput.Value())
	kt := keyTypes[m.selectedType]

	m.submitting = true
	m.err = ""
	client := m.computeClient

	if publicKey != "" {
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			shared.Debugf("[keypaircreate] importing keypair %q", name)
			kp, err := compute.ImportKeyPair(context.Background(), client, name, publicKey)
			if err != nil {
				shared.Debugf("[keypaircreate] error importing keypair %q: %v", name, err)
				return keypairCreateErrMsg{err: err}
			}
			shared.Debugf("[keypaircreate] imported keypair %q", name)
			return keypairCreatedMsg{kp: kp}
		})
	}

	algo := kt.algorithm
	keySize := kt.keySize
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		shared.Debugf("[keypaircreate] generating keypair %q (algo=%s)", name, algo)
		kp, err := compute.GenerateAndImportKeyPair(context.Background(), client, name, algo, keySize)
		if err != nil {
			shared.Debugf("[keypaircreate] error generating keypair %q: %v", name, err)
			return keypairCreateErrMsg{err: err}
		}
		shared.Debugf("[keypaircreate] generated keypair %q", name)
		return keypairCreatedMsg{kp: kp}
	})
}

// View renders the form.
func (m Model) View() string {
	if m.showPrivateKey {
		return m.renderPrivateKey()
	}

	var b strings.Builder

	title := shared.StyleTitle.Render("Create / Import Key Pair")
	if m.submitting {
		title += " " + m.spinner.View() + shared.StyleHelp.Render(" creating...")
	}
	b.WriteString(title + "\n\n")

	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  ⚠ "+m.err) + "\n\n")
	}

	type field struct {
		label   string
		value   string
		focused bool
	}
	fields := []field{
		{"Name", m.nameInput.View(), m.focusField == fieldName},
		{"Type", m.cycleDisplay(keyTypes, m.selectedType), m.focusField == fieldType},
		{"Public Key", m.publicKeyInput.View(), m.focusField == fieldPublicKey},
	}

	for _, f := range fields {
		cursor := "  "
		if f.focused {
			cursor = "▸ "
		}
		label := shared.StyleLabel.Render(f.label)
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if f.focused {
			style = style.Foreground(shared.ColorHighlight)
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, label, style.Render(f.value)))
	}

	// Browse button
	browseStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	browseCursor := "  "
	if m.focusField == fieldBrowse {
		browseStyle = browseStyle.Foreground(shared.ColorHighlight).Bold(true)
		browseCursor = "▸ "
	}
	b.WriteString(fmt.Sprintf("%s%s %s\n",
		browseCursor,
		shared.StyleLabel.Render(""),
		browseStyle.Render("[Browse ~/.ssh/ for public key]")))

	// File picker inline
	if m.filePicker {
		b.WriteString(m.renderFilePicker())
	}

	b.WriteString(shared.StyleHelp.Render("  (leave public key empty to generate a new key pair)") + "\n")

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
	b.WriteString(shared.StyleHelp.Render("  tab/↑↓ navigate • ←→ cycle type • ctrl+s submit • esc cancel") + "\n")

	return b.String()
}

func (m Model) renderFilePicker() string {
	var b strings.Builder
	maxShow := 8
	if len(m.pubKeyFiles) < maxShow {
		maxShow = len(m.pubKeyFiles)
	}
	start := 0
	if m.filePickerIdx >= maxShow {
		start = m.filePickerIdx - maxShow + 1
	}
	end := start + maxShow
	if end > len(m.pubKeyFiles) {
		end = len(m.pubKeyFiles)
	}
	for i := start; i < end; i++ {
		f := m.pubKeyFiles[i]
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.filePickerIdx {
			cursor = "▸ "
			style = style.Foreground(shared.ColorHighlight).Bold(true)
		}
		b.WriteString(fmt.Sprintf("      %s%s\n", cursor, style.Render(f.name)))
	}
	b.WriteString(shared.StyleHelp.Render("      ↑↓ navigate • enter select • esc cancel") + "\n")
	return b.String()
}

func (m Model) cycleDisplay(options []keyTypeOption, selected int) string {
	var parts []string
	for i, opt := range options {
		if i == selected {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(shared.ColorHighlight).Render("● "+opt.label))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("○ "+opt.label))
		}
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderPrivateKey() string {
	var b strings.Builder

	title := shared.StyleTitle.Render("Generated Private Key")
	b.WriteString(title + "\n\n")

	if m.savedPath != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorSuccess).Render(
			fmt.Sprintf("  ✓ Saved to %s", m.savedPath)) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorSuccess).Render(
			fmt.Sprintf("  ✓ Public key saved to %s.pub", m.savedPath)) + "\n\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorWarning).Render(
			"  ⚠ This private key will not be shown again. Press s to save to file.") + "\n\n")
	}

	if m.saveErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render(
			"  ⚠ Save error: "+m.saveErr) + "\n\n")
	}

	if m.showSaveInput {
		b.WriteString("  " + m.savePathInput.View() + "\n\n")
	}

	lines := strings.Split(m.privateKey, "\n")
	viewHeight := m.height - 12
	if viewHeight < 5 {
		viewHeight = 5
	}
	end := m.privateScroll + viewHeight
	if end > len(lines) {
		end = len(lines)
	}
	start := m.privateScroll
	if start > len(lines) {
		start = len(lines)
	}

	keyStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	for _, line := range lines[start:end] {
		b.WriteString("  " + keyStyle.Render(line) + "\n")
	}

	hint := "  ↑↓ scroll • s save to file • esc/enter to continue"
	if m.savedPath != "" {
		hint = "  ↑↓ scroll • esc/enter to continue"
	}
	if m.showSaveInput {
		hint = "  enter confirm path • esc cancel"
	}
	b.WriteString("\n" + shared.StyleHelp.Render(hint) + "\n")

	return b.String()
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Hints returns key hints for the status bar.
func (m Model) Hints() string {
	if m.showPrivateKey {
		if m.showSaveInput {
			return "enter confirm path • esc cancel"
		}
		if m.savedPath != "" {
			return "↑↓ scroll • esc/enter to continue"
		}
		return "↑↓ scroll • s save to file • esc/enter to continue"
	}
	if m.filePicker {
		return "↑↓ navigate • enter select • esc cancel"
	}
	return "tab/shift+tab fields • ←→ cycle type • ctrl+s submit • esc cancel"
}

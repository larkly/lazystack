package sshprompt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	fieldHost            = 0
	fieldUser            = 1
	fieldKeyPath         = 2
	fieldDebug           = 3
	fieldIgnoreHostKeys  = 4
	numFields            = 5
)

// SSHConnectMsg is emitted when the user confirms the SSH username.
type SSHConnectMsg struct {
	User           string
	IP             string
	KeyPath        string
	Debug          bool
	IgnoreHostKeys bool
}

// ipOption holds an IP address and its type label.
type ipOption struct {
	IP    string
	Label string // "floating", "ipv6", "ipv4"
}

// Model is the SSH username prompt overlay modal.
type Model struct {
	Active     bool
	serverName string
	ips        []ipOption
	ipIndex    int
	userInput  textinput.Model
	keyInput   textinput.Model
	debug      bool
	ignoreHostKeys bool
	focusField int
	err        string
	width      int
	height     int

	// IP picker state.
	ipPickerOpen   bool
	ipPickerCursor int

	// Key file picker state.
	pickerOpen   bool
	pickerCursor int
	pickerFilter textinput.Model
	pickerFiles  []string // full paths
}

// New creates an SSH prompt modal for the given server.
func New(serverName string, floatingIPs, ipv6, ipv4 []string, keyPath string, ignoreHostKeysDefault bool) Model {
	// Build IP options list with labels
	var ips []ipOption
	for _, ip := range floatingIPs {
		ips = append(ips, ipOption{IP: ip, Label: "floating"})
	}
	for _, ip := range ipv6 {
		ips = append(ips, ipOption{IP: ip, Label: "ipv6"})
	}
	for _, ip := range ipv4 {
		ips = append(ips, ipOption{IP: ip, Label: "ipv4"})
	}

	ui := textinput.New()
	ui.Prompt = ""
	ui.Placeholder = "username"
	ui.CharLimit = 64
	ui.SetWidth(30)
	ui.Blur()

	ki := textinput.New()
	ki.Prompt = ""
	ki.Placeholder = "~/.ssh/id_rsa"
	ki.CharLimit = 256
	ki.SetWidth(30)
	ki.SetValue(keyPath)
	ki.Blur()

	pf := textinput.New()
	pf.Prompt = "  / "
	pf.Placeholder = "filter"
	pf.CharLimit = 64
	pf.SetWidth(26)

	return Model{
		Active:       true,
		serverName:   serverName,
		ips:          ips,
		ipIndex:      0,
		userInput:    ui,
		keyInput:     ki,
		ignoreHostKeys: ignoreHostKeysDefault,
		pickerFilter: pf,
		pickerFiles:  listSSHKeys(),
		focusField:   fieldHost,
	}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	shared.Debugf("[sshprompt] Init() server=%q ips=%d", m.serverName, len(m.ips))
	return textinput.Blink
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	// Forward non-key messages (e.g. blink) to active text input.
	if m.pickerOpen {
		var cmd tea.Cmd
		m.pickerFilter, cmd = m.pickerFilter.Update(msg)
		return m, cmd
	}
	if m.isTextInput() {
		var cmd tea.Cmd
		if m.focusField == fieldUser {
			m.userInput, cmd = m.userInput.Update(msg)
		} else {
			m.keyInput, cmd = m.keyInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) isTextInput() bool {
	return m.focusField == fieldUser || m.focusField == fieldKeyPath
}

func (m *Model) updateFocus() {
	if m.focusField == fieldUser {
		m.userInput.Focus()
	} else {
		m.userInput.Blur()
	}
	if m.focusField == fieldKeyPath && !m.pickerOpen {
		m.keyInput.Focus()
	} else {
		m.keyInput.Blur()
	}
}

func (m *Model) advanceFocus() {
	m.focusField = (m.focusField + 1) % numFields
	m.updateFocus()
}

func (m *Model) retreatFocus() {
	m.focusField = (m.focusField - 1 + numFields) % numFields
	m.updateFocus()
}

func (m *Model) openPicker() {
	m.pickerOpen = true
	m.pickerCursor = 0
	m.pickerFilter.SetValue("")
	m.pickerFilter.Focus()
	m.keyInput.Blur()
}

func (m *Model) closePicker() {
	m.pickerOpen = false
	m.pickerFilter.Blur()
	m.updateFocus()
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.ipPickerOpen {
		return m.updateIPPicker(msg)
	}
	if m.pickerOpen {
		return m.updatePicker(msg)
	}

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Active = false
		return m, nil
	case key.Matches(msg, shared.Keys.Enter):
		if m.focusField == fieldHost && len(m.ips) > 1 {
			m.ipPickerOpen = true
			m.ipPickerCursor = m.ipIndex
			return m, nil
		}
		if m.focusField == fieldKeyPath {
			m.openPicker()
			return m, nil
		}
		return m.submit()
	case key.Matches(msg, shared.Keys.Tab):
		m.advanceFocus()
		return m, nil
	case key.Matches(msg, shared.Keys.ShiftTab):
		m.retreatFocus()
		return m, nil
	}

	// Host field: left/right to cycle IPs
	if m.focusField == fieldHost {
		switch {
		case key.Matches(msg, shared.Keys.Left) || key.Matches(msg, shared.Keys.Right):
			if len(m.ips) > 1 {
				if key.Matches(msg, shared.Keys.Right) {
					m.ipIndex = (m.ipIndex + 1) % len(m.ips)
				} else {
					m.ipIndex = (m.ipIndex - 1 + len(m.ips)) % len(m.ips)
				}
			}
			return m, nil
		}
		return m, nil
	}

	if m.isTextInput() {
		var cmd tea.Cmd
		if m.focusField == fieldUser {
			m.userInput, cmd = m.userInput.Update(msg)
		} else {
			m.keyInput, cmd = m.keyInput.Update(msg)
		}
		return m, cmd
	}

	// On checkbox fields.
	if key.Matches(msg, shared.Keys.Select) {
		if m.focusField == fieldIgnoreHostKeys {
			m.ignoreHostKeys = !m.ignoreHostKeys
			return m, nil
		}
		m.debug = !m.debug
	}
	return m, nil
}

func (m Model) updateIPPicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.ipPickerOpen = false
		return m, nil
	case "enter":
		if m.ipPickerCursor < len(m.ips) {
			m.ipIndex = m.ipPickerCursor
		}
		m.ipPickerOpen = false
		m.advanceFocus()
		return m, nil
	case "up", "k":
		if m.ipPickerCursor > 0 {
			m.ipPickerCursor--
		}
		return m, nil
	case "down", "j":
		if m.ipPickerCursor < len(m.ips)-1 {
			m.ipPickerCursor++
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updatePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	filtered := m.filteredPickerFiles()

	switch msg.String() {
	case "esc":
		m.closePicker()
		return m, nil
	case "enter":
		if len(filtered) > 0 && m.pickerCursor < len(filtered) {
			m.keyInput.SetValue(filtered[m.pickerCursor])
		}
		m.closePicker()
		m.advanceFocus()
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

func (m Model) filteredPickerFiles() []string {
	q := strings.ToLower(m.pickerFilter.Value())
	if q == "" {
		return m.pickerFiles
	}
	var out []string
	for _, f := range m.pickerFiles {
		if strings.Contains(strings.ToLower(filepath.Base(f)), q) {
			out = append(out, f)
		}
	}
	return out
}

func (m Model) selectedIP() string {
	if len(m.ips) == 0 {
		return ""
	}
	return m.ips[m.ipIndex].IP
}

func (m Model) submit() (Model, tea.Cmd) {
	user := strings.TrimSpace(m.userInput.Value())
	if user == "" {
		m.err = "Username cannot be empty"
		return m, nil
	}
	m.Active = false
	ip := m.selectedIP()
	shared.Debugf("[sshprompt] submit user=%q ip=%s", user, ip)
	return m, func() tea.Msg {
		return SSHConnectMsg{
			User:           user,
			IP:             ip,
			KeyPath:        strings.TrimSpace(m.keyInput.Value()),
			Debug:          m.debug,
			IgnoreHostKeys: m.ignoreHostKeys,
		}
	}
}

// View renders the SSH prompt overlay.
func (m Model) View() string {
	title := shared.StyleModalTitle.Render("SSH into Server")

	var body strings.Builder

	if m.err != "" {
		body.WriteString(lipgloss.NewStyle().Foreground(shared.ColorError).Render("  "+m.err) + "\n\n")
	}

	muted := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	label := lipgloss.NewStyle().Foreground(shared.ColorSecondary)
	cursor := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true)

	body.WriteString("  " + label.Render("Server") + "  " + m.serverName + "\n")

	// Host field with IP selector
	prefix := "  "
	if m.focusField == fieldHost {
		prefix = cursor.Render("> ")
	}
	if len(m.ips) > 0 {
		opt := m.ips[m.ipIndex]
		ipDisplay := opt.IP + " " + muted.Render(opt.Label)
		if len(m.ips) > 1 {
			counter := muted.Render(fmt.Sprintf("[%d/%d]", m.ipIndex+1, len(m.ips)))
			ipDisplay = ipDisplay + " " + counter
		}
		body.WriteString(prefix + label.Render("Host  ") + "  " + ipDisplay + "\n")

		// Inline IP picker
		if m.ipPickerOpen {
			body.WriteString(m.renderIPPicker())
		}
	}
	body.WriteString("\n")

	// User field
	prefix = "  "
	if m.focusField == fieldUser {
		prefix = cursor.Render("> ")
	}
	body.WriteString(prefix + label.Render("User  ") + "  " + m.userInput.View() + "\n")

	// Key field
	prefix = "  "
	if m.focusField == fieldKeyPath {
		prefix = cursor.Render("> ")
	}
	keyDisplay := m.keyInput.View()
	if m.keyInput.Value() == "" && !m.keyInput.Focused() {
		keyDisplay = muted.Render("(default)")
	}
	body.WriteString(prefix + label.Render("Key   ") + "  " + keyDisplay + "\n")

	// Inline picker
	if m.pickerOpen {
		body.WriteString(m.renderPicker())
	}

	// Debug checkbox
	prefix = "  "
	if m.focusField == fieldDebug {
		prefix = cursor.Render("> ")
	}
	check := "[ ]"
	if m.debug {
		check = "[x]"
	}
	body.WriteString(fmt.Sprintf("%s%s %s\n", prefix, check, muted.Render("verbose mode (-v)")))

	prefix = "  "
	if m.focusField == fieldIgnoreHostKeys {
		prefix = cursor.Render("> ")
	}
	check = "[ ]"
	if m.ignoreHostKeys {
		check = "[x]"
	}
	body.WriteString(fmt.Sprintf("%s%s %s\n\n", prefix, check, muted.Render("ignore host keys (unsafe)")))

	help := "tab: next  space: toggle  enter: connect  esc: cancel"
	if m.ipPickerOpen {
		help = "\u2191/\u2193: select  enter: confirm  esc: close"
	} else if m.focusField == fieldHost && len(m.ips) > 1 {
		help = "\u2190/\u2192: change IP  enter: pick from list  tab: next  esc: cancel"
	} else if m.focusField == fieldKeyPath && !m.pickerOpen {
		help = "enter: browse keys  tab: next  esc: cancel"
	}
	body.WriteString(shared.StyleHelp.Render("  " + help))

	content := title + "\n\n" + body.String()
	return m.renderModal(content)
}

func (m Model) renderIPPicker() string {
	var b strings.Builder
	for i, opt := range m.ips {
		cur := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.ipPickerCursor {
			cur = "\u25b8 "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		labelStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
		b.WriteString(fmt.Sprintf("      %s%s %s\n", cur, style.Render(opt.IP), labelStyle.Render(opt.Label)))
	}
	return b.String()
}

func (m Model) renderPicker() string {
	var b strings.Builder
	filtered := m.filteredPickerFiles()

	b.WriteString("    " + m.pickerFilter.View() + "\n")

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
		name := filepath.Base(filtered[i])
		cur := "  "
		style := lipgloss.NewStyle().Foreground(shared.ColorFg)
		if i == m.pickerCursor {
			cur = "\u25b8 "
			style = lipgloss.NewStyle().Foreground(shared.ColorHighlight).Bold(true)
		}
		b.WriteString(fmt.Sprintf("      %s%s\n", cur, style.Render(name)))
	}

	if len(filtered) == 0 {
		b.WriteString("      " + lipgloss.NewStyle().Foreground(shared.ColorMuted).Render("no keys found") + "\n")
	}

	return b.String()
}

func (m Model) renderModal(content string) string {
	modalWidth := 64
	if m.width > 0 && m.width < 72 {
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

// listSSHKeys returns private key file paths from ~/.ssh/, sorted by name.
func listSSHKeys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}

	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip public keys, known_hosts, config, and other non-key files.
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "known_hosts.old" ||
			name == "config" ||
			name == "authorized_keys" {
			continue
		}
		keys = append(keys, filepath.Join(sshDir, name))
	}
	sort.Strings(keys)
	return keys
}

package servermetadata

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"github.com/larkly/lazystack/internal/shared"
)

type metaOpDoneMsg struct {
	action string
	name   string
}

type metaOpErrMsg struct {
	action string
	name   string
	err    error
}

// Model is the server metadata editor modal.
type Model struct {
	Active      bool
	client      *gophercloud.ServiceClient
	serverID    string
	serverName  string
	width       int
	height      int
	metadata    map[string]string
	cursor      int
	mode        string // "" = viewing, "add" = adding, "edit" = editing
	editKey     string
	keyInput    textinput.Model
	valueInput  textinput.Model
	loading     bool
	submitting  bool
	err         string
	spinner     spinner.Model
}

// New creates a metadata editor modal.
func New(client *gophercloud.ServiceClient, serverID, serverName string, existingMeta map[string]string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ki := textinput.New()
	ki.Prompt = "Key:   "
	ki.Placeholder = "lazystack_ssh_user"
	ki.CharLimit = 255

	vi := textinput.New()
	vi.Prompt = "Value: "
	vi.Placeholder = "ubuntu"
	vi.CharLimit = 255

	meta := make(map[string]string)
	for k, v := range existingMeta {
		meta[k] = v
	}

	return Model{
		Active:     true,
		client:     client,
		serverID:   serverID,
		serverName: serverName,
		metadata:   meta,
		keyInput:   ki,
		valueInput: vi,
		spinner:    s,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// sortedKeys returns sorted metadata keys.
func (m Model) sortedKeys() []string {
	keys := make([]string, 0, len(m.metadata))
	for k := range m.metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case metaOpDoneMsg:
		m.submitting = false
		m.Active = false
		return m, func() tea.Msg {
			return shared.ServerActionMsg{Action: msg.action, Name: msg.name}
		}

	case metaOpErrMsg:
		m.submitting = false
		m.err = fmt.Sprintf("%s %s: %v", msg.action, msg.name, msg.err)
		return m, nil

	case spinner.TickMsg:
		if m.submitting || m.loading {
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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.submitting {
		return m, nil
	}

	if key.Matches(msg, shared.Keys.Back) {
		if m.mode != "" {
			m.mode = ""
			m.err = ""
			return m, nil
		}
		m.Active = false
		return m, nil
	}

	keys := m.sortedKeys()

	switch m.mode {
	case "":
		// Viewing mode
		switch {
		case key.Matches(msg, shared.Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, shared.Keys.Down):
			if m.cursor < len(keys) {
				m.cursor++
			}
		case msg.String() == "a":
			// Add new metadata
			m.mode = "add"
			m.keyInput.SetValue("")
			m.valueInput.SetValue("")
			m.keyInput.Focus()
			m.err = ""
			return m, textinput.Blink
		case msg.String() == "e":
			// Edit selected
			if m.cursor < len(keys) {
				m.mode = "edit"
				m.editKey = keys[m.cursor]
				m.keyInput.SetValue(keys[m.cursor])
				m.valueInput.SetValue(m.metadata[keys[m.cursor]])
				m.keyInput.Focus()
				m.err = ""
				return m, textinput.Blink
			}
		case msg.String() == "d":
			// Delete selected
			if m.cursor < len(keys) {
				return m, m.deleteMetadatum(keys[m.cursor])
			}
		}

	case "add":
		switch {
		case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
			// Switch to value input
			if m.keyInput.Focused() {
				m.keyInput.Blur()
				m.valueInput.Focus()
			} else {
				m.valueInput.Blur()
				m.keyInput.Focus()
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			return m, m.addMetadatum()
		default:
			var cmd tea.Cmd
			if m.keyInput.Focused() {
				m.keyInput, cmd = m.keyInput.Update(msg)
			} else {
				m.valueInput, cmd = m.valueInput.Update(msg)
			}
			return m, cmd
		}

	case "edit":
		switch {
		case key.Matches(msg, shared.Keys.Tab), key.Matches(msg, shared.Keys.Down):
			if m.keyInput.Focused() {
				m.keyInput.Blur()
				m.valueInput.Focus()
			} else {
				m.valueInput.Blur()
				m.keyInput.Focus()
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Enter):
			return m, m.updateMetadatum()
		default:
			var cmd tea.Cmd
			if m.keyInput.Focused() {
				m.keyInput, cmd = m.keyInput.Update(msg)
			} else {
				m.valueInput, cmd = m.valueInput.Update(msg)
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) addMetadatum() tea.Cmd {
	key := strings.TrimSpace(m.keyInput.Value())
	value := strings.TrimSpace(m.valueInput.Value())
	if key == "" {
		m.err = "Key is required"
		return nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	id := m.serverID
	name := m.serverName

	return func() tea.Msg {
		shared.Debugf("[servermetadata] creating metadata %s=%s for server %s", key, value, name)
		r := servers.CreateMetadatum(context.Background(), client, id, servers.MetadatumOpts{key: value})
		if r.Err != nil {
			shared.Debugf("[servermetadata] create metadata failed: %v", r.Err)
			return metaOpErrMsg{action: "Update metadata", name: name, err: r.Err}
		}
		shared.Debugf("[servermetadata] created metadata for %s", name)
		return metaOpDoneMsg{action: "Updated metadata", name: name}
	}
}

func (m Model) updateMetadatum() tea.Cmd {
	newKey := strings.TrimSpace(m.keyInput.Value())
	value := strings.TrimSpace(m.valueInput.Value())
	if newKey == "" {
		m.err = "Key is required"
		return nil
	}

	m.submitting = true
	m.err = ""
	client := m.client
	id := m.serverID
	name := m.serverName
	oldKey := m.editKey

	return func() tea.Msg {
		shared.Debugf("[servermetadata] updating metadata %s=%s for server %s", newKey, value, name)
		// CreateMetadatum handles both create and update (Nova PUT /metadata/{key})
		if oldKey != newKey {
			dr := servers.DeleteMetadatum(context.Background(), client, id, oldKey)
			if dr.Err != nil {
				shared.Debugf("[servermetadata] delete old key failed: %v", dr.Err)
			}
		}
		cr := servers.CreateMetadatum(context.Background(), client, id, servers.MetadatumOpts{newKey: value})
		if cr.Err != nil {
			shared.Debugf("[servermetadata] update metadata failed: %v", cr.Err)
			return metaOpErrMsg{action: "Update metadata", name: name, err: cr.Err}
		}
		shared.Debugf("[servermetadata] updated metadata for %s", name)
		return metaOpDoneMsg{action: "Updated metadata", name: name}
	}
}

func (m Model) deleteMetadatum(key string) tea.Cmd {
	m.submitting = true
	m.err = ""
	client := m.client
	id := m.serverID
	name := m.serverName

	return func() tea.Msg {
		shared.Debugf("[servermetadata] deleting metadata %s from server %s", key, name)
		r := servers.DeleteMetadatum(context.Background(), client, id, key)
		if r.Err != nil {
			shared.Debugf("[servermetadata] delete metadata failed: %v", r.Err)
			return metaOpErrMsg{action: "Delete metadata", name: name, err: r.Err}
		}
		shared.Debugf("[servermetadata] deleted metadata from %s", name)
		return metaOpDoneMsg{action: "Updated metadata", name: name}
	}
}

// View renders the metadata editor.
func (m Model) View() string {
	if !m.Active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(shared.ColorPrimary).
		Bold(true).
		Padding(0, 1)
	title := titleStyle.Render(fmt.Sprintf("Metadata — %s", m.serverName))

	var body string
	switch m.mode {
	case "":
		body = m.renderMetadataList()
	case "add":
		body = m.renderAddForm("Add")
	case "edit":
		body = m.renderAddForm("Edit")
	}

	if m.err != "" {
		body += "\n\n" + lipgloss.NewStyle().
			Foreground(shared.ColorError).
			Render("  Error: "+m.err)
	}

	if m.submitting {
		body += "\n\n  " + m.spinner.View() + " Saving..."
	}

	width := 60
	if m.width < 64 {
		width = m.width - 4
	}

	footer := lipgloss.NewStyle().
		Foreground(shared.ColorMuted).
		Render("  a add • e edit • d delete • ↑↓ navigate • esc back")

	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(shared.ColorPrimary).
		Padding(1, 2).
		Render(title + "\n\n" + body + "\n\n" + footer)
}

func (m Model) renderMetadataList() string {
	keys := m.sortedKeys()

	if len(keys) == 0 {
		return lipgloss.NewStyle().
			Foreground(shared.ColorMuted).
			Render("  No metadata set.\n\n  Press 'a' to add a key-value pair.")
	}

	keyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).
		Bold(true).
		Width(28)
	valueStyle := lipgloss.NewStyle().Foreground(shared.ColorFg)
	cursorMark := lipgloss.NewStyle().Foreground(shared.ColorPrimary).Bold(true).Render("▸ ")

	var lines []string
	for i, k := range keys {
		prefix := "  "
		kStyle := keyStyle
		vStyle := valueStyle
		if i == m.cursor {
			prefix = cursorMark
			kStyle = keyStyle.Copy().Foreground(shared.ColorHighlight).Bold(true)
			vStyle = valueStyle.Copy().Foreground(shared.ColorHighlight).Bold(true)
		}
		val := m.metadata[k]
		if len(val) > 30 {
			val = val[:29] + "…"
		}
		lines = append(lines, prefix+kStyle.Render(k)+vStyle.Render(val))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderAddForm(mode string) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(shared.ColorSecondary).
		Bold(true)

	focusedBorder := lipgloss.NewStyle().BorderForeground(shared.ColorPrimary)
	blurredBorder := lipgloss.NewStyle().BorderForeground(shared.ColorMuted)

	keyBox := focusedBorder
	valBox := blurredBorder
	if !m.keyInput.Focused() {
		keyBox = blurredBorder
		valBox = focusedBorder
	}

	keySection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("  Key:"),
		keyBox.Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(m.keyInput.View()),
	)
	valSection := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("  Value:"),
		valBox.Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(m.valueInput.View()),
	)

	hintStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)
	return lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render(fmt.Sprintf("  %s Metadata", mode)),
		"",
		keySection,
		"",
		valSection,
		"",
		hintStyle.Render("  enter save • tab switch • esc cancel"),
	)
}

// SetSize updates dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

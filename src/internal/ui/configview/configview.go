package configview

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/larkly/lazystack/internal/config"
	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type itemKind int

const (
	kindBool itemKind = iota
	kindNumeric
	kindColor
	kindKeybinding
)

type configItem struct {
	label string
	key   string
	kind  itemKind
	get   func() string
	set   func(string) error
}

type category struct {
	name  string
	items []configItem
}

// Model is the config overlay.
type Model struct {
	Visible bool
	Width   int
	Height  int

	cfg        *config.Config
	categories []category
	cursor     int // flat index across all items
	editing    bool
	textInput  textinput.Model
	keyCapture bool
	errMsg     string
	scroll     int
}

// New creates a new config overlay model.
func New(cfg *config.Config) Model {
	ti := textinput.New()
	ti.CharLimit = 32
	if cfg == nil {
		defaults := config.Defaults()
		cfg = &defaults
	}
	m := Model{
		cfg:       cfg,
		textInput: ti,
	}
	m.buildCategories()
	return m
}

// Cfg returns the config pointer for reading current values.
func (m *Model) Cfg() *config.Config {
	return m.cfg
}

// Open shows the config overlay.
func (m *Model) Open() {
	m.Visible = true
	m.cursor = 0
	m.editing = false
	m.keyCapture = false
	m.errMsg = ""
	m.scroll = 0
	m.buildCategories()
}

func (m *Model) buildCategories() {
	cfg := m.cfg
	m.categories = []category{
		{
			name: "General",
			items: []configItem{
				{label: "Refresh interval (s)", key: "refresh_interval", kind: kindNumeric,
					get: func() string { return strconv.Itoa(cfg.General.RefreshInterval) },
					set: func(v string) error {
						n, err := strconv.Atoi(v)
						if err != nil || n < 1 {
							return fmt.Errorf("must be a positive integer")
						}
						cfg.General.RefreshInterval = n
						return nil
					},
				},
				{label: "Idle timeout (min)", key: "idle_timeout", kind: kindNumeric,
					get: func() string { return strconv.Itoa(cfg.General.IdleTimeout) },
					set: func(v string) error {
						n, err := strconv.Atoi(v)
						if err != nil || n < 0 {
							return fmt.Errorf("must be a non-negative integer")
						}
						cfg.General.IdleTimeout = n
						return nil
					},
				},
				{label: "Plain mode", key: "plain_mode", kind: kindBool,
					get: func() string { return boolStr(cfg.General.PlainMode) },
					set: func(string) error {
						cfg.General.PlainMode = !cfg.General.PlainMode
						return nil
					},
				},
				{label: "Check for updates", key: "check_for_updates", kind: kindBool,
					get: func() string { return boolStr(cfg.General.CheckForUpdates) },
					set: func(string) error {
						cfg.General.CheckForUpdates = !cfg.General.CheckForUpdates
						return nil
					},
				},
				{label: "Always pick cloud", key: "always_pick_cloud", kind: kindBool,
					get: func() string { return boolStr(cfg.General.AlwaysPickCloud) },
					set: func(string) error {
						cfg.General.AlwaysPickCloud = !cfg.General.AlwaysPickCloud
						return nil
					},
				},
				{label: "Ignore SSH host keys", key: "ignore_ssh_host_keys", kind: kindBool,
					get: func() string { return boolStr(cfg.General.IgnoreSSHHostKeys) },
					set: func(string) error {
						cfg.General.IgnoreSSHHostKeys = !cfg.General.IgnoreSSHHostKeys
						return nil
					},
				},
			},
		},
		{
			name:  "Colors",
			items: m.buildColorItems(),
		},
		{
			name:  "Keybindings",
			items: m.buildKeybindingItems(),
		},
	}
}

func (m *Model) buildColorItems() []configItem {
	cfg := m.cfg
	type colorField struct {
		label string
		get   func() string
		set   func(string)
	}
	fields := []colorField{
		{"Primary", func() string { return cfg.Colors.Primary }, func(v string) { cfg.Colors.Primary = v }},
		{"Secondary", func() string { return cfg.Colors.Secondary }, func(v string) { cfg.Colors.Secondary = v }},
		{"Success", func() string { return cfg.Colors.Success }, func(v string) { cfg.Colors.Success = v }},
		{"Warning", func() string { return cfg.Colors.Warning }, func(v string) { cfg.Colors.Warning = v }},
		{"Error", func() string { return cfg.Colors.Error }, func(v string) { cfg.Colors.Error = v }},
		{"Muted", func() string { return cfg.Colors.Muted }, func(v string) { cfg.Colors.Muted = v }},
		{"Background", func() string { return cfg.Colors.Bg }, func(v string) { cfg.Colors.Bg = v }},
		{"Foreground", func() string { return cfg.Colors.Fg }, func(v string) { cfg.Colors.Fg = v }},
		{"Highlight", func() string { return cfg.Colors.Highlight }, func(v string) { cfg.Colors.Highlight = v }},
		{"Cyan", func() string { return cfg.Colors.Cyan }, func(v string) { cfg.Colors.Cyan = v }},
	}
	items := make([]configItem, len(fields))
	for i, f := range fields {
		f := f
		items[i] = configItem{
			label: f.label, kind: kindColor,
			get: f.get,
			set: func(v string) error {
				if !isValidHex(v) {
					return fmt.Errorf("invalid hex color")
				}
				f.set(v)
				return nil
			},
		}
	}
	return items
}

var reservedKeys = map[string]bool{
	"ctrl+a": true,
	"ctrl+b": true,
}

func (m *Model) buildKeybindingItems() []configItem {
	cfg := m.cfg
	// Use a stable order matching DefaultKeybindings
	order := keybindingOrder()
	var items []configItem
	for _, name := range order {
		name := name
		if _, ok := cfg.Keybindings[name]; !ok {
			continue
		}
		items = append(items, configItem{
			label: keybindingLabel(name), key: name, kind: kindKeybinding,
			get: func() string { return cfg.Keybindings[name] },
			set: func(v string) error {
				if reservedKeys[v] {
					return fmt.Errorf("reserved key (%s)", v)
				}
				cfg.Keybindings[name] = v
				return nil
			},
		})
	}
	return items
}

// Update handles messages for the config overlay.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.keyCapture {
			return m.handleKeyCapture(msg)
		}
		if m.editing {
			return m.handleEditing(msg)
		}
		return m.handleBrowsing(msg)

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

func (m Model) handleBrowsing(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.errMsg = ""
	total := m.totalItems()

	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.Visible = false
		return m, nil

	case key.Matches(msg, shared.Keys.Down):
		if m.cursor < total-1 {
			m.cursor++
			m.ensureVisible()
		}

	case key.Matches(msg, shared.Keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}

	case key.Matches(msg, shared.Keys.PageDown):
		m.cursor += m.viewHeight()
		if m.cursor >= total {
			m.cursor = total - 1
		}
		m.ensureVisible()

	case key.Matches(msg, shared.Keys.PageUp):
		m.cursor -= m.viewHeight()
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()

	case key.Matches(msg, shared.Keys.Enter), key.Matches(msg, shared.Keys.Select):
		item := m.currentItem()
		if item == nil {
			break
		}
		switch item.kind {
		case kindBool:
			if err := item.set(""); err != nil {
				m.errMsg = err.Error()
			} else {
				return m, m.applyAndSave()
			}
		case kindNumeric, kindColor:
			m.editing = true
			m.textInput.SetValue(item.get())
			m.textInput.Focus()
		case kindKeybinding:
			m.keyCapture = true
			m.errMsg = ""
		}
	}

	return m, nil
}

func (m Model) handleEditing(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, shared.Keys.Back):
		m.editing = false
		m.errMsg = ""
		return m, nil

	case key.Matches(msg, shared.Keys.Enter):
		item := m.currentItem()
		if item == nil {
			m.editing = false
			return m, nil
		}
		val := m.textInput.Value()
		if err := item.set(val); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.editing = false
		m.errMsg = ""
		return m, m.applyAndSave()

	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKeyCapture(msg tea.KeyMsg) (Model, tea.Cmd) {
	keyStr := msg.String()

	if key.Matches(msg, shared.Keys.Back) {
		m.keyCapture = false
		m.errMsg = ""
		return m, nil
	}

	if reservedKeys[keyStr] {
		m.errMsg = fmt.Sprintf("reserved key (%s)", keyStr)
		return m, nil
	}

	item := m.currentItem()
	if item == nil {
		m.keyCapture = false
		return m, nil
	}

	if err := item.set(keyStr); err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	m.keyCapture = false
	m.errMsg = ""
	return m, m.applyAndSave()
}

func (m *Model) applyAndSave() tea.Cmd {
	config.ApplyAll(*m.cfg)
	if err := m.cfg.Save(); err != nil {
		m.errMsg = "save failed: " + err.Error()
	}
	return func() tea.Msg { return shared.ConfigChangedMsg{} }
}

func (m Model) currentItem() *configItem {
	idx := 0
	for ci := range m.categories {
		for ii := range m.categories[ci].items {
			if idx == m.cursor {
				return &m.categories[ci].items[ii]
			}
			idx++
		}
	}
	return nil
}

func (m Model) totalItems() int {
	n := 0
	for _, c := range m.categories {
		n += len(c.items)
	}
	return n
}

func (m Model) viewHeight() int {
	h := m.Height - 8
	if h < 5 {
		h = 5
	}
	return h
}

func (m *Model) ensureVisible() {
	// Each category header + items; compute line index for cursor
	line := m.cursorLine()
	vh := m.viewHeight()
	if line < m.scroll {
		m.scroll = line
	}
	if line >= m.scroll+vh {
		m.scroll = line - vh + 1
	}
}

func (m Model) cursorLine() int {
	line := 0
	idx := 0
	for _, c := range m.categories {
		line++ // category header
		for range c.items {
			if idx == m.cursor {
				return line
			}
			line++
			idx++
		}
		line++ // blank line after category
	}
	return line
}

// Render returns the config overlay content.
func (m Model) Render() string {
	title := shared.StyleModalTitle.Render("Configuration")

	lines := m.buildLines()

	vh := m.viewHeight()
	start := m.scroll
	if start > len(lines) {
		start = len(lines)
	}
	end := start + vh
	if end > len(lines) {
		end = len(lines)
	}

	visible := strings.Join(lines[start:end], "\n")

	errLine := ""
	if m.errMsg != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(shared.ColorError).Render("  "+m.errMsg)
	}

	scrollHint := ""
	if m.scroll > 0 || end < len(lines) {
		scrollHint = shared.StyleHelp.Render(" ↑↓ scroll •")
	}

	hint := scrollHint + shared.StyleHelp.Render(" esc close")
	if m.editing {
		hint = shared.StyleHelp.Render(" enter confirm • esc cancel")
	} else if m.keyCapture {
		hint = shared.StyleHelp.Render(" press key to bind • esc cancel")
	}

	content := title + "\n\n" + visible + errLine + "\n\n" + hint
	box := shared.StyleModal.Width(58).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) buildLines() []string {
	var lines []string
	idx := 0
	labelStyle := lipgloss.NewStyle().Foreground(shared.ColorFg).Width(24)
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("#073642")).Foreground(shared.ColorHighlight)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(shared.ColorSecondary)

	for _, c := range m.categories {
		lines = append(lines, headerStyle.Render("  "+c.name))
		for _, item := range c.items {
			selected := idx == m.cursor
			val := item.get()

			var line string
			switch item.kind {
			case kindBool:
				check := "[ ]"
				if val == "true" {
					check = "[x]"
				}
				line = "    " + labelStyle.Render(item.label) + check

			case kindNumeric:
				if selected && m.editing {
					line = "    " + labelStyle.Render(item.label) + m.textInput.View()
				} else {
					line = "    " + labelStyle.Render(item.label) + val
				}

			case kindColor:
				if selected && m.editing {
					line = "    " + labelStyle.Render(item.label) + m.textInput.View()
				} else {
					swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(val)).Render("████")
					line = "    " + labelStyle.Render(item.label) + val + "  " + swatch
				}

			case kindKeybinding:
				if selected && m.keyCapture {
					line = "    " + labelStyle.Render(item.label) +
						lipgloss.NewStyle().Foreground(shared.ColorWarning).Render("Press key...")
				} else {
					line = "    " + labelStyle.Render(item.label) + val
				}
			}

			if selected && !m.editing && !m.keyCapture {
				line = selectedStyle.Render(line)
			}
			lines = append(lines, line)
			idx++
		}
		lines = append(lines, "")
	}
	return lines
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func isValidHex(s string) bool {
	return hexColorRe.MatchString(s)
}

// keybindingOrder returns the display order for keybinding items.
func keybindingOrder() []string {
	return []string{
		"quit", "help", "config", "cloud_pick", "project_pick",
		"filter", "enter", "back",
		"up", "down", "left", "right", "page_up", "page_down",
		"tab", "shift_tab",
		"create", "delete", "rename", "clone",
		"reboot", "hard_reboot", "pause", "suspend", "shelve",
		"stop_start", "lock", "rescue",
		"resize", "confirm_resize", "revert_resize", "rebuild", "snapshot",
		"refresh", "actions", "console", "select", "confirm", "deny", "restart",
		"attach", "detach", "allocate",
		"sort", "reverse_sort", "deactivate", "quota",
		"ssh", "copy_ssh", "console_url",
		"jump_volumes", "jump_sec_groups", "jump_networks",
	}
}

// keybindingLabel converts a snake_case config key to a display label.
func keybindingLabel(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

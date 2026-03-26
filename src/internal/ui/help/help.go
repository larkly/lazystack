package help

import (
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ToggleHelpMsg toggles the help overlay.
type ToggleHelpMsg struct{}

// Model is the help overlay.
type Model struct {
	Visible bool
	View    string // current view context
	Width   int
	Height  int
	scroll  int
	lines   []string
}

// New creates a help model.
func New() Model {
	m := Model{}
	m.buildLines()
	return m
}

func (m *Model) buildLines() {
	sections := []struct {
		name  string
		binds []string
	}{
		{
			name: "Global",
			binds: []string{
				"q / ctrl+c   quit",
				"?            toggle help",
				"C            switch cloud",
				"P            switch project",
				"1-5 / ←→     switch tab",
				"Q             resource quotas",
				"R             force refresh",
				"pgup/pgdn    page up/down",
				"s/S           sort / reverse sort",
				"ctrl+r       restart app",
			},
		},
		{
			name: "Server List",
			binds: []string{
				"↑/k ↓/j      navigate",
				"enter         view detail",
				"ctrl+n        create server",
				"ctrl+d        delete server",
				"ctrl+o        soft reboot",
				"p             pause/unpause",
				"ctrl+z        suspend/resume",
				"ctrl+e        shelve/unshelve",
				"ctrl+f        resize",
				"ctrl+a        assign floating IP",
				"L             console log",
				"a             action history",
				"space         select/deselect",
				"/             filter",
			},
		},
		{
			name: "Server Detail",
			binds: []string{
				"↑/k ↓/j      scroll",
				"ctrl+d        delete server",
				"ctrl+a        assign floating IP",
				"ctrl+o        soft reboot",
				"ctrl+p        hard reboot",
				"p             pause/unpause",
				"ctrl+z        suspend/resume",
				"ctrl+e        shelve/unshelve",
				"ctrl+f        resize",
				"L             console log",
				"a             action history",
				"esc           back to list",
			},
		},
		{
			name: "Console Log",
			binds: []string{
				"↑/k ↓/j      scroll",
				"g             top",
				"G             bottom",
				"esc           back",
			},
		},
		{
			name: "Create Form",
			binds: []string{
				"tab / ↓       next field",
				"shift+tab / ↑ prev field",
				"enter         open picker / activate button",
				"ctrl+s        submit",
				"esc           cancel",
			},
		},
		{
			name: "Volume List / Detail",
			binds: []string{
				"↑/k ↓/j      navigate / scroll",
				"enter         view detail",
				"ctrl+n        create volume",
				"ctrl+d        delete volume",
				"ctrl+a        attach to server",
				"ctrl+t        detach from server",
				"esc           back to list",
			},
		},
		{
			name: "Floating IPs",
			binds: []string{
				"↑/k ↓/j      navigate",
				"ctrl+n        allocate new IP",
				"ctrl+t        disassociate IP",
				"ctrl+d        release floating IP",
			},
		},
		{
			name: "Security Groups",
			binds: []string{
				"↑/k ↓/j      navigate groups / rules",
				"enter         expand / collapse group",
				"ctrl+n        add rule to group",
				"ctrl+d        delete selected rule",
				"esc           back to group list",
			},
		},
		{
			name: "Networks",
			binds: []string{
				"↑/k ↓/j      navigate networks",
				"enter         expand / collapse subnets",
			},
		},
		{
			name: "Key Pairs",
			binds: []string{
				"↑/k ↓/j      navigate",
				"enter         view detail (public key)",
				"ctrl+n        create / import key pair",
				"ctrl+d        delete key pair",
				"esc           back to list",
			},
		},
		{
			name: "Modals",
			binds: []string{
				"y             confirm",
				"n / esc       cancel",
				"←/→ ↑/↓ tab  navigate buttons",
				"enter         activate button",
			},
		},
	}

	m.lines = nil
	for _, s := range sections {
		m.lines = append(m.lines, lipgloss.NewStyle().
			Bold(true).
			Foreground(shared.ColorSecondary).
			Render(s.name))
		for _, bind := range s.binds {
			m.lines = append(m.lines, "  "+bind)
		}
		m.lines = append(m.lines, "")
	}
}

// Update handles input.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Help), key.Matches(msg, shared.Keys.Back):
			m.Visible = false
			m.scroll = 0
			return m, nil
		case key.Matches(msg, shared.Keys.Down):
			maxScroll := len(m.lines) - m.viewHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scroll < maxScroll {
				m.scroll++
			}
		case key.Matches(msg, shared.Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, shared.Keys.PageDown):
			maxScroll := len(m.lines) - m.viewHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scroll += m.viewHeight()
			if m.scroll > maxScroll {
				m.scroll = maxScroll
			}
		case key.Matches(msg, shared.Keys.PageUp):
			m.scroll -= m.viewHeight()
			if m.scroll < 0 {
				m.scroll = 0
			}
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	}
	return m, nil
}

func (m Model) viewHeight() int {
	// modal padding (border 1 + padding 1) * 2 + title + blank + hint = ~8 lines overhead
	h := m.Height - 8
	if h < 3 {
		h = 3
	}
	return h
}

// Render returns the help overlay content.
func (m Model) Render() string {
	title := shared.StyleModalTitle.Render("Keyboard Shortcuts")

	vh := m.viewHeight()
	end := m.scroll + vh
	if end > len(m.lines) {
		end = len(m.lines)
	}
	start := m.scroll
	if start > len(m.lines) {
		start = len(m.lines)
	}

	visible := strings.Join(m.lines[start:end], "\n")

	// Scroll indicator
	scrollHint := ""
	if m.scroll > 0 || end < len(m.lines) {
		scrollHint = shared.StyleHelp.Render(" ↑↓ scroll •")
	}

	hint := scrollHint + shared.StyleHelp.Render(" ? or esc to close")

	content := title + "\n\n" + visible + "\n\n" + hint
	box := shared.StyleModal.Width(50).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

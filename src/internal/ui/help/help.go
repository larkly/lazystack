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

type helpTier int

const (
	tierClosed   helpTier = 0
	tierProfiled helpTier = 1
	tierFull     helpTier = 2
)

// Model is the help overlay.
type Model struct {
	Visible bool
	View    string // current view context
	Width   int
	Height  int
	tier    helpTier
	scroll  int
	lines   []string
}

type section struct {
	name  string
	binds []string
}

var allSections = []section{
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
			"ctrl+k       configuration",
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
			"o             stop/start",
			"ctrl+l        lock/unlock",
			"ctrl+f        resize",
			"ctrl+g        rebuild",
			"ctrl+s        snapshot",
			"r             rename",
			"ctrl+a        assign floating IP",
			"c             clone server",
			"x             SSH into server",
			"y             copy SSH command",
			"V             console URL (noVNC)",
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
			"o             stop/start",
			"ctrl+l        lock/unlock",
			"ctrl+f        resize",
			"ctrl+g        rebuild",
			"ctrl+s        snapshot",
			"r             rename",
			"c             clone server",
			"v             jump to volumes",
			"g             jump to sec groups",
			"N             jump to networks",
			"x             SSH into server",
			"y             copy SSH command",
			"V             console URL (noVNC)",
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
		name: "Volume List",
		binds: []string{
			"↑/k ↓/j      navigate",
			"enter         view detail",
			"ctrl+n        create volume",
			"ctrl+d        delete volume",
			"ctrl+a        attach to server",
			"ctrl+t        detach from server",
			"/             filter",
		},
	},
	{
		name: "Volume Detail",
		binds: []string{
			"↑/k ↓/j      scroll",
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
			"ctrl+n        create group (or add rule in rules)",
			"ctrl+d        delete group (or rule in rules)",
			"esc           back to group list",
		},
	},
	{
		name: "Networks",
		binds: []string{
			"↑/k ↓/j      navigate networks / subnets",
			"enter         expand / collapse subnets",
			"ctrl+n        create network (or subnet in subnets)",
			"ctrl+d        delete network (or subnet in subnets)",
			"esc           back to network list",
		},
	},
	{
		name: "Routers",
		binds: []string{
			"↑/k ↓/j      navigate",
			"enter         view detail (interfaces)",
			"ctrl+n        create router",
			"ctrl+d        delete router",
			"ctrl+a        add interface (from detail)",
			"ctrl+t        remove interface (from detail)",
			"esc           back to list",
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
		name: "LB List",
		binds: []string{
			"↑/k ↓/j      navigate",
			"enter         view detail",
			"s/S           sort / reverse sort",
			"/             filter",
		},
	},
	{
		name: "LB Detail",
		binds: []string{
			"↑/k ↓/j      scroll",
			"esc           back to list",
		},
	},
	{
		name: "Image List",
		binds: []string{
			"↑/k ↓/j      navigate",
			"enter         view detail",
			"ctrl+d        delete image",
			"d             deactivate image",
			"s/S           sort / reverse sort",
			"/             filter",
		},
	},
	{
		name: "Image Detail",
		binds: []string{
			"↑/k ↓/j      scroll",
			"ctrl+d        delete image",
			"d             deactivate image",
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

// viewSections maps view names to the section names shown in profiled help.
// "Global" is always prepended automatically.
var viewSections = map[string][]string{
	"serverlist":    {"Server List"},
	"serverdetail":  {"Server Detail"},
	"servercreate":  {"Create Form"},
	"consolelog":    {"Console Log"},
	"actionlog":     {"Console Log"},
	"volumelist":    {"Volume List"},
	"volumedetail":  {"Volume Detail"},
	"volumecreate":  {"Create Form"},
	"floatingiplist": {"Floating IPs"},
	"secgroupview":  {"Security Groups"},
	"networkview":   {"Networks"},
	"keypairlist":   {"Key Pairs"},
	"keypairdetail": {"Key Pairs"},
	"keypaircreate": {"Create Form"},
	"routerlist":    {"Routers"},
	"routerdetail":  {"Routers"},
	"lblist":        {"LB List"},
	"lbdetail":      {"LB Detail"},
	"imagelist":     {"Image List"},
	"imagedetail":   {"Image Detail"},
	"cloudpicker":   {},
}

// New creates a help model.
func New() Model {
	return Model{}
}

// Open toggles the help overlay, cycling through tiers.
func (m *Model) Open(view string) {
	m.View = view
	switch m.tier {
	case tierClosed:
		m.tier = tierProfiled
	case tierProfiled:
		m.tier = tierFull
	default:
		m.tier = tierClosed
	}
	m.scroll = 0
	m.Visible = m.tier != tierClosed
	if m.Visible {
		m.buildLines()
	}
}

func (m *Model) buildLines() {
	m.lines = nil

	var sections []section
	if m.tier == tierProfiled {
		// Global + view-specific sections
		sections = append(sections, findSection("Global"))
		if names, ok := viewSections[m.View]; ok {
			for _, name := range names {
				sections = append(sections, findSection(name))
			}
		}
	} else {
		sections = allSections
	}

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

func findSection(name string) section {
	for _, s := range allSections {
		if s.name == name {
			return s
		}
	}
	return section{name: name}
}

// Update handles input.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, shared.Keys.Help):
			if m.tier == tierProfiled {
				m.tier = tierFull
				m.scroll = 0
				m.buildLines()
			} else {
				m.tier = tierClosed
				m.scroll = 0
				m.Visible = false
			}
			return m, nil
		case key.Matches(msg, shared.Keys.Back):
			m.tier = tierClosed
			m.scroll = 0
			m.Visible = false
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

	var hint string
	if m.tier == tierProfiled {
		hint = scrollHint + shared.StyleHelp.Render(" ? all shortcuts • esc close")
	} else {
		hint = scrollHint + shared.StyleHelp.Render(" ? or esc to close")
	}

	content := title + "\n\n" + visible + "\n\n" + hint
	box := shared.StyleModal.Width(50).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

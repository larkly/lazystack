package help

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/larkly/lazystack/internal/shared"
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
	binds []bind
}

// bind is one help entry. Entries for shared key bindings reference the
// binding so the overlay shows the keys actually bound (including any
// rebinding from config.yaml); view-local keys use a literal.
type bind struct {
	key     string
	binding *key.Binding
	desc    string
}

func (b bind) keys() string {
	if b.binding != nil {
		return strings.Join(b.binding.Keys(), "/")
	}
	return b.key
}

// k is a help entry shown with the current keys of a shared binding.
func k(b *key.Binding, desc string) bind { return bind{binding: b, desc: desc} }

// l is a help entry with literal key text.
func l(keys, desc string) bind { return bind{key: keys, desc: desc} }

var allSections = []section{
	{
		name: "Global",
		binds: []bind{
			k(&shared.Keys.Quit, "quit"),
			k(&shared.Keys.Help, "help (again: all shortcuts)"),
			k(&shared.Keys.CloudPick, "switch cloud"),
			k(&shared.Keys.ProjectPick, "switch project"),
			l("1-9 ←/→ h/l", "switch tab (list views)"),
			k(&shared.Keys.Quota, "resource quotas"),
			k(&shared.Keys.Refresh, "force refresh"),
			l("pgup/pgdn", "page up/down"),
			l("s/S", "sort / reverse sort"),
			k(&shared.Keys.Copy, "copy field..."),
			k(&shared.Keys.Hypervisors, "hypervisors (admin)"),
			k(&shared.Keys.Browse, "service catalog"),
			k(&shared.Keys.Config, "configuration"),
			k(&shared.Keys.Restart, "restart app"),
		},
	},
	{
		name: "Server List",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			k(&shared.Keys.Enter, "view detail"),
			k(&shared.Keys.Select, "select/deselect"),
			k(&shared.Keys.Filter, "filter"),
			k(&shared.Keys.SaveFilter, "save current filter"),
			k(&shared.Keys.LoadFilter, "load next saved filter"),
			l("esc", "clear filter / selection"),
			k(&shared.Keys.Create, "create server"),
			k(&shared.Keys.Delete, "delete server"),
			k(&shared.Keys.Reboot, "soft reboot"),
			k(&shared.Keys.HardReboot, "hard reboot"),
			k(&shared.Keys.StopStart, "stop/start"),
			k(&shared.Keys.Pause, "pause/unpause"),
			k(&shared.Keys.Suspend, "suspend/resume"),
			k(&shared.Keys.Shelve, "shelve/unshelve"),
			k(&shared.Keys.Lock, "lock/unlock"),
			k(&shared.Keys.Rescue, "rescue/unrescue"),
			k(&shared.Keys.Resize, "resize"),
			k(&shared.Keys.ConfirmResize, "confirm resize"),
			k(&shared.Keys.RevertResize, "revert resize"),
			k(&shared.Keys.Rebuild, "rebuild"),
			k(&shared.Keys.Snapshot, "snapshot"),
			k(&shared.Keys.Rename, "rename"),
			k(&shared.Keys.Clone, "clone server"),
			k(&shared.Keys.Attach, "attach volume"),
			k(&shared.Keys.AssignFIP, "assign floating IP"),
			k(&shared.Keys.SSH, "SSH into server"),
			k(&shared.Keys.CopySSH, "copy SSH command"),
			k(&shared.Keys.ConsoleURL, "console URL (noVNC)"),
			k(&shared.Keys.GetPassword, "admin password"),
			k(&shared.Keys.Console, "console log"),
			k(&shared.Keys.Actions, "action history"),
			k(&shared.Keys.AuditLog, "audit trail"),
			k(&shared.Keys.AdminActions, "admin actions"),
			k(&shared.Keys.Metadata, "metadata"),
			k(&shared.Keys.UserManagement, "user management"),
			k(&shared.Keys.ColumnPick, "choose columns"),
		},
	},
	{
		name: "Server Detail",
		binds: []bind{
			l("↑/k ↓/j", "scroll / select in pane"),
			l("tab/shift+tab", "cycle panes"),
			l("enter", "open volume (volumes pane)"),
			k(&shared.Keys.Detach, "detach volume (volumes pane)"),
			k(&shared.Keys.Delete, "delete server"),
			k(&shared.Keys.Reboot, "soft reboot"),
			k(&shared.Keys.HardReboot, "hard reboot"),
			k(&shared.Keys.StopStart, "stop/start"),
			k(&shared.Keys.Pause, "pause/unpause"),
			k(&shared.Keys.Suspend, "suspend/resume"),
			k(&shared.Keys.Shelve, "shelve/unshelve"),
			k(&shared.Keys.Lock, "lock/unlock"),
			k(&shared.Keys.Rescue, "rescue/unrescue"),
			k(&shared.Keys.Resize, "resize"),
			k(&shared.Keys.ConfirmResize, "confirm resize"),
			k(&shared.Keys.RevertResize, "revert resize"),
			k(&shared.Keys.Rebuild, "rebuild"),
			k(&shared.Keys.Snapshot, "snapshot"),
			k(&shared.Keys.Rename, "rename"),
			k(&shared.Keys.Clone, "clone server"),
			k(&shared.Keys.Attach, "attach volume"),
			k(&shared.Keys.AssignFIP, "assign floating IP"),
			k(&shared.Keys.JumpVolumes, "jump to volumes"),
			k(&shared.Keys.JumpSecGroups, "jump to sec groups"),
			k(&shared.Keys.JumpNetworks, "jump to networks"),
			l("g / G", "top / bottom (console pane)"),
			k(&shared.Keys.SSH, "SSH into server"),
			k(&shared.Keys.CopySSH, "copy SSH command"),
			k(&shared.Keys.ConsoleURL, "console URL (noVNC)"),
			k(&shared.Keys.GetPassword, "admin password"),
			k(&shared.Keys.Console, "console log"),
			k(&shared.Keys.Actions, "action history"),
			k(&shared.Keys.AuditLog, "audit trail"),
			k(&shared.Keys.AdminActions, "admin actions"),
			k(&shared.Keys.Metadata, "metadata"),
			k(&shared.Keys.UserManagement, "user management"),
			k(&shared.Keys.Back, "back to list"),
		},
	},
	{
		name: "Console Log",
		binds: []bind{
			l("↑/k ↓/j", "scroll"),
			l("g", "top"),
			l("G", "bottom"),
			l("esc", "back"),
		},
	},
	{
		name: "Create Form",
		binds: []bind{
			l("tab / ↓", "next field"),
			l("shift+tab / ↑", "prev field"),
			l("enter", "open picker / activate button"),
			l("ctrl+s", "submit"),
			l("esc", "cancel"),
		},
	},
	{
		name: "Volume List",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			k(&shared.Keys.Enter, "view detail"),
			k(&shared.Keys.Select, "select/deselect"),
			k(&shared.Keys.Create, "create volume"),
			k(&shared.Keys.Delete, "delete volume(s)"),
			k(&shared.Keys.Attach, "attach to server"),
			k(&shared.Keys.Detach, "detach from server"),
			l("Y", "copy field..."),
		},
	},
	{
		name: "Volume Detail",
		binds: []bind{
			l("↑/k ↓/j", "scroll"),
			k(&shared.Keys.Delete, "delete volume"),
			k(&shared.Keys.Attach, "attach to server"),
			k(&shared.Keys.Detach, "detach from server"),
			l("Y", "copy field..."),
			l("esc", "back to list"),
		},
	},
	{
		name: "Floating IPs",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			k(&shared.Keys.Allocate, "allocate new IP"),
			k(&shared.Keys.Detach, "disassociate IP"),
			k(&shared.Keys.Delete, "release floating IP"),
			l("Y", "copy field..."),
		},
	},
	{
		name: "Security Groups",
		binds: []bind{
			l("↑/k ↓/j", "navigate groups / rules"),
			l("enter", "expand / collapse group"),
			l("ctrl+n", "create group (or add rule in rules)"),
			l("ctrl+d", "delete group (or rule in rules)"),
			l("Y", "copy field..."),
			l("esc", "back to group list"),
		},
	},
	{
		name: "Networks",
		binds: []bind{
			l("↑/k ↓/j", "navigate networks / subnets"),
			l("enter", "expand / collapse subnets"),
			l("ctrl+n", "create network (or subnet in subnets)"),
			l("ctrl+d", "delete network (or subnet in subnets)"),
			l("Y", "copy field..."),
			l("esc", "back to network list"),
		},
	},
	{
		name: "Routers",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("tab/shift+tab", "cycle panes"),
			l("enter", "view detail (interfaces)"),
			l("ctrl+n", "create router"),
			l("ctrl+d", "delete router"),
			k(&shared.Keys.Attach, "add interface (from detail)"),
			k(&shared.Keys.Detach, "remove interface (from detail)"),
			l("Y", "copy field..."),
			l("esc", "back to list"),
		},
	},
	{
		name: "Key Pairs",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("enter", "view detail (public key)"),
			l("ctrl+n", "create / import key pair"),
			l("ctrl+d", "delete key pair"),
			l("Y", "copy field..."),
			l("esc", "back to list"),
		},
	},
	{
		name: "LB List",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("enter", "view detail"),
			l("ctrl+n", "create load balancer"),
			l("ctrl+d", "delete load balancer"),
			l("s/S", "sort / reverse sort"),
			l("Y", "copy field..."),
			l("/", "filter"),
		},
	},
	{
		name: "LB Detail",
		binds: []bind{
			l("↑/k ↓/j", "navigate in pane"),
			l("tab/shift+tab", "cycle panes"),
			l("enter", "edit (context-sensitive)"),
			l("ctrl+n", "add listener/pool/member"),
			l("ctrl+d", "delete (context-sensitive)"),
			l("ctrl+h", "add/edit health monitor"),
			l("o", "enable/disable (admin state)"),
			l("w", "drain member (weight=0)"),
			l("space", "toggle member selection"),
			l("x", "select/deselect all members"),
			l("Y", "copy field..."),
			l("esc", "back to list"),
		},
	},
	{
		name: "Images",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("tab/shift+tab", "cycle panes"),
			l("/", "search/filter images"),
			l("s/S", "sort / reverse sort"),
			k(&shared.Keys.Select, "select/deselect (bulk delete)"),
			k(&shared.Keys.Create, "upload image (list pane)"),
			k(&shared.Keys.Delete, "delete image(s)"),
			k(&shared.Keys.Deactivate, "deactivate/reactivate (list/info pane)"),
			l("ctrl+g", "download image (properties pane)"),
			l("enter", "edit image (info pane) / open server (servers pane)"),
			l("Y", "copy field..."),
			l("esc", "clear filter / selection"),
		},
	},
	{
		name: "Hypervisors",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("pgup/pgdn", "page up/down"),
			k(&shared.Keys.Refresh, "refresh"),
			l("esc", "back"),
		},
	},
	{
		name: "Service Catalog",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("pgup/pgdn", "page up/down"),
			l("esc", "back"),
		},
	},
	{
		name: "DNS",
		binds: []bind{
			l("↑/k ↓/j", "select zone (shows its records)"),
			l("pgup/pgdn", "page up/down"),
			k(&shared.Keys.Refresh, "refresh"),
			l("esc", "back"),
		},
	},
	{
		name: "User Management",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("enter", "enable/disable user"),
			l("d", "delete user (y confirms, n cancels)"),
			k(&shared.Keys.Refresh, "refresh"),
			l("esc", "back"),
		},
	},
	{
		name: "Audit Trail",
		binds: []bind{
			l("↑/k ↓/j", "navigate"),
			l("a", "filter by action"),
			l("r", "filter by resource"),
			l("d", "filter by date"),
			l("c", "clear filter"),
			l("enter", "apply typed filter"),
			l("esc", "cancel filter / back"),
		},
	},
	{
		name: "Modals",
		binds: []bind{
			l("y", "confirm"),
			l("n / esc", "cancel"),
			l("←/→ ↑/↓ tab", "navigate buttons"),
			l("enter", "activate button"),
		},
	},
}

// viewSections maps view names to the section names shown in profiled help.
// "Global" is always prepended automatically.
var viewSections = map[string][]string{
	"serverlist":     {"Server List"},
	"serverdetail":   {"Server Detail"},
	"servercreate":   {"Create Form"},
	"consolelog":     {"Console Log"},
	"actionlog":      {"Console Log"},
	"volumelist":     {"Volume List"},
	"volumedetail":   {"Volume Detail"},
	"volumecreate":   {"Create Form"},
	"floatingiplist": {"Floating IPs"},
	"secgroupview":   {"Security Groups"},
	"networkview":    {"Networks"},
	"keypairlist":    {"Key Pairs"},
	"keypairdetail":  {"Key Pairs"},
	"keypaircreate":  {"Create Form"},
	"routerlist":     {"Routers"},
	"routerdetail":   {"Routers"},
	"routerview":     {"Routers"},
	"lbview":         {"LB List", "LB Detail"},
	"imageview":      {"Images"},
	"hypervisorlist": {"Hypervisors"},
	"servicecatalog": {"Service Catalog"},
	"dnslist":        {"DNS"},
	"usermanagement": {"User Management"},
	"auditlog":       {"Audit Trail"},
	"cloudpicker":    {},
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
	shared.Debugf("[help] Open view=%s tier=%d visible=%v", view, m.tier, m.Visible)
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
		for _, b := range s.binds {
			m.lines = append(m.lines, fmt.Sprintf("  %-14s %s", b.keys(), b.desc))
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

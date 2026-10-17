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

// helpWidth is the overlay width; entryWidth is the widest help line that
// fits inside its border and padding without wrapping.
const (
	helpWidth  = 60
	entryWidth = helpWidth - 6
)

type section struct {
	name  string
	binds []bind
}

// bind is one help entry. Entries for shared key bindings reference the
// bindings so the overlay shows the keys actually bound (including any
// rebinding from config.yaml); keys no binding describes use a literal.
type bind struct {
	key      string
	bindings []*key.Binding
	desc     string
}

func (b bind) keys() string {
	if len(b.bindings) == 0 {
		return b.key
	}
	parts := make([]string, 0, len(b.bindings))
	for _, kb := range b.bindings {
		parts = append(parts, strings.Join(kb.Keys(), "/"))
	}
	return strings.Join(parts, "/")
}

// k is a help entry shown with the current keys of a shared binding.
func k(b *key.Binding, desc string) bind { return bind{bindings: []*key.Binding{b}, desc: desc} }

// kk is a help entry shown with the keys of several shared bindings, such
// as sort and reverse sort.
func kk(desc string, bs ...*key.Binding) bind { return bind{bindings: bs, desc: desc} }

// l is a help entry with literal key text. It is for keys no shared
// binding describes on its own: the number-key tab range and the arrow,
// paging and field navigation shorthand.
func l(keys, desc string) bind { return bind{key: keys, desc: desc} }

var (
	keys = &shared.Keys

	navigate   = l("↑/k ↓/j", "navigate")
	paging     = l("pgup/pgdn", "page up/down")
	cyclePanes = kk("cycle panes", &keys.Tab, &keys.ShiftTab)
	sortRev    = kk("sort / reverse sort", &keys.Sort, &keys.ReverseSort)
	copyField  = k(&keys.Copy, "copy field...")
)

var allSections = []section{
	{
		name: "Global",
		binds: []bind{
			k(&keys.Quit, "quit"),
			k(&keys.Help, "help (again: all shortcuts)"),
			k(&keys.CloudPick, "switch cloud"),
			k(&keys.ProjectPick, "switch project"),
			l("1-9, 0", "switch tab (list views)"),
			l("←/→ h/l", "previous/next tab"),
			k(&keys.Quota, "resource quotas"),
			k(&keys.Refresh, "force refresh"),
			paging,
			k(&keys.Back, "clear filter, selection, then back"),
			copyField,
			k(&keys.Hypervisors, "hypervisors (admin)"),
			k(&keys.Browse, "service catalog"),
			k(&keys.Config, "configuration"),
			k(&keys.Restart, "restart app"),
		},
	},
	{
		name: "Server List",
		binds: []bind{
			navigate,
			k(&keys.Enter, "view detail"),
			k(&keys.Select, "select/deselect"),
			k(&keys.Filter, "filter"),
			k(&keys.SaveFilter, "save current filter"),
			k(&keys.LoadFilter, "load next saved filter"),
			sortRev,
			k(&keys.Back, "clear filter / selection"),
			k(&keys.Create, "create server"),
			k(&keys.Delete, "delete server"),
			k(&keys.Reboot, "soft reboot"),
			k(&keys.HardReboot, "hard reboot"),
			k(&keys.StopStart, "stop/start"),
			k(&keys.Pause, "pause/unpause"),
			k(&keys.Suspend, "suspend/resume"),
			k(&keys.Shelve, "shelve/unshelve"),
			k(&keys.Lock, "lock/unlock"),
			k(&keys.Rescue, "rescue/unrescue"),
			k(&keys.Resize, "resize"),
			k(&keys.ConfirmResize, "confirm resize"),
			k(&keys.RevertResize, "revert resize"),
			k(&keys.Rebuild, "rebuild"),
			k(&keys.Snapshot, "snapshot"),
			k(&keys.Rename, "rename"),
			k(&keys.Clone, "clone server"),
			k(&keys.Attach, "attach volume"),
			k(&keys.AssignFIP, "assign floating IP"),
			k(&keys.SSH, "SSH into server"),
			k(&keys.CopySSH, "copy SSH command"),
			k(&keys.ConsoleURL, "console URL (noVNC)"),
			k(&keys.GetPassword, "admin password"),
			k(&keys.Console, "console log"),
			k(&keys.Actions, "action history"),
			k(&keys.AuditLog, "audit trail"),
			k(&keys.AdminActions, "admin actions"),
			k(&keys.Metadata, "metadata"),
			k(&keys.UserManagement, "user management"),
			k(&keys.ColumnPick, "choose columns"),
		},
	},
	{
		name: "Server Detail",
		binds: []bind{
			l("↑/k ↓/j", "scroll / select in pane"),
			cyclePanes,
			k(&keys.Enter, "open volume (volumes pane)"),
			k(&keys.Detach, "detach volume (volumes pane)"),
			k(&keys.Delete, "delete server"),
			k(&keys.Reboot, "soft reboot"),
			k(&keys.HardReboot, "hard reboot"),
			k(&keys.StopStart, "stop/start"),
			k(&keys.Pause, "pause/unpause"),
			k(&keys.Suspend, "suspend/resume"),
			k(&keys.Shelve, "shelve/unshelve"),
			k(&keys.Lock, "lock/unlock"),
			k(&keys.Rescue, "rescue/unrescue"),
			k(&keys.Resize, "resize"),
			k(&keys.ConfirmResize, "confirm resize"),
			k(&keys.RevertResize, "revert resize"),
			k(&keys.Rebuild, "rebuild"),
			k(&keys.Snapshot, "snapshot"),
			k(&keys.Rename, "rename"),
			k(&keys.Clone, "clone server"),
			k(&keys.Attach, "attach volume"),
			k(&keys.AssignFIP, "assign floating IP"),
			k(&keys.JumpVolumes, "jump to volumes"),
			k(&keys.JumpSecGroups, "jump to sec groups"),
			k(&keys.JumpNetworks, "jump to networks"),
			kk("top / bottom (console pane)", &keys.JumpStart, &keys.JumpEnd),
			k(&keys.SSH, "SSH into server"),
			k(&keys.CopySSH, "copy SSH command"),
			k(&keys.ConsoleURL, "console URL (noVNC)"),
			k(&keys.GetPassword, "admin password"),
			k(&keys.Console, "console log"),
			k(&keys.Actions, "action history"),
			k(&keys.AuditLog, "audit trail"),
			k(&keys.AdminActions, "admin actions"),
			k(&keys.Metadata, "metadata"),
			k(&keys.UserManagement, "user management"),
			k(&keys.Back, "back to list"),
		},
	},
	{
		name: "Console Log",
		binds: []bind{
			l("↑/k ↓/j", "scroll"),
			paging,
			k(&keys.JumpStart, "top"),
			k(&keys.JumpEnd, "bottom"),
			k(&keys.Back, "back"),
		},
	},
	{
		name: "Action History",
		binds: []bind{
			l("↑/k ↓/j", "scroll"),
			paging,
			k(&keys.Back, "back to server"),
		},
	},
	{
		name: "Create Form",
		binds: []bind{
			l("tab / ↓", "next field"),
			l("shift+tab / ↑", "prev field"),
			k(&keys.Enter, "open picker / activate button"),
			k(&keys.Submit, "submit"),
			k(&keys.Back, "cancel"),
		},
	},
	{
		name: "Volume List",
		binds: []bind{
			navigate,
			k(&keys.Enter, "view detail"),
			k(&keys.Select, "select/deselect"),
			k(&keys.Filter, "filter"),
			sortRev,
			k(&keys.Back, "clear filter / selection"),
			k(&keys.Create, "create volume"),
			k(&keys.Delete, "delete volume(s)"),
			k(&keys.Attach, "attach to server"),
			k(&keys.Detach, "detach from server"),
			copyField,
		},
	},
	{
		name: "Volume Detail",
		binds: []bind{
			l("↑/k ↓/j", "scroll"),
			k(&keys.Delete, "delete volume"),
			k(&keys.Attach, "attach to server"),
			k(&keys.Detach, "detach from server"),
			copyField,
			k(&keys.Back, "back"),
		},
	},
	{
		name: "Floating IPs",
		binds: []bind{
			navigate,
			k(&keys.Filter, "filter"),
			sortRev,
			k(&keys.Back, "clear filter"),
			k(&keys.Allocate, "allocate (asks, picks network)"),
			k(&keys.Detach, "disassociate IP"),
			k(&keys.Delete, "release floating IP"),
			copyField,
		},
	},
	{
		name: "Security Groups",
		binds: []bind{
			l("↑/k ↓/j", "navigate in pane"),
			cyclePanes,
			k(&keys.Enter, "edit rule / open server (pane)"),
			k(&keys.Create, "create group (add rule in rules)"),
			k(&keys.Delete, "delete group (rule in rules)"),
			k(&keys.Rename, "rename group (not default)"),
			k(&keys.Clone, "clone group"),
			k(&keys.Filter, "filter groups"),
			kk("sort groups / reverse sort", &keys.Sort, &keys.ReverseSort),
			copyField,
			k(&keys.Back, "clear filter"),
		},
	},
	{
		name: "Networks",
		binds: []bind{
			l("↑/k ↓/j", "navigate in pane"),
			cyclePanes,
			k(&keys.Enter, "edit subnet / port (pane)"),
			k(&keys.Create, "create network / subnet / port"),
			k(&keys.Delete, "delete network / subnet / port"),
			k(&keys.Filter, "filter networks"),
			kk("sort networks / reverse sort", &keys.Sort, &keys.ReverseSort),
			copyField,
			k(&keys.Back, "clear filter"),
		},
	},
	{
		name: "Routers",
		binds: []bind{
			l("↑/k ↓/j", "navigate in pane"),
			cyclePanes,
			k(&keys.Enter, "focus interfaces"),
			k(&keys.Create, "create router (add interface)"),
			k(&keys.Delete, "delete router"),
			k(&keys.Attach, "add interface"),
			k(&keys.Detach, "remove interface (interfaces)"),
			k(&keys.Filter, "filter routers"),
			kk("sort routers / reverse sort", &keys.Sort, &keys.ReverseSort),
			copyField,
			k(&keys.Back, "clear filter / focus router list"),
		},
	},
	{
		name: "Key Pairs",
		binds: []bind{
			navigate,
			k(&keys.Enter, "view detail (public key)"),
			k(&keys.Filter, "filter (list)"),
			sortRev,
			k(&keys.Create, "create / import key pair"),
			k(&keys.Delete, "delete key pair"),
			copyField,
			k(&keys.Back, "clear filter (list) / back (detail)"),
		},
	},
	{
		name: "LB List",
		binds: []bind{
			navigate,
			k(&keys.Filter, "filter"),
			sortRev,
			k(&keys.Create, "create load balancer"),
			k(&keys.Delete, "delete load balancer"),
			copyField,
			k(&keys.Back, "clear filter / member selection"),
		},
	},
	{
		name: "LB Detail",
		binds: []bind{
			l("↑/k ↓/j", "navigate in pane"),
			cyclePanes,
			k(&keys.Enter, "edit (focused pane)"),
			k(&keys.Create, "add listener / pool / member"),
			k(&keys.Delete, "delete (focused pane)"),
			k(&keys.HealthMonitor, "health monitor (pools pane)"),
			k(&keys.StopStart, "enable/disable (admin state)"),
			k(&keys.DrainMember, "drain member (weight 0)"),
			k(&keys.Select, "toggle member selection"),
			k(&keys.SelectAll, "select/deselect all members"),
			copyField,
		},
	},
	{
		name: "Images",
		binds: []bind{
			navigate,
			cyclePanes,
			k(&keys.Filter, "search/filter images"),
			sortRev,
			k(&keys.Select, "select/deselect (bulk delete)"),
			k(&keys.Create, "upload image (list pane)"),
			k(&keys.Delete, "delete image(s)"),
			k(&keys.Deactivate, "(de)activate (list/info pane)"),
			k(&keys.Download, "download image (properties pane)"),
			k(&keys.Enter, "edit image / open server (pane)"),
			copyField,
			k(&keys.Back, "clear filter / selection"),
		},
	},
	{
		name: "Image Transfers",
		binds: []bind{
			k(&keys.Enter, "browse (path field)"),
			k(&keys.Submit, "start upload / download"),
			k(&keys.Background, "continue in background"),
			k(&keys.Back, "close / cancel transfer (asks)"),
		},
	},
	{
		name: "Hypervisors",
		binds: []bind{
			navigate,
			paging,
			k(&keys.Refresh, "refresh"),
			k(&keys.Back, "back"),
		},
	},
	{
		name: "Service Catalog",
		binds: []bind{
			navigate,
			paging,
			k(&keys.Back, "back"),
		},
	},
	{
		name: "DNS",
		binds: []bind{
			l("↑/k ↓/j", "select zone (shows its records)"),
			paging,
			k(&keys.Filter, "filter zones"),
			kk("sort zones / reverse sort", &keys.Sort, &keys.ReverseSort),
			k(&keys.Back, "clear filter"),
			k(&keys.Refresh, "refresh"),
		},
	},
	{
		name: "User Management",
		binds: []bind{
			navigate,
			k(&keys.Enter, "enable/disable user (asks)"),
			k(&keys.Delete, "delete user (asks)"),
			k(&keys.Refresh, "refresh"),
			k(&keys.Back, "cancel prompt / back"),
		},
	},
	{
		name: "Audit Trail",
		binds: []bind{
			navigate,
			k(&keys.AuditFilterAction, "filter by action"),
			k(&keys.AuditFilterResource, "filter by resource"),
			k(&keys.AuditFilterDate, "filter by date"),
			k(&keys.AuditFilterClear, "clear filter"),
			k(&keys.Enter, "apply typed filter"),
			k(&keys.Back, "cancel filter / back"),
		},
	},
	{
		name: "Metadata Editor",
		binds: []bind{
			navigate,
			k(&keys.MetadataAdd, "add entry"),
			k(&keys.MetadataEdit, "edit entry"),
			k(&keys.MetadataDelete, "delete entry (asks)"),
			cyclePanes.withDesc("switch key / value field"),
			k(&keys.Enter, "save entry"),
			k(&keys.Back, "cancel edit / close"),
		},
	},
	{
		name: "Dialogs",
		binds: []bind{
			k(&keys.Confirm, "confirm"),
			kk("cancel", &keys.Deny, &keys.Back),
			l("←/→ ↑/↓ tab", "navigate buttons"),
			k(&keys.Enter, "activate button"),
			k(&keys.Select, "toggle volume deletion (if shown)"),
			k(&keys.ToggleDetails, "error details"),
			k(&keys.CopyValue, "copy (console URL, password)"),
			k(&keys.Reveal, "reveal/hide password"),
			k(&keys.SaveKey, "save new private key"),
		},
	},
}

// withDesc returns the entry with another description.
func (b bind) withDesc(desc string) bind {
	b.desc = desc
	return b
}

// viewSections maps view names to the section names shown in profiled help.
// "Global" is always prepended automatically.
var viewSections = map[string][]string{
	"serverlist":     {"Server List"},
	"serverdetail":   {"Server Detail"},
	"servercreate":   {"Create Form"},
	"consolelog":     {"Console Log"},
	"actionlog":      {"Action History"},
	"volumelist":     {"Volume List"},
	"volumedetail":   {"Volume Detail"},
	"volumecreate":   {"Create Form"},
	"floatingiplist": {"Floating IPs"},
	"secgroupview":   {"Security Groups"},
	"networkview":    {"Networks"},
	"keypairlist":    {"Key Pairs"},
	"keypairdetail":  {"Key Pairs"},
	"keypaircreate":  {"Create Form"},
	"routerview":     {"Routers"},
	"lbview":         {"LB List", "LB Detail"},
	"imageview":      {"Images", "Image Transfers"},
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

// boxWidth is the overlay width: wide enough for every entry on one line,
// narrowed to fit small terminals.
func (m Model) boxWidth() int {
	w := helpWidth
	if m.Width > 0 && m.Width-2 < w {
		w = max(m.Width-2, 30)
	}
	return w
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
	box := shared.StyleModal.Width(m.boxWidth()).Render(content)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

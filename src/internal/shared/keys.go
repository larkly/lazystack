package shared

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// reservedKeys can never be bound: Ctrl+A is the GNU Screen prefix and
// Ctrl+B the tmux prefix, so binding either would make lazystack unusable
// (or steal the multiplexer's prefix) inside those tools.
var reservedKeys = map[string]bool{
	"ctrl+a": true,
	"ctrl+b": true,
}

// IsReservedKey reports whether k is a key string that must never be bound.
func IsReservedKey(k string) bool {
	return reservedKeys[strings.ToLower(strings.TrimSpace(k))]
}

// ContainsReservedKey reports whether any key in a comma-separated
// keybinding list is reserved.
func ContainsReservedKey(keys string) bool {
	for _, k := range strings.Split(keys, ",") {
		if IsReservedKey(k) {
			return true
		}
	}
	return false
}

type KeyMap struct {
	Quit           key.Binding
	Help           key.Binding
	CloudPick      key.Binding
	Filter         key.Binding
	Enter          key.Binding
	Back           key.Binding
	Create         key.Binding
	Delete         key.Binding
	Reboot         key.Binding
	HardReboot     key.Binding
	Refresh        key.Binding
	Up             key.Binding
	Down           key.Binding
	Left           key.Binding
	Right          key.Binding
	Tab            key.Binding
	ShiftTab       key.Binding
	Pause          key.Binding
	Suspend        key.Binding
	Shelve         key.Binding
	Resize         key.Binding
	ConfirmResize  key.Binding
	RevertResize   key.Binding
	Actions        key.Binding
	Console        key.Binding
	AuditLog       key.Binding
	Select         key.Binding
	Confirm        key.Binding
	Deny           key.Binding
	Restart        key.Binding
	Attach         key.Binding
	AssignFIP      key.Binding
	Detach         key.Binding
	Allocate       key.Binding
	PageUp         key.Binding
	PageDown       key.Binding
	Sort           key.Binding
	ReverseSort    key.Binding
	ProjectPick    key.Binding
	Quota          key.Binding
	StopStart      key.Binding
	Lock           key.Binding
	Rename         key.Binding
	Rebuild        key.Binding
	Snapshot       key.Binding
	Deactivate     key.Binding
	Rescue         key.Binding
	Clone          key.Binding
	JumpVolumes    key.Binding
	JumpSecGroups  key.Binding
	JumpNetworks   key.Binding
	SSH            key.Binding
	CopySSH        key.Binding
	Copy           key.Binding
	ConsoleURL     key.Binding
	GetPassword    key.Binding
	Config         key.Binding
	Hypervisors    key.Binding
	SaveFilter     key.Binding
	LoadFilter     key.Binding
	Browse         key.Binding
	AdminActions   key.Binding
	Metadata       key.Binding
	UserManagement key.Binding
	ColumnPick     key.Binding

	// Form and pane-specific actions.
	Submit              key.Binding
	HealthMonitor       key.Binding
	DrainMember         key.Binding
	SelectAll           key.Binding
	Download            key.Binding
	Background          key.Binding
	ToggleDetails       key.Binding
	MetadataAdd         key.Binding
	MetadataEdit        key.Binding
	MetadataDelete      key.Binding
	SaveKey             key.Binding
	JumpStart           key.Binding
	JumpEnd             key.Binding
	AuditFilterAction   key.Binding
	AuditFilterResource key.Binding
	AuditFilterDate     key.Binding
	AuditFilterClear    key.Binding
	CopyValue           key.Binding
	Reveal              key.Binding
}

var Keys = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	CloudPick: key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "switch cloud"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Create: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "create server"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "delete"),
	),
	Reboot: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "soft reboot"),
	),
	HardReboot: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "hard reboot"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "refresh"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Pause: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pause/unpause"),
	),
	Suspend: key.NewBinding(
		key.WithKeys("ctrl+z"),
		key.WithHelp("ctrl+z", "suspend/resume"),
	),
	Shelve: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "shelve/unshelve"),
	),
	Resize: key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "resize"),
	),
	ConfirmResize: key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("ctrl+y", "confirm resize"),
	),
	RevertResize: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "revert resize"),
	),
	Actions: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "action history"),
	),
	Console: key.NewBinding(
		key.WithKeys("L"),
		key.WithHelp("L", "console log"),
	),
	AuditLog: key.NewBinding(
		key.WithKeys("T"),
		key.WithHelp("T", "audit trail"),
	),
	Select: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "select"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"),
	),
	Deny: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"),
	),
	Restart: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "restart"),
	),
	Attach: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "attach"),
	),
	AssignFIP: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "assign floating IP"),
	),
	Detach: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "detach"),
	),
	Allocate: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "allocate"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdown", "page down"),
	),
	Sort: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "sort"),
	),
	ReverseSort: key.NewBinding(
		key.WithKeys("S"),
		key.WithHelp("S", "reverse sort"),
	),
	ProjectPick: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "switch project"),
	),
	Quota: key.NewBinding(
		key.WithKeys("Q"),
		key.WithHelp("Q", "quotas"),
	),
	StopStart: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "stop/start"),
	),
	Lock: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "lock/unlock"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Rebuild: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "rebuild"),
	),
	Snapshot: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "snapshot"),
	),
	Deactivate: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "deactivate/reactivate"),
	),
	Rescue: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("ctrl+w", "rescue/unrescue"),
	),
	Clone: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clone"),
	),
	JumpVolumes: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "jump to volumes"),
	),
	JumpSecGroups: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "jump to sec groups"),
	),
	JumpNetworks: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "jump to networks"),
	),
	SSH: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "SSH into server"),
	),
	CopySSH: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy SSH command"),
	),
	Copy: key.NewBinding(
		key.WithKeys("Y"),
		key.WithHelp("Y", "copy field..."),
	),
	ConsoleURL: key.NewBinding(
		key.WithKeys("V"),
		key.WithHelp("V", "console URL (noVNC)"),
	),
	GetPassword: key.NewBinding(
		key.WithKeys("W"),
		key.WithHelp("W", "admin password"),
	),
	Config: key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "config"),
	),
	Hypervisors: key.NewBinding(
		key.WithKeys("H"),
		key.WithHelp("H", "hypervisors"),
	),
	SaveFilter: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "save filter"),
	),
	LoadFilter: key.NewBinding(
		key.WithKeys("F"),
		key.WithHelp("F", "load filter"),
	),
	Browse: key.NewBinding(
		key.WithKeys("B"),
		key.WithHelp("B", "browse catalog"),
	),
	AdminActions: key.NewBinding(
		key.WithKeys("A"),
		key.WithHelp("A", "admin actions"),
	),
	Metadata: key.NewBinding(
		key.WithKeys("M"),
		key.WithHelp("M", "metadata"),
	),
	UserManagement: key.NewBinding(
		key.WithKeys("U"),
		key.WithHelp("U", "user management"),
	),
	ColumnPick: key.NewBinding(
		key.WithKeys("O"),
		key.WithHelp("O", "columns"),
	),
	Submit: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "submit"),
	),
	HealthMonitor: key.NewBinding(
		key.WithKeys("ctrl+h"),
		key.WithHelp("ctrl+h", "health monitor"),
	),
	DrainMember: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "drain member"),
	),
	SelectAll: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "select all"),
	),
	Download: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "download"),
	),
	Background: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "background"),
	),
	ToggleDetails: key.NewBinding(
		key.WithKeys("d", "D"),
		key.WithHelp("d", "details"),
	),
	MetadataAdd: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add metadata"),
	),
	MetadataEdit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit metadata"),
	),
	MetadataDelete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete metadata"),
	),
	SaveKey: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "save private key"),
	),
	JumpStart: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "jump to start"),
	),
	JumpEnd: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "jump to end"),
	),
	AuditFilterAction: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "filter by action"),
	),
	AuditFilterResource: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "filter by resource"),
	),
	AuditFilterDate: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "filter by date"),
	),
	AuditFilterClear: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clear filter"),
	),
	CopyValue: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "copy"),
	),
	Reveal: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reveal/hide"),
	),
}

// TextKeys are the editing keys of the hand-rolled filter inputs. They mirror
// the editing keys of bubbles' textinput, which are not configurable either,
// so they are deliberately absent from the keybinding config.
var TextKeys = struct {
	DeleteChar key.Binding
	ClearLine  key.Binding
}{
	DeleteChar: key.NewBinding(key.WithKeys("backspace", "delete")),
	ClearLine:  key.NewBinding(key.WithKeys("ctrl+u")),
}

// MatchesNonText reports whether msg matches b and is not a single printable
// character. Pickers with a type-to-filter input navigate with it, so a
// letter in the binding (k/j by default) still reaches the filter as text,
// while arrows and any non-character key the user binds still navigate.
func MatchesNonText(msg tea.KeyMsg, b key.Binding) bool {
	return key.Matches(msg, b) && utf8.RuneCountInString(msg.String()) != 1
}

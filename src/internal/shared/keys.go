package shared

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Quit       key.Binding
	Help       key.Binding
	CloudPick  key.Binding
	Filter     key.Binding
	Enter      key.Binding
	Back       key.Binding
	Create     key.Binding
	Delete     key.Binding
	Reboot     key.Binding
	HardReboot key.Binding
	Refresh    key.Binding
	Up         key.Binding
	Down       key.Binding
	Left       key.Binding
	Right      key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Pause      key.Binding
	Suspend    key.Binding
	Shelve     key.Binding
	Resize        key.Binding
	ConfirmResize key.Binding
	RevertResize  key.Binding
	Actions       key.Binding
	Console       key.Binding
	Select     key.Binding
	Confirm    key.Binding
	Deny       key.Binding
	Restart    key.Binding
	Attach     key.Binding
	Detach     key.Binding
	Allocate   key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Sort        key.Binding
	ReverseSort key.Binding
	ProjectPick key.Binding
	Quota       key.Binding
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
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "attach"),
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
}

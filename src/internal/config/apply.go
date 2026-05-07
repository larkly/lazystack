package config

import (
	"strings"

	"github.com/larkly/lazystack/internal/shared"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// ApplyAll applies all config sections to the shared globals.
func ApplyAll(cfg Config) {
	shared.Debugf("[config] ApplyAll: start")
	ApplyGeneral(cfg.General)
	ApplyColors(cfg.Colors)
	ApplyKeybindings(cfg.Keybindings)
	shared.Debugf("[config] ApplyAll: done")
}

// ApplyGeneral sets shared.PlainMode from config.
func ApplyGeneral(g GeneralConfig) {
	shared.PlainMode = g.PlainMode
}

// ApplyColors sets shared.Color* vars and rebuilds shared.Style* vars.
func ApplyColors(c ColorConfig) {
	shared.Debugf("[config] ApplyColors: start")
	if c.Primary != "" {
		shared.ColorPrimary = lipgloss.Color(c.Primary)
	}
	if c.Secondary != "" {
		shared.ColorSecondary = lipgloss.Color(c.Secondary)
	}
	if c.Success != "" {
		shared.ColorSuccess = lipgloss.Color(c.Success)
	}
	if c.Warning != "" {
		shared.ColorWarning = lipgloss.Color(c.Warning)
	}
	if c.Error != "" {
		shared.ColorError = lipgloss.Color(c.Error)
	}
	if c.Muted != "" {
		shared.ColorMuted = lipgloss.Color(c.Muted)
	}
	if c.Bg != "" {
		shared.ColorBg = lipgloss.Color(c.Bg)
	}
	if c.Fg != "" {
		shared.ColorFg = lipgloss.Color(c.Fg)
	}
	if c.Highlight != "" {
		shared.ColorHighlight = lipgloss.Color(c.Highlight)
	}
	if c.Cyan != "" {
		shared.ColorCyan = lipgloss.Color(c.Cyan)
	}

	shared.RebuildStyles()
	shared.Debugf("[config] ApplyColors: done")
}

// keybindingFieldMap maps config keybinding names to KeyMap field setters.
var keybindingFieldMap = map[string]func(b key.Binding){
	"quit":            func(b key.Binding) { shared.Keys.Quit = b },
	"help":            func(b key.Binding) { shared.Keys.Help = b },
	"cloud_pick":      func(b key.Binding) { shared.Keys.CloudPick = b },
	"filter":          func(b key.Binding) { shared.Keys.Filter = b },
	"enter":           func(b key.Binding) { shared.Keys.Enter = b },
	"back":            func(b key.Binding) { shared.Keys.Back = b },
	"create":          func(b key.Binding) { shared.Keys.Create = b },
	"delete":          func(b key.Binding) { shared.Keys.Delete = b },
	"reboot":          func(b key.Binding) { shared.Keys.Reboot = b },
	"hard_reboot":     func(b key.Binding) { shared.Keys.HardReboot = b },
	"refresh":         func(b key.Binding) { shared.Keys.Refresh = b },
	"up":              func(b key.Binding) { shared.Keys.Up = b },
	"down":            func(b key.Binding) { shared.Keys.Down = b },
	"left":            func(b key.Binding) { shared.Keys.Left = b },
	"right":           func(b key.Binding) { shared.Keys.Right = b },
	"tab":             func(b key.Binding) { shared.Keys.Tab = b },
	"shift_tab":       func(b key.Binding) { shared.Keys.ShiftTab = b },
	"pause":           func(b key.Binding) { shared.Keys.Pause = b },
	"suspend":         func(b key.Binding) { shared.Keys.Suspend = b },
	"shelve":          func(b key.Binding) { shared.Keys.Shelve = b },
	"resize":          func(b key.Binding) { shared.Keys.Resize = b },
	"confirm_resize":  func(b key.Binding) { shared.Keys.ConfirmResize = b },
	"revert_resize":   func(b key.Binding) { shared.Keys.RevertResize = b },
	"actions":         func(b key.Binding) { shared.Keys.Actions = b },
	"console":         func(b key.Binding) { shared.Keys.Console = b },
	"select":          func(b key.Binding) { shared.Keys.Select = b },
	"confirm":         func(b key.Binding) { shared.Keys.Confirm = b },
	"deny":            func(b key.Binding) { shared.Keys.Deny = b },
	"restart":         func(b key.Binding) { shared.Keys.Restart = b },
	"attach":          func(b key.Binding) { shared.Keys.Attach = b },
	"assign_fip":      func(b key.Binding) { shared.Keys.AssignFIP = b },
	"detach":          func(b key.Binding) { shared.Keys.Detach = b },
	"allocate":        func(b key.Binding) { shared.Keys.Allocate = b },
	"page_up":         func(b key.Binding) { shared.Keys.PageUp = b },
	"page_down":       func(b key.Binding) { shared.Keys.PageDown = b },
	"sort":            func(b key.Binding) { shared.Keys.Sort = b },
	"reverse_sort":    func(b key.Binding) { shared.Keys.ReverseSort = b },
	"project_pick":    func(b key.Binding) { shared.Keys.ProjectPick = b },
	"quota":           func(b key.Binding) { shared.Keys.Quota = b },
	"stop_start":      func(b key.Binding) { shared.Keys.StopStart = b },
	"lock":            func(b key.Binding) { shared.Keys.Lock = b },
	"rename":          func(b key.Binding) { shared.Keys.Rename = b },
	"rebuild":         func(b key.Binding) { shared.Keys.Rebuild = b },
	"snapshot":        func(b key.Binding) { shared.Keys.Snapshot = b },
	"deactivate":      func(b key.Binding) { shared.Keys.Deactivate = b },
	"rescue":          func(b key.Binding) { shared.Keys.Rescue = b },
	"clone":           func(b key.Binding) { shared.Keys.Clone = b },
	"jump_volumes":    func(b key.Binding) { shared.Keys.JumpVolumes = b },
	"jump_sec_groups": func(b key.Binding) { shared.Keys.JumpSecGroups = b },
	"jump_networks":   func(b key.Binding) { shared.Keys.JumpNetworks = b },
	"ssh":             func(b key.Binding) { shared.Keys.SSH = b },
	"copy_ssh":        func(b key.Binding) { shared.Keys.CopySSH = b },
	"copy":            func(b key.Binding) { shared.Keys.Copy = b },
	"console_url":     func(b key.Binding) { shared.Keys.ConsoleURL = b },
	"config":          func(b key.Binding) { shared.Keys.Config = b },
	"hypervisors":         func(b key.Binding) { shared.Keys.Hypervisors = b },
	"user_management":     func(b key.Binding) { shared.Keys.UserManagement = b },
}

// defaultHelpText maps config keybinding names to their help descriptions.
var defaultHelpText = map[string]string{
	"quit":            "quit",
	"help":            "help",
	"cloud_pick":      "switch cloud",
	"filter":          "filter",
	"enter":           "select",
	"back":            "back",
	"create":          "create",
	"delete":          "delete",
	"reboot":          "soft reboot",
	"hard_reboot":     "hard reboot",
	"refresh":         "refresh",
	"up":              "up",
	"down":            "down",
	"left":            "left",
	"right":           "right",
	"tab":             "next field",
	"shift_tab":       "prev field",
	"pause":           "pause/unpause",
	"suspend":         "suspend/resume",
	"shelve":          "shelve/unshelve",
	"resize":          "resize",
	"confirm_resize":  "confirm resize",
	"revert_resize":   "revert resize",
	"actions":         "action history",
	"console":         "console log",
	"select":          "select",
	"confirm":         "confirm",
	"deny":            "cancel",
	"restart":         "restart",
	"attach":          "attach",
	"assign_fip":      "assign floating IP",
	"detach":          "detach",
	"allocate":        "allocate",
	"page_up":         "page up",
	"page_down":       "page down",
	"sort":            "sort",
	"reverse_sort":    "reverse sort",
	"project_pick":    "switch project",
	"quota":           "quotas",
	"stop_start":      "stop/start",
	"lock":            "lock/unlock",
	"rename":          "rename",
	"rebuild":         "rebuild",
	"snapshot":        "snapshot",
	"deactivate":      "deactivate/reactivate",
	"rescue":          "rescue/unrescue",
	"clone":           "clone",
	"jump_volumes":    "jump to volumes",
	"jump_sec_groups": "jump to sec groups",
	"jump_networks":   "jump to networks",
	"ssh":             "SSH into server",
	"copy_ssh":        "copy SSH command",
	"copy":            "copy field...",
	"console_url":     "console URL (noVNC)",
	"config":          "config",
	"hypervisors":     "hypervisors",
	"user_management": "user management",
}
// ApplyKeybindings sets shared.Keys fields from the config map.
func ApplyKeybindings(kb map[string]string) {
	shared.Debugf("[config] ApplyKeybindings: start bindings=%d", len(kb))
	if kb == nil {
		shared.Debugf("[config] ApplyKeybindings: nil map, skipping")
		return
	}
	for name, keys := range kb {
		setter, ok := keybindingFieldMap[name]
		if !ok {
			continue
		}
		keyList := strings.Split(keys, ",")
		for i := range keyList {
			keyList[i] = strings.TrimSpace(keyList[i])
		}

		helpText := name
		if ht, ok := defaultHelpText[name]; ok {
			helpText = ht
		}

		helpKey := keys
		if len(keyList) > 0 {
			helpKey = keyList[0]
		}

		setter(key.NewBinding(
			key.WithKeys(keyList...),
			key.WithHelp(helpKey, helpText),
		))
	}
	shared.Debugf("[config] ApplyKeybindings: done")
}

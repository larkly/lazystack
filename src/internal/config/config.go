package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/larkly/lazystack/internal/shared"
	"gopkg.in/yaml.v3"
)

// ColumnConfig holds a single column's key and visible flag.
type ColumnConfig struct {
	Key    string `yaml:"key"`
	Hidden bool   `yaml:"hidden"`
}

// Config is the full persisted configuration.
type Config struct {
	General      GeneralConfig     `yaml:"general"`
	Colors       ColorConfig       `yaml:"colors"`
	Keybindings  map[string]string `yaml:"keybindings,omitempty"`
	SavedFilters []SavedFilter     `yaml:"saved_filters,omitempty"`
	Columns      []ColumnConfig    `yaml:"columns,omitempty"`
}

// SavedFilter is a named filter query for quick server list filtering.
type SavedFilter struct {
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"`
}

// GeneralConfig holds non-visual, non-keybinding settings.
type GeneralConfig struct {
	RefreshInterval     int  `yaml:"refresh_interval"`
	IdleTimeout         int  `yaml:"idle_timeout"`
	PlainMode           bool `yaml:"plain_mode"`
	CheckForUpdates     bool `yaml:"check_for_updates"`
	AlwaysPickCloud     bool `yaml:"always_pick_cloud"`
	IgnoreSSHHostKeys   bool `yaml:"ignore_ssh_host_keys"`
	UpdateCheckInterval int  `yaml:"update_check_interval"`
}

// ColorConfig holds hex color strings for the UI palette.
type ColorConfig struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Success   string `yaml:"success"`
	Warning   string `yaml:"warning"`
	Error     string `yaml:"error"`
	Muted     string `yaml:"muted"`
	Bg        string `yaml:"bg"`
	Fg        string `yaml:"fg"`
	Highlight string `yaml:"highlight"`
	Cyan      string `yaml:"cyan"`
}

// CLIFlags captures which CLI flags were explicitly set.
// Nil pointers mean "not set by user" (use config file value).
type CLIFlags struct {
	RefreshInterval *time.Duration
	IdleTimeout     *time.Duration
	PlainMode       *bool
	CheckForUpdates *bool
	AlwaysPickCloud *bool
	Cloud           string
}

// DefaultPath returns ~/.config/lazystack/config.yaml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "lazystack", "config.yaml")
}

// Defaults returns the hardcoded default configuration.
func Defaults() Config {
	return Config{
		General: GeneralConfig{
			RefreshInterval: 5,
			IdleTimeout:     0,
			PlainMode:       false,
			CheckForUpdates:     true,
			AlwaysPickCloud:     false,
			IgnoreSSHHostKeys:   false,
			UpdateCheckInterval: 24,
		},
		Colors: ColorConfig{
			Primary:   "#7D56F4",
			Secondary: "#6C71C4",
			Success:   "#2AA198",
			Warning:   "#B58900",
			Error:     "#DC322F",
			Muted:     "#657B83",
			Bg:        "#002B36",
			Fg:        "#839496",
			Highlight: "#FDF6E3",
			Cyan:      "#2AA198",
		},
		Keybindings: DefaultKeybindings(),
	}
}

// DefaultKeybindings returns the default key bindings map.
func DefaultKeybindings() map[string]string {
	return map[string]string{
		"quit":           "q,ctrl+c",
		"help":           "?",
		"cloud_pick":     "C",
		"filter":         "/",
		"enter":          "enter",
		"back":           "esc",
		"create":         "ctrl+n",
		"delete":         "ctrl+d",
		"reboot":         "ctrl+o",
		"hard_reboot":    "ctrl+p",
		"refresh":        "R",
		"up":             "up,k",
		"down":           "down,j",
		"left":           "left,h",
		"right":          "right,l",
		"tab":            "tab",
		"shift_tab":      "shift+tab",
		"pause":          "p",
		"suspend":        "ctrl+z",
		"shelve":         "ctrl+e",
		"resize":         "ctrl+f",
		"confirm_resize": "ctrl+y",
		"revert_resize":  "ctrl+x",
		"actions":        "a",
		"console":        "L",
		"select":         "space",
		"confirm":        "y",
		"deny":           "n",
		"restart":        "ctrl+r",
		"attach":         "ctrl+a",
		"assign_fip":     "ctrl+u",
		"detach":         "ctrl+t",
		"allocate":       "ctrl+n",
		"page_up":        "pgup",
		"page_down":      "pgdown",
		"sort":           "s",
		"reverse_sort":   "S",
		"project_pick":   "P",
		"quota":          "Q",
		"stop_start":     "o",
		"lock":           "ctrl+l",
		"rename":         "r",
		"rebuild":        "ctrl+g",
		"snapshot":       "ctrl+s",
		"deactivate":     "d",
		"rescue":         "ctrl+w",
		"clone":          "c",
		"jump_volumes":   "v",
		"jump_sec_groups": "g",
		"jump_networks":  "N",
		"ssh":            "x",
		"copy_ssh":       "y",
		"console_url":    "V",
		"config":         "ctrl+k",
		"column_pick":    "ctrl+shift+c",
	}
}

// Load reads config from DefaultPath. Returns Defaults() if file does not exist.
func Load() (Config, error) {
	shared.Debugf("[config] Load: start path=%s", DefaultPath())
	return LoadFrom(DefaultPath())
}

// rawGeneral mirrors GeneralConfig with pointer bools to detect presence in YAML.
type rawGeneral struct {
	RefreshInterval     int   `yaml:"refresh_interval"`
	IdleTimeout         int   `yaml:"idle_timeout"`
	PlainMode           *bool `yaml:"plain_mode"`
	CheckForUpdates     *bool `yaml:"check_for_updates"`
	AlwaysPickCloud     *bool `yaml:"always_pick_cloud"`
	IgnoreSSHHostKeys   *bool `yaml:"ignore_ssh_host_keys"`
	UpdateCheckInterval *int  `yaml:"update_check_interval"`
}

type rawConfig struct {
	General     rawGeneral        `yaml:"general"`
	Colors      ColorConfig       `yaml:"colors"`
	Keybindings map[string]string `yaml:"keybindings,omitempty"`
	Columns     []ColumnConfig    `yaml:"columns,omitempty"`
}

// LoadFrom reads config from the given path.
func LoadFrom(path string) (Config, error) {
	shared.Debugf("[config] LoadFrom: start path=%s", path)
	defaults := Defaults()
	if path == "" {
		shared.Debugf("[config] LoadFrom: empty path, using defaults")
		return defaults, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			shared.Debugf("[config] LoadFrom: file not found, using defaults")
			return defaults, nil
		}
		shared.Debugf("[config] LoadFrom: error reading file: %v", err)
		return defaults, err
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		shared.Debugf("[config] LoadFrom: error parsing YAML: %v", err)
		return defaults, err
	}
	shared.Debugf("[config] LoadFrom: loaded config from %s", path)

	file := Config{
		General: GeneralConfig{
			RefreshInterval: raw.General.RefreshInterval,
			IdleTimeout:     raw.General.IdleTimeout,
		},
		Colors:      raw.Colors,
		Keybindings: raw.Keybindings,
		Columns:     raw.Columns,
	}

	// Use raw pointer bools to distinguish "explicitly false" from "absent".
	if raw.General.PlainMode != nil {
		file.General.PlainMode = *raw.General.PlainMode
	} else {
		file.General.PlainMode = defaults.General.PlainMode
	}
	if raw.General.CheckForUpdates != nil {
		file.General.CheckForUpdates = *raw.General.CheckForUpdates
	} else {
		file.General.CheckForUpdates = defaults.General.CheckForUpdates
	}
	if raw.General.AlwaysPickCloud != nil {
		file.General.AlwaysPickCloud = *raw.General.AlwaysPickCloud
	} else {
		file.General.AlwaysPickCloud = defaults.General.AlwaysPickCloud
	}
	if raw.General.IgnoreSSHHostKeys != nil {
		file.General.IgnoreSSHHostKeys = *raw.General.IgnoreSSHHostKeys
	} else {
		file.General.IgnoreSSHHostKeys = defaults.General.IgnoreSSHHostKeys
	}
	if raw.General.UpdateCheckInterval != nil {
		file.General.UpdateCheckInterval = *raw.General.UpdateCheckInterval
	} else {
		file.General.UpdateCheckInterval = defaults.General.UpdateCheckInterval
	}

	return mergeWithDefaults(file, defaults), nil
}

// mergeWithDefaults fills zero-valued fields in file with defaults.
// Bool fields are handled in LoadFrom via rawGeneral pointer detection.
func mergeWithDefaults(file, defaults Config) Config {
	if file.General.RefreshInterval == 0 {
		file.General.RefreshInterval = defaults.General.RefreshInterval
	}
	if file.General.UpdateCheckInterval == 0 {
		file.General.UpdateCheckInterval = defaults.General.UpdateCheckInterval
	}

	if file.Colors.Primary == "" {
		file.Colors.Primary = defaults.Colors.Primary
	}
	if file.Colors.Secondary == "" {
		file.Colors.Secondary = defaults.Colors.Secondary
	}
	if file.Colors.Success == "" {
		file.Colors.Success = defaults.Colors.Success
	}
	if file.Colors.Warning == "" {
		file.Colors.Warning = defaults.Colors.Warning
	}
	if file.Colors.Error == "" {
		file.Colors.Error = defaults.Colors.Error
	}
	if file.Colors.Muted == "" {
		file.Colors.Muted = defaults.Colors.Muted
	}
	if file.Colors.Bg == "" {
		file.Colors.Bg = defaults.Colors.Bg
	}
	if file.Colors.Fg == "" {
		file.Colors.Fg = defaults.Colors.Fg
	}
	if file.Colors.Highlight == "" {
		file.Colors.Highlight = defaults.Colors.Highlight
	}
	if file.Colors.Cyan == "" {
		file.Colors.Cyan = defaults.Colors.Cyan
	}

	if file.Keybindings == nil {
		file.Keybindings = defaults.Keybindings
	} else {
		for k, v := range defaults.Keybindings {
			if _, ok := file.Keybindings[k]; !ok {
				file.Keybindings[k] = v
			}
		}
	}

	return file
}

// Merge applies CLI flag overrides on top of file config.
// CLI flags take precedence when explicitly set (non-nil pointers).
func Merge(file Config, flags CLIFlags) Config {
	if flags.RefreshInterval != nil {
		file.General.RefreshInterval = int(flags.RefreshInterval.Seconds())
	}
	if flags.IdleTimeout != nil {
		file.General.IdleTimeout = int(flags.IdleTimeout.Minutes())
	}
	if flags.PlainMode != nil {
		file.General.PlainMode = *flags.PlainMode
	}
	if flags.CheckForUpdates != nil {
		file.General.CheckForUpdates = *flags.CheckForUpdates
	}
	if flags.AlwaysPickCloud != nil {
		file.General.AlwaysPickCloud = *flags.AlwaysPickCloud
	}
	return file
}

// Save writes config to DefaultPath, creating directories as needed.
func (c *Config) Save() error {
	shared.Debugf("[config] Save: start path=%s", DefaultPath())
	return c.SaveTo(DefaultPath())
}

// SaveTo writes config to the given path.
func (c *Config) SaveTo(path string) error {
	shared.Debugf("[config] SaveTo: start path=%s", path)
	if path == "" {
		shared.Debugf("[config] SaveTo: error empty path")
		return errors.New("config: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		shared.Debugf("[config] SaveTo: error creating dir: %v", err)
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		shared.Debugf("[config] SaveTo: error marshaling: %v", err)
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		shared.Debugf("[config] SaveTo: error writing: %v", err)
		return err
	}
	shared.Debugf("[config] SaveTo: success")
	return nil
}

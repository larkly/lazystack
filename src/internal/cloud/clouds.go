package cloud

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/larkly/lazystack/internal/shared"

	"gopkg.in/yaml.v3"
)

// systemCloudsYaml is the system-wide clouds.yaml. It is a variable so tests
// can keep the host's /etc out of the search.
var systemCloudsYaml = "/etc/openstack/clouds.yaml"

// cloudsFile represents the top-level structure of clouds.yaml.
type cloudsFile struct {
	Clouds map[string]interface{} `yaml:"clouds"`
}

// ListCloudNames parses clouds.yaml and returns sorted cloud names from the
// file chosen by selectCloudsYaml, the same file Connect authenticates with.
func ListCloudNames() ([]string, error) {
	shared.Debugf("[cloud] ListCloudNames: starting")
	path, names, err := selectCloudsYaml()
	if err != nil {
		shared.Debugf("[cloud] ListCloudNames: %v", err)
		return nil, err
	}
	shared.Debugf("[cloud] ListCloudNames: success, count=%d from=%s", len(names), path)
	return names, nil
}

// selectCloudsYaml picks the clouds.yaml to use and returns its path and
// sorted cloud names. Listing and connecting both go through it so they can
// never disagree about which file is in effect.
//
// When OS_CLIENT_CONFIG_FILE is set it is the only candidate: if it is
// missing, unreadable, malformed or defines no clouds, selection fails
// instead of falling back to another (possibly planted) file.
//
// Otherwise CloudsYamlPaths is walked in order. Missing files and parseable
// files with zero clouds are skipped, so an empty config does not shadow a
// real one further down the list. A file that exists but cannot be read or
// parsed aborts the search with an error rather than being silently bypassed.
func selectCloudsYaml() (string, []string, error) {
	if override := os.Getenv("OS_CLIENT_CONFIG_FILE"); override != "" {
		names, err := readCloudNames(override)
		if err != nil {
			return "", nil, fmt.Errorf("OS_CLIENT_CONFIG_FILE: %w", err)
		}
		if len(names) == 0 {
			return "", nil, fmt.Errorf("OS_CLIENT_CONFIG_FILE: %s defines no clouds", override)
		}
		return override, names, nil
	}

	paths := CloudsYamlPaths()
	for _, p := range paths {
		names, err := readCloudNames(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		if len(names) == 0 {
			shared.Debugf("[cloud] selectCloudsYaml: %s contains no clouds, continuing search", p)
			continue
		}
		return p, names, nil
	}
	return "", nil, fmt.Errorf("no usable clouds.yaml found (no clouds defined; searched: %v)", paths)
}

// readCloudNames reads and parses a clouds.yaml, returning its sorted cloud
// names. Read errors are returned unwrapped (they already name the path) so
// callers can test for fs.ErrNotExist.
func readCloudNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf cloudsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	names := make([]string, 0, len(cf.Clouds))
	for name := range cf.Clouds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// CloudsYamlPaths returns the list of paths searched for clouds.yaml, in
// priority order. When $OS_CLIENT_CONFIG_FILE is set it is the only entry
// (an explicit override is exclusive, as in python-openstackclient).
// Otherwise:
//  1. $XDG_CONFIG_HOME/openstack/clouds.yaml, or ~/.config/openstack/clouds.yaml
//     when XDG_CONFIG_HOME is unset (per-user config)
//  2. /etc/openstack/clouds.yaml (system-wide config)
//  3. ./clouds.yaml (current working directory)
//
// The current directory is searched LAST: trusting a clouds.yaml found in
// whatever directory the user happens to be in is a credential-phishing
// surface (a planted file would silently override the real configuration).
func CloudsYamlPaths() []string {
	if env := os.Getenv("OS_CLIENT_CONFIG_FILE"); env != "" {
		return []string{env}
	}

	var paths []string
	if dir := userConfigDir(); dir != "" {
		paths = append(paths, filepath.Join(dir, "openstack", "clouds.yaml"))
	}
	paths = append(paths, systemCloudsYaml)

	// ./clouds.yaml — demoted to last, see doc comment
	paths = append(paths, "clouds.yaml")

	return paths
}

// userConfigDir returns $XDG_CONFIG_HOME, or ~/.config when it is unset. A
// relative XDG_CONFIG_HOME is invalid per the XDG spec and is ignored.
func userConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return xdg
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config")
	}
	return ""
}

package cloud

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/larkly/lazystack/internal/shared"

	"gopkg.in/yaml.v3"
)

// cloudsFile represents the top-level structure of clouds.yaml.
type cloudsFile struct {
	Clouds map[string]interface{} `yaml:"clouds"`
}

// ListCloudNames parses clouds.yaml and returns sorted cloud names.
// The search walks CloudsYamlPaths in order and stops at the first file that
// exists and defines at least one cloud. A parseable file with zero clouds is
// skipped so an empty config does not shadow a real one further down the list;
// only a file that exists but fails to parse aborts the search with an error.
func ListCloudNames() ([]string, error) {
	shared.Debugf("[cloud] ListCloudNames: starting")
	paths := CloudsYamlPaths()

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var cf cloudsFile
		if err := yaml.Unmarshal(data, &cf); err != nil {
			shared.Debugf("[cloud] ListCloudNames: error parsing %s: %v", p, err)
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}

		if len(cf.Clouds) == 0 {
			shared.Debugf("[cloud] ListCloudNames: %s contains no clouds, continuing search", p)
			continue
		}

		names := make([]string, 0, len(cf.Clouds))
		for name := range cf.Clouds {
			names = append(names, name)
		}
		sort.Strings(names)
		shared.Debugf("[cloud] ListCloudNames: success, count=%d from=%s", len(names), p)
		return names, nil
	}

	shared.Debugf("[cloud] ListCloudNames: no usable clouds.yaml found")
	return nil, fmt.Errorf("no usable clouds.yaml found (no clouds defined; searched: %v)", paths)
}

// CloudsYamlPaths returns the list of paths searched for clouds.yaml, in
// priority order:
//  1. $OS_CLIENT_CONFIG_FILE (explicit user override)
//  2. ~/.config/openstack/clouds.yaml (per-user config)
//  3. /etc/openstack/clouds.yaml (system-wide config)
//  4. ./clouds.yaml (current working directory)
//
// The current directory is searched LAST: trusting a clouds.yaml found in
// whatever directory the user happens to be in is a credential-phishing
// surface (a planted file would silently override the real configuration),
// and python-openstackclient does not search the CWD either.
func CloudsYamlPaths() []string {
	var paths []string

	// OS_CLIENT_CONFIG_FILE — explicit override, always first
	if env := os.Getenv("OS_CLIENT_CONFIG_FILE"); env != "" {
		paths = append(paths, env)
	}

	// ~/.config/openstack/clouds.yaml
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "openstack", "clouds.yaml"))
	}

	// /etc/openstack/clouds.yaml
	paths = append(paths, "/etc/openstack/clouds.yaml")

	// ./clouds.yaml — demoted to last, see doc comment
	paths = append(paths, "clouds.yaml")

	return paths
}

package cloud

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// cloudsFile represents the top-level structure of clouds.yaml.
type cloudsFile struct {
	Clouds map[string]interface{} `yaml:"clouds"`
}

// ListCloudNames parses clouds.yaml and returns sorted cloud names.
func ListCloudNames() ([]string, error) {
	paths := CloudsYamlPaths()

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var cf cloudsFile
		if err := yaml.Unmarshal(data, &cf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}

		names := make([]string, 0, len(cf.Clouds))
		for name := range cf.Clouds {
			names = append(names, name)
		}
		sort.Strings(names)
		return names, nil
	}

	return nil, fmt.Errorf("no clouds.yaml found (searched: %v)", paths)
}

// CloudsYamlPaths returns the list of paths searched for clouds.yaml.
func CloudsYamlPaths() []string {
	var paths []string

	// Current directory
	paths = append(paths, "clouds.yaml")

	// OS_CLIENT_CONFIG_FILE
	if env := os.Getenv("OS_CLIENT_CONFIG_FILE"); env != "" {
		paths = append(paths, env)
	}

	// ~/.config/openstack/clouds.yaml
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "openstack", "clouds.yaml"))
	}

	// /etc/openstack/clouds.yaml
	paths = append(paths, "/etc/openstack/clouds.yaml")

	return paths
}

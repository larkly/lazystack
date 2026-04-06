package cloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListCloudNames(t *testing.T) {
	// Create a temp clouds.yaml
	dir := t.TempDir()
	content := `clouds:
  prod:
    auth:
      auth_url: https://prod.example.com:5000
  staging:
    auth:
      auth_url: https://staging.example.com:5000
  dev:
    auth:
      auth_url: https://dev.example.com:5000
`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Point to our temp file
	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(names) != 3 {
		t.Fatalf("expected 3 clouds, got %d: %v", len(names), names)
	}

	// Should be sorted
	expected := []string{"dev", "prod", "staging"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected %s at index %d, got %s", expected[i], i, name)
		}
	}
}

func TestListCloudNames_Empty(t *testing.T) {
	dir := t.TempDir()
	content := `clouds: {}`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 clouds, got %d", len(names))
	}
}

func TestListCloudNames_NoFile(t *testing.T) {
	// Point to nonexistent file, use empty temp dir as cwd
	dir := t.TempDir()
	t.Setenv("OS_CLIENT_CONFIG_FILE", filepath.Join(dir, "nonexistent.yaml"))
	t.Setenv("HOME", dir) // prevent ~/.config/openstack/clouds.yaml
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := ListCloudNames()
	if err == nil {
		t.Error("expected error when no clouds.yaml exists")
	}
}

func TestCloudsYamlPaths_Order(t *testing.T) {
	t.Setenv("OS_CLIENT_CONFIG_FILE", "/custom/path/clouds.yaml")
	paths := CloudsYamlPaths()

	// First path should always be relative clouds.yaml
	if paths[0] != "clouds.yaml" {
		t.Errorf("first path should be 'clouds.yaml', got %s", paths[0])
	}

	// Second path should be OS_CLIENT_CONFIG_FILE (when set)
	if paths[1] != "/custom/path/clouds.yaml" {
		t.Errorf("second path should be OS_CLIENT_CONFIG_FILE, got %s", paths[1])
	}

	// Last path should be the system-wide path
	if paths[len(paths)-1] != "/etc/openstack/clouds.yaml" {
		t.Errorf("last path should be /etc/openstack/clouds.yaml, got %s", paths[len(paths)-1])
	}
}

func TestCloudsYamlPaths_WithoutEnv(t *testing.T) {
	// Ensure OS_CLIENT_CONFIG_FILE is not set
	t.Setenv("OS_CLIENT_CONFIG_FILE", "")
	paths := CloudsYamlPaths()

	// Should have 3 paths: relative, home, system
	if len(paths) != 3 {
		t.Errorf("expected 3 paths without env, got %d: %v", len(paths), paths)
	}

	if paths[0] != "clouds.yaml" {
		t.Errorf("first path should be 'clouds.yaml', got %s", paths[0])
	}
}

func TestCloudsYamlPaths_WithEnv(t *testing.T) {
	t.Setenv("OS_CLIENT_CONFIG_FILE", "/my/custom/clouds.yaml")
	paths := CloudsYamlPaths()

	// Should have 4 paths: relative, env, home, system
	if len(paths) != 4 {
		t.Errorf("expected 4 paths with env, got %d: %v", len(paths), paths)
	}

	if paths[1] != "/my/custom/clouds.yaml" {
		t.Errorf("second path should be env path, got %s", paths[1])
	}
}

func TestListCloudNames_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  bad:
    [invalid yaml {{{
`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	_, err := ListCloudNames()
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

func TestListCloudNames_NoCloudsKey(t *testing.T) {
	dir := t.TempDir()
	content := `not_clouds:
  foo: bar
`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Missing 'clouds' key means empty map, so 0 names
	if len(names) != 0 {
		t.Errorf("expected 0 clouds when 'clouds' key is missing, got %d", len(names))
	}
}

func TestListCloudNames_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 clouds for empty file, got %d", len(names))
	}
}

func TestListCloudNames_Precedence(t *testing.T) {
	// When OS_CLIENT_CONFIG_FILE points to a valid file, it should be found
	// before the home path
	dir := t.TempDir()
	content := `clouds:
  env_cloud:
    auth:
      auth_url: https://env.example.com:5000
`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)
	t.Setenv("HOME", dir) // ensure home path doesn't interfere

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(names) != 1 || names[0] != "env_cloud" {
		t.Errorf("expected [env_cloud], got %v", names)
	}
}

func TestListCloudNames_DetailedCloudsYaml(t *testing.T) {
	dir := t.TempDir()
	content := `clouds:
  production:
    auth:
      auth_url: https://keystone.prod.example.com:5000
      username: admin
      password: secret
      project_name: ops
      user_domain_name: Default
      project_domain_name: Default
    region_name: RegionOne
    interface: public
  development:
    auth:
      auth_url: https://keystone.dev.example.com:5000/v3
      username: developer
      project_name: dev-team
    region_name: RegionOne
    identity_api_version: "3"
`
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OS_CLIENT_CONFIG_FILE", path)

	names, err := ListCloudNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 clouds, got %d", len(names))
	}

	expected := []string{"development", "production"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected %s at index %d, got %s", expected[i], i, name)
		}
	}
}

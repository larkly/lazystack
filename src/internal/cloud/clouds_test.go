package cloud

import (
	"os"
	"path/filepath"
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

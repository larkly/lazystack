package selfupdate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCache(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "update-check.json")
	origCachePath := CachePath
	CachePath = func() string { return path }
	defer func() { CachePath = origCachePath }()

	entry := CacheEntry{
		CheckedAt:      time.Now().Truncate(time.Second),
		LatestVersion:  "v1.2.0",
		DownloadURL:    "https://example.com/bin",
		ChecksumsURL:   "https://example.com/SHA256SUMS",
		CurrentVersion: "v1.1.0",
	}

	if err := SaveCache(entry); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	got, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if got == nil {
		t.Fatal("LoadCache returned nil")
	}
	if got.LatestVersion != entry.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, entry.LatestVersion)
	}
	if got.CurrentVersion != entry.CurrentVersion {
		t.Errorf("CurrentVersion = %q, want %q", got.CurrentVersion, entry.CurrentVersion)
	}
	if got.DownloadURL != entry.DownloadURL {
		t.Errorf("DownloadURL = %q, want %q", got.DownloadURL, entry.DownloadURL)
	}
}

func TestLoadCache_NotExist(t *testing.T) {
	origCachePath := CachePath
	CachePath = func() string { return filepath.Join(t.TempDir(), "nonexistent.json") }
	defer func() { CachePath = origCachePath }()

	got, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent cache file")
	}
}

func TestCheckLatestCached_UsesCacheWithinTTL(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "update-check.json")
	origCachePath := CachePath
	CachePath = func() string { return path }
	defer func() { CachePath = origCachePath }()

	// Pre-populate cache with a fresh entry
	entry := CacheEntry{
		CheckedAt:      time.Now(),
		LatestVersion:  "v2.0.0",
		DownloadURL:    "https://example.com/bin",
		ChecksumsURL:   "https://example.com/SHA256SUMS",
		CurrentVersion: "v1.0.0",
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(path, data, 0o600)

	// Override checkFn to track if CheckLatest is called
	called := false
	origCheckFn := checkFn
	checkFn = func(_ context.Context, ver string) (string, string, string, error) {
		called = true
		return "", "", "", nil
	}
	defer func() { checkFn = origCheckFn }()

	latest, _, _, err := CheckLatestCached(context.Background(), "v1.0.0", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckLatestCached: %v", err)
	}
	if called {
		t.Error("expected CheckLatest not to be called when cache is fresh")
	}
	if latest != "v2.0.0" {
		t.Errorf("latest = %q, want %q", latest, "v2.0.0")
	}
}

func TestCheckLatestCached_RefreshesExpiredCache(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "update-check.json")
	origCachePath := CachePath
	CachePath = func() string { return path }
	defer func() { CachePath = origCachePath }()

	// Pre-populate cache with an expired entry
	entry := CacheEntry{
		CheckedAt:      time.Now().Add(-48 * time.Hour),
		LatestVersion:  "v1.5.0",
		CurrentVersion: "v1.0.0",
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(path, data, 0o600)

	called := false
	origCheckFn := checkFn
	checkFn = func(_ context.Context, ver string) (string, string, string, error) {
		called = true
		return "v2.0.0", "https://example.com/bin2", "https://example.com/SHA256SUMS2", nil
	}
	defer func() { checkFn = origCheckFn }()

	latest, _, _, err := CheckLatestCached(context.Background(), "v1.0.0", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckLatestCached: %v", err)
	}
	if !called {
		t.Error("expected CheckLatest to be called when cache is expired")
	}
	if latest != "v2.0.0" {
		t.Errorf("latest = %q, want %q", latest, "v2.0.0")
	}
}

func TestCheckLatestCached_InvalidatesOnVersionChange(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "update-check.json")
	origCachePath := CachePath
	CachePath = func() string { return path }
	defer func() { CachePath = origCachePath }()

	// Cache was written for v1.0.0 but we're now running v1.5.0
	entry := CacheEntry{
		CheckedAt:      time.Now(),
		LatestVersion:  "v2.0.0",
		CurrentVersion: "v1.0.0",
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(path, data, 0o600)

	called := false
	origCheckFn := checkFn
	checkFn = func(_ context.Context, ver string) (string, string, string, error) {
		called = true
		return "v2.0.0", "https://example.com/bin", "", nil
	}
	defer func() { checkFn = origCheckFn }()

	_, _, _, err := CheckLatestCached(context.Background(), "v1.5.0", 24*time.Hour)
	if err != nil {
		t.Fatalf("CheckLatestCached: %v", err)
	}
	if !called {
		t.Error("expected CheckLatest to be called when current version differs from cached")
	}
}

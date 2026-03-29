package selfupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// CacheEntry holds the result of a previous update check.
type CacheEntry struct {
	CheckedAt      time.Time `json:"checked_at"`
	LatestVersion  string    `json:"latest_version"`
	DownloadURL    string    `json:"download_url"`
	ChecksumsURL   string    `json:"checksums_url"`
	CurrentVersion string    `json:"current_version"`
}

// CachePath returns the path to the update-check cache file.
// It is a variable so tests can override it.
var CachePath = func() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lazystack", "update-check.json")
}

// checkFn is the function used to query for the latest version.
// It is a variable so tests can override it.
var checkFn = CheckLatest

// LoadCache reads the cached update-check result from disk.
// Returns nil, nil if the file does not exist.
func LoadCache() (*CacheEntry, error) {
	path := CachePath()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// SaveCache writes the update-check result to disk.
func SaveCache(entry CacheEntry) error {
	path := CachePath()
	if path == "" {
		return errors.New("cannot determine cache directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// CheckLatestCached wraps CheckLatest with a disk cache to avoid
// hitting the GitHub API on every launch. The API is only queried
// if the cache is missing, expired (older than ttl), or was written
// for a different binary version (i.e. the user upgraded via their
// package manager).
func CheckLatestCached(currentVersion string, ttl time.Duration) (latest, downloadURL, checksumsURL string, err error) {
	cache, _ := LoadCache()
	if cache != nil && cache.CurrentVersion == currentVersion && time.Since(cache.CheckedAt) < ttl {
		if cache.LatestVersion == "" {
			return "", "", "", nil
		}
		return cache.LatestVersion, cache.DownloadURL, cache.ChecksumsURL, nil
	}

	latest, downloadURL, checksumsURL, err = checkFn(currentVersion)
	if err != nil {
		return "", "", "", err
	}

	_ = SaveCache(CacheEntry{
		CheckedAt:      time.Now(),
		LatestVersion:  latest,
		DownloadURL:    downloadURL,
		ChecksumsURL:   checksumsURL,
		CurrentVersion: currentVersion,
	})

	return latest, downloadURL, checksumsURL, nil
}

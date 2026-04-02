package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/larkly/lazystack/internal/shared"
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
var checkFn func(context.Context, string) (string, string, string, error) = CheckLatest // overridable for tests

// LoadCache reads the cached update-check result from disk.
// Returns nil, nil if the file does not exist.
func LoadCache() (*CacheEntry, error) {
	shared.Debugf("[selfupdate] LoadCache: start")
	path := CachePath()
	if path == "" {
		shared.Debugf("[selfupdate] LoadCache: no cache path")
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			shared.Debugf("[selfupdate] LoadCache: miss (file not found)")
			return nil, nil
		}
		shared.Debugf("[selfupdate] LoadCache: error reading: %v", err)
		return nil, err
	}
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		shared.Debugf("[selfupdate] LoadCache: error unmarshaling: %v", err)
		return nil, err
	}
	shared.Debugf("[selfupdate] LoadCache: hit version=%s checkedAt=%s", entry.LatestVersion, entry.CheckedAt)
	return &entry, nil
}

// SaveCache writes the update-check result to disk.
func SaveCache(entry CacheEntry) error {
	shared.Debugf("[selfupdate] SaveCache: start version=%s", entry.LatestVersion)
	path := CachePath()
	if path == "" {
		shared.Debugf("[selfupdate] SaveCache: error no cache path")
		return errors.New("cannot determine cache directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		shared.Debugf("[selfupdate] SaveCache: error creating dir: %v", err)
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		shared.Debugf("[selfupdate] SaveCache: error marshaling: %v", err)
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		shared.Debugf("[selfupdate] SaveCache: error writing: %v", err)
		return err
	}
	shared.Debugf("[selfupdate] SaveCache: success")
	return nil
}

// CheckLatestCached wraps CheckLatest with a disk cache to avoid
// hitting the GitHub API on every launch. The API is only queried
// if the cache is missing, expired (older than ttl), or was written
// for a different binary version (i.e. the user upgraded via their
// package manager).
func CheckLatestCached(ctx context.Context, currentVersion string, ttl time.Duration) (latest, downloadURL, checksumsURL string, err error) {
	shared.Debugf("[selfupdate] CheckLatestCached: start currentVersion=%s ttl=%s", currentVersion, ttl)
	cache, _ := LoadCache()
	if cache != nil && cache.CurrentVersion == currentVersion && time.Since(cache.CheckedAt) < ttl {
		shared.Debugf("[selfupdate] CheckLatestCached: using cached result (age=%s)", time.Since(cache.CheckedAt))
		if cache.LatestVersion == "" {
			shared.Debugf("[selfupdate] CheckLatestCached: cache says up to date")
			return "", "", "", nil
		}
		shared.Debugf("[selfupdate] CheckLatestCached: cache says latest=%s", cache.LatestVersion)
		return cache.LatestVersion, cache.DownloadURL, cache.ChecksumsURL, nil
	}

	shared.Debugf("[selfupdate] CheckLatestCached: cache miss or expired, querying API")
	latest, downloadURL, checksumsURL, err = checkFn(ctx, currentVersion)
	if err != nil {
		shared.Debugf("[selfupdate] CheckLatestCached: error from API: %v", err)
		return "", "", "", err
	}

	_ = SaveCache(CacheEntry{
		CheckedAt:      time.Now(),
		LatestVersion:  latest,
		DownloadURL:    downloadURL,
		ChecksumsURL:   checksumsURL,
		CurrentVersion: currentVersion,
	})

	shared.Debugf("[selfupdate] CheckLatestCached: result latest=%s", latest)
	return latest, downloadURL, checksumsURL, nil
}

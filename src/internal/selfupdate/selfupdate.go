package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
)

const releaseAPI = "https://api.github.com/repos/larkly/lazystack/releases/latest"

// httpClient is used for API/metadata requests (30s timeout).
var httpClient = &http.Client{Timeout: 30 * time.Second}

// downloadClient is used for binary downloads (5 minute timeout).
var downloadClient = &http.Client{Timeout: 5 * time.Minute}

// githubRelease is the subset of the GitHub release API response we need.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset is a single asset in a GitHub release.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckLatest checks GitHub for a newer release. Returns empty strings if
// already up to date. Returns an error if currentVersion is "dev".
func CheckLatest(ctx context.Context, currentVersion string) (latest, downloadURL, checksumsURL string, err error) {
	shared.Debugf("[selfupdate] CheckLatest: start currentVersion=%s", currentVersion)
	if currentVersion == "dev" {
		shared.Debugf("[selfupdate] CheckLatest: error dev build")
		return "", "", "", errors.New("cannot check for updates on a dev build; build with -ldflags \"-X main.version=vX.Y.Z\"")
	}

	body, err := httpGet(ctx, releaseAPI)
	if err != nil {
		shared.Debugf("[selfupdate] CheckLatest: error fetching release: %v", err)
		return "", "", "", fmt.Errorf("fetching latest release: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		shared.Debugf("[selfupdate] CheckLatest: error parsing release JSON: %v", err)
		return "", "", "", fmt.Errorf("parsing release response: %w", err)
	}

	if release.TagName == "" {
		shared.Debugf("[selfupdate] CheckLatest: error empty tag_name")
		return "", "", "", errors.New("could not parse tag_name from release response")
	}
	shared.Debugf("[selfupdate] CheckLatest: found tagName=%s", release.TagName)

	if !isNewer(release.TagName, currentVersion) {
		shared.Debugf("[selfupdate] CheckLatest: already up to date")
		return "", "", "", nil
	}

	assetName := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	shared.Debugf("[selfupdate] CheckLatest: looking for asset %s", assetName)
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "SHA256SUMS" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		shared.Debugf("[selfupdate] CheckLatest: error no asset found for %s", assetName)
		return "", "", "", fmt.Errorf("no asset found for %s", assetName)
	}

	shared.Debugf("[selfupdate] CheckLatest: success latest=%s downloadURL=%s", release.TagName, downloadURL)
	return release.TagName, downloadURL, checksumsURL, nil
}

// Apply downloads the binary from downloadURL, optionally verifies its checksum
// using checksumsURL, and replaces the current executable.
func Apply(ctx context.Context, downloadURL, checksumsURL string) error {
	shared.Debugf("[selfupdate] Apply: start downloadURL=%s", downloadURL)
	exePath, err := os.Executable()
	if err != nil {
		shared.Debugf("[selfupdate] Apply: error locating binary: %v", err)
		return fmt.Errorf("locating current binary: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		shared.Debugf("[selfupdate] Apply: error resolving symlinks: %v", err)
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, "lazystack-update-*")
	if err != nil {
		shared.Debugf("[selfupdate] Apply: error creating temp file: %v", err)
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	shared.Debugf("[selfupdate] Apply: downloading binary")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		shared.Debugf("[selfupdate] Apply: error creating request: %v", err)
		return fmt.Errorf("creating download request: %w", err)
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		shared.Debugf("[selfupdate] Apply: error downloading: %v", err)
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		shared.Debugf("[selfupdate] Apply: error HTTP %d", resp.StatusCode)
		return fmt.Errorf("downloading binary: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)
	if _, err := io.Copy(w, resp.Body); err != nil {
		shared.Debugf("[selfupdate] Apply: error writing binary: %v", err)
		return fmt.Errorf("writing binary: %w", err)
	}
	tmp.Close()

	got := hex.EncodeToString(hasher.Sum(nil))

	if checksumsURL != "" {
		shared.Debugf("[selfupdate] Apply: verifying checksum")
		if err := verifyChecksum(ctx, checksumsURL, got); err != nil {
			shared.Debugf("[selfupdate] Apply: error checksum verification: %v", err)
			return err
		}
		shared.Debugf("[selfupdate] Apply: checksum verified")
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		shared.Debugf("[selfupdate] Apply: error setting permissions: %v", err)
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		shared.Debugf("[selfupdate] Apply: error replacing binary: %v", err)
		return fmt.Errorf("replacing binary: %w", err)
	}

	shared.Debugf("[selfupdate] Apply: success")
	return nil
}

func verifyChecksum(ctx context.Context, checksumsURL, gotHash string) error {
	body, err := httpGet(ctx, checksumsURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	assetName := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			if parts[0] != gotHash {
				return fmt.Errorf("checksum mismatch: expected %s, got %s", parts[0], gotHash)
			}
			return nil
		}
	}

	return fmt.Errorf("no checksum found for %s in SHA256SUMS", assetName)
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// isNewer returns true if latest is a higher semver than current.
// Both must be in "vX.Y.Z" format.
func isNewer(latest, current string) bool {
	l := parseVersion(latest)
	c := parseVersion(current)
	if l == nil || c == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	// Strip git-describe suffix (e.g. "0.3.0-7-g09160b8" → "0.3.0")
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return nums
}

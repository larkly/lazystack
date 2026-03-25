package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const releaseAPI = "https://api.github.com/repos/larkly/lazystack/releases/latest"

// CheckLatest checks GitHub for a newer release. Returns empty strings if
// already up to date. Returns an error if currentVersion is "dev".
func CheckLatest(currentVersion string) (latest, downloadURL, checksumsURL string, err error) {
	if currentVersion == "dev" {
		return "", "", "", errors.New("cannot check for updates on a dev build; build with -ldflags \"-X main.version=vX.Y.Z\"")
	}

	body, err := httpGet(releaseAPI)
	if err != nil {
		return "", "", "", fmt.Errorf("fetching latest release: %w", err)
	}

	tagName := jsonString(body, "tag_name")
	if tagName == "" {
		return "", "", "", errors.New("could not parse tag_name from release response")
	}

	if !isNewer(tagName, currentVersion) {
		return "", "", "", nil
	}

	assetName := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	assets := jsonArray(body, "assets")
	for _, asset := range assets {
		name := jsonString(asset, "name")
		url := jsonString(asset, "browser_download_url")
		if name == assetName {
			downloadURL = url
		}
		if name == "SHA256SUMS" {
			checksumsURL = url
		}
	}

	if downloadURL == "" {
		return "", "", "", fmt.Errorf("no asset found for %s", assetName)
	}

	return tagName, downloadURL, checksumsURL, nil
}

// Apply downloads the binary from downloadURL, optionally verifies its checksum
// using checksumsURL, and replaces the current executable.
func Apply(downloadURL, checksumsURL string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current binary: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, "lazystack-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading binary: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	w := io.MultiWriter(tmp, hasher)
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("writing binary: %w", err)
	}
	tmp.Close()

	got := hex.EncodeToString(hasher.Sum(nil))

	if checksumsURL != "" {
		if err := verifyChecksum(checksumsURL, got); err != nil {
			return err
		}
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}

func verifyChecksum(checksumsURL, gotHash string) error {
	body, err := httpGet(checksumsURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	assetName := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	for _, line := range strings.Split(body, "\n") {
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

func httpGet(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
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

// Minimal JSON helpers — avoids encoding/json for simple field extraction.

func jsonString(json, key string) string {
	needle := fmt.Sprintf("%q", key)
	idx := strings.Index(json, needle)
	if idx < 0 {
		return ""
	}
	rest := json[idx+len(needle):]
	// skip `: `
	rest = strings.TrimLeft(rest, " \t\n\r:")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func jsonArray(json, key string) []string {
	needle := fmt.Sprintf("%q", key)
	idx := strings.Index(json, needle)
	if idx < 0 {
		return nil
	}
	rest := json[idx+len(needle):]
	rest = strings.TrimLeft(rest, " \t\n\r:")
	if len(rest) == 0 || rest[0] != '[' {
		return nil
	}
	rest = rest[1:]

	var items []string
	depth := 0
	start := -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				items = append(items, rest[start:i+1])
				start = -1
			}
		case ']':
			if depth == 0 {
				return items
			}
		}
	}
	return items
}

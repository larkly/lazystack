package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/larkly/lazystack/internal/shared"
)

// releaseAPI is a variable so tests can point it at a local TLS server.
var releaseAPI = "https://api.github.com/repos/larkly/lazystack/releases/latest"

// httpClient is used for API/metadata requests (30s timeout).
var httpClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: httpsOnlyRedirects}

// downloadClient is used for binary downloads (5 minute timeout).
var downloadClient = &http.Client{Timeout: 5 * time.Minute, CheckRedirect: httpsOnlyRedirects}

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
	for _, u := range []string{downloadURL, checksumsURL} {
		if u == "" {
			continue
		}
		if err := requireHTTPS(u); err != nil {
			shared.Debugf("[selfupdate] CheckLatest: rejecting asset URL: %v", err)
			return "", "", "", fmt.Errorf("release asset: %w", err)
		}
	}

	shared.Debugf("[selfupdate] CheckLatest: success latest=%s downloadURL=%s", release.TagName, downloadURL)
	return release.TagName, downloadURL, checksumsURL, nil
}

// Apply downloads the binary from downloadURL, verifies its checksum using
// checksumsURL, and replaces the current executable.
//
// Both URLs must be HTTPS, and redirects to anything else are refused, so a
// network attacker cannot substitute the binary and its checksum together.
//
// The update is staged next to the executable and fsynced, verified
// (checksum and executable format), and only then swapped in with an atomic
// rename, followed by an fsync of the directory. Any failure before the
// rename leaves the current binary untouched. The previous binary is kept as
// "<executable>.old"; to roll back, move it over the executable.
func Apply(ctx context.Context, downloadURL, checksumsURL string) error {
	shared.Debugf("[selfupdate] Apply: start downloadURL=%s", downloadURL)
	if checksumsURL == "" {
		shared.Debugf("[selfupdate] Apply: refusing to install without checksums")
		return fmt.Errorf("refusing to apply update: release has no SHA256SUMS asset")
	}
	for _, u := range []string{downloadURL, checksumsURL} {
		if err := requireHTTPS(u); err != nil {
			shared.Debugf("[selfupdate] Apply: %v", err)
			return fmt.Errorf("refusing to apply update: %w", err)
		}
	}

	exePath, err := executablePath()
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
	closed := false
	installed := false
	defer func() {
		if !closed {
			tmp.Close()
		}
		if !installed {
			os.Remove(tmpPath)
		}
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
	// Make the staged bytes durable before they can replace the binary:
	// an atomic rename of unsynced data can leave an empty file after a
	// crash.
	if err := syncFile(tmp); err != nil {
		shared.Debugf("[selfupdate] Apply: error syncing staged binary: %v", err)
		return fmt.Errorf("syncing staged binary: %w", err)
	}
	closed = true
	if err := closeFile(tmp); err != nil {
		shared.Debugf("[selfupdate] Apply: error closing staged binary: %v", err)
		return fmt.Errorf("closing staged binary: %w", err)
	}

	got := hex.EncodeToString(hasher.Sum(nil))

	shared.Debugf("[selfupdate] Apply: verifying checksum")
	if err := verifyChecksum(ctx, checksumsURL, got); err != nil {
		shared.Debugf("[selfupdate] Apply: error checksum verification: %v", err)
		return err
	}
	shared.Debugf("[selfupdate] Apply: checksum verified")

	if err := checkExecutableFormat(tmpPath); err != nil {
		shared.Debugf("[selfupdate] Apply: staged binary rejected: %v", err)
		return fmt.Errorf("refusing to apply update: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		shared.Debugf("[selfupdate] Apply: error setting permissions: %v", err)
		return fmt.Errorf("setting permissions: %w", err)
	}

	backupPath := exePath + ".old"
	if err := backupBinary(exePath, backupPath); err != nil {
		shared.Debugf("[selfupdate] Apply: error backing up current binary: %v", err)
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := renameFile(tmpPath, exePath); err != nil {
		shared.Debugf("[selfupdate] Apply: error replacing binary: %v", err)
		return fmt.Errorf("replacing binary: %w", err)
	}
	installed = true

	// Persist the rename itself.
	if err := syncDir(dir); err != nil {
		shared.Debugf("[selfupdate] Apply: error syncing directory: %v", err)
		return fmt.Errorf("binary replaced but syncing %s failed (previous version kept at %s): %w", dir, backupPath, err)
	}

	shared.Debugf("[selfupdate] Apply: success, previous binary kept at %s", backupPath)
	return nil
}

// File operations are variables so tests can inject failures.
var (
	executablePath = os.Executable
	syncFile       = func(f *os.File) error { return f.Sync() }
	closeFile      = func(f *os.File) error { return f.Close() }
	renameFile     = os.Rename
	syncDir        = func(dir string) error {
		d, err := os.Open(dir)
		if err != nil {
			return err
		}
		defer d.Close()
		return d.Sync()
	}
)

// backupBinary keeps the current binary at backupPath. A hard link is used
// so the executable path is never missing; if the filesystem cannot link,
// the file is copied and synced instead.
func backupBinary(exePath, backupPath string) error {
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Link(exePath, backupPath); err == nil {
		return nil
	}
	src, err := os.Open(exePath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(backupPath)
		return err
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		os.Remove(backupPath)
		return err
	}
	return dst.Close()
}

// executableMagic lists the file signatures a lazystack release binary can
// start with on each supported platform.
var executableMagic = map[string][]string{
	"linux":   {"\x7fELF"},
	"freebsd": {"\x7fELF"},
	"netbsd":  {"\x7fELF"},
	"openbsd": {"\x7fELF"},
	"darwin": {
		"\xcf\xfa\xed\xfe", // Mach-O 64-bit
		"\xca\xfe\xba\xbe", // universal binary
	},
}

// checkExecutableFormat rejects a staged file that is not an executable for
// this platform (for example an HTML error page), so a bad download can
// never replace the only working copy. Platforms without a known signature
// are not checked.
func checkExecutableFormat(path string) error {
	magics, ok := executableMagic[runtime.GOOS]
	if !ok {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("downloaded file is not a %s executable", runtime.GOOS)
	}
	for _, m := range magics {
		if string(head) == m {
			return nil
		}
	}
	return fmt.Errorf("downloaded file is not a %s executable", runtime.GOOS)
}

// requireHTTPS rejects anything but an absolute https:// URL.
func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%q is not an https URL", raw)
	}
	return nil
}

// httpsOnlyRedirects refuses to follow a redirect to a non-HTTPS URL (and
// keeps net/http's default limit of 10 redirects).
func httpsOnlyRedirects(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to non-https URL %s", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func verifyChecksum(ctx context.Context, checksumsURL, gotHash string) error {
	body, err := httpGet(ctx, checksumsURL)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}
	if err := verifyChecksumsSignature(ctx, checksumsURL, body); err != nil {
		return err
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
	if err := requireHTTPS(url); err != nil {
		return nil, err
	}
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

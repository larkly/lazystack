package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

var assetName = fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)

// validBinary returns bytes that pass the executable format check for the
// platform the tests run on.
func validBinary(t *testing.T) []byte {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly":
		return []byte("\x7fELF new lazystack build")
	case "darwin":
		return []byte("\xcf\xfa\xed\xfe new lazystack build")
	}
	t.Skip("no executable format check on " + runtime.GOOS)
	return nil
}

func sumLine(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]) + "  " + assetName + "\n"
}

// release serves a binary, its SHA256SUMS and a signature by a test key over TLS and routes the
// package's HTTP clients to it by swapping only their transport, so the
// production HTTPS and redirect checks stay in force.
type release struct {
	srv      *httptest.Server
	hits     atomic.Int32
	bin      []byte
	sums     string
	sigTag   string // tag the signature is made for (default SignatureRequiredFrom)
	truncate bool
	unsigned bool // serve no SHA256SUMS.sig
	extra    http.HandlerFunc
}

// newRelease starts the server after configure has adjusted the release,
// so the handler never races with test setup.
func newRelease(t *testing.T, bin []byte, configure func(*release)) *release {
	t.Helper()
	r := &release{bin: bin, sums: sumLine(bin), sigTag: SignatureRequiredFrom}
	if configure != nil {
		configure(r)
	}
	pub, priv := testKey(7)
	useReleaseKey(t, pub)
	r.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		switch req.URL.Path {
		case "/bin":
			if r.truncate {
				w.Header().Set("Content-Length", "1000")
				w.Write(r.bin[:4])
				return
			}
			w.Write(r.bin)
		case "/SHA256SUMS":
			fmt.Fprint(w, r.sums)
		case "/SHA256SUMS.sig":
			if r.unsigned {
				http.NotFound(w, req)
				return
			}
			w.Write(signRelease(priv, r.sigTag, []byte(r.sums)))
		default:
			if r.extra != nil {
				r.extra(w, req)
				return
			}
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(r.srv.Close)
	for _, c := range []*http.Client{httpClient, downloadClient} {
		prev := c.Transport
		c.Transport = r.srv.Client().Transport
		t.Cleanup(func() { c.Transport = prev })
	}
	return r
}

// installed creates the "current" binary that Apply replaces.
func installed(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "lazystack")
	if err := os.WriteFile(exe, []byte("\x7fELF old lazystack build"), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := executablePath
	executablePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePath = prev })
	return exe
}

func assertOriginal(t *testing.T, exe string) {
	t.Helper()
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "\x7fELF old lazystack build" {
		t.Fatalf("original binary changed or lost: %q, %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "lazystack-update-") {
			t.Errorf("staging file %s left behind", e.Name())
		}
	}
}

func TestApplyInstallsVerifiedBinaryAndKeepsBackup(t *testing.T) {
	bin := validBinary(t)
	r := newRelease(t, bin, nil)
	exe := installed(t)

	var synced []string
	prevSync, prevDir := syncFile, syncDir
	syncFile = func(f *os.File) error { synced = append(synced, "file"); return prevSync(f) }
	syncDir = func(dir string) error { synced = append(synced, "dir"); return prevDir(dir) }
	t.Cleanup(func() { syncFile, syncDir = prevSync, prevDir })

	if err := applyStrict(context.Background(), r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(bin) {
		t.Fatalf("binary not replaced: %q", got)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm()&0o111 == 0 {
		t.Errorf("new binary not executable: %v", info.Mode())
	}
	if got, err := os.ReadFile(exe + ".old"); err != nil || string(got) != "\x7fELF old lazystack build" {
		t.Errorf("previous binary not kept as backup: %q, %v", got, err)
	}
	if strings.Join(synced, ",") != "file,dir" {
		t.Errorf("sync order = %v, want the staged file synced before the rename and the directory after", synced)
	}
}

func TestApplyFailuresPreserveOriginal(t *testing.T) {
	bin := validBinary(t)
	boom := errors.New("injected failure")
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, r *release)
	}{
		{"truncated download", func(t *testing.T, r *release) { r.truncate = true }},
		{"checksum mismatch", func(t *testing.T, r *release) { r.sums = sumLine([]byte("something else")) }},
		{"sync", func(t *testing.T, r *release) {
			prev := syncFile
			syncFile = func(*os.File) error { return boom }
			t.Cleanup(func() { syncFile = prev })
		}},
		{"close", func(t *testing.T, r *release) {
			prev := closeFile
			closeFile = func(f *os.File) error { f.Close(); return boom }
			t.Cleanup(func() { closeFile = prev })
		}},
		{"rename", func(t *testing.T, r *release) {
			prev := renameFile
			renameFile = func(string, string) error { return boom }
			t.Cleanup(func() { renameFile = prev })
		}},
		{"signature for another tag", func(t *testing.T, r *release) { r.sigTag = "v0.19.0" }},
		{"invalid staged binary with matching checksum", func(t *testing.T, r *release) {
			r.bin = []byte("<html>502 Bad Gateway</html>")
			r.sums = sumLine(r.bin)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRelease(t, bin, func(r *release) { tc.setup(t, r) })
			exe := installed(t)
			err := applyStrict(context.Background(), r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS")
			if err == nil {
				t.Fatal("Apply succeeded")
			}
			assertOriginal(t, exe)
		})
	}
}

func TestApplyRejectsPlainHTTPBeforeTouchingAnything(t *testing.T) {
	bin := validBinary(t)
	r := newRelease(t, bin, nil)
	exe := installed(t)
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		plainHits.Add(1)
		w.Write(bin)
	}))
	t.Cleanup(plain.Close)

	for _, urls := range [][2]string{
		{plain.URL + "/bin", r.srv.URL + "/SHA256SUMS"},
		{r.srv.URL + "/bin", plain.URL + "/SHA256SUMS"},
		{"ftp://example.com/bin", r.srv.URL + "/SHA256SUMS"},
	} {
		err := applyStrict(context.Background(), urls[0], urls[1])
		if err == nil || !strings.Contains(err.Error(), "https") {
			t.Errorf("Apply(%s, %s) = %v, want an HTTPS error", urls[0], urls[1], err)
		}
	}
	if plainHits.Load() != 0 || r.hits.Load() != 0 {
		t.Errorf("requests sent before the scheme check: plain=%d tls=%d", plainHits.Load(), r.hits.Load())
	}
	assertOriginal(t, exe)
}

func TestApplyRejectsDowngradeRedirects(t *testing.T) {
	bin := validBinary(t)
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		plainHits.Add(1)
		if strings.HasSuffix(req.URL.Path, "SHA256SUMS") {
			fmt.Fprint(w, sumLine(bin))
			return
		}
		w.Write(bin)
	}))
	t.Cleanup(plain.Close)

	for _, target := range []string{"/bin", "/SHA256SUMS"} {
		t.Run(target, func(t *testing.T) {
			r := newRelease(t, bin, func(r *release) {
				r.extra = func(w http.ResponseWriter, req *http.Request) {
					http.Redirect(w, req, plain.URL+strings.TrimPrefix(req.URL.Path, "/redirect"), http.StatusFound)
				}
			})
			exe := installed(t)
			binURL, sumsURL := r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS"
			if target == "/bin" {
				binURL = r.srv.URL + "/redirect/bin"
			} else {
				sumsURL = r.srv.URL + "/redirect/SHA256SUMS"
			}
			err := applyStrict(context.Background(), binURL, sumsURL)
			if err == nil || !strings.Contains(err.Error(), "https") {
				t.Fatalf("Apply = %v, want the HTTPS-to-HTTP redirect rejected", err)
			}
			if plainHits.Load() != 0 {
				t.Fatal("client followed a redirect to plain HTTP")
			}
			assertOriginal(t, exe)
		})
	}
}

func TestApplyFollowsHTTPSRedirects(t *testing.T) {
	bin := validBinary(t)
	// GitHub release assets redirect to another HTTPS host (objects.githubusercontent.com).
	r := newRelease(t, bin, func(r *release) {
		r.extra = func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, strings.TrimPrefix(req.URL.Path, "/redirect"), http.StatusFound)
		}
	})
	exe := installed(t)
	if err := applyStrict(context.Background(), r.srv.URL+"/redirect/bin", r.srv.URL+"/redirect/SHA256SUMS"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(bin) {
		t.Fatalf("binary not replaced after HTTPS redirect: %q", got)
	}
}

func TestCheckLatestRejectsPlainHTTPAssetURLs(t *testing.T) {
	for _, tc := range []struct{ bin, sums string }{
		{"http://example.com/bin", "https://example.com/SHA256SUMS"},
		{"https://example.com/bin", "http://example.com/SHA256SUMS"},
	} {
		body := fmt.Sprintf(`{"tag_name":"v9.9.9","assets":[{"name":%q,"browser_download_url":%q},{"name":"SHA256SUMS","browser_download_url":%q}]}`, assetName, tc.bin, tc.sums)
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { fmt.Fprint(w, body) }))
		prevAPI, prevTransport := releaseAPI, httpClient.Transport
		releaseAPI, httpClient.Transport = srv.URL+"/latest", srv.Client().Transport
		_, _, _, err := CheckLatest(context.Background(), "v0.0.1")
		releaseAPI, httpClient.Transport = prevAPI, prevTransport
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "https") {
			t.Errorf("CheckLatest accepted %v: err=%v", tc, err)
		}
	}
}

// applyStrict applies an update to the first release that requires a
// signature, so the tests exercise the mandatory-signature path.
func applyStrict(ctx context.Context, downloadURL, checksumsURL string) error {
	_, err := Apply(ctx, "v0.19.0", SignatureRequiredFrom, downloadURL, checksumsURL)
	return err
}

func TestApplyUnsignedReleaseBeforeSignatureRequirement(t *testing.T) {
	bin := validBinary(t)
	r := newRelease(t, bin, func(r *release) { r.unsigned = true })
	exe := installed(t)

	signed, err := Apply(context.Background(), "v0.12.0", "v0.13.0", r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS")
	if err != nil {
		t.Fatalf("unsigned pre-v0.20.0 update refused: %v", err)
	}
	if signed {
		t.Fatal("unsigned release reported as signed")
	}
	if got, _ := os.ReadFile(exe); string(got) != string(bin) {
		t.Fatalf("binary not replaced: %q", got)
	}

	exe = installed(t)
	if _, err := Apply(context.Background(), "v0.19.0", "v0.20.0", r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS"); err == nil || !strings.Contains(err.Error(), "SHA256SUMS.sig") {
		t.Fatalf("unsigned v0.20.0 update: err = %v, want a missing-signature error", err)
	}
	assertOriginal(t, exe)
}

// setLimit lowers a download size limit for the duration of a test.
func setLimit(t *testing.T, limit *int64, v int64) {
	t.Helper()
	prev := *limit
	*limit = v
	t.Cleanup(func() { *limit = prev })
}

func TestApplyEnforcesDownloadLimits(t *testing.T) {
	bin := validBinary(t)
	_, priv := testKey(7)
	for _, tc := range []struct {
		name  string
		limit *int64
		size  int64
	}{
		{"binary", &maxBinarySize, int64(len(bin))},
		{"checksums", &maxChecksumsSize, int64(len(sumLine(bin)))},
		{"signature", &maxSignatureSize, int64(len(encodeSig(priv, nil)))},
	} {
		t.Run(tc.name+" over limit", func(t *testing.T) {
			r := newRelease(t, bin, nil)
			exe := installed(t)
			setLimit(t, tc.limit, tc.size-1)
			err := applyStrict(context.Background(), r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS")
			if err == nil || !strings.Contains(err.Error(), "byte limit") {
				t.Fatalf("Apply = %v, want a size limit error", err)
			}
			assertOriginal(t, exe)
		})
		t.Run(tc.name+" at limit", func(t *testing.T) {
			r := newRelease(t, bin, nil)
			exe := installed(t)
			setLimit(t, tc.limit, tc.size)
			if err := applyStrict(context.Background(), r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS"); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(exe); string(got) != string(bin) {
				t.Fatalf("binary not replaced: %q", got)
			}
		})
	}
}

func TestCheckLatestEnforcesResponseLimit(t *testing.T) {
	body := fmt.Sprintf(`{"tag_name":"v9.9.9","assets":[{"name":%q,"browser_download_url":"https://example.com/bin"}]}`, assetName)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { fmt.Fprint(w, body) }))
	t.Cleanup(srv.Close)
	prevAPI, prevTransport := releaseAPI, httpClient.Transport
	releaseAPI, httpClient.Transport = srv.URL+"/latest", srv.Client().Transport
	t.Cleanup(func() { releaseAPI, httpClient.Transport = prevAPI, prevTransport })

	setLimit(t, &maxReleaseJSONSize, int64(len(body))-1)
	if _, _, _, err := CheckLatest(context.Background(), "v0.0.1"); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("CheckLatest = %v, want a size limit error", err)
	}
	setLimit(t, &maxReleaseJSONSize, int64(len(body)))
	if latest, _, _, err := CheckLatest(context.Background(), "v0.0.1"); err != nil || latest != "v9.9.9" {
		t.Fatalf("CheckLatest = %q, %v", latest, err)
	}
}

// A signed older release re-published under a newer tag must not install:
// the signature names the tag it was made for.
func TestApplyRejectsReleaseRepublishedUnderAnotherTag(t *testing.T) {
	bin := validBinary(t)
	for _, target := range []string{"v0.13.0", "v0.21.0"} {
		t.Run(target, func(t *testing.T) {
			r := newRelease(t, bin, func(r *release) { r.sigTag = "v0.12.0" })
			exe := installed(t)
			_, err := Apply(context.Background(), "v0.12.0", target, r.srv.URL+"/bin", r.srv.URL+"/SHA256SUMS")
			if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
				t.Fatalf("Apply = %v, want a signature error", err)
			}
			assertOriginal(t, exe)
		})
	}
}

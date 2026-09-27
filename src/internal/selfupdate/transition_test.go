package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestSignatureRequired(t *testing.T) {
	tests := []struct {
		current, target string
		want            bool
	}{
		{"v0.11.0", "v0.12.0", false},
		{"v0.14.0", "v0.15.0", false},
		{"v0.19.0", "v0.19.10", false},
		{"v0.19.9", "v0.20.0", true}, // updating to the first mandatory release
		{"v0.20.0", "v0.20.1", true}, // clients from v0.20.0 always require it
		{"v0.20.0-3-gabc1234", "v0.19.9", true},
		{"v0.19.9", "v0.20.0-rc1", true}, // pre-releases of v0.20.0 are covered
		{"v0.20.0-rc1", "v0.19.9", true},
		{"v0.19.0", "v0.19.1-rc1", false},
		{"v1.0.0", "v1.0.1", true},
		{"v0.12.0", "not-a-version", true}, // unknown target fails closed
		{"custom-build", "v0.13.0", false}, // unparseable current version doesn't force it
	}
	for _, tc := range tests {
		if got := signatureRequired(tc.current, tc.target); got != tc.want {
			t.Errorf("signatureRequired(%q, %q) = %v, want %v", tc.current, tc.target, got, tc.want)
		}
	}
}

// Before v0.20.0 a release without a signature, or a build without a
// configured key, falls back to SHA256SUMS alone. A signature that is present
// but invalid is always rejected, and from v0.20.0 the signature is mandatory.
func TestVerifyChecksum_TransitionPolicy(t *testing.T) {
	pub, priv := testKey(5)
	_, otherPriv := testKey(6)

	binary := []byte("new lazystack binary")
	sum := sha256.Sum256(binary)
	hash := hex.EncodeToString(sum[:])
	asset := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	sums := []byte(hash + "  " + asset + "\n")

	tests := []struct {
		name       string
		required   bool
		noKey      bool
		sig        []byte
		sigStatus  int
		hash       string
		wantSigned bool
		wantErr    string
	}{
		{name: "unsigned release falls back before v0.20.0", sig: nil, hash: hash},
		{name: "signed release is verified before v0.20.0", sig: encodeSig(priv, sums), hash: hash, wantSigned: true},
		{name: "invalid signature is rejected before v0.20.0", sig: encodeSig(otherPriv, sums), hash: hash, wantErr: "signature verification failed"},
		{name: "no embedded key falls back before v0.20.0", noKey: true, sig: encodeSig(priv, sums), hash: hash},
		{name: "fallback still checks the checksum", sig: nil, hash: strings.Repeat("f", 64), wantErr: "checksum mismatch"},
		{name: "signature server error is not treated as unsigned", sigStatus: http.StatusInternalServerError, hash: hash, wantErr: "HTTP 500"},
		{name: "unsigned release is rejected from v0.20.0", required: true, sig: nil, hash: hash, wantErr: "SHA256SUMS.sig"},
		{name: "no embedded key is rejected from v0.20.0", required: true, noKey: true, sig: encodeSig(priv, sums), hash: hash, wantErr: "no release signing key"},
		{name: "signed release is accepted from v0.20.0", required: true, sig: encodeSig(priv, sums), hash: hash, wantSigned: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.noKey {
				useReleaseKey(t, nil)
			} else {
				useReleaseKey(t, pub)
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/SHA256SUMS":
					w.Write(sums)
				case r.URL.Path == "/SHA256SUMS.sig" && tc.sigStatus != 0:
					w.WriteHeader(tc.sigStatus)
				case r.URL.Path == "/SHA256SUMS.sig" && tc.sig != nil:
					w.Write(tc.sig)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			prev := httpClient.Transport
			httpClient.Transport = srv.Client().Transport
			t.Cleanup(func() { httpClient.Transport = prev })

			signed, err := verifyChecksum(context.Background(), srv.URL+"/SHA256SUMS", tc.hash, tc.required)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if signed != tc.wantSigned {
				t.Fatalf("signed = %v, want %v", signed, tc.wantSigned)
			}
		})
	}
}

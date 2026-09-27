package selfupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// testKey derives a deterministic Ed25519 key pair from a one-byte seed.
func testKey(b byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = b
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

func encodeSig(priv ed25519.PrivateKey, msg []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)) + "\n")
}

// useReleaseKey swaps the trusted release key for the duration of a test.
func useReleaseKey(t *testing.T, pub ed25519.PublicKey) {
	t.Helper()
	prev := releasePublicKey
	releasePublicKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { releasePublicKey = prev })
}

func TestVerifyReleaseSignature(t *testing.T) {
	pub, priv := testKey(1)
	_, otherPriv := testKey(2)
	sums := []byte("abc123  lazystack-linux-amd64\n")
	good := encodeSig(priv, sums)

	tests := []struct {
		name    string
		key     string
		sums    []byte
		sig     []byte
		wantErr string
	}{
		{"valid", base64.StdEncoding.EncodeToString(pub), sums, good, ""},
		{"valid without trailing newline", base64.StdEncoding.EncodeToString(pub), sums, []byte(strings.TrimSpace(string(good))), ""},
		{"modified checksums", base64.StdEncoding.EncodeToString(pub), []byte("def456  lazystack-linux-amd64\n"), good, "signature verification failed"},
		{"signed by unexpected key", base64.StdEncoding.EncodeToString(pub), sums, encodeSig(otherPriv, sums), "signature verification failed"},
		{"missing signature", base64.StdEncoding.EncodeToString(pub), sums, nil, "empty signature"},
		{"malformed signature", base64.StdEncoding.EncodeToString(pub), sums, []byte("not base64!"), "decoding signature"},
		{"truncated signature", base64.StdEncoding.EncodeToString(pub), sums, []byte(base64.StdEncoding.EncodeToString([]byte("short"))), "signature length"},
		{"no trusted key configured", "", sums, good, "no release signing key"},
		{"malformed trusted key", "@@@", sums, good, "decoding release public key"},
		{"wrong-size trusted key", base64.StdEncoding.EncodeToString([]byte("tiny")), sums, good, "release public key length"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := releasePublicKey
			releasePublicKey = tc.key
			defer func() { releasePublicKey = prev }()

			err := VerifyReleaseSignature(tc.sums, tc.sig)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// releaseServer serves SHA256SUMS and (optionally) SHA256SUMS.sig over TLS
// and points the package HTTP client at it for the duration of the test.
func releaseServer(t *testing.T, sums, sig []byte) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			w.Write(sums)
		case "/SHA256SUMS.sig":
			if sig == nil {
				http.NotFound(w, r)
				return
			}
			w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	prev := httpClient
	httpClient = srv.Client()
	t.Cleanup(func() { httpClient = prev })
	return srv.URL + "/SHA256SUMS"
}

func TestVerifyChecksum_RequiresValidSignature(t *testing.T) {
	pub, priv := testKey(3)
	_, otherPriv := testKey(4)
	useReleaseKey(t, pub)

	binary := []byte("new lazystack binary")
	sum := sha256.Sum256(binary)
	hash := hex.EncodeToString(sum[:])
	asset := fmt.Sprintf("lazystack-%s-%s", runtime.GOOS, runtime.GOARCH)
	sums := []byte(hash + "  " + asset + "\n")
	tampered := []byte(strings.Repeat("0", 64) + "  " + asset + "\n")

	tests := []struct {
		name    string
		sums    []byte
		sig     []byte
		hash    string
		wantErr string
	}{
		{"signed and matching", sums, encodeSig(priv, sums), hash, ""},
		{"signature missing", sums, nil, hash, "SHA256SUMS.sig"},
		{"checksums replaced after signing", tampered, encodeSig(priv, sums), strings.Repeat("0", 64), "signature verification failed"},
		{"signed by unexpected key", sums, encodeSig(otherPriv, sums), hash, "signature verification failed"},
		{"signed but binary modified", sums, encodeSig(priv, sums), strings.Repeat("f", 64), "checksum mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := releaseServer(t, tc.sums, tc.sig)
			_, err := verifyChecksum(context.Background(), url, tc.hash, true)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// The embedded release key must be a well-formed Ed25519 public key, so a
// typo cannot silently disable signature verification.
func TestEmbeddedReleaseKeyIsValid(t *testing.T) {
	pub, err := base64.StdEncoding.DecodeString(ReleaseSigningPublicKey)
	if err != nil {
		t.Fatalf("ReleaseSigningPublicKey is not valid base64: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("ReleaseSigningPublicKey is %d bytes, want %d", len(pub), ed25519.PublicKeySize)
	}
}

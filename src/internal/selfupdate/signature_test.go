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

// signRelease returns SHA256SUMS.sig contents for sums published as tag.
func signRelease(priv ed25519.PrivateKey, tag string, sums []byte) []byte {
	return encodeSig(priv, ReleaseSignatureMessage(tag, sums))
}

func TestReleaseSignatureMessage(t *testing.T) {
	got := string(ReleaseSignatureMessage("v0.12.0", []byte("abc  lazystack-linux-amd64\n")))
	want := "lazystack-release-v1\nv0.12.0\nabc  lazystack-linux-amd64\n"
	if got != want {
		t.Fatalf("ReleaseSignatureMessage = %q, want %q", got, want)
	}
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
	const tag = "v0.13.0"
	good := signRelease(priv, tag, sums)
	key := base64.StdEncoding.EncodeToString(pub)

	tests := []struct {
		name    string
		key     string
		tag     string
		sums    []byte
		sig     []byte
		wantErr string
	}{
		{"valid", key, tag, sums, good, ""},
		{"valid without trailing newline", key, tag, sums, []byte(strings.TrimSpace(string(good))), ""},
		{"valid pre-release tag", key, "v0.13.0-rc.1", sums, signRelease(priv, "v0.13.0-rc.1", sums), ""},
		{"modified checksums", key, tag, []byte("def456  lazystack-linux-amd64\n"), good, "signature verification failed"},
		{"signed by unexpected key", key, tag, sums, signRelease(otherPriv, tag, sums), "signature verification failed"},
		// An older signed release re-published under a newer tag.
		{"signed for another tag", key, "v0.14.0", sums, good, "signature verification failed"},
		{"signed for a pre-release of the tag", key, tag, sums, signRelease(priv, tag+"-rc1", sums), "signature verification failed"},
		{"signed without the tag binding", key, tag, sums, encodeSig(priv, sums), "signature verification failed"},
		{"malformed tag", key, "v0.13.0\nx", sums, good, "malformed release tag"},
		{"empty tag", key, "", sums, good, "malformed release tag"},
		{"missing signature", key, tag, sums, nil, "empty signature"},
		{"malformed signature", key, tag, sums, []byte("not base64!"), "decoding signature"},
		{"truncated signature", key, tag, sums, []byte(base64.StdEncoding.EncodeToString([]byte("short"))), "signature length"},
		{"no trusted key configured", "", tag, sums, good, "no release signing key"},
		{"malformed trusted key", "@@@", tag, sums, good, "decoding release public key"},
		{"wrong-size trusted key", base64.StdEncoding.EncodeToString([]byte("tiny")), tag, sums, good, "release public key length"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prev := releasePublicKey
			releasePublicKey = tc.key
			defer func() { releasePublicKey = prev }()

			err := VerifyReleaseSignature(tc.tag, tc.sums, tc.sig)
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
	const tag = "v0.20.0"

	tests := []struct {
		name    string
		sums    []byte
		sig     []byte
		hash    string
		wantErr string
	}{
		{"signed and matching", sums, signRelease(priv, tag, sums), hash, ""},
		{"signature missing", sums, nil, hash, "SHA256SUMS.sig"},
		{"checksums replaced after signing", tampered, signRelease(priv, tag, sums), strings.Repeat("0", 64), "signature verification failed"},
		{"signed by unexpected key", sums, signRelease(otherPriv, tag, sums), hash, "signature verification failed"},
		{"signed for another tag", sums, signRelease(priv, "v0.19.0", sums), hash, "signature verification failed"},
		{"signed but binary modified", sums, signRelease(priv, tag, sums), strings.Repeat("f", 64), "checksum mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := releaseServer(t, tc.sums, tc.sig)
			_, err := verifyChecksum(context.Background(), url, tag, tc.hash, true)
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

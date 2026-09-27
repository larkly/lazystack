package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larkly/lazystack/internal/selfupdate"
)

func seedB64(b byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, ed25519.SeedSize))
}

func pubB64(b byte) string {
	priv := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))
	return base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
}

func writeSums(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(path, []byte("abc  lazystack-linux-amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSign_WritesVerifiableSignature(t *testing.T) {
	path := writeSums(t)
	var stdout, stderr bytes.Buffer
	env := map[string]string{"RELEASE_SIGNING_KEY": seedB64(7)}
	code := run([]string{"sign", path, "v1.2.3-rc.1"}, func(k string) string { return env[k] }, pubB64(7), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	sig, err := os.ReadFile(path + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		t.Fatal(err)
	}
	sums, _ := os.ReadFile(path)
	pub, _ := base64.StdEncoding.DecodeString(pubB64(7))
	if !ed25519.Verify(pub, selfupdate.ReleaseSignatureMessage("v1.2.3-rc.1", sums), raw) {
		t.Fatal("signature does not verify for the tag it was made for")
	}
	// The signature covers the tag, not the bare checksums.
	if ed25519.Verify(pub, selfupdate.ReleaseSignatureMessage("v1.2.4", sums), raw) {
		t.Fatal("signature verifies for another tag")
	}
	if ed25519.Verify(pub, sums, raw) {
		t.Fatal("signature verifies over SHA256SUMS without the tag")
	}
}

func TestSign_FailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		trusted string
		tag     string
		wantErr string
	}{
		{"secret missing", "", pubB64(7), "v1.2.3", "RELEASE_SIGNING_KEY is not set"},
		{"secret not base64", "!!", pubB64(7), "v1.2.3", "decoding RELEASE_SIGNING_KEY"},
		{"secret wrong size", base64.StdEncoding.EncodeToString([]byte("short")), pubB64(7), "v1.2.3", "RELEASE_SIGNING_KEY"},
		{"key does not match embedded public key", seedB64(8), pubB64(7), "v1.2.3", "does not match"},
		{"embedded public key not configured", seedB64(7), "", "v1.2.3", "does not match"},
		{"empty tag", seedB64(7), pubB64(7), "", "release tag"},
		{"tag without v", seedB64(7), pubB64(7), "1.2.3", "release tag"},
		{"incomplete tag", seedB64(7), pubB64(7), "v1.2", "release tag"},
		{"tag with newline", seedB64(7), pubB64(7), "v1.2.3\nv9.9.9", "release tag"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSums(t)
			var stdout, stderr bytes.Buffer
			env := map[string]string{"RELEASE_SIGNING_KEY": tc.key}
			code := run([]string{"sign", path, tc.tag}, func(k string) string { return env[k] }, tc.trusted, &stdout, &stderr)
			if code == 0 {
				t.Fatal("expected non-zero exit")
			}
			if !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr = %q, want containing %q", stderr.String(), tc.wantErr)
			}
			if _, err := os.Stat(path + ".sig"); !os.IsNotExist(err) {
				t.Fatalf("signature file must not be left behind, stat err = %v", err)
			}
		})
	}
}

func TestKeygen_PrintsMatchingPair(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"keygen"}, func(string) string { return "" }, "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	var seed, pub string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if v, ok := strings.CutPrefix(line, "RELEASE_SIGNING_KEY="); ok {
			seed = v
		}
		if v, ok := strings.CutPrefix(line, "ReleaseSigningPublicKey="); ok {
			pub = v
		}
	}
	rawSeed, err := base64.StdEncoding.DecodeString(seed)
	if err != nil || len(rawSeed) != ed25519.SeedSize {
		t.Fatalf("bad seed %q: %v", seed, err)
	}
	want := base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(rawSeed).Public().(ed25519.PublicKey))
	if pub != want {
		t.Fatalf("public key %q does not match seed (want %q)", pub, want)
	}
}

func TestUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, func(string) string { return "" }, "", &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	for _, args := range [][]string{{"sign"}, {"sign", "SHA256SUMS"}, {"sign", "SHA256SUMS", "v1.2.3", "extra"}} {
		if code := run(args, func(string) string { return "" }, "", &stdout, &stderr); code != 2 {
			t.Fatalf("run(%q) exit %d, want 2", args, code)
		}
	}
}

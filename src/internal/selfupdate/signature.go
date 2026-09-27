package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/larkly/lazystack/internal/shared"
)

// ReleaseSigningPublicKey is the base64-encoded Ed25519 public key that signs
// the SHA256SUMS manifest of every release. The release workflow signs
// SHA256SUMS with the matching private key (repository secret
// RELEASE_SIGNING_KEY) and publishes the result as SHA256SUMS.sig.
//
// PLACEHOLDER: until this is set, self-update refuses every release. Generate
// a key pair with
//
//	cd src && go run ./internal/selfupdate/cmd/releasesign keygen
//
// store the printed private key as the RELEASE_SIGNING_KEY secret and paste
// the printed public key here. See SECURITY.md.
const ReleaseSigningPublicKey = ""

// releasePublicKey is the key that verification trusts. It is a variable only
// so tests can substitute their own key.
var releasePublicKey = ReleaseSigningPublicKey

// signatureSuffix is appended to the SHA256SUMS asset URL to locate its
// detached signature (GitHub serves both assets under the same release path).
const signatureSuffix = ".sig"

// VerifyReleaseSignature checks that sig is a valid Ed25519 signature of sums
// made by the trusted release key. sig is the base64-encoded signature as
// published in SHA256SUMS.sig; surrounding whitespace is ignored. It fails
// closed: a missing, malformed or foreign signature, or a build without a
// configured key, is an error.
func VerifyReleaseSignature(sums, sig []byte) error {
	if releasePublicKey == "" {
		return errors.New("no release signing key is configured in this build; refusing to trust SHA256SUMS")
	}
	pub, err := base64.StdEncoding.DecodeString(releasePublicKey)
	if err != nil {
		return fmt.Errorf("decoding release public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("release public key length %d, want %d", len(pub), ed25519.PublicKeySize)
	}

	sig = bytes.TrimSpace(sig)
	if len(sig) == 0 {
		return errors.New("empty signature for SHA256SUMS")
	}
	raw, err := base64.StdEncoding.DecodeString(string(sig))
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}
	if len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("signature length %d, want %d", len(raw), ed25519.SignatureSize)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sums, raw) {
		return errors.New("SHA256SUMS signature verification failed")
	}
	return nil
}

// verifyChecksumsSignature downloads SHA256SUMS.sig next to checksumsURL and
// verifies it over sums. Any failure, including a missing signature asset,
// means the checksums must not be trusted.
func verifyChecksumsSignature(ctx context.Context, checksumsURL string, sums []byte) error {
	sig, err := httpGet(ctx, checksumsURL+signatureSuffix)
	if err != nil {
		shared.Debugf("[selfupdate] verifyChecksumsSignature: error downloading signature: %v", err)
		return fmt.Errorf("downloading SHA256SUMS.sig: %w", err)
	}
	if err := VerifyReleaseSignature(sums, sig); err != nil {
		shared.Debugf("[selfupdate] verifyChecksumsSignature: %v", err)
		return fmt.Errorf("verifying SHA256SUMS.sig: %w", err)
	}
	shared.Debugf("[selfupdate] verifyChecksumsSignature: signature valid")
	return nil
}

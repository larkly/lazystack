package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/larkly/lazystack/internal/shared"
)

// ReleaseSigningPublicKey is the base64-encoded Ed25519 public key that signs
// the SHA256SUMS manifest of every release. The release workflow signs
// SHA256SUMS with the matching private key (repository secret
// RELEASE_SIGNING_KEY) and publishes the result as SHA256SUMS.sig.
//
// To rotate the key, generate a new pair with
//
//	cd src && go run ./internal/selfupdate/cmd/releasesign keygen
//
// store the printed private key as the RELEASE_SIGNING_KEY secret and replace
// the value below with the printed public key. Clients built with the old key
// then fall back or refuse updates as described in SECURITY.md.
const ReleaseSigningPublicKey = "Ky7UMN69rec2L8N74Cmcig55fvuz/tOwGAbByvzEmtk="

// releasePublicKey is the key that verification trusts. It is a variable only
// so tests can substitute their own key.
var releasePublicKey = ReleaseSigningPublicKey

// SignatureRequiredFrom is the first release for which a valid SHA256SUMS.sig
// is mandatory. Before it, releases may be unsigned so that clients can keep
// updating with --update while signing is rolled out; see
// verifyChecksumsSignature for the fallback rules.
const SignatureRequiredFrom = "v0.20.0"

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
// verifies it over sums. It reports whether the checksums were verified by a
// signature.
//
// When required is false (the transition period before
// SignatureRequiredFrom), a release without SHA256SUMS.sig or a build without
// a configured key falls back to trusting SHA256SUMS alone, delivered over
// HTTPS. A signature that is present but does not verify is always an error:
// that is a sign of tampering, not of an older release.
func verifyChecksumsSignature(ctx context.Context, checksumsURL string, sums []byte, required bool) (bool, error) {
	if releasePublicKey == "" && !required {
		shared.Debugf("[selfupdate] verifyChecksumsSignature: no release key in this build; using SHA256SUMS only")
		return false, nil
	}
	sig, err := httpGet(ctx, checksumsURL+signatureSuffix)
	if err != nil {
		var status *httpStatusError
		if !required && errors.As(err, &status) && status.Code == http.StatusNotFound {
			shared.Debugf("[selfupdate] verifyChecksumsSignature: release has no SHA256SUMS.sig; using SHA256SUMS only")
			return false, nil
		}
		shared.Debugf("[selfupdate] verifyChecksumsSignature: error downloading signature: %v", err)
		return false, fmt.Errorf("downloading SHA256SUMS.sig: %w", err)
	}
	if err := VerifyReleaseSignature(sums, sig); err != nil {
		shared.Debugf("[selfupdate] verifyChecksumsSignature: %v", err)
		return false, fmt.Errorf("verifying SHA256SUMS.sig: %w", err)
	}
	shared.Debugf("[selfupdate] verifyChecksumsSignature: signature valid")
	return true, nil
}

// signatureRequired reports whether updating from current to target must be
// verified by a release signature. It is required when either version is
// SignatureRequiredFrom or later, so clients from that version on never
// accept an unsigned release and every release from it on must be signed.
// An unparseable target fails closed; an unparseable current version (a
// custom build) does not by itself require a signature.
func signatureRequired(current, target string) bool {
	if parseVersion(target) == nil || !isNewer(SignatureRequiredFrom, target) {
		return true
	}
	return parseVersion(current) != nil && !isNewer(SignatureRequiredFrom, current)
}

// Command releasesign manages the Ed25519 key that signs release checksums.
//
//	releasesign keygen          print a new key pair
//	releasesign sign FILE TAG   sign FILE for release TAG with $RELEASE_SIGNING_KEY, writing FILE.sig
//
// sign signs selfupdate.ReleaseSignatureMessage(TAG, contents of FILE), which
// binds the checksums to the release tag; TAG must be of the form
// vMAJOR.MINOR.PATCH[-PRERELEASE] and must be the tag being released.
//
// RELEASE_SIGNING_KEY is the base64-encoded 32-byte Ed25519 seed printed by
// keygen. sign refuses to run when the key is missing or when it does not
// match selfupdate.ReleaseSigningPublicKey, so a release is never published
// with a signature that installed clients cannot verify.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/larkly/lazystack/internal/selfupdate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, selfupdate.ReleaseSigningPublicKey, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, trustedPub string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "keygen":
		if err := keygen(stdout); err != nil {
			fmt.Fprintf(stderr, "releasesign: %v\n", err)
			return 1
		}
		return 0
	case "sign":
		if len(args) != 3 {
			usage(stderr)
			return 2
		}
		if err := sign(args[1], args[2], getenv("RELEASE_SIGNING_KEY"), trustedPub); err != nil {
			fmt.Fprintf(stderr, "releasesign: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s.sig for %s\n", args[1], args[2])
		return 0
	default:
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: releasesign keygen | releasesign sign FILE TAG")
}

func keygen(w io.Writer) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "# Store as the RELEASE_SIGNING_KEY repository secret. Keep it private.")
	fmt.Fprintf(w, "RELEASE_SIGNING_KEY=%s\n", base64.StdEncoding.EncodeToString(priv.Seed()))
	fmt.Fprintln(w, "# Set ReleaseSigningPublicKey in src/internal/selfupdate/signature.go to:")
	fmt.Fprintf(w, "ReleaseSigningPublicKey=%s\n", base64.StdEncoding.EncodeToString(pub))
	return nil
}

func sign(path, tag, keyB64, trustedPub string) error {
	if !selfupdate.ValidReleaseTag(tag) {
		return fmt.Errorf("release tag %q is not of the form vMAJOR.MINOR.PATCH[-PRERELEASE]", tag)
	}
	if keyB64 == "" {
		return errors.New("RELEASE_SIGNING_KEY is not set; refusing to publish unsigned checksums")
	}
	seed, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return fmt.Errorf("decoding RELEASE_SIGNING_KEY: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return fmt.Errorf("RELEASE_SIGNING_KEY is %d bytes, want a %d-byte Ed25519 seed", len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if pub != trustedPub {
		return fmt.Errorf("RELEASE_SIGNING_KEY public key %s does not match ReleaseSigningPublicKey %q", pub, trustedPub)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	msg := selfupdate.ReleaseSignatureMessage(tag, data)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)) + "\n"
	return os.WriteFile(path+".sig", []byte(sig), 0o644)
}

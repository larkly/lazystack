# Security Policy

This document describes how to report security vulnerabilities for this repository.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, use GitHub's **Private Vulnerability Reporting**:

1. Navigate to the **Security** tab of this repository on GitHub.
2. Click **Report a vulnerability**.
3. Fill in the form with details: affected component, version, reproduction steps, and impact.

You will receive an acknowledgement within **5 business days**. If the report is accepted, you will be kept informed of remediation progress and notified when a fix is released.

## Scope

This policy covers security vulnerabilities in:

- Source code in this repository
- Published release artifacts produced by this repository's CI (binaries, container images, packages)
- CI/CD workflows when they affect release integrity

Out of scope:

- Vulnerabilities in third-party dependencies that have not been patched upstream (report to the upstream maintainer; consider opening a Dependabot advisory if you are a maintainer)
- Theoretical attacks without a concrete exploit path
- Self-XSS or social-engineering attacks requiring user cooperation

## Supported Versions

Only the latest release line of this project receives security updates.

## Release Signing and Self-Update Verification

Every release publishes `SHA256SUMS` (one SHA-256 line per released binary and
package) and `SHA256SUMS.sig`, a detached Ed25519 signature (base64-encoded,
one line) that binds `SHA256SUMS` to the release tag. The signed message is
not `SHA256SUMS` alone but:

```text
lazystack-release-v1\n<tag>\n<contents of SHA256SUMS>
```

that is, the fixed domain string `lazystack-release-v1`, a newline, the tag
(for example `v0.13.0` or `v0.13.0-rc1`), a newline, and then the exact bytes
of `SHA256SUMS`. The client verifies the signature against the tag of the
release it is installing, the same tag it compared against its own version to
decide that the release is newer.

`lazystack --update` always requires HTTPS for every download, caps the size
of every download (release metadata, `SHA256SUMS`, its signature and the
binary), and checks the downloaded binary's SHA-256 against its line in
`SHA256SUMS`. How `SHA256SUMS` itself is trusted depends on the versions
involved.

**From v0.20.0 (mandatory signatures).** When the release being installed
or the running binary is v0.20.0 or later, the update is refused unless:

1. `SHA256SUMS.sig` exists next to `SHA256SUMS` in the release, and
2. the signature over the message above, with the tag of the release being
   installed, verifies against the Ed25519 public key compiled into the
   running binary (`ReleaseSigningPublicKey` in
   `src/internal/selfupdate/signature.go`).

A release without a signature, with a malformed signature, signed by any
other key, or signed for a different tag is rejected, and so is every such
update from a build that has no embedded key (development builds made before
the key was added). Pre-releases count as the version they precede, so
`v0.20.0-rc1` already requires a signature.

**Before v0.20.0 (transition).** While both versions are older than v0.20.0,
signing is being rolled out and older clients must be able to keep updating
without a reinstall:

- If the release has `SHA256SUMS.sig` and the running binary has a key, the
  signature is verified, and an invalid signature (including one made for a
  different tag) is rejected just as above.
- If the release has no `SHA256SUMS.sig` (HTTP 404), or the running binary has
  no key configured, the update falls back to `SHA256SUMS` alone and prints a
  warning that it was not signature-verified.
- Any other failure to fetch the signature (for example an HTTP 5xx) is an
  error, not a fallback.

The switch-over version is `SignatureRequiredFrom` in
`src/internal/selfupdate/signature.go`.

### What this protects against, and what it does not

For a signature-verified update, the signature protects against substituted
or modified release assets by anyone who does not hold the signing key: a
replaced binary, an edited `SHA256SUMS`, or a validly signed `SHA256SUMS`,
signature and binaries from an older release re-uploaded under a newer tag
(the signature names the tag it was made for, and release binary names carry
no version). During the transition before v0.20.0, a release that has no
`SHA256SUMS.sig` at all is trusted on `SHA256SUMS` over HTTPS alone, so this
protection only becomes unconditional from v0.20.0.

It does **not** protect against a compromised signer: anyone who can run the
release workflow with the `RELEASE_SIGNING_KEY` secret, or who obtains that
secret, or who gets malicious code into a tagged commit that the workflow
builds, can produce a validly signed malicious release. The signature proves
who published the checksums, not that the build is benign. Homebrew, AUR,
`.deb` and `.rpm` installs are verified by those package managers, not by
this mechanism.

### Maintainer setup

1. Generate a key pair (offline, on a trusted machine):

   ```bash
   cd src && go run ./internal/selfupdate/cmd/releasesign keygen
   ```

2. Store the printed `RELEASE_SIGNING_KEY` value (base64 of the 32-byte
   Ed25519 seed) as the `RELEASE_SIGNING_KEY` repository secret, restricted to
   the release workflow. Keep an offline backup; never commit it.
3. Set `ReleaseSigningPublicKey` in `src/internal/selfupdate/signature.go` to
   the printed public key and commit that before tagging.

The release job signs with

```bash
cd src && go run ./internal/selfupdate/cmd/releasesign sign FILE TAG
```

which signs the message above for `TAG` and writes `FILE.sig`. `TAG` is
required and must be the tag being released, of the form
`vMAJOR.MINOR.PATCH[-PRERELEASE]`; the workflow passes the validated tag.
The release job fails, and nothing is published, if the secret is missing,
if the tag is malformed, or if the secret's public key does not match
`ReleaseSigningPublicKey` at the tag being released.

Installed clients trust only the key they were built with. After rotating the
key (new secret plus new `ReleaseSigningPublicKey`), clients built with the old
key always reject the new releases (a signature by another key is never
accepted, even before v0.20.0), so their users must reinstall once from the
releases page or a package manager. Rotate only when the key is lost or
compromised.

### Verifying a download manually

With OpenSSL 3, the base64 public key from `signature.go` in `$PUB` and the
release tag (for example `v0.13.0`) in `$TAG`:

```bash
{ printf '302a300506032b6570032100' | xxd -r -p; printf '%s' "$PUB" | base64 -d; } > release.der
openssl pkey -pubin -inform DER -in release.der -out release.pem
{ printf 'lazystack-release-v1\n%s\n' "$TAG"; cat SHA256SUMS; } > SHA256SUMS.msg
base64 -d SHA256SUMS.sig > SHA256SUMS.sig.bin
openssl pkeyutl -verify -pubin -inkey release.pem -rawin -in SHA256SUMS.msg -sigfile SHA256SUMS.sig.bin
sha256sum --ignore-missing -c SHA256SUMS
```

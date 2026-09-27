# Index — lazystack

> **Navigable table of contents for lazystack documentation**

---

## Getting Started

| Document | Description |
|---|---|
| [`README.md`](README.md) | Overview, installation, features, keybindings, configuration, and usage. |
| [`LICENSE`](LICENSE) | Apache 2.0 license. |

---

## Product Requirements

| Document | Description |
|---|---|
| [`PRD.md`](PRD.md) | Full product requirements document covering all implemented features and phases. |

---

## Design & Specifications

| Document | Description |
|---|---|
| [`docs/superpowers/specs/2026-04-17-serverlist-adaptive-columns-design.md`](docs/superpowers/specs/2026-04-17-serverlist-adaptive-columns-design.md) | Design spec for adaptive column widths in the server list view. |

---

## Package & Distribution

| File | Purpose |
|---|---|
| [`nfpm.yaml`](nfpm.yaml) | nFPM packaging configuration for the `.deb`, `.rpm` and Arch `.pkg.tar.zst` release packages. |
| [`Makefile`](Makefile) | Repository-root targets: `build` (writes `bin/lazystack`, version `dev`) and `test` (`go test -race ./...`). |
| [`src/Makefile`](src/Makefile) | Module targets, run from `src/`: `build` (writes `src/lazystack`, version from `git describe`), `test`, `vet`, `clean`, and `all` (vet, test, build). |

The AUR recipe is not kept in this repository: it lives in the separate `larkly/aur-lazystack` repository, which the release workflow notifies (repository dispatch with the new version and source checksum) on every release.

There is no `install` target; copy the built binary onto your `PATH` yourself (see [README.md](README.md#from-source)).

---

## Contributing

- All code is in `src/`. Run `make test` (from the repository root) before committing.
- Release workflow is in [`.github/workflows/`](.github/workflows/) — CI workflows removed 2026-07-19 by release-only policy.

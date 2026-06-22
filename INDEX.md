# Index — lazystack

> **Navigable table of contents for lazystack documentation**

---

## Getting Started

| Document | Description |
|---|---|
| [`README.md`](README.md) | Overview, installation, features, keybindings, configuration, and usage. |
| [`LICENSE`](LICENSE) | MPL-2.0 license. |

---

## Product Requirements

| Document | Description |
|---|---|
| [`PRD.md`](PRD.md) | Full product requirements document covering all implemented features and phases. |
| [`PRD-new.md`](PRD-new.md) | Refreshed PRD with updated structure and requirements. |

---

## Design & Specifications

| Document | Description |
|---|---|
| [`docs/superpowers/specs/2026-04-17-serverlist-adaptive-columns-design.md`](docs/superpowers/specs/2026-04-17-serverlist-adaptive-columns-design.md) | Design spec for adaptive column widths in the server list view. |

---

## Package & Distribution

| File | Purpose |
|---|---|
| [`nfpm.yaml`](nfpm.yaml) | NFPM packaging configuration for `.deb` and `.rpm` releases. |
| [`aur/`](aur/) | AUR packaging files. |
| [`Makefile`](Makefile) | Build targets (`build`, `test`, `install`). |

---

## Contributing

- All code is in `src/`. Run `make test` before committing.
- CI pipelines are in [`.github/workflows/`](.github/workflows/).

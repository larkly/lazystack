<p align="center">
  <img src="assets/lazystack-logo.png" alt="LAZYSTACK" width="500">
</p>

<p align="center">
  A keyboard-driven terminal UI for OpenStack.
</p>

<p align="center">
  <a href="#installation">Installation</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#keybindings">Keybindings</a> &middot;
  <a href="#configuration">Configuration</a> &middot;
  <a href="#license">License</a>
</p>

---

**lazystack** is a fast, keyboard-first TUI for managing OpenStack resources from the terminal. It follows the "lazy" convention ([lazygit](https://github.com/jesseduffield/lazygit), [lazydocker](https://github.com/jesseduffield/lazydocker)) to provide an intuitive alternative to Horizon and the verbose OpenStack CLI.

Single binary. No runtime dependencies. Reads your standard `clouds.yaml`.

## Features

- **Server management** — list, create, delete, reboot, pause, suspend, shelve, resize with bulk operations
- **Volume management** — list, detail, delete, detach
- **Floating IPs** — allocate, associate, disassociate, release
- **Security groups** — browse groups, expand rules, delete rules
- **Key pairs** — list and delete
- **Load balancers** (Octavia) — list, detail tree (listeners/pools/members), cascade delete
- **Project switching** — switch between accessible Keystone projects without restarting
- **Quota overlay** — compute, network, and storage quotas with color-coded progress bars
- **Dynamic tabs** — tabs appear based on available services (no Cinder? no Volumes tab)
- **Auto-refresh** — all views refresh in the background at a configurable interval
- **Console log** and **action history** per server
- **Column sorting** on all list views
- **Client-side filtering** with `/`
- **Bulk select** with `space` for multi-server operations
- **Solarized Dark** color scheme with status-aware coloring

## Installation

### From source

```bash
cd src
make build
```

### With `go install`

```bash
go install github.com/bosse/lazystack/cmd/lazystack@latest
```

### Requirements

- Go 1.26+ (build only)
- OpenStack cloud with Keystone v3 and Nova v2.1+
- A valid `clouds.yaml`

## Configuration

lazystack reads `clouds.yaml` from these locations (first match wins):

1. `./clouds.yaml` (current directory)
2. `$OS_CLIENT_CONFIG_FILE`
3. `~/.config/openstack/clouds.yaml`
4. `/etc/openstack/clouds.yaml`

No additional configuration is needed. If only one cloud is defined, lazystack connects automatically.

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `--pick-cloud` | `false` | Always show cloud picker, even with one cloud |
| `--refresh N` | `5` | Auto-refresh interval in seconds |
| `--version` | | Print version and exit |

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `?` | Toggle help overlay |
| `C` | Switch cloud |
| `P` | Switch project |
| `Q` | Quota overlay |
| `1-9` / `Left` / `Right` | Switch tab |
| `R` | Force refresh |
| `s` / `S` | Sort column / reverse sort |
| `PgUp` / `PgDn` | Page up / down |
| `Ctrl+R` | Restart app |

### Server list

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `Enter` | View detail |
| `Space` | Select for bulk action |
| `/` | Filter |
| `Ctrl+N` | Create server |
| `Ctrl+D` | Delete |
| `Ctrl+O` | Soft reboot |
| `p` | Pause / unpause |
| `Ctrl+Z` | Suspend / resume |
| `Ctrl+E` | Shelve / unshelve |
| `Ctrl+F` | Resize |
| `Ctrl+A` | Assign floating IP |
| `l` | Console log |
| `a` | Action history |

### Server detail

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll |
| `Ctrl+D` | Delete |
| `Ctrl+O` / `Ctrl+P` | Soft / hard reboot |
| `p` | Pause / unpause |
| `Ctrl+Z` | Suspend / resume |
| `Ctrl+E` | Shelve / unshelve |
| `Ctrl+F` | Resize |
| `Ctrl+Y` / `Ctrl+X` | Confirm / revert resize |
| `Ctrl+A` | Assign floating IP |
| `l` | Console log |
| `a` | Action history |
| `Esc` | Back to list |

### Volumes

| Key | Action |
|-----|--------|
| `Enter` | View detail |
| `Ctrl+D` | Delete |
| `Ctrl+T` | Detach (from detail) |

### Floating IPs

| Key | Action |
|-----|--------|
| `Ctrl+N` | Allocate |
| `Ctrl+T` | Disassociate |
| `Ctrl+D` | Release |

### Security groups

| Key | Action |
|-----|--------|
| `Enter` | Expand / collapse rules |
| `Ctrl+D` | Delete selected rule |

### Load balancers

| Key | Action |
|-----|--------|
| `Enter` | View detail tree |
| `Ctrl+D` | Delete (cascade) |

### Create form

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next / prev field |
| `Enter` | Open picker |
| `Ctrl+S` | Submit |
| `Esc` | Cancel |

## Architecture

Built with:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2 — TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) v2 — styling
- [Bubbles](https://github.com/charmbracelet/bubbles) v2 — UI components
- [gophercloud](https://github.com/gophercloud/gophercloud) v2 — OpenStack SDK

```
src/internal/
  app/            # Root model, routing, actions, rendering (7 files)
  cloud/          # Auth, service detection, project listing
  compute/        # Nova: servers, flavors, keypairs, actions
  network/        # Neutron: networks, floating IPs, security groups
  volume/         # Cinder: volumes
  loadbalancer/   # Octavia: LBs, listeners, pools, members
  quota/          # Quota fetching (compute, network, storage)
  shared/         # Keys, styles, messages
  ui/             # All view components (14 packages)
```

## License

[Apache 2.0](LICENSE)

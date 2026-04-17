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

<p align="center">
  <img src="assets/lazystack-demo.gif" alt="lazystack demo" width="800">
</p>

**lazystack** is a fast, keyboard-first TUI for managing OpenStack resources from the terminal. It follows the "lazy" convention ([lazygit](https://github.com/jesseduffield/lazygit), [lazydocker](https://github.com/jesseduffield/lazydocker)) to provide an intuitive alternative to Horizon and the verbose OpenStack CLI.

Single binary. No runtime dependencies. Reads your standard `clouds.yaml`.

## Features

- **Server management** — list, create, delete, rename, rebuild, reboot, stop/start, pause, suspend, shelve, lock/unlock, rescue/unrescue, resize, snapshot, with bulk operations
- **Volume management** — list, detail, create, delete, attach (server picker), detach
- **Floating IPs** — allocate, associate, disassociate, release
- **Security groups** — create/delete groups, create/delete rules, expandable rule view
- **Networks** — create/delete networks, create/delete subnets, read-only port listing
- **Routers** — create/delete routers, add/remove interfaces (subnet picker), detail with routes
- **Key pairs** — create (RSA 2048/4096, ED25519), import with ~/.ssh/ file browser, detail view, save private key to file
- **Images** — list, detail, delete, deactivate/reactivate with status-aware coloring
- **Load balancers** (Octavia) — list, detail tree (listeners/pools/members), cascade delete
- **Project switching** — switch between accessible Keystone projects without restarting
- **Quota overlay** — compute, network, and storage quotas with color-coded progress bars
- **Confirmation dialogs** — all server state-change actions require explicit confirmation
- **Dynamic tabs** — tabs appear based on available services (no Cinder? no Volumes tab)
- **Auto-refresh** — all views refresh in the background at a configurable interval
- **SSH integration** — launch SSH sessions directly from the TUI, or copy the SSH command to clipboard
- **Console access** — retrieve noVNC console URL, open in browser or copy to clipboard
- **Server cloning** — clone servers with one keypress
- **Cross-resource navigation** — jump from server detail to attached volumes, security groups, or networks
- **Console log** and **action history** per server
- **Column sorting** on all list views
- **Client-side filtering** with `/`
- **Bulk select** with `space` for multi-server operations
- **Self-update** — `--update` flag to update to latest release
- **Solarized Dark** color scheme with status-aware coloring

## Installation

### Homebrew (macOS & Linux)

```bash
brew install larkly/tap/lazystack
```

### Arch Linux (AUR)

```bash
yay -S lazystack
```

### Debian / Ubuntu

Download the `.deb` from the [releases page](https://github.com/larkly/lazystack/releases/latest) and install:

```bash
sudo dpkg -i lazystack_*.deb
```

### Fedora / RHEL

Download the `.rpm` from the [releases page](https://github.com/larkly/lazystack/releases/latest) and install:

```bash
sudo rpm -i lazystack-*.rpm
```

### Pre-built binaries

Grab the latest release for your platform from the [releases page](https://github.com/larkly/lazystack/releases/latest).

### From source

```bash
cd src
make build
```

### With `go install`

```bash
go install github.com/larkly/lazystack/cmd/lazystack@latest
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
| `--cloud NAME` | | Connect directly to named cloud, skip picker |
| `--refresh N` | `5` | Auto-refresh interval in seconds |
| `--idle-timeout N` | `0` | Pause polling after N minutes of no input (0 = disabled) |
| `--no-check-update` | `false` | Skip the automatic update check on startup |
| `--update` | `false` | Self-update to the latest release |
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
| `o` | Stop / start |
| `p` | Pause / unpause |
| `Ctrl+Z` | Suspend / resume |
| `Ctrl+E` | Shelve / unshelve |
| `Ctrl+L` | Lock / unlock |
| `Ctrl+W` | Rescue / unrescue |
| `Ctrl+F` | Resize |
| `Ctrl+A` | Attach volume |
| `Ctrl+U` | Assign floating IP |
| `r` | Rename |
| `Ctrl+G` | Rebuild with new image |
| `Ctrl+S` | Create snapshot |
| `c` | Clone server |
| `x` | SSH into server |
| `y` | Copy SSH command |
| `Y` | Copy field (ID, IP, name, …) |
| `V` | Console URL (noVNC) |
| `L` | Console log |
| `a` | Action history |

### Server detail

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll |
| `Ctrl+D` | Delete |
| `Ctrl+O` / `Ctrl+P` | Soft / hard reboot |
| `o` | Stop / start |
| `p` | Pause / unpause |
| `Ctrl+Z` | Suspend / resume |
| `Ctrl+E` | Shelve / unshelve |
| `Ctrl+L` | Lock / unlock |
| `Ctrl+W` | Rescue / unrescue |
| `Ctrl+F` | Resize |
| `Ctrl+Y` / `Ctrl+X` | Confirm / revert resize |
| `Ctrl+A` | Attach volume |
| `Ctrl+U` | Assign floating IP |
| `r` | Rename |
| `Ctrl+G` | Rebuild with new image |
| `Ctrl+S` | Create snapshot |
| `c` | Clone server |
| `x` | SSH into server |
| `y` | Copy SSH command |
| `Y` | Copy field (ID, IP, name, …) |
| `V` | Console URL (noVNC) |
| `v` | Jump to volumes |
| `g` | Jump to security groups |
| `N` | Jump to networks |
| `L` | Console log |
| `a` | Action history |
| `Esc` | Back to list |

### Volumes

| Key | Action |
|-----|--------|
| `Enter` | View detail |
| `Ctrl+N` | Create volume |
| `Ctrl+D` | Delete |
| `Ctrl+A` | Attach to server (from detail) |
| `Ctrl+T` | Detach (from detail) |
| `Y` | Copy field (ID, name) |

### Floating IPs

| Key | Action |
|-----|--------|
| `Ctrl+N` | Allocate |
| `Ctrl+T` | Disassociate |
| `Ctrl+D` | Release |
| `Y` | Copy field (ID, floating IP, fixed IP, port ID) |

### Security groups

| Key | Action |
|-----|--------|
| `Enter` | Expand / collapse rules |
| `Ctrl+N` | Create group (or add rule when in rules) |
| `Ctrl+D` | Delete group (or rule when in rules) |
| `Y` | Copy field (ID, name, rule/server ID when focused) |

### Networks

| Key | Action |
|-----|--------|
| `Enter` | Expand / collapse subnets |
| `Ctrl+N` | Create network (or subnet when expanded) |
| `Ctrl+D` | Delete network (or subnet in subnets) |
| `Y` | Copy field (network/subnet/port ID, CIDR, IP, MAC) |

### Routers

| Key | Action |
|-----|--------|
| `Enter` | View detail (interfaces) |
| `Ctrl+N` | Create router |
| `Ctrl+D` | Delete router |
| `Ctrl+A` | Add interface (from detail) |
| `Ctrl+T` | Remove interface (from detail) |
| `Y` | Copy field (ID, name, gateway IP, interface subnet/port/IP) |

### Key pairs

| Key | Action |
|-----|--------|
| `Enter` | View detail (public key) |
| `Ctrl+N` | Create / import |
| `Ctrl+D` | Delete |
| `Y` | Copy field (name, public key in detail) |

### Load balancers

| Key | Action |
|-----|--------|
| `Enter` | View detail tree |
| `Ctrl+D` | Delete (cascade) |
| `Y` | Copy field (LB ID/name/VIP, listener/pool/member ID when focused) |

### Images

| Key | Action |
|-----|--------|
| `Enter` | View detail |
| `Ctrl+D` | Delete image |
| `Y` | Copy field (ID, name, checksum, owner, attached server ID) |

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
  app/            # Root model, routing, actions, rendering (8 files)
  cloud/          # Auth, service detection, project listing
  compute/        # Nova: servers, flavors, keypairs, actions
  image/          # Glance: images
  network/        # Neutron: networks, subnets, ports, routers, floating IPs, security groups
  volume/         # Cinder: volumes, volume types
  loadbalancer/   # Octavia: LBs, listeners, pools, members
  quota/          # Quota fetching (compute, network, storage)
  selfupdate/     # GitHub release self-update
  shared/         # Keys, styles, messages
  ui/             # All view components (38 packages)
```

## License

[Apache 2.0](LICENSE)

# lazystack — Product Requirements Document

## Overview

**lazystack** is a keyboard-driven terminal UI for OpenStack, built with Go. It follows the "lazy" convention (lazygit, lazydocker) to signal a fast, keyboard-first alternative to Horizon and the verbose OpenStack CLI. The goal is to make day-to-day OpenStack operations — especially VM lifecycle management — fast and intuitive from the terminal.

**License**: Apache 2.0
**Language**: Go
**Target cloud**: Any OpenStack cloud with Keystone v3, Nova v2.1+ (tested against cloud.rlnc.eu with microversion 2.100)

## Problem Statement

OpenStack operators and developers lack a fast, keyboard-driven terminal interface:

- **Horizon** is slow, requires a browser, and is painful for repetitive tasks
- **The OpenStack CLI** is verbose — simple operations require long commands with many flags
- **No Go-based TUI alternative exists** for OpenStack management

lazystack fills this gap by providing a single binary that connects to any OpenStack cloud via `clouds.yaml` and presents a navigable, auto-refreshing interface for the most common operations.

## Core Principles

1. **Keyboard-first**: Every action is reachable via keyboard shortcuts. Mouse support is not a goal.
2. **Fast startup**: Connect and show servers in under 2 seconds on a healthy cloud.
3. **Non-destructive by default**: Destructive actions (delete, reboot) always require confirmation.
4. **Minimal configuration**: Reads standard `clouds.yaml` — no additional config files needed.
5. **Single binary**: No runtime dependencies beyond the compiled Go binary.

## Target Users

- OpenStack operators managing VMs across one or more clouds
- Developers who provision and tear down test instances frequently
- Anyone who prefers terminal workflows over web UIs

## Architecture

### Technology Stack

- **TUI framework**: [Bubble Tea](https://charm.land/bubbletea) v2
- **UI components**: [Bubbles](https://charm.land/bubbles) v2 (spinner, text input)
- **Styling**: [Lip Gloss](https://charm.land/lipgloss) v2
- **OpenStack SDK**: [gophercloud](https://github.com/gophercloud/gophercloud) v2
- **YAML parsing**: `gopkg.in/yaml.v3` (for clouds.yaml cloud name extraction)

### Project Structure

```
lazystack/
  cmd/lazystack/main.go           # Entry point, CLI flags
  internal/
    app/
      app.go                      # Root model, view routing, modal overlay
    shared/
      keys.go                     # Global key bindings
      styles.go                   # Lipgloss theme constants (Solarized Dark)
      messages.go                 # Shared message types for inter-component communication
    cloud/
      client.go                   # Auth, service client initialization
      clouds.go                   # clouds.yaml parser
    compute/
      servers.go                  # Server CRUD (list, get, create, delete, reboot)
      flavors.go                  # Flavor listing
      keypairs.go                 # Keypair listing
    image/
      images.go                   # Image listing
    network/
      networks.go                 # Network listing
    ui/
      serverlist/
        serverlist.go             # Server table with auto-refresh, filtering
        columns.go                # Column definitions, status colors
      serverdetail/
        serverdetail.go           # Server property view
      servercreate/
        servercreate.go           # Create form with inline filterable pickers
      modal/
        confirm.go                # Confirmation dialog with focusable buttons
        error.go                  # Error modal
      cloudpicker/
        cloudpicker.go            # Cloud selection overlay
      statusbar/
        statusbar.go              # Bottom bar: cloud, region, context hints
      help/
        help.go                   # Help overlay
```

### View State Machine

```
                    ┌─────────────┐
        start ────→ │ Cloud Picker │
                    └──────┬──────┘
                           │ select cloud
                           ▼
                    ┌─────────────┐
              ┌───→ │ Server List  │ ←───────────────┐
              │     └──┬───┬───┬──┘                   │
              │    Enter│   │c  │d/r                   │
              │        ▼   ▼   ▼                      │
              │  ┌────────┐ ┌────────┐ ┌─────────┐   │
              │  │ Detail  │ │ Create │ │ Confirm │   │
              │  └───┬────┘ └───┬────┘ └────┬────┘   │
              │   Esc│      Esc/│Submit  y/n │        │
              └──────┘         └─────────────┘────────┘
```

Modals (confirm, error, help) overlay the active view and intercept all input until dismissed.

## Features

### MVP (Current)

#### Cloud Connection
- Parse `clouds.yaml` from standard locations: `./clouds.yaml`, `$OS_CLIENT_CONFIG_FILE`, `~/.config/openstack/clouds.yaml`, `/etc/openstack/clouds.yaml`
- Cloud picker overlay when multiple clouds are configured
- Auto-select when only one cloud exists (override with `--pick-cloud` flag)
- Switch clouds at any time with `C`
- Authentication via Keystone v3 using gophercloud's `clouds.Parse` + `config.NewProviderClient`

#### Server List
- Tabular display: Name, Status, IP, Flavor, Key, ID
- Auto-refresh at configurable interval (default 5s, set with `--refresh` flag)
- Auto-refresh persists across view changes (ticks always route to server list)
- Client-side filtering with `/` (case-insensitive match on name, ID, status, IP)
- Status colors: ACTIVE=green, BUILD=yellow, SHUTOFF=gray, ERROR=red, REBOOT=cyan
- Scrollable with cursor tracking

#### Server Detail
- Two-column property list: name, ID, status, flavor, image, IP, keypair, tenant, AZ, created, security groups, volumes
- Scrollable viewport
- Actions available: delete, soft reboot, hard reboot

#### Server Create
- Form fields: Name (text input), Image, Flavor, Network, Key Pair (inline filterable pickers)
- Parallel resource fetching on form open (images, flavors, networks, keypairs)
- Type-to-filter in picker dropdowns
- Focusable Submit/Cancel buttons with hotkey labels
- Navigation: Tab/Shift+Tab/Arrow keys between fields, Enter to open pickers
- Submit via button or Ctrl+S hotkey
- Keypair properly passed via `keypairs.CreateOptsExt`

#### Confirmation Modals
- Required for all destructive actions (delete, soft reboot, hard reboot)
- Focusable [y] Confirm / [n] Cancel buttons
- Navigate buttons with arrow keys, Tab, or use hotkeys directly
- Defaults to Cancel for safety

#### Error Handling
- API errors displayed in modal with context
- Dismissible with Enter or Esc
- Network errors during auto-refresh shown in status bar (non-blocking)
- Missing clouds.yaml shows helpful error with search paths

#### Help Overlay
- `?` toggles help overlay
- Keybindings grouped by context (Global, Server List, Server Detail, Create Form, Modals)

#### Status Bar
- Shows current cloud name and region
- Context-sensitive key hints per view
- Error/warning display

#### Edge Cases
- Terminal too small: centered warning with required dimensions (80x20 minimum)
- Empty server list: "press [c] to create" message
- No clouds.yaml: error modal with guidance

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--pick-cloud` | bool | false | Always show cloud picker, even with one cloud |
| `--refresh` | duration | 5s | Server list auto-refresh interval |

### Keybindings

#### Global
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `?` | Toggle help |
| `C` | Switch cloud |

#### Server List
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail |
| `c` | Create server |
| `d` | Delete server |
| `r` | Soft reboot |
| `R` | Force refresh |
| `/` | Filter |
| `Esc` | Clear filter |

#### Server Detail
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Scroll |
| `d` | Delete server |
| `r` | Soft reboot |
| `R` | Hard reboot |
| `Esc` | Back to list |

#### Create Form
| Key | Action |
|-----|--------|
| `Tab` / `↓` | Next field |
| `Shift+Tab` / `↑` | Previous field |
| `Enter` | Open picker / activate button |
| `Ctrl+S` | Submit (hotkey) |
| `Esc` | Cancel |

#### Modals
| Key | Action |
|-----|--------|
| `y` | Confirm |
| `n` / `Esc` | Cancel |
| `←/→` `↑/↓` `Tab` | Navigate buttons |
| `Enter` | Activate focused button |

### Visual Design

- **Color palette**: Solarized Dark base
- **Primary accent**: `#7D56F4` (purple)
- **Secondary**: `#6C71C4`
- **Status indicators**: Green (active/success), Yellow (building/warning), Red (error), Gray (stopped), Cyan (rebooting)
- **Selected row**: `#073642` background with bold text
- **Buttons**: Styled with background color, highlight on focus (green for confirm/submit, red for cancel/deny)

## Future Roadmap

These features are not yet implemented but represent the natural evolution of the tool.

### Phase 2: Extended Compute
- Server console log viewer
- Server action history
- Resize (flavor change)
- Pause/unpause, suspend/resume, shelve/unshelve
- Server group awareness
- Bulk actions (multi-select with space)

### Phase 3: Additional Resources
- Volume management (list, create, attach, detach, delete)
- Floating IP management (allocate, associate, release)
- Security group rule viewer/editor
- Keypair management (create, import, delete)
- Network/subnet/port browsing

### Phase 4: Multi-Cloud
- Side-by-side cloud comparison
- Cross-cloud server migration assistance
- Cloud health dashboard (API latency, quota usage)

### Phase 5: Quality of Life
- Configuration file (`~/.config/lazystack/config.yaml`) for defaults
- Custom column selection and ordering
- Saved filters
- Server name templates for create
- SSH integration (launch SSH session to selected server)
- Copy-to-clipboard for IDs, IPs
- Log/audit trail of actions taken

### Phase 6: Operational
- Quota display and warnings
- Hypervisor view (admin)
- Project/tenant switcher
- User management (admin)
- Service catalog browser

## Non-Goals

- **Mouse-driven interaction**: This is a keyboard-first tool. Mouse support may come later but is not a priority.
- **Full Horizon replacement**: lazystack focuses on the most common operations. Rarely-used Horizon features (e.g., orchestration, identity management) are out of scope.
- **Multi-platform OpenStack alternatives**: This tool is specifically for OpenStack, not a generic cloud TUI.
- **Configuration management**: lazystack is for interactive operations, not declarative infrastructure management (use Terraform/OpenTofu for that).

## Success Criteria

1. `go build` produces a single binary with zero runtime dependencies
2. Connects to any standard OpenStack cloud via `clouds.yaml`
3. Server list loads and auto-refreshes without manual intervention
4. Full VM lifecycle (list, inspect, create, delete, reboot) from keyboard
5. All destructive actions require explicit confirmation
6. Responsive at terminal sizes from 80x20 upward

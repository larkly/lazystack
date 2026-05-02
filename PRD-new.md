# lazystack — Product Requirements Document

## Overview

**lazystack** is a keyboard-driven terminal user interface (TUI) for managing OpenStack cloud resources. It provides a fast, terminal-native alternative to Horizon (the OpenStack web dashboard) and the verbose OpenStack CLI. The application follows the "lazy" naming convention (lazygit, lazydocker) to signal its keyboard-first, single-binary design philosophy.

- **Language**: Go (module: `github.com/larkly/lazystack`)
- **License**: Apache 2.0
- **Target**: Any OpenStack cloud with Keystone v3 identity, Nova v2.1+ compute, and optionally Cinder, Neutron, Glance, and Octavia services

## Problem Statement

OpenStack operators and developers lack a fast, keyboard-driven terminal interface for day-to-day operations. Horizon requires a browser and is slow for repetitive tasks. The OpenStack CLI demands verbose commands with many flags. No Go-based TUI alternative exists for OpenStack management.

lazystack fills this gap with a single binary that connects via standard `clouds.yaml` and presents a navigable, auto-refreshing interface for common operations.

## Architecture

### Technology Stack

| Component | Library | Version |
|-----------|---------|---------|
| TUI framework | charm.land/bubbletea | v2 |
| UI components | charm.land/bubbles | v2 |
| Styling | charm.land/lipgloss | v2 |
| OpenStack SDK | github.com/gophercloud/gophercloud | v2 |
| YAML parsing | gopkg.in/yaml.v3 | — |
| Clipboard | github.com/atotto/clipboard | — |
| Cryptography | golang.org/x/crypto | — |
| Go toolchain | — | 1.26+ |

### Project Structure

```
src/
  cmd/lazystack/main.go              # Entry point, CLI flag parsing, app bootstrap
  internal/
    app/                             # Root model, message routing, actions, rendering
      app.go                         # Root Model, Init, Update routing, type definitions
      actions_server.go              # Server CRUD, lifecycle actions, bulk operations, SSH
      actions_resource.go            # Volume, FIP, security group, network, router, LB, keypair actions
      actions_image.go               # Image CRUD, deactivate/reactivate
      routing.go                     # View routing, tick management, cross-resource navigation
      render.go                      # View rendering, tab bar, overlays, branding
      connect.go                     # Cloud connection and picker switching
      tabs.go                        # Dynamic tab registry (TabDef), tab switching, tab bar rendering
    shared/                          # Global constants, types, and utilities
      keys.go                        # Key binding definitions (60+ bindings)
      styles.go                      # Lipgloss theme (Solarized Dark), status colors, Unicode icons
      messages.go                    # Shared message types for inter-component communication
      format.go                      # Formatting helpers
      errors.go                      # Error utilities
      debug.go                       # Debug logging
      namededup.go                   # Name deduplication
    cloud/                           # OpenStack authentication and service client management
      client.go                      # Auth, service client initialization, optional service detection
      clouds.go                      # clouds.yaml parser and cloud name listing
      projects.go                    # Keystone project listing for project switching
    config/                          # Application configuration management
      config.go                      # Config struct, Load, Merge, Save, defaults
      apply.go                       # Apply config to runtime (shared styles, keys)
    compute/                         # Nova compute service wrappers
      servers.go                     # Server CRUD, pause, suspend, shelve, resize, reboot, lock, rescue
      flavors.go                     # Flavor listing
      keypairs.go                    # Keypair CRUD (list, get, create/generate, import, delete)
      actions.go                     # Instance action history
    image/                           # Glance image service wrappers
      images.go                      # Image listing, create, delete, update, deactivate/reactivate
    network/                         # Neutron network service wrappers
      networks.go                    # Network listing, external networks
      subnets.go                     # Subnet management (implied from subnetcreate/subnetedit)
      ports.go                       # Port CRUD (list, create, update, delete)
      routers.go                     # Router CRUD, interfaces, static routes
      floatingips.go                 # Floating IP CRUD (allocate, associate, disassociate, release)
      secgroups.go                   # Security group listing, rule create/delete
    volume/                          # Cinder block storage service wrappers
      volumes.go                     # Volume CRUD (list, get, create, delete, attach, detach, types)
    loadbalancer/                    # Octavia load balancer service wrappers
      lb.go                          # LB, listener, pool, member, health monitor CRUD
    quota/                           # Quota service wrappers
      quota.go                       # Compute, network, block storage quota fetching
    selfupdate/                      # GitHub release self-update
      selfupdate.go                  # Version check, download, apply
      cache.go                       # Update check caching
    ssh/                             # SSH utilities
      ssh.go                         # Key path finding, command building, IP selection
    ui/                              # All view components (51 packages)
      actionlog/                     # Instance action history viewer
      cloneprogress/                 # Server clone progress view
      cloudpicker/                   # Cloud selection overlay
      configview/                    # Configuration display and editor
      consolelog/                    # Scrollable console output viewer
      consoleurl/                    # VNC console URL retrieval and browser launch
      fippicker/                     # Floating IP picker modal
      floatingiplist/                # Floating IP table
      help/                          # Scrollable help overlay
      imagecreate/                   # Image upload form (file picker or URL)
      imagedownload/                 # Image download with directory picker
      imageedit/                     # Image edit form
      imageview/                     # Combined image view (list + detail + servers)
      keypaircreate/                 # Key pair create/import form
      keypairdetail/                 # Key pair detail view (public key)
      keypairlist/                   # Key pair table
      lbcreate/                      # Load balancer create/edit form
      lblistenercreate/              # LB listener create/edit form
      lbmembercreate/                # LB member create/edit form
      lbmonitorcreate/               # LB health monitor create/edit form
      lbpoolcreate/                  # LB pool create/edit form
      lbview/                        # Combined LB view (list + detail tree)
      modal/                         # Confirmation dialog, error modal
      networkcreate/                 # Network create form
      networkview/                   # Combined network view (list + subnets + ports)
      portcreate/                    # Port create form
      portedit/                      # Port edit form
      projectpicker/                 # Project picker modal
      quotaview/                     # Quota overlay with ASCII progress bars
      routercreate/                  # Router create form
      routerview/                    # Combined router list + detail with interfaces
      secgroupview/                  # Security group viewer with expandable rules
      servercreate/                  # Server create form with inline pickers, cloud-init
      serverdetail/                  # Server property view with networks, volumes, security groups
      serverlist/                    # Server table with adaptive columns
      serverpicker/                  # Server picker modal
      serverrebuild/                 # Server rebuild with image picker
      serverrename/                  # Server rename inline input
      serverresize/                  # Resize modal with flavor picker
      serversnapshot/                # Server snapshot creation
      sgcreate/                      # Security group create/rename/clone form
      sgrulecreate/                  # Security group rule create/edit modal
      sshprompt/                     # SSH IP selection prompt
      statusbar/                     # Bottom bar (cloud, project, region, hints)
      subnetcreate/                  # Subnet create form with IPv6 support
      subnetedit/                    # Subnet edit modal
      subnetpicker/                  # Subnet picker modal
      volumecreate/                  # Volume create form
      volumedetail/                  # Volume property view
      volumelist/                    # Volume table
      volumepicker/                  # Volume picker modal
```

### Architecture Patterns

- **Bubble Tea v2 Model Pattern**: The root `app.Model` implements `tea.Model` with value receivers. All sub-views follow the same pattern (struct with `Init()`, `Update()`, `View()` or equivalent) but are composed as fields on the root model rather than independent `tea.Model` instances.

- **Centralized Message Routing**: A single `Update` method on the root model dispatches messages to sub-views via a `switch` on the active view and active modal state. Non-key messages are broadcast to all views via `updateAllViews` to keep background auto-refresh ticks flowing.

- **Tick Chain Architecture**: Auto-refresh uses a single centralized tick timer (`refreshTickCmd`) that fires `TickMsg` at a configurable interval. The root model chains ticks by scheduling the next tick when processing each `TickMsg`. Views never create their own tick timers.

- **Modal Overlay Pattern**: Transient selection UI (resize picker, FIP picker, create forms, etc.) is implemented as modal overlays that sit on top of the current view. Each modal has an `Active` bool flag; when active, it intercepts all key messages before they reach the underlying view.

- **Dynamic Tab Registry**: Tabs are data-driven via `TabDef` structs (`{Name, Key}`). Tab availability is determined at connection time based on service catalog detection. Tabs are lazily initialized on first visit.

- **Import Cycle Avoidance**: Shared types (keys, styles, messages) live in `internal/shared/` rather than `internal/app/` to avoid import cycles between `app` and the UI packages.

- **Optimistic UI**: Server state-change actions (confirm resize, revert resize) use a `pendingAction` state that shows the expected state immediately and suppresses stale API responses until the real state catches up.

## Features

### Cloud Connection

- Parse `clouds.yaml` from standard paths: `./clouds.yaml`, `$OS_CLIENT_CONFIG_FILE`, `~/.config/openstack/clouds.yaml`, `/etc/openstack/clouds.yaml`
- Cloud picker overlay when multiple clouds are configured
- Auto-select when only one cloud exists (override with `--pick-cloud` or `--cloud NAME`)
- Switch clouds at any time with `C`
- Authentication via Keystone v3 using gophercloud's `clouds.Parse` + `config.NewProviderClient`
- Service catalog detection at connect time to conditionally enable optional features

### Server Management

- **Server List**: Adaptive columns with flex weights and priority-based hiding on narrow terminals. Columns include Name, Status (with power state), IPv4, IPv6, Floating IP, Flavor, Image, Age, Key. Auto-refresh, client-side filtering (`/`), column sorting (`s`/`S`), bulk selection (`space`). Lock indicator on names. Status/power state color coding.
- **Server Detail**: Two-column property list showing all metadata (name, ID, status, power state, flavor, image, keypair, locked, tenant, AZ, created, security groups). Networks section with IPs grouped by network name. Volumes section with attach/detach actions. Security groups section with cross-navigation. Auto-refresh with `pendingAction` for optimistic UI.
- **Server Create**: Form with name, image, flavor, network, key pair pickers (all with type-to-filter), user data/cloud-init file picker, and count (1–100). Batch creation via Nova `min_count`/`max_count`. Tab/Shift+Tab navigation.
- **Server Delete**: With optional volume detachment and deletion. Confirmation required.
- **Server Actions**: Soft reboot (`ctrl+o`), hard reboot (`ctrl+p`), stop/start (`o`), pause/unpause (`p`), suspend/resume (`ctrl+z`), shelve/unshelve (`ctrl+e`), lock/unlock (`ctrl+l`), rescue/unrescue (`ctrl+w`), resize (`ctrl+f`), rename (`r`), rebuild (`ctrl+g`), snapshot (`ctrl+s`), clone (`c`). All destructive actions require confirmation. All toggle actions auto-detect current state.
- **Bulk Operations**: Space-select for multi-server delete, reboot, pause, suspend, shelve, resize. Selection count shown in status bar. Mixed action support (e.g., some paused, others unpaused from the same confirm dialog).
- **Resize Flow**: Flavor picker modal with current flavor indicator. Optimistic UI for confirm/revert with pending action state. Staggered re-fetch (0.5s, 2s) for non-resize actions to catch state transitions.
- **Console Log** (`L`): Scrollable console output from Nova. Top/bottom navigation (`g`/`G`). Refresh.
- **Action History** (`a`): Scrollable list of instance actions with timestamps and request IDs. Failed actions highlighted.
- **Server Cloning**: Clone server configuration (flavor, network, keypair, security groups) with deduplicated name. Volume cloning with progress view and rollback on failure.
- **SSH Integration** (`x`): Launch SSH session with IP selection (floating IP, IPv6, IPv4 - IPv6 preferred). Key path auto-detection from keypair name. Option to ignore host key checking.
- **Copy SSH Command** (`y`): Copy SSH command string to clipboard.
- **Console Access** (`V`): Retrieve noVNC console URL from Nova. Open in browser or copy.
- **Cross-Resource Navigation**: From server detail, jump to attached volumes (`v`), security groups (`g`), or networks (`N`). Navigate back to server detail from those views.

### Volume Management

- **Volume List**: Adaptive columns (Name, Status, Size, Type, Attached To, Device, Bootable). Auto-refresh, sorting, filtering.
- **Volume Detail**: Full properties (Name, ID, Status, Size, Type, AZ, Bootable, Encrypted, Multiattach, Description, Created, Updated, Snapshot ID, Source Volume ID, Attached Server, Device, Metadata).
- **Create** (`ctrl+n`): Form with name, size (GB), type picker (from volume types API), AZ, description.
- **Delete** (`ctrl+d`): Confirmation modal. Works from list or detail.
- **Attach** (`ctrl+a`): Server picker modal with type-to-filter, showing only ACTIVE/SHUTOFF servers.
- **Detach** (`ctrl+t`): From detail or list view. Finds attached server and detaches.

### Floating IP Management

- **List**: Columns (Floating IP, Status, Fixed IP, Port ID). Auto-refresh, sorting, filtering.
- **Allocate** (`ctrl+n`): Allocates from first external network.
- **Associate** (`ctrl+u` from server): Opens FIP picker modal. If no unassociated IPs exist, auto-allocates and assigns.
- **Disassociate** (`ctrl+t`): Confirmation modal. Only enabled when FIP has a port.
- **Release** (`ctrl+d`): Confirmation modal.

### Security Group Management

- **Combined View**: Three-pane view with group selector, group info, and expandable rules.
- **Group List**: Expandable groups showing name, description, rule count.
- **Rule View**: Enter expands/collapses rules. Rules show direction, protocol, port range, remote, ethertype.
- **Create Group** (`ctrl+n` at group level): Form with name and description.
- **Delete Group** (`ctrl+d` at group level): Confirmation modal. Default group protected.
- **Create Rule** (`ctrl+n` in rules): Modal with cycle pickers for direction, ethertype, protocol; port range; remote IP prefix.
- **Edit Rule** (`enter` on rule): Edit existing rule properties.
- **Delete Rule** (`ctrl+d` in rules): Confirmation modal.
- **Rename Group** (`r`): Inline rename form.
- **Clone Group** (`c`): Clone security group and all its rules.
- **Server Listing**: Shows servers using each security group with cross-navigation.

### Network Management

- **Combined View**: Three-pane view with network list, expandable subnets, and port management.
- **Expandable Subnets**: Enter expands/collapses to show subnet details (name, CIDR, gateway, IP version, DHCP status, IPv6 address/RA modes).
- **Create Network** (`ctrl+n`): Form with name, admin state, shared option.
- **Delete Network** (`ctrl+d`): Confirmation modal.
- **Create Subnet** (`ctrl+n` when expanded): Form with name, CIDR, gateway, IP version, DHCP. Full IPv6 support with address mode (DHCP stateful, DHCP stateless, SLAAC, unmanaged) and RA mode configuration.
- **Edit Subnet** (`enter` on subnet): Modal to modify subnet properties. Hidden IPv6-specific fields skipped during tab navigation when IP version is v4.
- **Delete Subnet** (`ctrl+d` in subnets): Confirmation modal.
- **Port CRUD**: Create ports with port security toggle and allowed address pairs. Edit existing ports. Delete ports with confirmation (warns if port is in use by device owner).

### Router Management

- **Combined View**: Router list with detail pane showing properties, interfaces, and static routes.
- **Router List**: Columns (Name, Status, External Gateway, Routes). Auto-refresh, sorting, filtering.
- **Router Detail** (`enter`): Properties plus interfaces section.
- **Create Router** (`ctrl+n`): Form with name, external network selection, admin state.
- **Delete Router** (`ctrl+d`): Confirmation modal.
- **Add Interface** (`ctrl+a`/`ctrl+n` from detail): Subnet picker modal with optional custom IP assignment.
- **Remove Interface** (`ctrl+t` from detail): Handles single-IP and multi-IP router ports correctly. For multi-IP ports, removes only the target fixed IP.
- **IPv6 Handling**: Auto-addressed IPv6 subnets handled correctly.

### Load Balancer Management (Octavia)

- **Combined View**: Five-pane view with LB selector, LB info, listeners, pools, and members in a tree structure.
- **Create LB** (`ctrl+n`): Form with name, VIP subnet, VIP address.
- **Edit LB** (`enter` on info pane): Edit name and description.
- **Delete LB** (`ctrl+d`): Cascade delete showing counts of listeners, pools, and members to be removed.
- **Admin State Toggle** (`o`): Toggle admin state on LBs, listeners, pools, and members contextually.
- **Listeners**: Create, edit (name, description, connection limit), delete.
- **Pools**: Create (algorithm, protocol, session persistence), edit, delete (warns about members and monitors).
- **Members**: Create (with subnet picker, server picker for Nova instances, duplicate detection), edit (weight, admin state, backup, monitor address/port, tags), delete, bulk delete with selection (`space`/`x`), drain (`w` to set weight to 0, bulk and individual).
- **Health Monitors**: Create (type, delay, timeout, max retries), edit, delete.
- **Pending State Guard**: All mutation actions blocked while any LB is in PENDING_* provisioning status.
- **Bulk Operations**: Bulk member delete and bulk member drain with sequential execution and wait-for-active between operations.

### Image Management

- **Combined View**: Three-pane view with image list, image info/properties, and servers-using-image.
- **Image Detail** (`enter`): Full properties view with servers using this image.
- **Upload** (`ctrl+n`): Local file picker or URL import. Disk format auto-detection (qcow2, raw, vmdk, vdi, iso, vhd, aki, ari, ami). Auto-fills name from filename. Progress bar.
- **Download** (`ctrl+g`): Stream image to local file with directory picker and overwrite protection. Progress bar.
- **Edit** (`enter` on info): Modify name, visibility, min disk/RAM, tags, protected flag.
- **Delete** (`ctrl+d`): Confirmation modal. Protected images blocked.
- **Deactivate/Reactivate** (`d`): Toggle image availability. Status-aware confirm message.
- **Cross-Resource Navigation**: Servers using the image shown with navigable links.

### Key Pair Management

- **Keypair List**: Columns (Name, Type). Sorting, auto-refresh, filtering.
- **Keypair Detail** (`enter`): Shows name, type, and full public key with scroll.
- **Create/Import** (`ctrl+n`): Form with name, type picker (RSA 2048/RSA 4096/ED25519). Keys generated locally using Go crypto (`crypto/rsa`, `crypto/ed25519`, `x/crypto/ssh`). Import via Nova API with `~/.ssh/` file browser.
- **Save Private Key** (`s`): Save generated private key to file (default `~/.ssh/<name>`, 0600 permissions). Public key saved alongside as `.pub`.
- **Delete** (`ctrl+d`): Confirmation modal. Works from list or detail.

### Project Switching

- After cloud connection, accessible projects fetched in background via Keystone `ListAvailable`.
- `P` key opens project picker modal when multiple projects available. Current project marked with `*`.
- On selection, re-authenticates scoped to new project via `ConnectWithProject` (overrides TenantID).
- All tabs reset on project switch (same as cloud switch).
- Project name shown in status bar between cloud and region.

### Quota Overlay

- `Q` key opens full-screen quota overlay (same pattern as help).
- Three sections: Compute (instances, cores, RAM, key pairs, server groups), Network (floating IPs, networks, ports, routers, security groups, subnets), Block Storage (volumes, gigabytes, snapshots, backups).
- ASCII progress bars with color coding: green (<70%), yellow (70-90%), red (>90%).
- Unlimited quotas (limit=-1) shown as "used / unlimited" with no bar.
- Lazy fetch on open, cached for 30 seconds.
- Block Storage section omitted if Cinder unavailable.
- Scroll support, close with `Q` or `Esc`.

### Configuration

- Config file at `~/.config/lazystack/config.yaml` (auto-created on first save).
- Configurable settings: refresh interval, idle timeout, plain mode, update checks, cloud picker behavior, SSH host key checking, update check interval.
- Full color palette customization (primary, secondary, success, warning, error, muted, bg, fg, highlight, cyan hex colors).
- Full keybinding remapping (60+ actions mappable).
- In-app config viewer/editor (`ctrl+k`).
- CLI flags take precedence over config file values.

### UI Components

- **Status Bar**: Shows cloud name, project name, and region. Context-sensitive key hints per view (adapts to server state, selection count). Sticky hints for action success messages. Error/warning display. Truncates when overflow.
- **Help Overlay** (`?`): Scrollable help with keybindings grouped by context (Global, Server List, Server Detail, Create Form, Console Log, Modals). Scrollable with `↑/↓`/`pgup`/`pgdn`.
- **Confirmation Modals**: For all destructive actions. Focusable [y] Confirm / [n] Cancel buttons. Navigate with arrow keys, Tab, or hotkeys. Supports single-server and bulk operations with custom title/body. Defaults to Cancel.
- **Error Modals**: API errors displayed with context. Dismissible with Enter or Esc.
- **Terminal Too Small**: Centered warning with required dimensions (80×20 minimum).
- **Empty States**: "press [ctrl+n] to create" message for empty lists.
- **LAZYSTACK Branding**: Purple badge in top-right corner. Version number displayed. Update available indicator.

### Self-Update

- `--update` flag downloads latest release from GitHub.
- `--no-check-update` skips automatic version check on startup.
- Downloads binary for current OS/architecture.
- SHA256 checksum verification.
- Update check result cached (configurable interval, default 24h).

### Build and Distribution

- Cross-platform builds: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64.
- Package formats: `.deb` (Debian/Ubuntu), `.rpm` (Fedora/RHEL), `.pkg.tar.zst` (Arch Linux).
- Distribution channels: Homebrew (`brew install larkly/tap/lazystack`), AUR (`yay -S lazystack`), direct binary download from GitHub releases.
- Pre-built binaries available on GitHub releases page.
- Build from source: `cd src && make build`.
- Install via `go install github.com/larkly/lazystack/cmd/lazystack@latest`.
- Single binary with zero runtime dependencies.
- GitHub Actions CI/CD: automated release pipeline triggered by version tags.

### Visual Design

- **Color Palette**: Solarized Dark base with customizable hex colors.
- **Primary Accent**: Purple (`#7D56F4`) — used for branding badge, selected items, focused buttons, active tab.
- **Status Indicators**: Green (ACTIVE/RUNNING), Yellow (BUILD/warning), Red (ERROR), Gray (SHUTOFF/inactive), Cyan (REBOOT/transitional).
- **Status Icons**: Unicode prefixes — ● (healthy/active), ▲ (in-progress/transitional), ✘ (error/failed), ○ (off/inactive), ↻ (transient), ■ (paused/held). Disable with `--plain` flag.
- **Power State Colors**: Green (RUNNING), Gray (SHUTDOWN), Red (CRASHED), Muted (PAUSED/SUSPENDED).
- **Selected Row**: Darker background with bold text.
- **Bulk Selected**: ● prefix with purple text.
- **Locked Servers**: 🔒 prefix on name.
- **Tab Bar**: Active tab highlighted with background accent. Inactive tabs muted. Number shortcuts `1-9` shown.
- **Buttons**: Styled with background color. Highlight on focus (green for confirm/submit, red for cancel/deny).

## CLI Interface

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--version` | bool | false | Print version and exit |
| `--pick-cloud` | bool | false | Always show cloud picker, even with one cloud |
| `--cloud NAME` | string | "" | Connect directly to named cloud, skip picker |
| `--refresh N` | int | 5 | Auto-refresh interval in seconds |
| `--idle-timeout N` | int | 0 | Pause polling after N minutes of no input (0 = disabled) |
| `--no-check-update` | bool | false | Skip automatic update check on startup |
| `--update` | bool | false | Self-update to the latest version |
| `--plain` | bool | false | Disable Unicode status icons |
| `--debug` | bool | false | Write debug log to `~/.cache/lazystack/debug.log` |

## Configuration File

File location: `~/.config/lazystack/config.yaml`

```yaml
general:
  refresh_interval: 5          # seconds
  idle_timeout: 0              # minutes (0 = disabled)
  plain_mode: false            # disable Unicode icons
  check_for_updates: true      # auto-check on startup
  always_pick_cloud: false     # always show cloud picker
  ignore_ssh_host_keys: false  # skip host key verification for SSH
  update_check_interval: 24    # hours between update checks

colors:
  primary: "#7D56F4"
  secondary: "#6C71C4"
  success: "#2AA198"
  warning: "#B58900"
  error: "#DC322F"
  muted: "#657B83"
  bg: "#002B36"
  fg: "#839496"
  highlight: "#FDF6E3"
  cyan: "#2AA198"

keybindings:
  quit: "q,ctrl+c"
  help: "?"
  cloud_pick: "C"
  filter: "/"
  # ... 50+ more mappable actions
```

## OpenStack Service Coverage

| Service | API | Usage |
|---------|-----|-------|
| Keystone | v3 Identity | Authentication, project listing |
| Nova | v2.1+ Compute (microversion 2.100) | Server CRUD, flavors, keypairs, actions, console |
| Glance | v2 Image | Image CRUD, upload, download, deactivate/reactivate |
| Neutron | v2 Network | Networks, subnets, ports, routers, floating IPs, security groups |
| Cinder | v1/v2/v3 Block Storage | Volume CRUD, attach, detach, volume types |
| Octavia | v2 Load Balancer | LB, listener, pool, member, health monitor CRUD |

## Design Principles

1. **Keyboard-First**: Every action reachable via keyboard shortcuts. No mouse dependency.
2. **Single Binary**: Zero runtime dependencies beyond the compiled Go binary.
3. **Standard Configuration**: Reads standard `clouds.yaml`. No additional config required for basic use.
4. **Safe by Default**: Destructive actions require Ctrl-prefixed shortcuts and explicit confirmation modals. Cannot trigger destructive operations by typing in the wrong window.
5. **Fast Startup**: Connect and show resources in under 2 seconds on a healthy cloud.
6. **Non-Destructive Defaults**: All confirmation dialogs default to Cancel for safety.
7. **Adaptive Display**: Columns adapt to terminal size. Minimum 80×20 terminal supported.

## Success Criteria

1. `go build` produces a single binary with zero runtime dependencies
2. Connects to any standard OpenStack cloud via `clouds.yaml`
3. Resource lists load and auto-refresh without manual intervention
4. Full CRUD lifecycle for all supported resource types from keyboard
5. All destructive actions require Ctrl-prefix and explicit confirmation
6. Responsive at terminal sizes from 80×20 upward
7. Bulk operations work on multi-selected resources
8. Cross-platform support (Linux x86_64/ARM64, macOS x86_64/ARM64)

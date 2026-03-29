# lazystack — Product Requirements Document

## Overview

**lazystack** is a keyboard-driven terminal UI for OpenStack, built with Go. It follows the "lazy" convention (lazygit, lazydocker) to signal a fast, keyboard-first alternative to Horizon and the verbose OpenStack CLI. The goal is to make day-to-day OpenStack operations — especially VM lifecycle management — fast and intuitive from the terminal.

**License**: Apache 2.0
**Language**: Go
**Target cloud**: Any OpenStack cloud with Keystone v3, Nova v2.1+ (tested against cloud.rlnc.eu with microversion 2.100)

## Implementation Status

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 1: MVP | ✓ Complete | Cloud connection, server CRUD, modals, help |
| Phase 2: Extended Compute | ✓ Complete | All actions, console log, resize, bulk ops, action history |
| Phase 3: Additional Resources | ✓ Complete | Tabbed navigation, volumes, floating IPs, security groups, key pairs, networks (CRUD), routers (CRUD) |
| Phase 4: Refactor, Octavia, Projects, Quotas | ✓ Complete | App refactor, dynamic tabs, Octavia LB tab, project switching, quota overlay |
| Phase 5: Quality of Life | Partial | Server rename, rebuild, snapshot, rescue/unrescue, image management, confirmation dialogs for all actions, SSH integration, console access (noVNC), server cloning, cross-resource navigation — done. Clipboard (general), config file, DNS — not started |
| Phase 6: Operational | Not started | Admin views, hypervisor view, service catalog browser |

## Concerns and Considerations

### Architecture

- **Value receiver pattern**: Bubble Tea v2 uses value receivers for `Update()`, which means model mutations require returning new values. This interacts poorly with optimistic UI updates — changes made before an async command fires can be overwritten when the command's response arrives and gets routed through `updateActiveView`. This caused the resize confirmation banner to flicker (optimistic status set to ACTIVE, then stale API response overwrote it back to VERIFY_RESIZE). Solved with a `pendingAction` state that suppresses stale updates until the real state catches up.

- **Message routing complexity**: The root model routes messages to sub-views via a `switch` on the active view. Adding overlay modals (resize picker, confirm, error, help) that intercept messages creates ordering dependencies in the `Update` method. The resize modal being `Active` was swallowing messages meant for other views. Each new modal/overlay adds routing complexity — consider a message bus or middleware pattern if this grows further. Phase 4's refactor split app.go from 1,643 lines into 7 focused files (app.go, actions_server.go, actions_resource.go, routing.go, render.go, connect.go, tabs.go), which makes this manageable for now.

- **Import cycle avoidance**: Shared types (keys, styles, messages) live in `internal/shared/` rather than `internal/app/` to avoid import cycles between `app` and the UI packages. This is a pragmatic workaround but means the "app" package is really just the root model + view routing.

### UX Lessons Learned

- **Ctrl-prefixed dangerous actions**: Originally used `c`/`d`/`r` for create/delete/reboot. Changed to `ctrl+n`/`ctrl+d`/`ctrl+o` after realizing typing in the wrong terminal window could trigger destructive operations. This is a good pattern for any keyboard-first TUI.

- **Optimistic UI is essential for async APIs**: OpenStack actions like `confirmResize` return 202 (accepted) but the server state doesn't change immediately. Polling the API right after the action often returns stale state. The `pendingAction` pattern — show the expected state immediately and suppress stale API responses until the real state catches up — provides much better UX than waiting for the tick.

- **Column adaptivity matters**: Fixed-width columns don't work across terminal sizes. The flex-weight + priority system (columns get proportional extra space, and low-priority columns hide on narrow terminals) works well. IPv6 addresses (39 chars) are particularly challenging — they need the highest flex weight but lowest display priority since they're rarely needed at a glance.

- **Modal vs view**: The resize flavor picker was initially a full view, which caused navigation issues (Esc from resize opened from the list panicked because there was no detail view to go back to). Converting it to a modal overlay that sits on top of the current view eliminated the problem entirely. Prefer modals for transient selection UI.

- **Auto-refresh must survive view changes**: The server list's auto-refresh tick was breaking when navigating to other views because the tick message got routed to the wrong view. Fixed with `updateAllViews` that routes non-key messages to all initialized tab views. Modal overlays (resize, FIP picker) must not swallow background ticks — route to all views first, then to the active modal.

- **Tick chain fragility**: Each view's auto-refresh is a chain: tick fires → fetch + schedule next tick. If any message in the chain gets swallowed (e.g., by a modal that doesn't handle it), the chain breaks permanently. The fix is to always route messages to background views before routing to modal overlays.

### Technical Debt

- **Bulk action support is partial**: Space-select works for delete, reboot, pause, suspend, shelve, and resize. Console log only operates on a single server (the cursor, not the selection), which is intentional since console log is inherently single-server. Bulk resize applies the same target flavor to all selected servers.

- **Image name resolution**: Nova's server response doesn't include the image name (only ID) with newer microversions. The server list fetches all images from Glance to resolve names, which works but adds an extra API call on every refresh. Should cache more aggressively or resolve lazily.

- **Server detail refresh creates a new model**: After actions from the detail view, the detail model is recreated with `New()` + `Init()` to force a fresh fetch. This resets scroll position and loses the pending action state if not carefully managed. A proper `Refresh()` method on the detail model would be cleaner.

- **Limited tests**: The codebase has minimal test coverage (cloud, compute, serverlist columns). The compute layer functions are thin wrappers around gophercloud and would benefit from interface-based mocking. The UI components are harder to test but snapshot testing of `View()` output would catch rendering regressions.

- **Error handling in bulk operations**: Bulk actions collect errors and report them as a single concatenated string. Individual failure tracking and partial success reporting would be better UX.

### OpenStack API Considerations

- **Microversion dependency**: The app sets microversion `2.100` on the compute client. This means it requires a relatively recent Nova deployment. The `original_name` field in the flavor response (used for display) requires microversion 2.47+. Should gracefully degrade for older clouds.

- **Image map structure varies by microversion**: The `image` field in the server response is `map[string]any` and its contents depend on the microversion. With 2.100, it typically only contains `id` and `links` — no `name`. Boot-from-volume servers have an empty image map.

- **Unshelve requires non-nil opts**: gophercloud's `Unshelve()` panics if passed `nil` opts (it calls `opts.ToUnshelveMap()` on the nil interface). Must pass `servers.UnshelveOpts{}`. This is arguably a gophercloud bug.

- **Locked server awareness**: The server list shows a 🔒 icon for locked servers, but doesn't prevent actions on them — the API will reject the action and the error modal will display. Could pre-check lock status and show a more helpful message.

## Problem Statement

OpenStack operators and developers lack a fast, keyboard-driven terminal interface:

- **Horizon** is slow, requires a browser, and is painful for repetitive tasks
- **The OpenStack CLI** is verbose — simple operations require long commands with many flags
- **No Go-based TUI alternative exists** for OpenStack management

lazystack fills this gap by providing a single binary that connects to any OpenStack cloud via `clouds.yaml` and presents a navigable, auto-refreshing interface for the most common operations.

## Core Principles

1. **Keyboard-first**: Every action is reachable via keyboard shortcuts. Mouse support is not a goal.
2. **Fast startup**: Connect and show servers in under 2 seconds on a healthy cloud.
3. **Non-destructive by default**: Destructive actions require Ctrl-prefixed shortcuts and confirmation modals.
4. **Minimal configuration**: Reads standard `clouds.yaml` — no additional config files needed.
5. **Single binary**: No runtime dependencies beyond the compiled Go binary.
6. **Safe by default**: Can't accidentally trigger destructive actions by typing in the wrong window.

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
src/
  cmd/lazystack/main.go             # Entry point, CLI flags, restart via syscall.Exec
  internal/
    app/
      app.go                        # Root model, New(), Init(), Update() routing, type defs
      actions_server.go             # Server CRUD/lifecycle actions, bulk operations
      actions_image.go              # Image CRUD actions
      actions_resource.go           # Volume, FIP, security group, keypair, LB actions
      routing.go                    # View routing, modal updates, view change handling
      render.go                     # View(), viewContent(), viewName()
      connect.go                    # Cloud connection and picker switching
      tabs.go                       # Dynamic tab registry (TabDef), tab switching, tab bar
    shared/
      keys.go                       # Global key bindings (Ctrl-prefixed for dangerous ops)
      styles.go                     # Lipgloss theme constants (Solarized Dark)
      messages.go                   # Shared message types for inter-component communication
    cloud/
      client.go                     # Auth, service client initialization, optional service detection
      clouds.go                     # clouds.yaml parser
      projects.go                   # Keystone project listing for project switching
    compute/
      servers.go                    # Server CRUD + pause/suspend/shelve/resize/reboot
      actions.go                    # Instance action history
      flavors.go                    # Flavor listing
      keypairs.go                   # Keypair CRUD (list, get, create/generate, import, delete)
    image/
      images.go                     # Image listing
    network/
      networks.go                   # Network listing, external networks, port lookup
      routers.go                    # Router CRUD, interfaces, static routes
      floatingips.go                # Floating IP CRUD (allocate, associate, disassociate, release)
      secgroups.go                  # Security group listing, rule create/delete
    volume/
      volumes.go                    # Volume CRUD (list, get, create, delete, attach, detach, volume types)
    selfupdate/                     # GitHub release self-update
    loadbalancer/
      lb.go                         # Octavia LB, listener, pool, member CRUD
    quota/
      quota.go                      # Compute, network, block storage quota fetching
    ui/
      serverlist/
        serverlist.go               # Server table with auto-refresh, filtering, bulk select, sorting
        columns.go                  # Adaptive columns with flex weights and priority hiding
      serverdetail/
        serverdetail.go             # Server property view with auto-refresh, network names
      servercreate/
        servercreate.go             # Create form with inline pickers, count field
      serverresize/
        serverresize.go             # Resize modal with flavor picker, current flavor indicator
      consolelog/
        consolelog.go               # Scrollable console output viewer
      actionlog/
        actionlog.go                # Instance action history viewer
      volumelist/
        volumelist.go               # Volume table with sorting, server name resolution
      volumedetail/
        volumedetail.go             # Volume property view with metadata, attachment info
      floatingiplist/
        floatingiplist.go           # Floating IP table with sorting
      secgroupview/
        secgroupview.go             # Security group viewer with expandable rules, rule deletion
      keypaircreate/
        keypaircreate.go            # Key pair create/import form with type picker, file browser
      keypairdetail/
        keypairdetail.go            # Key pair detail view showing public key
      keypairlist/
        keypairlist.go              # Key pair table with sorting, delete, auto-refresh
      lblist/
        lblist.go                   # Load balancer table with status colors, sorting
      lbdetail/
        lbdetail.go                 # LB detail with listener/pool/member tree
      serverrename/                 # Server rename inline input
      serverrebuild/                # Server rebuild with image picker
      serversnapshot/               # Server snapshot creation
      networklist/
        networklist.go              # Network browser with expandable subnets
      networkcreate/                # Network create form
      subnetcreate/                 # Subnet create form
      routerlist/                   # Router list with sorting
      routerdetail/                 # Router detail with interfaces
      routercreate/                 # Router create form
      subnetpicker/                 # Subnet picker modal
      sgcreate/                     # Security group create form
      imagelist/                    # Image list with sorting
      imagedetail/                  # Image detail properties view
      volumecreate/
        volumecreate.go             # Volume create form with type/AZ pickers
      serverpicker/
        serverpicker.go             # Server picker modal for volume attach
      sgrulecreate/
        sgrulecreate.go             # Security group rule create modal
      fippicker/
        fippicker.go                # Floating IP picker modal for server association
      projectpicker/
        projectpicker.go            # Project picker modal for project switching
      quotaview/
        quotaview.go                # Quota overlay with ASCII progress bars
      modal/
        confirm.go                  # Confirmation dialog (single + bulk), custom body/title
        error.go                    # Error modal
      cloudpicker/
        cloudpicker.go              # Cloud selection overlay
      statusbar/
        statusbar.go                # Bottom bar: cloud, project, region, context hints
      help/
        help.go                     # Scrollable help overlay
```

### View State Machine

```
                    ┌─────────────┐
        start ────→ │ Cloud Picker │
                    └──────┬──────┘
                           │ select cloud (auto if single)
                           ▼
  ┌────────────── Dynamic Tab Bar (1-N / ←→) ──────────────────────┐
  │  Tabs built from service catalog: Servers always, Volumes if   │
  │  Cinder, Floating IPs/Sec Groups always, LBs if Octavia,      │
  │  Key Pairs always                                              │
  │                                                                │
  │ ┌──────────┐ ┌──────────┐ ┌────────┐ ┌───────┐ ┌────┐ ┌─────┐│
  │ │Server    │ │Volume    │ │Float   │ │SecGrp │ │LBs │ │Keys ││
  │ │List      │ │List      │ │IP List │ │View   │ │List│ │List ││
  │ └──┬───┬───┘ └──┬───────┘ └────────┘ └───────┘ └─┬──┘ └─────┘│
  │    │   │ ^n   Enter                             Enter         │
  │    │   │       │                                  │           │
  │  Enter ▼       ▼                                  ▼           │
  │    │ ┌──────┐ ┌───────────┐              ┌───────────┐        │
  │    │ │Create│ │Vol Detail │              │LB Detail  │        │
  │    ▼ └──────┘ └───────────┘              └───────────┘        │
  │ ┌──────────┐                                                  │
  │ │Server    │                                                  │
  │ │Detail    │                                                  │
  │ └──┬──┬────┘                                                  │
  │    l  a                                                       │
  │    │  │                                                       │
  │    ▼  ▼                                                       │
  │ ┌────────┐ ┌──────────┐                                       │
  │ │Console │ │Action Log│                                       │
  │ └────────┘ └──────────┘                                       │
  └────────────────────────────────────────────────────────────────┘

  Overlays (always available):
  ┌─────────────┐ ┌──────────┐ ┌──────┐ ┌────────┐ ┌──────────┐
  │Confirm Modal│ │Error     │ │Help  │ │Resize  │ │FIP Picker│
  │(y/n/enter)  │ │(enter)   │ │(?)   │ │(^f)    │ │(^a)      │
  └─────────────┘ └──────────┘ └──────┘ └────────┘ └──────────┘
  ┌──────────────┐ ┌──────────┐
  │Project Picker│ │Quotas    │
  │(P)           │ │(Q)       │
  └──────────────┘ └──────────┘
```

## Features

### Phase 1: MVP (Complete)

#### Cloud Connection
- Parse `clouds.yaml` from standard locations: `./clouds.yaml`, `$OS_CLIENT_CONFIG_FILE`, `~/.config/openstack/clouds.yaml`, `/etc/openstack/clouds.yaml`
- Cloud picker overlay when multiple clouds are configured
- Auto-select when only one cloud exists (override with `--pick-cloud` flag)
- Switch clouds at any time with `C`
- Authentication via Keystone v3 using gophercloud's `clouds.Parse` + `config.NewProviderClient`

#### Server List
- Adaptive columns with flex weights — columns grow to fill terminal width
- Priority-based column hiding on narrow terminals (Name/Status always visible, IPv6 hides first)
- Columns: Name, Status (with power state), IPv4, IPv6, Floating IP, Flavor, Image, Age, Key
- Image names resolved from Glance (Nova only returns image ID with microversion 2.100)
- Lock indicator (🔒) on server names
- Auto-refresh at configurable interval (default 5s, set with `--refresh` flag)
- Auto-refresh persists across view changes (ticks always route to server list)
- Client-side filtering with `/` (case-insensitive match on name, ID, status, IPs, flavor, image)
- Status/power colors: ACTIVE/RUNNING=green, BUILD=yellow, SHUTOFF=gray, ERROR=red, REBOOT=cyan
- Bulk selection with `space` (selected servers shown with ● prefix in purple)
- LAZYSTACK branding badge in top-right corner

#### Server Detail
- Two-column property list: name, ID, status, power state, flavor, image (name + ID), keypair, locked, tenant, AZ, created, security groups, volumes
- Networks section: IPs grouped by network name with type and version info
- Auto-refresh at same interval as server list
- Scrollable viewport
- Resize pending banner with confirm/revert actions
- Pending action state — optimistic UI that suppresses stale API responses
- Empty fields hidden for cleaner display

#### Server Create
- Form fields: Name, Image, Flavor, Network, Key Pair (inline filterable pickers), Count
- Count field (1–100) for batch creation using Nova's `min_count`/`max_count`
- Parallel resource fetching on form open (images, flavors, networks, keypairs)
- Type-to-filter in picker dropdowns
- Cursor advances to next field after picker selection
- Focusable Submit/Cancel buttons with hotkey labels
- Navigation: Tab/Shift+Tab/Arrow keys between fields, Enter to open pickers
- Submit via button or Ctrl+S hotkey

#### Confirmation Modals
- Required for all server state-change actions (delete, reboot, stop/start, pause, suspend, shelve, lock/unlock, rescue/unrescue)
- Supports both single-server and bulk operations
- Focusable [y] Confirm / [n] Cancel buttons
- Navigate buttons with arrow keys, Tab, or use hotkeys directly
- Defaults to Cancel for safety

#### Error Handling
- API errors displayed in modal with context
- Dismissible with Enter or Esc
- Network errors during auto-refresh shown in status bar (non-blocking)
- Missing clouds.yaml shows helpful error with search paths

#### Help Overlay
- `?` toggles scrollable help overlay
- Keybindings grouped by context (Global, Server List, Server Detail, Create Form, Console Log, Modals)
- Scrollable with ↑/↓ when content doesn't fit

#### Status Bar
- Shows current cloud name, project, and region
- Context-sensitive key hints per view (adapts to server state, selection count)
- Sticky hints for action success messages (survives background auto-refresh, clears on next key press)
- Error/warning display
- Truncates gracefully when bar overflows

#### Edge Cases
- Terminal too small: centered warning with required dimensions (80x20 minimum)
- Empty server list: "press [ctrl+n] to create" message
- No clouds.yaml: error modal with guidance
- Ctrl+R restarts the app (re-exec with same flags) for rapid testing after rebuilds

### Phase 2: Extended Compute (Complete)

#### Server Actions
- Pause/unpause (`p`) — auto-detects current state
- Suspend/resume (`ctrl+z`) — auto-detects current state
- Shelve/unshelve (`ctrl+e`) — auto-detects current state
- Resize (`ctrl+f`) — modal flavor picker with filter, current flavor marked with ★
- Confirm resize (`ctrl+y`) / Revert resize (`ctrl+x`) with optimistic UI
- Hard reboot (`ctrl+p`)
- All toggle actions read server status from both list and detail views

#### Console Log Viewer (`l`)
- Scrollable console output (last 500 lines from Nova)
- `g`/`G` for top/bottom navigation
- `R` to refresh
- `Esc` returns to previous view

#### Action History (`a`)
- Scrollable list of all instance actions (create, reboot, resize, etc.)
- Shows action name, timestamp with relative age, request ID
- Failed actions highlighted in red
- `R` to refresh

#### Resize Flow
- Modal overlay (not a separate view) — sits on top of list or detail
- Flavor list auto-sizes to content (no wrapping)
- Current flavor dimmed with ★ indicator
- After resize, server enters VERIFY_RESIZE state
- Detail view shows contextual banner: "Resize pending — ctrl+y confirm • ctrl+x revert"
- Confirm/revert uses optimistic UI with pending action state
- Staggered re-fetch (0.5s, 2s) for non-resize actions to catch state transitions

#### Bulk Actions
- `space` toggles selection on current server (advances cursor)
- Selected servers shown with ● prefix and purple highlighting
- Status bar shows selection count and available bulk actions
- Bulk delete, reboot, pause, suspend, shelve, resize all work on selection
- Confirm modal shows "N servers" for bulk operations
- Selection auto-clears after action execution
- Errors collected and reported as single modal (partial success visible)

#### Server Detail Auto-Refresh
- Same configurable interval as server list
- `R` for manual refresh
- Immediate re-fetch after actions (delete navigates to list)
- Pending action state prevents stale responses from overwriting optimistic updates

### Phase 3: Additional Resources (Complete)

#### Tabbed Navigation
- Dynamic tabs built from service catalog (see Phase 4 refactor): Servers, Volumes, Floating IPs, Security Groups, Networks, Key Pairs (always present), Load Balancers (if Octavia available)
- Switch with number keys `1-9` or `←/→` from any top-level list view
- Tab bar with active tab highlighted, inactive tabs muted
- Each tab lazily initializes on first visit, auto-refreshes independently
- Background tick routing ensures all initialized tabs keep refreshing even when not active

#### Volume Management
- **Volume List**: Adaptive columns (Name, Status, Size, Type, Attached To, Device, Bootable), auto-refresh, sorting
- **Volume Detail**: Enter on a volume shows full properties — Name, ID, Status, Size, Type, AZ, Bootable, Encrypted, Multiattach, Description, Created, Updated, Snapshot ID, Source Volume ID, Attached Server (resolved name), Device, Metadata (key=value)
- **Create** (`Ctrl+N`): Form with name, size (GB), type picker (from volume types API), AZ, description
- **Delete** (`Ctrl+D`): Confirmation modal, works from list or detail
- **Attach** (`Ctrl+A`): Server picker modal showing ACTIVE/SHUTOFF servers with type-to-filter
- **Detach** (`Ctrl+T`): From detail view, finds attached server and detaches
- Status colors: available=green, in-use=cyan, creating/extending=yellow, error=red, deleting=muted

#### Floating IP Management
- **List**: Columns (Floating IP, Status, Fixed IP, Port ID), auto-refresh, sorting
- **Allocate** (`Ctrl+N`): Allocates from first external network, shows progress in status bar
- **Associate** (`Ctrl+A` from server list/detail): Opens FIP picker modal showing unassociated IPs + "Allocate new" option. If no unassociated IPs exist, auto-allocates and assigns
- **Disassociate** (`Ctrl+T`): Confirmation modal, only enabled when FIP has a port
- **Release** (`Ctrl+D`): Confirmation modal

#### Security Group Management
- **Group List**: Expandable groups showing name, description, rule count
- **Rule View**: Enter expands/collapses group rules. Rules show direction, protocol, port range, remote, ethertype
- **Rule Navigation**: Down arrow enters rule list within expanded group, Up arrow exits back to group level
- **Create Rule** (`Ctrl+N` in rules): Modal with cycle pickers for direction/ethertype/protocol, port range, remote IP prefix
- **Delete Rule** (`Ctrl+D` in rules): When cursor is on a rule, confirmation modal then deletes
- **Create Security Group** (`Ctrl+N` at group level): Form with name and description
- **Delete Security Group** (`Ctrl+D` at group level): Confirmation modal
- Selected rule highlighted with `▸` prefix and background color

#### Key Pair Management
- **List**: Columns (Name, Type), sorting, auto-refresh
- **Detail** (`Enter`): Shows name, type, and full public key with scroll
- **Create/Import** (`Ctrl+N`): Form with name, type picker (RSA 2048/RSA 4096/ED25519), public key field with `~/.ssh/` file browser. Keys generated locally using Go crypto (`crypto/rsa`, `crypto/ed25519`, `x/crypto/ssh`), imported via Nova API
- **Save Private Key** (`s` in private key view): Save generated private key to file (default `~/.ssh/<name>`, 0600 permissions), public key saved alongside as `.pub`
- **Delete** (`Ctrl+D`): Confirmation modal, works from list or detail

#### Network Management
- **Networks Tab**: Network list with Name, Status, Subnets count, Shared columns
- **Expandable Subnets**: Enter expands/collapses network to show subnet details (name, CIDR, gateway, IP version, DHCP status)
- **Create Network** (`Ctrl+N`): Form with name, admin state, shared option
- **Delete Network** (`Ctrl+D`): Confirmation modal
- **Create Subnet** (`Ctrl+N` when expanded): Form with name, CIDR, gateway, IP version, DHCP
- **Delete Subnet** (`Ctrl+D` when in subnets): Confirmation modal
- **Port Listing**: Read-only port listing per network
- Auto-refresh, sorting

#### Router Management
- **Router List**: Columns (Name, Status, External Gateway, Routes), auto-refresh, sorting
- **Router Detail** (`Enter`): Properties view with interfaces section and static routes
- **Create Router** (`Ctrl+N`): Form with name, external network selection, admin state
- **Delete Router** (`Ctrl+D`): Confirmation modal
- **Add Interface** (`Ctrl+A` from detail): Subnet picker modal
- **Remove Interface** (`Ctrl+T` from detail): Confirmation modal

#### Column Sorting
- `s` cycles sort to next visible column (ascending), `S` toggles sort direction
- Active sort column shows ▲/▼ indicator
- Column header briefly highlights on sort change (1.5s)
- Sort persists through data refreshes
- Available on all list views (servers, volumes, floating IPs, key pairs)

#### Server Detail Enhancements
- **Networks section**: Shows IPs grouped by network name instead of flat lists
- **Assign Floating IP** (`Ctrl+A`): Opens FIP picker modal

#### Global Keybindings
- `R` force refresh — handled globally, dispatches to active view
- `PgUp/PgDn` — page navigation everywhere arrow keys work (lists, detail views, help modal)

#### Confirmation Modals
- Custom title and body text per resource type (e.g., "Delete Volume", "Release Floating IP")
- Reused across all resource types via generic action routing

### Phase 4: Refactor, Octavia, Projects, Quotas (Complete)

#### App Refactor
- Split monolithic `app.go` (1,643 lines) into 7 focused files with no logic changes
- Replaced hardcoded `activeTab` enum with dynamic `TabDef` registry — tabs are now data-driven
- Tab number keys `1-9` map dynamically to index position; arrow keys cycle with modulo
- Tab availability determined at connection time based on service catalog

#### Service Catalog Detection
- Octavia (Load Balancer) service detected optionally via `openstack.NewLoadBalancerV2`
- Block Storage detected optionally (try v3, v2, v1 — same as before)
- Tabs built conditionally: Volumes only if Cinder, Load Balancers only if Octavia
- Clouds without optional services work normally — those tabs simply don't appear

#### Load Balancer Management (Octavia)
- **LB List**: Columns (Name, VIP Address, Provisioning Status, Operating Status), auto-refresh, sorting
- **LB Detail**: Properties display plus tree view of Listeners → Pools → Members
- **Delete** (`Ctrl+D`): Cascade delete (removes listeners, pools, members along with LB)
- Status colors: ACTIVE/ONLINE=green, PENDING_*=yellow, ERROR/OFFLINE=red

#### Project Switching (ALPHA — UNTESTED)
- After cloud connection, accessible projects fetched in background via Keystone `ListAvailable`
- If more than one project available, `P` key opens project picker modal
- Current project marked with `*` in picker
- On selection, re-authenticates scoped to new project via `ConnectWithProject` (overrides TenantID)
- All tabs reset on project switch (same as cloud switch)
- Project name shown in status bar between cloud and region

#### Quota Overlay
- `Q` key opens full-screen quota overlay (same pattern as help)
- Three sections: Compute (instances, cores, RAM, key pairs, server groups), Network (floating IPs, networks, ports, routers, security groups, subnets), Block Storage (volumes, gigabytes, snapshots, backups)
- ASCII progress bars (18 chars wide) with color coding: green (<70%), yellow (70-90%), red (>90%)
- Unlimited quotas (limit=-1) shown as "used / unlimited" with no bar
- Lazy fetch on open, cached for 30 seconds
- Scroll support, close with `Q` or `Esc`
- Block Storage section omitted if Cinder unavailable

### Phase 5: Quality of Life (Partial)

#### Server Rename (`r`)
- Inline rename from server list or detail view
- Text input pre-filled with current name
- Uses `servers.Update()` API

#### Server Rebuild (`Ctrl+G`)
- Rebuild server with new image, keeping same ID/IPs
- Image picker modal with type-to-filter
- Confirmation before rebuild (destructive operation)

#### Server Snapshot (`Ctrl+S`)
- Create image snapshot from running server
- Name input pre-filled with server name
- Progress shown in status bar
- Handles 409 conflict (snapshot already in progress) with friendly error

#### Rescue/Unrescue (`Ctrl+W`)
- Toggle rescue mode for broken servers
- Confirmation modal
- Rescue mode returns admin password displayed in status bar
- Supports bulk rescue operations

#### Image Management
- **Image List Tab**: Columns (Name, Status, Size, Visibility, Created), auto-refresh, sorting
- **Image Detail** (`Enter`): Full properties view
- **Delete Image** (`Ctrl+D`): Confirmation modal, works from list or detail
- **Deactivate/Reactivate**: Toggle image availability

#### Self-Update
- `--update` flag downloads latest release from GitHub
- `--no-check-update` skips automatic version check on startup
- Downloads binary for current OS/architecture with SHA256 checksum verification

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--version` | bool | false | Print version and exit |
| `--pick-cloud` | bool | false | Always show cloud picker, even with one cloud |
| `--cloud NAME` | string | "" | Connect directly to named cloud, skip picker |
| `--refresh N` | int | 5 | Auto-refresh interval in seconds |
| `--idle-timeout N` | int | 0 | Pause polling after N minutes of no input (0 = disabled) |
| `--no-check-update` | bool | false | Skip automatic update check on startup |
| `--update` | bool | false | Self-update to the latest version |

### Keybindings

#### Global
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `?` | Toggle help (scrollable) |
| `C` | Switch cloud |
| `1-9` / `←/→` | Switch tab (dynamic based on available services) |
| `R` | Force refresh |
| `s` / `S` | Sort column / reverse sort |
| `P` | Switch project (when multiple projects available) |
| `Q` | Resource quotas overlay |
| `PgUp` / `PgDn` | Page up / page down |
| `Ctrl+R` | Restart app (re-exec binary) |

#### Server List
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `space` | Select/deselect for bulk actions |
| `Enter` | View detail |
| `Ctrl+N` | Create server |
| `Ctrl+D` | Delete server (or selected) |
| `Ctrl+O` | Soft reboot (or selected) |
| `p` | Pause/unpause (or selected) |
| `Ctrl+Z` | Suspend/resume (or selected) |
| `o` | Stop/start (or selected) |
| `Ctrl+E` | Shelve/unshelve (or selected) |
| `Ctrl+L` | Lock/unlock (or selected) |
| `Ctrl+W` | Rescue/unrescue (or selected) |
| `Ctrl+F` | Resize (modal) |
| `r` | Rename server |
| `Ctrl+G` | Rebuild with new image |
| `Ctrl+S` | Create snapshot |
| `Ctrl+A` | Assign floating IP (FIP picker modal) |
| `l` | Console log |
| `a` | Action history |
| `/` | Filter |
| `Esc` | Clear filter / clear selection |

#### Server Detail
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Scroll |
| `Ctrl+D` | Delete server |
| `Ctrl+O` | Soft reboot |
| `Ctrl+P` | Hard reboot |
| `p` | Pause/unpause |
| `Ctrl+Z` | Suspend/resume |
| `Ctrl+E` | Shelve/unshelve |
| `Ctrl+W` | Rescue/unrescue |
| `Ctrl+F` | Resize (modal) |
| `r` | Rename server |
| `Ctrl+G` | Rebuild with new image |
| `Ctrl+S` | Create snapshot |
| `Ctrl+A` | Assign floating IP (FIP picker modal) |
| `Ctrl+Y` | Confirm resize (when VERIFY_RESIZE) |
| `Ctrl+X` | Revert resize (when VERIFY_RESIZE) |
| `l` | Console log |
| `a` | Action history |
| `Esc` | Back to list |

#### Create Form
| Key | Action |
|-----|--------|
| `Tab` / `↓` | Next field |
| `Shift+Tab` / `↑` | Previous field |
| `Enter` | Open picker / activate button / advance |
| `Ctrl+S` | Submit (hotkey) |
| `Esc` | Cancel |

#### Volume List
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail |
| `Ctrl+N` | Create volume |
| `Ctrl+D` | Delete volume |

#### Volume Detail
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Scroll |
| `Ctrl+D` | Delete volume |
| `Ctrl+A` | Attach to server (server picker modal) |
| `Ctrl+T` | Detach from server |
| `Esc` | Back to list |

#### Floating IP List
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Ctrl+N` | Allocate new floating IP |
| `Ctrl+T` | Disassociate from port |
| `Ctrl+D` | Release floating IP |

#### Security Groups
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate groups / rules |
| `Enter` | Expand / collapse group |
| `Ctrl+N` | Create group (or add rule when in rules) |
| `Ctrl+D` | Delete group (or rule when in rules) |
| `Esc` | Back to group level (from rules) |

#### Key Pairs
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail (public key) |
| `Ctrl+N` | Create / import key pair |
| `Ctrl+D` | Delete key pair |

#### Networks
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | Expand / collapse subnets |
| `Ctrl+N` | Create network (or subnet when expanded) |
| `Ctrl+D` | Delete network (or subnet in subnets) |

#### Routers
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail (interfaces) |
| `Ctrl+N` | Create router |
| `Ctrl+D` | Delete router |
| `Ctrl+A` | Add interface (from detail) |
| `Ctrl+T` | Remove interface (from detail) |
| `Esc` | Back to list (from detail) |

#### Load Balancers
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail (listener/pool/member tree) |
| `Ctrl+D` | Delete load balancer (cascade) |
| `Esc` | Back to list (from detail) |

#### Images
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | View detail |
| `Ctrl+D` | Delete image |

#### Console Log / Action History
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Scroll |
| `g` / `G` | Top / Bottom (console only) |
| `Esc` | Back to previous view |

#### Modals
| Key | Action |
|-----|--------|
| `y` | Confirm |
| `n` / `Esc` | Cancel |
| `←/→` `↑/↓` `Tab` | Navigate buttons |
| `Enter` | Activate focused button |

### Visual Design

- **Color palette**: Solarized Dark base
- **Primary accent**: `#7D56F4` (purple) — used for branding badge, selected items, focused buttons
- **Secondary**: `#6C71C4`
- **Status indicators**: Green (ACTIVE/RUNNING), Yellow (BUILD/warning), Red (ERROR), Gray (SHUTOFF), Cyan (REBOOT)
- **Power state colors**: Green (RUNNING), Gray (SHUTDOWN), Red (CRASHED), Muted (PAUSED/SUSPENDED)
- **Selected row**: `#073642` background with bold text
- **Bulk selected**: ● prefix with purple text
- **Locked servers**: 🔒 prefix on name
- **Current flavor in resize**: Dimmed with ★ suffix
- **Buttons**: Styled with background color, highlight on focus (green for confirm/submit, red for cancel/deny)
- **Branding**: LAZYSTACK pill badge in top-right, dark text on purple background

## Future Roadmap

### Backlog (deferred from Phase 3 — all complete)
- ~~**Create Volume form**~~: ✓ Complete — name, size, type picker, AZ, description
- ~~**Create/Import Key Pair**~~: ✓ Complete — RSA/ED25519, file browser, save-to-file
- ~~**Create Security Group Rule**~~: ✓ Complete — modal with cycle pickers
- ~~**Volume Attach from detail**~~: ✓ Complete — server picker modal
- ~~**Network/subnet browsing**~~: ✓ Complete — Networks tab with expandable subnets

### Server Action Gaps

Actions available in Nova but not yet implemented, prioritized by usefulness:

#### High-value (all complete)
- ~~**Rename server**~~: ✓ Complete — `r` key, inline rename
- ~~**Rebuild**~~: ✓ Complete — `Ctrl+G`, image picker modal
- ~~**Create snapshot**~~: ✓ Complete — `Ctrl+S`, creates image from server
- ~~**Rescue/Unrescue**~~: ✓ Complete — `Ctrl+W`, toggle rescue mode

#### Medium-value (specific scenarios)
- ~~**Console access (noVNC)**~~ ✓ Done — Retrieve and open VNC console URL via `V` key. Opens in browser or copies to clipboard.
- **Get password** — Retrieve auto-generated password for Windows VMs (`servers.GetPassword`).

#### Admin-only (Phase 6)
- **Migrate / Live Migrate** — Move server to different hypervisor. Admin privilege required.
- **Evacuate** — Emergency recovery from failed host. Admin-only.
- **Force Delete** — Bypass normal deletion flow. Admin-only.
- **Reset State** — Force server into a specific state. Recovery from stuck transitions.
- **Metadata browser** — Full CRUD on server metadata key-value pairs.

### Phase 5: Quality of Life (remaining)
- Configuration file (`~/.config/lazystack/config.yaml`) for defaults
- Custom column selection and ordering
- Saved filters
- Server name templates for create
- ~~SSH integration (launch SSH session to selected server)~~ ✓ Done (#27)
- Copy-to-clipboard for IDs, IPs (general — SSH command copy done via `y`)
- Log/audit trail of actions taken
- Designate (DNS) tab
- ~~Console access (noVNC URL retrieval and browser launch)~~ ✓ Done (#50)

### Phase 6: Operational
- Hypervisor view (admin)
- User management (admin)
- Service catalog browser
- Migrate/evacuate, force delete, reset state (see Server Action Gaps above)

## Non-Goals

- **Mouse-driven interaction**: This is a keyboard-first tool. Mouse support may come later but is not a priority.
- **Full Horizon replacement**: lazystack focuses on the most common operations. Rarely-used Horizon features (e.g., orchestration, identity management) are out of scope.
- **Multi-platform OpenStack alternatives**: This tool is specifically for OpenStack, not a generic cloud TUI.
- **Configuration management**: lazystack is for interactive operations, not declarative infrastructure management (use Terraform/OpenTofu for that).

## Success Criteria

1. `go build` produces a single binary with zero runtime dependencies
2. Connects to any standard OpenStack cloud via `clouds.yaml`
3. Server list loads and auto-refreshes without manual intervention
4. Full VM lifecycle (list, inspect, create, delete, reboot, pause, suspend, shelve, resize) from keyboard
5. All destructive actions require Ctrl-prefix and explicit confirmation
6. Responsive at terminal sizes from 80x20 upward
7. Bulk operations work on multi-selected servers
8. Console log and action history accessible per server

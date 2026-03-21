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
| Phase 3: Additional Resources | Not started | Volumes, floating IPs, security groups |
| Phase 4: Multi-Cloud | Not started | |
| Phase 5: Quality of Life | Not started | SSH, clipboard, config file |
| Phase 6: Operational | Not started | Quotas, admin views |

**Current version**: v0.0.1 (tagged at end of Phase 1)

## Concerns and Considerations

### Architecture

- **Value receiver pattern**: Bubble Tea v2 uses value receivers for `Update()`, which means model mutations require returning new values. This interacts poorly with optimistic UI updates — changes made before an async command fires can be overwritten when the command's response arrives and gets routed through `updateActiveView`. This caused the resize confirmation banner to flicker (optimistic status set to ACTIVE, then stale API response overwrote it back to VERIFY_RESIZE). Solved with a `pendingAction` state that suppresses stale updates until the real state catches up.

- **Message routing complexity**: The root model routes messages to sub-views via a `switch` on the active view. Adding overlay modals (resize picker, confirm, error, help) that intercept messages creates ordering dependencies in the `Update` method. The resize modal being `Active` was swallowing messages meant for other views. Each new modal/overlay adds routing complexity — consider a message bus or middleware pattern if this grows further.

- **Import cycle avoidance**: Shared types (keys, styles, messages) live in `internal/shared/` rather than `internal/app/` to avoid import cycles between `app` and the UI packages. This is a pragmatic workaround but means the "app" package is really just the root model + view routing.

### UX Lessons Learned

- **Ctrl-prefixed dangerous actions**: Originally used `c`/`d`/`r` for create/delete/reboot. Changed to `ctrl+n`/`ctrl+d`/`ctrl+o` after realizing typing in the wrong terminal window could trigger destructive operations. This is a good pattern for any keyboard-first TUI.

- **Optimistic UI is essential for async APIs**: OpenStack actions like `confirmResize` return 202 (accepted) but the server state doesn't change immediately. Polling the API right after the action often returns stale state. The `pendingAction` pattern — show the expected state immediately and suppress stale API responses until the real state catches up — provides much better UX than waiting for the tick.

- **Column adaptivity matters**: Fixed-width columns don't work across terminal sizes. The flex-weight + priority system (columns get proportional extra space, and low-priority columns hide on narrow terminals) works well. IPv6 addresses (39 chars) are particularly challenging — they need the highest flex weight but lowest display priority since they're rarely needed at a glance.

- **Modal vs view**: The resize flavor picker was initially a full view, which caused navigation issues (Esc from resize opened from the list panicked because there was no detail view to go back to). Converting it to a modal overlay that sits on top of the current view eliminated the problem entirely. Prefer modals for transient selection UI.

- **Auto-refresh must survive view changes**: The server list's auto-refresh tick was breaking when navigating to other views because the tick message got routed to the wrong view. Fixed by always routing `TickMsg` to the server list regardless of active view.

### Technical Debt

- **Bulk action support is partial**: Space-select works for delete, reboot, pause, suspend, shelve. Resize and console log only operate on a single server (the cursor, not the selection). This is intentional — resize needs per-server flavor choice, and console log is inherently single-server.

- **Image name resolution**: Nova's server response doesn't include the image name (only ID) with newer microversions. The server list fetches all images from Glance to resolve names, which works but adds an extra API call on every refresh. Should cache more aggressively or resolve lazily.

- **Server detail refresh creates a new model**: After actions from the detail view, the detail model is recreated with `New()` + `Init()` to force a fresh fetch. This resets scroll position and loses the pending action state if not carefully managed. A proper `Refresh()` method on the detail model would be cleaner.

- **No tests**: The codebase has zero test coverage. The compute layer functions are thin wrappers around gophercloud and would benefit from interface-based mocking. The UI components are harder to test but snapshot testing of `View()` output would catch rendering regressions.

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
      app.go                        # Root model, view routing, modal overlay, bulk actions
    shared/
      keys.go                       # Global key bindings (Ctrl-prefixed for dangerous ops)
      styles.go                     # Lipgloss theme constants (Solarized Dark)
      messages.go                   # Shared message types for inter-component communication
    cloud/
      client.go                     # Auth, service client initialization
      clouds.go                     # clouds.yaml parser
    compute/
      servers.go                    # Server CRUD + pause/suspend/shelve/resize/reboot
      actions.go                    # Instance action history
      flavors.go                    # Flavor listing
      keypairs.go                   # Keypair listing
    image/
      images.go                     # Image listing
    network/
      networks.go                   # Network listing
    ui/
      serverlist/
        serverlist.go               # Server table with auto-refresh, filtering, bulk select
        columns.go                  # Adaptive columns with flex weights and priority hiding
      serverdetail/
        serverdetail.go             # Server property view with auto-refresh, pending action state
      servercreate/
        servercreate.go             # Create form with inline pickers, count field
      serverresize/
        serverresize.go             # Resize modal with flavor picker, current flavor indicator
      consolelog/
        consolelog.go               # Scrollable console output viewer
      actionlog/
        actionlog.go                # Instance action history viewer
      modal/
        confirm.go                  # Confirmation dialog (single + bulk), focusable buttons
        error.go                    # Error modal
      cloudpicker/
        cloudpicker.go              # Cloud selection overlay
      statusbar/
        statusbar.go                # Bottom bar: cloud, region, context hints
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
                    ┌─────────────┐
              ┌───→ │ Server List  │ ←───────────────────────┐
              │     └──┬───┬───┬──┘                           │
              │  Enter │  ^n  │ ^d/^o/p/^z/^e                │
              │        │   │  │ (via confirm modal)           │
              │        ▼   ▼  ▼                               │
              │  ┌────────┐ ┌────────┐                        │
              │  │ Detail  │ │ Create │                        │
              │  └┬──┬──┬─┘ └───┬────┘                        │
              │   │  │  │   Esc/│Submit                       │
              │   │  l  a      └──────────────────────────────┘
              │   │  │  │
              │   │  ▼  ▼
              │   │ ┌────────┐ ┌────────────┐
              │   │ │Console │ │Action Log  │
              │   │ └───┬────┘ └─────┬──────┘
              │   │  Esc│         Esc│
              │   │  ───┘         ───┘ (back to previous view)
              │   │
              │  Esc
              └───┘

  Overlays (always available):
  ┌─────────────┐  ┌──────────┐  ┌──────┐  ┌────────┐
  │Confirm Modal│  │Error     │  │Help  │  │Resize  │
  │(y/n/enter)  │  │(enter)   │  │(?)   │  │(^f)    │
  └─────────────┘  └──────────┘  └──────┘  └────────┘
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
- Two-column property list: name, ID, status, power state, flavor, image (name + ID), IPv4, IPv6, floating IP, keypair, locked, tenant, AZ, created, security groups, volumes
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
- Required for all destructive actions (delete, reboot, pause, suspend, shelve)
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
- Shows current cloud name and region
- Context-sensitive key hints per view (adapts to server state, selection count)
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
- Bulk delete, reboot, pause, suspend, shelve all work on selection
- Confirm modal shows "N servers" for bulk operations
- Selection auto-clears after action execution
- Errors collected and reported as single modal (partial success visible)

#### Server Detail Auto-Refresh
- Same configurable interval as server list
- `R` for manual refresh
- Immediate re-fetch after actions (delete navigates to list)
- Pending action state prevents stale responses from overwriting optimistic updates

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--pick-cloud` | bool | false | Always show cloud picker, even with one cloud |
| `--refresh` | int | 5 | Server list/detail auto-refresh interval in seconds |

### Keybindings

#### Global
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `?` | Toggle help (scrollable) |
| `C` | Switch cloud |
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
| `Ctrl+E` | Shelve/unshelve (or selected) |
| `Ctrl+F` | Resize (modal) |
| `l` | Console log |
| `a` | Action history |
| `R` | Force refresh |
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
| `Ctrl+F` | Resize (modal) |
| `Ctrl+Y` | Confirm resize (when VERIFY_RESIZE) |
| `Ctrl+X` | Revert resize (when VERIFY_RESIZE) |
| `l` | Console log |
| `a` | Action history |
| `R` | Refresh |
| `Esc` | Back to list |

#### Create Form
| Key | Action |
|-----|--------|
| `Tab` / `↓` | Next field |
| `Shift+Tab` / `↑` | Previous field |
| `Enter` | Open picker / activate button / advance |
| `Ctrl+S` | Submit (hotkey) |
| `Esc` | Cancel |

#### Console Log / Action History
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Scroll |
| `g` / `G` | Top / Bottom (console only) |
| `R` | Refresh |
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
4. Full VM lifecycle (list, inspect, create, delete, reboot, pause, suspend, shelve, resize) from keyboard
5. All destructive actions require Ctrl-prefix and explicit confirmation
6. Responsive at terminal sizes from 80x20 upward
7. Bulk operations work on multi-selected servers
8. Console log and action history accessible per server

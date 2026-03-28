# Accessibility: Status Icons Design

## Problem

Server status and resource states are communicated primarily through color (Solarized Dark palette). Users who are colorblind or using monochrome terminals cannot distinguish states at a glance. While status text is already displayed (e.g., "ACTIVE/RUNNING"), scanning a list of servers requires reading each status string rather than recognizing a visual pattern.

## Solution

Add Unicode shape icon prefixes to all status indicators across every resource view. Icons encode the status *category* by shape, providing a second visual channel alongside color. A `--plain` CLI flag disables icons for terminals with poor Unicode support.

## Icon Mapping

Six categories cover all resource states. For any unmapped status, return `""` (no icon) to avoid visual noise from unknown states.

States prefixed with `PENDING_` (e.g., `PENDING_CREATE`, `PENDING_UPDATE`, `PENDING_DELETE`) are matched via `strings.HasPrefix`.

| Icon | Category | States |
|------|----------|--------|
| ● | Healthy/Active | ACTIVE, RUNNING, available, ONLINE, active |
| ▲ | In-progress | BUILD, RESIZE, VERIFY_RESIZE, MIGRATING, creating, downloading, uploading, extending, saving, PENDING_*, NOSTATE |
| ✘ | Error | ERROR, CRASHED, DELETED, SOFT_DELETED, error, error_deleting, error_restoring, killed, OFFLINE |
| ○ | Off/Inactive | SHUTOFF, SHUTDOWN, DOWN, deleting, deleted, pending_delete |
| ↻ | Transitional | REBOOT, HARD_REBOOT, in-use, queued, importing, DEGRADED, NO_MONITOR, DRAINING |
| ■ | Paused/Held | PAUSED, SUSPENDED, SHELVED, SHELVED_OFFLOADED, deactivated |

## Architecture

### `src/internal/shared/styles.go`

Add a `PlainMode` package-level bool and a `StatusIcon(status string) string` function. The function maps a raw status string to its icon prefix (e.g., `"ACTIVE"` → `"● "`). When `PlainMode` is true, it returns `""`. For unmapped statuses, it returns `""`.

This is a new parallel structure (not modifying `StatusColors`): a `statusIconMap` from status string to icon string, plus a `strings.HasPrefix` check for `PENDING_` states.

### `src/cmd/lazystack/main.go` + `src/internal/app/app.go`

Add `--plain` bool flag. Add `Plain bool` to `app.Options`. In `app.New()`, set `shared.PlainMode = opts.Plain` before initializing sub-models.

### View changes

Each view's status rendering prepends `shared.StatusIcon(status)` to the status text before styling. Files to modify:

**List views:**
- `src/internal/ui/serverlist/serverlist.go` — `renderServerRow()`: prepend icon to `statusVal`
- `src/internal/ui/volumelist/volumelist.go` — status column rendering
- `src/internal/ui/imagelist/imagelist.go` — status column rendering
- `src/internal/ui/floatingiplist/floatingiplist.go` — status column rendering
- `src/internal/ui/routerlist/routerlist.go` — status column rendering
- `src/internal/ui/networklist/networklist.go` — status column rendering
- `src/internal/ui/lblist/lblist.go` — provisioning + operating status rendering

**Detail views:**
- `src/internal/ui/serverdetail/serverdetail.go` — status field
- `src/internal/ui/volumedetail/volumedetail.go` — status field
- `src/internal/ui/imagedetail/imagedetail.go` — status field
- `src/internal/ui/routerdetail/routerdetail.go` — status field
- `src/internal/ui/lbdetail/lbdetail.go` — provisioning + operating status fields

**Picker views:**
- `src/internal/ui/serverpicker/serverpicker.go` — inline server status

### Column width

Status columns may need +2 character width to accommodate the icon prefix. Adjust `MinWidth` in column definitions where applicable.

## CLI Usage

```
lazystack                  # icons enabled (default)
lazystack --plain          # icons disabled
```

## Verification

1. Run `go build ./cmd/lazystack` — compiles without errors
2. Run `go test ./...` — all tests pass
3. Launch app, verify icons appear in server list, volume list, image list, floating IP list, router list, LB detail
4. Launch with `--plain`, verify no icons appear
5. Visually confirm each status category shows the correct icon shape

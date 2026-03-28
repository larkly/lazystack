# Accessibility Status Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Unicode shape icons (●▲✘○↻■) as prefixes to status text across all resource views, with a `--plain` CLI flag to disable.

**Architecture:** A centralized `StatusIcon(status)` function in `shared/styles.go` maps status strings to icon prefixes. Each view prepends the icon at its status render site. A package-level `PlainMode` bool (set via `--plain` flag) disables icons.

**Tech Stack:** Go, Bubble Tea, Lipgloss

**Spec:** `docs/superpowers/specs/2026-03-28-accessibility-status-icons-design.md`

---

### Task 1: Add StatusIcon function and PlainMode to shared/styles.go

**Files:**
- Modify: `src/internal/shared/styles.go`
- Create: `src/internal/shared/styles_test.go`

- [ ] **Step 1: Write failing tests for StatusIcon**

```go
package shared

import "testing"

func TestStatusIcon_KnownStates(t *testing.T) {
	PlainMode = false
	tests := []struct {
		status string
		want   string
	}{
		{"ACTIVE", "● "},
		{"RUNNING", "● "},
		{"available", "● "},
		{"ONLINE", "● "},
		{"active", "● "},
		{"BUILD", "▲ "},
		{"RESIZE", "▲ "},
		{"NOSTATE", "▲ "},
		{"ERROR", "✘ "},
		{"CRASHED", "✘ "},
		{"DELETED", "✘ "},
		{"OFFLINE", "✘ "},
		{"SHUTOFF", "○ "},
		{"SHUTDOWN", "○ "},
		{"DOWN", "○ "},
		{"REBOOT", "↻ "},
		{"HARD_REBOOT", "↻ "},
		{"in-use", "↻ "},
		{"PAUSED", "■ "},
		{"SUSPENDED", "■ "},
		{"SHELVED", "■ "},
		{"deactivated", "■ "},
	}
	for _, tt := range tests {
		got := StatusIcon(tt.status)
		if got != tt.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestStatusIcon_PendingPrefix(t *testing.T) {
	PlainMode = false
	tests := []string{"PENDING_CREATE", "PENDING_UPDATE", "PENDING_DELETE"}
	for _, s := range tests {
		got := StatusIcon(s)
		if got != "▲ " {
			t.Errorf("StatusIcon(%q) = %q, want %q", s, got, "▲ ")
		}
	}
}

func TestStatusIcon_UnknownState(t *testing.T) {
	PlainMode = false
	got := StatusIcon("UNKNOWN_STATE")
	if got != "" {
		t.Errorf("StatusIcon(UNKNOWN_STATE) = %q, want empty", got)
	}
}

func TestStatusIcon_PlainMode(t *testing.T) {
	PlainMode = true
	defer func() { PlainMode = false }()
	got := StatusIcon("ACTIVE")
	if got != "" {
		t.Errorf("StatusIcon in PlainMode = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src && go test ./internal/shared/ -v -run TestStatusIcon`
Expected: FAIL — `StatusIcon` undefined

- [ ] **Step 3: Implement StatusIcon and PlainMode**

Add to `src/internal/shared/styles.go` after the existing `PowerColors` map:

```go
// PlainMode disables status icons when true (set via --plain flag).
var PlainMode bool

// statusIconMap maps status strings to their Unicode icon prefix.
var statusIconMap = map[string]string{
	// Healthy/Active — ●
	"ACTIVE":    "● ",
	"RUNNING":   "● ",
	"available": "● ",
	"ONLINE":    "● ",
	"active":    "● ",
	// In-progress — ▲
	"BUILD":         "▲ ",
	"RESIZE":        "▲ ",
	"VERIFY_RESIZE": "▲ ",
	"MIGRATING":     "▲ ",
	"creating":      "▲ ",
	"downloading":   "▲ ",
	"uploading":     "▲ ",
	"extending":     "▲ ",
	"saving":        "▲ ",
	"NOSTATE":       "▲ ",
	// Error — ✘
	"ERROR":           "✘ ",
	"CRASHED":         "✘ ",
	"DELETED":         "✘ ",
	"SOFT_DELETED":    "✘ ",
	"error":           "✘ ",
	"error_deleting":  "✘ ",
	"error_restoring": "✘ ",
	"killed":          "✘ ",
	"OFFLINE":         "✘ ",
	// Off/Inactive — ○
	"SHUTOFF":        "○ ",
	"SHUTDOWN":       "○ ",
	"DOWN":           "○ ",
	"deleting":       "○ ",
	"deleted":        "○ ",
	"pending_delete": "○ ",
	// Transitional — ↻
	"REBOOT":      "↻ ",
	"HARD_REBOOT": "↻ ",
	"in-use":      "↻ ",
	"queued":      "↻ ",
	"importing":   "↻ ",
	"DEGRADED":    "↻ ",
	"NO_MONITOR":  "↻ ",
	"DRAINING":    "↻ ",
	// Paused/Held — ■
	"PAUSED":            "■ ",
	"SUSPENDED":         "■ ",
	"SHELVED":           "■ ",
	"SHELVED_OFFLOADED": "■ ",
	"deactivated":       "■ ",
}

// StatusIcon returns the icon prefix for a status string.
// Returns "" for unknown statuses or when PlainMode is true.
func StatusIcon(status string) string {
	if PlainMode {
		return ""
	}
	if icon, ok := statusIconMap[status]; ok {
		return icon
	}
	if strings.HasPrefix(status, "PENDING_") {
		return "▲ "
	}
	return ""
}
```

Add `"strings"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src && go test ./internal/shared/ -v -run TestStatusIcon`
Expected: PASS (all 4 test functions)

- [ ] **Step 5: Commit**

```bash
git add src/internal/shared/styles.go src/internal/shared/styles_test.go
git commit -m "feat: add StatusIcon function with PlainMode support (#60)"
```

---

### Task 2: Add --plain CLI flag

**Files:**
- Modify: `src/cmd/lazystack/main.go:21-28` (flag definitions)
- Modify: `src/internal/app/app.go:184-191` (Options struct), lines 210-225 and 228-243 (New function)

- [ ] **Step 1: Add Plain field to Options struct**

In `src/internal/app/app.go`, add `Plain bool` to the `Options` struct (line ~190):

```go
type Options struct {
	AlwaysPickCloud bool
	Cloud           string
	RefreshInterval time.Duration
	IdleTimeout     time.Duration
	Version         string
	CheckUpdate     bool
	Plain           bool
}
```

- [ ] **Step 2: Set shared.PlainMode in New()**

In `src/internal/app/app.go`, add at the top of `func New(opts Options)` (line ~194, before the `clouds` call):

```go
shared.PlainMode = opts.Plain
```

- [ ] **Step 3: Add --plain flag to main.go**

In `src/cmd/lazystack/main.go`, add after the existing flag definitions (line ~28):

```go
plainMode := flag.Bool("plain", false, "disable Unicode status icons")
```

And in the Options construction (where other flags are mapped to Options fields), add:

```go
Plain: *plainMode,
```

- [ ] **Step 4: Verify build compiles**

Run: `cd src && go build ./cmd/lazystack`
Expected: compiles without errors

- [ ] **Step 5: Commit**

```bash
git add src/cmd/lazystack/main.go src/internal/app/app.go
git commit -m "feat: add --plain CLI flag to disable status icons (#60)"
```

---

### Task 3: Add icons to server list and detail views

**Files:**
- Modify: `src/internal/ui/serverlist/serverlist.go:502` (renderServerRow)
- Modify: `src/internal/ui/serverlist/columns.go:23` (status column MinWidth)
- Modify: `src/internal/ui/serverdetail/serverdetail.go:216-218` (status rendering)

- [ ] **Step 1: Prepend icon to server list status**

In `src/internal/ui/serverlist/serverlist.go`, change line 502 from:

```go
statusVal := s.Status + "/" + s.PowerState
```

to:

```go
statusVal := shared.StatusIcon(s.Status) + s.Status + "/" + s.PowerState
```

- [ ] **Step 2: Increase status column MinWidth**

In `src/internal/ui/serverlist/columns.go`, change the status column MinWidth from 18 to 20:

```go
{Title: "Status", MinWidth: 20, Flex: 0, Priority: 0, Key: "status"},
```

- [ ] **Step 3: Prepend icon to server detail status**

In `src/internal/ui/serverdetail/serverdetail.go`, change lines 216-218 from:

```go
if p.label == "Status" {
    value = StatusStyle(p.value).Render(p.value)
}
```

to:

```go
if p.label == "Status" {
    value = StatusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
}
```

- [ ] **Step 4: Verify build and existing tests pass**

Run: `cd src && go build ./cmd/lazystack && go test ./internal/ui/serverlist/ -v`
Expected: compiles and tests pass

- [ ] **Step 5: Commit**

```bash
git add src/internal/ui/serverlist/serverlist.go src/internal/ui/serverlist/columns.go src/internal/ui/serverdetail/serverdetail.go
git commit -m "feat: add status icons to server list and detail views (#60)"
```

---

### Task 4: Add icons to volume views

**Files:**
- Modify: `src/internal/ui/volumelist/volumelist.go:433` (status value)
- Modify: `src/internal/ui/volumelist/volumelist.go:41` (status column MinWidth)
- Modify: `src/internal/ui/volumedetail/volumedetail.go:210-212` (status rendering)

- [ ] **Step 1: Prepend icon to volume list status**

In `src/internal/ui/volumelist/volumelist.go`, change line 433 from:

```go
"status":   v.Status,
```

to:

```go
"status":   shared.StatusIcon(v.Status) + v.Status,
```

- [ ] **Step 2: Increase volume status column MinWidth**

Change line 41 MinWidth from 12 to 14.

- [ ] **Step 3: Prepend icon to volume detail status**

In `src/internal/ui/volumedetail/volumedetail.go`, change lines 210-212 from:

```go
if p.label == "Status" {
    value = volumeStatusStyle(p.value).Render(p.value)
}
```

to:

```go
if p.label == "Status" {
    value = volumeStatusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
}
```

- [ ] **Step 4: Verify build compiles**

Run: `cd src && go build ./cmd/lazystack`
Expected: compiles without errors

- [ ] **Step 5: Commit**

```bash
git add src/internal/ui/volumelist/volumelist.go src/internal/ui/volumedetail/volumedetail.go
git commit -m "feat: add status icons to volume list and detail views (#60)"
```

---

### Task 5: Add icons to image views

**Files:**
- Modify: `src/internal/ui/imagelist/imagelist.go:402` (status value)
- Modify: `src/internal/ui/imagelist/imagelist.go:39` (status column MinWidth)
- Modify: `src/internal/ui/imagedetail/imagedetail.go:190-192` (status rendering)

- [ ] **Step 1: Prepend icon to image list status**

In `src/internal/ui/imagelist/imagelist.go`, change line 402 from:

```go
"status":     img.Status,
```

to:

```go
"status":     shared.StatusIcon(img.Status) + img.Status,
```

- [ ] **Step 2: Increase image status column MinWidth**

Change line 39 MinWidth from 12 to 14.

- [ ] **Step 3: Prepend icon to image detail status**

In `src/internal/ui/imagedetail/imagedetail.go`, change lines 190-192 from:

```go
if p.label == "Status" {
    value = statusStyle(p.value).Render(p.value)
}
```

to:

```go
if p.label == "Status" {
    value = statusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
}
```

- [ ] **Step 4: Verify build compiles**

Run: `cd src && go build ./cmd/lazystack`
Expected: compiles without errors

- [ ] **Step 5: Commit**

```bash
git add src/internal/ui/imagelist/imagelist.go src/internal/ui/imagedetail/imagedetail.go
git commit -m "feat: add status icons to image list and detail views (#60)"
```

---

### Task 6: Add icons to floating IP, router, and network views

**Files:**
- Modify: `src/internal/ui/floatingiplist/floatingiplist.go:256` (status render)
- Modify: `src/internal/ui/routerlist/routerlist.go:252` (status render)
- Modify: `src/internal/ui/routerdetail/routerdetail.go:209-212` (status rendering)
- Modify: `src/internal/ui/networklist/networklist.go:267` (status render)

- [ ] **Step 1: Prepend icon to floating IP status**

In `src/internal/ui/floatingiplist/floatingiplist.go`, change line 256 from:

```go
statusStr := statusStyle.Width(12).Render(f.Status)
```

to:

```go
statusStr := statusStyle.Width(14).Render(shared.StatusIcon(f.Status) + f.Status)
```

- [ ] **Step 2: Prepend icon to router list status**

In `src/internal/ui/routerlist/routerlist.go`, change line 252 from:

```go
stStyle.Render(truncate(r.Status, 12)),
```

to:

```go
stStyle.Render(shared.StatusIcon(r.Status) + truncate(r.Status, 14)),
```

Also increase the status width in the row layout if there's a hardcoded width for this column.

- [ ] **Step 3: Prepend icon to router detail status**

In `src/internal/ui/routerdetail/routerdetail.go`, change lines 209-212. For the Status label:

```go
if p.label == "Status" {
    value = statusStyle(p.value).Render(shared.StatusIcon(p.value) + p.value)
}
```

- [ ] **Step 4: Prepend icon to network list status**

In `src/internal/ui/networklist/networklist.go`, change line 267 from:

```go
statusStyle.Width(statusW).Render(net.Status),
```

to:

```go
statusStyle.Width(statusW).Render(shared.StatusIcon(net.Status) + net.Status),
```

- [ ] **Step 5: Verify build compiles**

Run: `cd src && go build ./cmd/lazystack`
Expected: compiles without errors

- [ ] **Step 6: Commit**

```bash
git add src/internal/ui/floatingiplist/floatingiplist.go src/internal/ui/routerlist/routerlist.go src/internal/ui/routerdetail/routerdetail.go src/internal/ui/networklist/networklist.go
git commit -m "feat: add status icons to floating IP, router, and network views (#60)"
```

---

### Task 7: Add icons to load balancer and server picker views

**Files:**
- Modify: `src/internal/ui/lblist/lblist.go:247-248` (dual status render)
- Modify: `src/internal/ui/lbdetail/lbdetail.go:178-179` (status properties)
- Modify: `src/internal/ui/serverpicker/serverpicker.go:218` (inline status)

- [ ] **Step 1: Prepend icons to LB list statuses**

In `src/internal/ui/lblist/lblist.go`, change lines 247-248. For provisioning status render:

```go
psStyle.Render(shared.StatusIcon(lb.ProvisioningStatus) + truncate(lb.ProvisioningStatus, 18)),
```

For operating status render:

```go
osStyle.Render(shared.StatusIcon(lb.OperatingStatus) + truncate(lb.OperatingStatus, 16)),
```

- [ ] **Step 2: Prepend icons to LB detail statuses**

In `src/internal/ui/lbdetail/lbdetail.go`, change the render logic (around line 190-191) where style functions are applied. The rendered value should include the icon:

```go
if p.style != nil {
    value = p.style(p.value).Render(shared.StatusIcon(p.value) + p.value)
```

- [ ] **Step 3: Prepend icon to server picker status**

In `src/internal/ui/serverpicker/serverpicker.go`, change line 218 from:

```go
line := cursor + style.Render(srv.Name) + " " + statusStyle.Render(srv.Status)
```

to:

```go
line := cursor + style.Render(srv.Name) + " " + statusStyle.Render(shared.StatusIcon(srv.Status) + srv.Status)
```

- [ ] **Step 4: Verify build and all tests pass**

Run: `cd src && go build ./cmd/lazystack && go test ./... -v`
Expected: compiles and all tests pass

- [ ] **Step 5: Commit**

```bash
git add src/internal/ui/lblist/lblist.go src/internal/ui/lbdetail/lbdetail.go src/internal/ui/serverpicker/serverpicker.go
git commit -m "feat: add status icons to load balancer and server picker views (#60)"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run full test suite**

Run: `cd src && go test ./... -v`
Expected: all tests pass

- [ ] **Step 2: Verify --plain flag works**

Run: `cd src && go build ./cmd/lazystack && ./lazystack --help`
Expected: `--plain` flag appears in help output with description "disable Unicode status icons"

- [ ] **Step 3: Build final binary**

Run: `cd src && go build -o lazystack ./cmd/lazystack`
Expected: binary compiles successfully

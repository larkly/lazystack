# Server List Adaptive Columns

## Problem

The server list column layout wastes space on IPv6 at the expense of Name and Status:

- `IPv6` has `Flex: 4` — 31% of all growth weight. When IPv6 addresses are short (or the network prefix dominates), the column grows far beyond what its content needs, leaving visible dead space before the next column.
- `Name` has `MinWidth: 10, Flex: 2`. Server names are a primary identifier but can be clipped to 10 chars on narrower terminals.
- `Status` has `MinWidth: 20, Flex: 0` — fixed. `■ SHELVED_OFFLOADED/SHUTDOWN` (28 display cols) gets clipped even on wide terminals.
- Reaching a layout where all info is visible without truncation requires ~300 cols of terminal width.

## Goals

1. Server Name visible to at least 20 chars even on narrow terminals.
2. Status fits the common case (`■ ACTIVE/RUNNING`, 16 cols) without waste, and grows toward the longest combo when space allows.
3. IPv6 no longer consumes the bulk of the flex budget; truncates earlier when long.
4. When IPv6 is truncated, show the host suffix (differs per server) rather than the shared network prefix.

## Non-goals

- Data-driven column sizing (sizing to the longest *actual* value in the current server list). Considered but deferred; out of scope for this change.
- Changes to other list views (router, network, volume, etc.) — those don't share `columns.go`.
- Changes to column order, visibility priorities, or the general `ComputeWidths` algorithm.

## Changes

### 1. `DefaultColumns` in [columns.go:20-32](src/internal/ui/serverlist/columns.go:20)

| Column | MinWidth | Flex | Priority |
|---|---|---|---|
| Name | 10 → **20** | 2 → **4** | 0 |
| Status | 20 → **16** | 0 → **1** | 0 |
| IPv4 | 12 | 1 | 1 |
| IPv6 | 20 → **15** | 4 → **1** | 5 |
| Floating IP | 12 | 1 | 3 |
| Flavor | 10 | 2 | 1 |
| Image | 10 | 2 | 2 |
| Age | 5 | 0 | 1 |
| Key | 8 | 1 | 4 |

Total flex weight stays 13, but Name now claims 4/13 of the growth budget (was 2/13) and IPv6 claims 1/13 (was 4/13).

### 2. IPv6 suffix truncation in [serverlist.go:598](src/internal/ui/serverlist/serverlist.go:598)

Replace the single prefix-truncation branch with a key-aware variant:

```go
if len(val) > w && w > 1 {
    if col.Key == "ipv6" {
        val = "…" + val[len(val)-(w-1):]
    } else {
        val = val[:w-1] + "…"
    }
}
```

Byte-indexing is safe: IPv6 addresses are ASCII-only.

## Behavior examples

### Narrow terminal (100 cols)

- Name: guaranteed 20 cols (was ~10).
- Status: 16 cols, common case fits exactly; SHELVED_OFFLOADED clipped.
- IPv6: likely hidden (priority 5) — unchanged from today.

### Medium terminal (180 cols)

- Name: ~42 cols (was ~28).
- Status: ~19 cols (was 20 fixed).
- IPv6: ~20 cols (was ~45).
- Gap between IPv6 and Floating IP reduced.

### Wide terminal (300 cols)

- All columns visible with less slack around IPv6. Name and Flavor/Image get the slack instead.

## Testing

Extend [columns_test.go](src/internal/ui/serverlist/columns_test.go):

1. `TestComputeWidths_NameMinWidth` — verify Name width ≥ 20 at a width where Name is visible.
2. `TestComputeWidths_IPv6NoLongerDominates` — at width 180, Name width > IPv6 width.

Add to [serverlist_test.go](src/internal/ui/serverlist/serverlist_test.go):

3. `TestRenderServerRow_IPv6SuffixTruncation` — long IPv6 truncates with leading ellipsis, preserving the suffix.
4. `TestRenderServerRow_OtherColumnsPrefixTruncation` — ensure other columns still use prefix truncation.

## Risks

- Users with very narrow terminals (< ~60 cols) may see fewer columns than before because Name now reserves 20 cols instead of 10. Mitigation: lower-priority columns hide first (unchanged priority order), so the user still sees Name + Status.
- IPv6 suffix truncation changes what users see at a glance. Minor UX shift; intentional per design.

# Plan: fix the 2026-10-17 code review findings

Spec: the review report delivered in chat on 2026-10-17 (service layer, list/detail
views, forms). Each task below carries the file:line evidence from that review.
All paths are relative to `src/` unless they start with `README.md`, `PRD.md`,
`Makefile` or `docs/`.

## Global Constraints

- Go module is in `src/`. Build and test with `cd src && go vet ./... && gofmt -l . && go test -race ./...`
  before every commit; all three must be clean (gofmt prints nothing).
- Bubble Tea v2 (`charm.land/bubbletea/v2`), lipgloss v2, gophercloud v2.14. Value-receiver
  models; every API call takes a `context.Context` from `shared.RequestCtx()` or
  `shared.LongRequestCtx()`.
- One atomic commit per logical fix, conventional-commit subjects (`fix(scope): …`,
  `feat(scope): …`, `refactor(scope): …`, `test(scope): …`, `docs: …`). Body explains
  the user-visible symptom. **No `Co-Authored-By` or any AI-attribution trailer, and no
  mention of AI tooling anywhere in commits.**
- Every bug fix ships with a regression test in the same commit.
- Ctrl+A and Ctrl+B are reserved (GNU Screen / tmux) and must never be bound.
- Prefer IPv6 over IPv4 wherever an address is chosen.
- Key bindings live in `shared/keys.go` (`shared.Keys.X`) and are rebindable through
  `config.DefaultKeybindings()`; never compare `msg.String()` to a literal for an
  action key (literal "esc" and "ctrl+c" are the only exceptions).
- Status bar glyphs must respect `shared.PlainMode` (ASCII fallbacks).
- Do not restructure beyond the task. The `ui/form` mega-refactor and a full
  `ui/table` package are explicitly OUT of scope for this plan (follow-up).
- Help overlay (`ui/help/help.go`) and README keybinding tables must match the code.
  Tasks that change or add a key MUST update help.go in the same commit; README is
  regenerated in Task 11.

---

## Task 1: Service-layer correctness (compute, errors, transport, ssh, config)

Each bullet is one commit with its own test.

1. **Evacuate sends `onSharedStorage` which Nova ≥ 2.14 rejects.** `compute/servers.go:460`
   uses `servers.EvacuateOpts` whose `OnSharedStorage bool` has no `omitempty`
   (gophercloud `compute/v2/servers/requests.go:1139`). lazystack negotiates up to 2.100
   (`cloud/client.go:24`), so every evacuation fails with HTTP 400. Implement a local
   `EvacuateOptsBuilder` that emits only `host` (and `adminPass` when set) when
   `client.Microversion >= 2.14`, and the legacy body otherwise. Test: marshal both
   paths and assert the 2.14+ body has no `onSharedStorage` key. Keep the
   `onSharedStorage bool` parameter in `EvacuateServer`'s signature so callers
   (serveradminact) need no change; document that it is ignored at ≥ 2.14.
2. **Live migrate hard-codes `block_migration: false`.** `compute/servers.go:438`. With
   microversion ≥ 2.25 Nova expects `"auto"`; `false` rejects local-storage instances.
   Implement a local `LiveMigrateOptsBuilder`: `block_migration: "auto"` at ≥ 2.25,
   omit the field below 2.25. Test both bodies.
3. **Quota errors misclassified.** `shared/errors.go:57-96` checks status codes before
   the quota text match at `:101`. Nova OverQuota is 403 → "Permission denied"; Neutron
   OverQuota is 409 → "Resource is busy". Move the quota body check (match
   `quota exceeded`, `limit exceeded`, `overquota`, `exceeded for`, case-insensitive)
   BEFORE the status branches. Tests: 403-with-quota-body and 409-with-quota-body
   classify as `quota`; plain 403 and 409 unchanged.
4. **Blanket 409/400 text hides the API's reason.** `compute/servers.go:355`
   `novaFaultMessage` extracts the API message but is private. Move it to
   `shared/errors.go` as `APIFaultMessage(err error) string` (handles Nova
   `{"<fault>": {"message": …}}`, Neutron `{"NeutronError": {"message": …}}` and
   Octavia `{"faultstring": …}` bodies, from `gophercloud.ErrUnexpectedResponseCode.Body`).
   For 409 and 400, append it to the friendly message as
   `<friendly> (<api message>)` when non-empty. Change the 409 text to
   "Conflict — the resource is busy, in use, or the name already exists." Tests for
   each body shape and for the fallback when the body is not JSON.
5. **No 401 / 5xx categories.** Add to `ParseError`: 401 → category `unauthorized`,
   message "Session expired or credentials rejected — press C to reconnect."; 500/502/503/504
   → category `unavailable`, message "The cloud API is unavailable (HTTP <code>) — try
   again shortly." Tests.
6. **Retried DELETE surfaces false "not found".** `cloud/http.go:243-247` retries DELETE
   and PUT on 502/504. A DELETE that executed before the gateway timed out is replayed,
   gets 404, and the user is told the resource was deleted by someone else. Fix in the
   transport: when a retry of a DELETE returns 404, return the response as 204 No
   Content with an empty body (log via `shared.Debugf`). Test with an httptest server
   that returns 504 then 404.
7. **Hypervisor listing returns a raw unwrapped error with a partial result.**
   `compute/hypervisors.go:56`. Return `nil, fmt.Errorf("listing hypervisors: %w", err)`
   like every other lister. Test.
8. **SSH destination lacks `--`.** `ssh/ssh.go:102` and `:144`. A user name beginning
   with `-` is parsed as an ssh option. Insert `--` before `user@host` in both
   `BuildArgs` and `BuildCommandString`. Update existing tests' expectations and add
   one with a `-oProxyCommand=x` user.
9. **Duplicated nil-client guards.** `volume/volumes.go:21-35`, `quota/quota.go:32-57`,
   `loadbalancer/lb.go:23-28`. Add `shared.RequireClient(sc *gophercloud.ServiceClient,
   service string) error` returning `fmt.Errorf("%s service is not available in this
   cloud", service)` on nil, replace the three local helpers, and add the guard to the
   public list/get functions in `compute`, `network`, `image` that lack one. Test the
   helper.
10. **Config accepts a negative `update_check_interval` and unvalidated colors.**
    `config/config.go:336-338`. Clamp negative or zero `update_check_interval` to the
    default (24) with a `cfg.Warnings` entry; validate every `colors.*` value as
    `#RGB`/`#RRGGBB` hex and drop invalid ones with a warning. Tests.

Files: `compute/servers.go`, `compute/hypervisors.go`, `shared/errors.go`,
`cloud/http.go`, `ssh/ssh.go`, `shared/` (new `client.go`), `volume/volumes.go`,
`quota/quota.go`, `loadbalancer/lb.go`, `config/config.go` and their `_test.go`.

---

## Task 2: Service-layer robustness (discovery, microversion floors, no-gateway subnets, pagination)

1. **Version discovery 404s on legacy project-suffixed endpoints.** `cloud/client.go:80-88`
   GETs `compute.ServiceURL("")`. Catalogs with `/v2.1/<project_id>` return 404 and every
   launch falls back to 2.1 (losing flavor name/vcpu/ram). On failure, retry once against
   the URL with the trailing path segment stripped (what keystoneauth does). Test with an
   httptest server that 404s on `/v2.1/<id>/` and serves `/v2.1/`.
2. **Version-gated calls give no hint when the negotiated microversion is too low.**
   `compute/servers.go:508-514` (remote console, needs 2.6), volume attach fields (2.89),
   `Locked` (2.9). Add `compute.RequireMicroversion(client, "2.6", "remote console")`
   returning `fmt.Errorf("%s requires Nova microversion %s; connected with %s", …)` and
   call it before each gated request. Test the comparison helper (including "2.10" > "2.9").
3. **A subnet cannot be created without a gateway.** `network/networks.go:222-224` and
   `ui/subnetcreate/subnetcreate.go:429`, `ui/subnetedit/subnetedit.go`. Blank gateway
   omits the field so Neutron auto-assigns `.1`. Accept the literal `none`
   (case-insensitive) in the gateway field: pass a pointer to `""` so gophercloud emits
   `"gateway_ip": null`. Update the field's placeholder/help text to
   `IP, blank = auto, "none" = no gateway`. Tests at the network layer (request body has
   `"gateway_ip":null`) and in the form (value `none` maps to the pointer).
4. **Keypair and instance-action listings are single-page in gophercloud though Nova
   paginates them** (≥ 2.35 / 2.58). `compute/keypairs.go:29`, `compute/actions.go:27`.
   Follow `keypairs_links` / `links` with `rel=next` manually until exhausted (cap at 100
   pages). Test with an httptest server returning two pages.

Files: `cloud/client.go`, `compute/servers.go`, `compute/keypairs.go`,
`compute/actions.go`, `network/networks.go`, `ui/subnetcreate/subnetcreate.go`,
`ui/subnetedit/subnetedit.go` and tests.

---

## Task 3: Server list, status bar and tab bar bugs

1. **Server list snaps to top on every auto-refresh.** `ui/serverlist/serverlist.go:405`
   `applyFilter` sets `scrollOff = 0`; it is called from `serversLoadedMsg` (`:154`) and
   `imageNamesMsg` (`:169`). Replace the reset with a clamp + `ensureVisible()`. Test:
   cursor at row 30 of 50 with tableHeight 10, deliver a loaded msg, assert the cursor
   row is still within `[scrollOff, scrollOff+tableHeight)`.
2. **Esc does not clear a committed filter** though `:571` and `help.go:94` say it does.
   `serverlist.go:301-304`. Order: if `filter.Value() != ""` clear it and `applyFilter`;
   else if selection non-empty clear selection. Test both.
3. **Empty-state hint says `[c]`** (`:582`); create is Ctrl+N, c is clone. Use the key
   from `shared.Keys.Create.Help().Key`. Snapshot/test.
4. **Bulk count lies under a filter.** `Hints()` shows `len(m.selected)` (`:928`) but
   `SelectedServers` returns only filtered ones (`:941-955`). Count only selected servers
   present in `m.filtered` (mirror `volumelist.go:201-217 listedSelection`). Test.
5. **Bottom table row clipped.** `serverlist.go:547`, `volumelist.go:451-457`,
   `floatingiplist.go:239-245` use `height-5`; the root prepends a tab bar
   (`app/render.go:157-159`) and clips to `height-1`. keypairlist already uses
   `height-6` (`keypairlist.go:278`). Change all three to `height-6` and add a test per
   view that the rendered output of a full list fits in `height-1` lines after the tab
   bar is accounted for (count lines of `View()` ≤ height-2).
6. **Status-bar hints vanish at normal widths.** `ui/statusbar/statusbar.go:66-72` drops
   the entire right side when it does not fit. Instead truncate the hint with
   `shared.TruncateCells` to the available width (minimum 10 cells, else drop). Also
   replace the hardcoded `⚠` with an ASCII `!` when `shared.PlainMode`. Tests at widths
   80 and 120 asserting a non-empty, width-bounded bar.
7. **Plain-mode glyph leaks.** `app/actions_results.go:55,94` (`✓`), `app/modal.go:268-270`
   (`✗`/`✓`), `ui/cloudpicker/cloudpicker.go:83,89` (`▸`, `↑/↓`). Add
   `shared.Glyph(unicode, ascii string) string` and use it at these sites (`✓`→`OK`,
   `✗`→`FAIL`, `⚠`→`!`, `▸`→`>`, `↑/↓`→`up/down`). Test the helper under both modes.
8. **Tenth tab unreachable and bar overflows.** `app/app.go:555-560` handles only
   `'1'..'9'`; `app/tabs.go:202` renders `"10:Key Pairs"`. Bind `0` to tab 10 (document
   as `1-9, 0` in help's Global section and `Hints`), and truncate the rendered tab bar
   to `m.width` with `shared.TruncateCells` so it never wraps. Tests: `0` switches to the
   tenth tab; a 10-tab bar at width 80 is one line and ≤ 80 cells.

Files: `ui/serverlist/serverlist.go`, `ui/volumelist/volumelist.go`,
`ui/floatingiplist/floatingiplist.go`, `ui/statusbar/statusbar.go`, `shared/format.go`
(or new `shared/glyph.go`), `app/actions_results.go`, `app/modal.go`,
`ui/cloudpicker/cloudpicker.go`, `app/app.go`, `app/tabs.go`, `ui/help/help.go`, tests.

---

## Task 4: Error state, DNS, search cursor, detail paging across list views

1. **A failed refresh wipes the whole table in 19 views.** Pattern
   `if m.err != "" { write "Error: "+err; return }` e.g. `ui/serverlist/serverlist.go:574-577`,
   `ui/volumelist/volumelist.go:544-547`, and in floatingiplist, keypairlist, imageview,
   lbview, secgroupview, networkview, routerview, dnslist, hypervisorlist,
   usermanagement, servicecatalog, volumedetail, serverdetail, keypairdetail, actionlog,
   consolelog, auditlog (grep `m.err != ""` under `ui/` to find them all; list every file
   touched in your report). New behaviour: keep rendering the last good rows; render the
   error as ONE line under the title, through `shared.ParseError(err).FriendlyMessage`
   (store the `error`, not its string, in the model), prefixed with the plain-mode-aware
   warning glyph; clear it on the next successful load. If there has never been a
   successful load, the error line plus the existing empty/loading text is shown. Add a
   test in serverlist and volumelist: rows loaded → error msg → `View()` still contains a
   row name and contains the friendly error.
2. **DNS: R resets cursor to zone 0 and DNS never auto-refreshes.** `ui/dnslist/dnslist.go:73-77`
   and no `shared.TickMsg` case (`:104`) though `app/routing.go:168` feeds it ticks.
   Restore the cursor by zone ID after reload; add a `TickMsg` case gated by a
   `shared.RefreshGate` like the other lists. Tests.
3. **Image/LB search resets the cursor on every keystroke and on Enter.**
   `ui/imageview/imageview.go:581-586`, `ui/lbview/lbview.go:579-600`. Preserve the
   cursor by ID when the previously selected item is still in the filtered set (mirror
   `serverlist.go:372-400`). Tests.
4. **imageview Tab clears the bulk selection** (`imageview.go:487-495`), so peeking at the
   info pane throws it away. Keep the selection across pane changes; only Esc clears it.
   Test.
5. **keypairdetail lacks PgUp/PgDn** (`ui/keypairdetail/keypairdetail.go:79-88`) while
   volumedetail has them. Add. Test.
6. **Dead `R` handler in lbview** (`lbview.go:569`): `app/app.go:600` consumes R first.
   Remove the dead case.

Files: the 19 view files above, `ui/dnslist/dnslist.go`, `ui/imageview/imageview.go`,
`ui/lbview/lbview.go`, `ui/keypairdetail/keypairdetail.go`, tests.

---

## Task 5: Form and picker bugs

1. **volumecreate double-submits.** `ui/volumecreate/volumecreate.go:181-185` routes keys
   with no `submitting` guard; `submit()` at `:415` has none. Add `if m.submitting {
   return m, nil }` at the top of `submit()` (same as `servercreate.go:841`,
   `keypaircreate.go:162`). Test: two submits produce one command.
2. **Backgrounded image upload loses its result.** `ui/imagecreate/imagecreate.go:232-235`
   sets `Active=false` on `b`; `app/modal.go:353-356` forwards only while Active. Mirror
   downloads (`app/modal.go:265-277`): add `imagecreate.UploadFinishedMsg{Name string;
   Err error}` emitted when an upload completes while inactive, route upload progress /
   done / error messages to imageCreate regardless of Active in
   `updateAnyModalBackground`, and show `✓ Image <name> uploaded` / `✗ Image upload
   failed: …` (plain-mode aware) in the status bar. Test at the app level: background the
   upload, deliver the done msg, assert the sticky hint.
3. **Esc dead while loading** in `ui/routercreate/routercreate.go:128`,
   `ui/serverpicker/serverpicker.go:110`, `ui/volumepicker/volumepicker.go:408`,
   `ui/subnetpicker/subnetpicker.go:123`, `ui/fippicker/fippicker.go:702` (all
   `if m.loading || m.submitting { return m, nil }` before Back). Check `shared.Keys.Back`
   BEFORE the guard so Esc closes the modal while loading (results arriving later are
   dropped because the modal is inactive). Keep the guard for `submitting`. Test in each.
4. **Floating IP auto-allocated with no confirmation.** `fippicker.go:647-651`: with no
   free FIP, `fipsLoadedMsg` immediately runs `resolveTargets(nil)` → allocate, and the
   external network is `nets[0]` (`:984`); `app/actions_resource.go:256-280 doAllocateFIP`
   also picks `nets[0]`. New behaviour: when no FIP is free the picker shows the single
   "+ Allocate new floating IP" row and waits for Enter. When more than one external
   network exists (both sites), show a network picker step (reuse the picker's list
   rendering) before allocating. Tests: no auto-allocate on load; Enter allocates;
   multiple networks → network step.
5. **SG rename/clone open with no selection.** `app/actions_resource.go:354-364` skip the
   `SelectedGroupID() == ""` guard every other opener has. Add it. `actions_resource.go:455`
   `p.ID[:8]` panics on a short ID: use `shared.TruncateCells(p.ID, 8)` or a length
   check. Tests.

Files: as listed plus `app/modal.go`, `app/actions_resource.go`, tests.

---

## Task 6: Overlay interface, modal hints, create feedback, widths

1. **Four parallel 35-entry switches in `app/modal.go`** (`activeModalView :12-119`,
   `updateAnyModal :123-259`, `updateAnyModalBackground :280-391`,
   `setSizeAllModals :394-430`); the background switch has drifted (sshPrompt,
   serverAdminAct, columnPicker missing, so sshprompt's cursor Blink from Init is
   dropped). Introduce in `app/` an `overlay` interface
   `{ IsActive() bool; View() string; Hints() string }` plus per-model adapters where
   needed (most modal models already expose `Active` and `Hints()`), and an ordered
   `func (m *Model) overlays() []overlayRef` slice that the four functions iterate.
   Order must match the current `activeModalView` precedence exactly. Every modal that
   today lacks a `Hints()` gets one. Existing tests in `app/modal_parity_test.go` must
   keep passing; extend them to assert every overlay appears in all four paths.
2. **Modal forms' `Hints()` are never wired.** `lbcreate.go:468`, `lblistenercreate.go:484`,
   `lbpoolcreate.go:586`, `lbmembercreate.go:742`, `lbmonitorcreate.go:740`,
   `imagecreate.go:857`, `imageedit`, `imagedownload.go:540` have no caller; the status bar
   shows the underlying list's hints under an open form. In `render.go`, when an overlay is
   active, render the status bar with that overlay's `Hints()`. Test: open a modal, assert
   the status bar text equals its hints.
3. **Full-screen create forms close silently.** `ui/servercreate/servercreate.go:264-266`,
   `ui/volumecreate/volumecreate.go:160-162`, `ui/keypaircreate/keypaircreate.go:137-139`
   emit only `ViewChangeMsg`. Also emit `shared.ResourceActionMsg{Action: "Created",
   Name: …}` (so `actions_results.go:94` toasts) and pass the new ID so the list highlights
   it (`volumeList.SetHighlight` exists, `routing.go:263`; add equivalent for serverlist
   and keypairlist if missing). Tests.
4. **Fixed modal widths ignore narrow terminals.** `lbcreate.go:552` Width(55),
   `lblistenercreate.go:585` Width(50), `lbpoolcreate.go:677` Width(60),
   `lbmonitorcreate.go:818` Width(55), `cloneprogress.go:468` Width(60),
   `app/modal.go:641-645` Width(50/70), `projectpicker.go:146` Width(40). Clamp each to
   `min(fixed, m.width-6)` (minimum 20). Test one representative at width 40.

Files: `app/modal.go`, `app/render.go`, the listed form files, tests.

---

## Task 7: Confirmations, bulk prompts, metadata flow, transfer cancel, Enter semantics

1. **No confirmation for disruptive actions:** single-server resize
   (`ui/serverresize/serverresize.go:167-176` runs on Enter; bulk confirms at `:169-174`),
   cold migrate / live migrate / evacuate / reset state (`ui/serveradminact/serveradminact.go:162-180`,
   only Force Delete confirms), metadata delete (`ui/servermetadata/servermetadata.go:646-650`).
   Reuse the inline y/n step pattern from `ui/serverrebuild/serverrebuild.go:165-179`,
   naming the server and the target flavor/host/state/key in the prompt. Tests.
2. **Bulk confirmation does not name the servers.** `app/modal.go:534-540` sets
   `Name="N servers"` and the body (`:616`) reads `delete server "3 servers"`. Build the
   body as `<Action> 3 servers: a, b, c` listing up to 5 names then `and N more`. Used by
   `app/actions_server.go:230, :259, :299`. Test.
3. **servermetadata closes after every operation** (`servermetadata.go:556-565`). Stay
   open, update the local map, show `✓ saved` in the modal's status line; Esc closes.
   Test.
4. **Transfers are not cancellable** and `imagecreate.go:859` hints "esc cancel" though
   `:230-237` ignores Esc; `imagedownload.go:269-274` same. Wire Esc → inline y/n →
   cancel through the `StallCtx` cancel func; on cancel delete the partial local file
   (download) or the Glance image record created for the upload. Tests for the y/n state
   machine and that the cancel func is called.
5. **Enter semantics.** Keep Enter-submits in the two single-field modals
   (serverrename, serversnapshot) but make their footer say `enter save`. In
   `ui/imagedownload/imagedownload.go:298-304`, Enter on the path field only opens the
   directory picker; Ctrl+S submits. Test.

Files: as listed, tests.

---

## Task 8: Form validation and API error sanitizing

1. **Validation gaps surface raw Neutron/Octavia 400s.**
   - `ui/subnetcreate/subnetcreate.go:398-404`: parse CIDR with `netip.ParsePrefix`;
     reject when the address family does not match the selected IP version; validate
     gateway (unless blank or `none`) as an IP inside the CIDR; validate DNS servers.
   - `ui/subnetedit/subnetedit.go:299-321`: validate gateway and DNS the same way.
   - `ui/portcreate/portcreate.go:555-571` and `ui/portedit/portedit.go:462-481`
     `parseAddressPairs` never fail: validate each `ip[/prefix][,mac]` entry.
   - `ui/lblistenercreate/lblistenercreate.go:401-409`: in edit mode an empty connection
     limit must send `-1` (unlimited) rather than leave it unchanged; show the current
     value as placeholder.
   - `ui/lbcreate/lbcreate.go:428-431`: show "Loading subnets…" while loading, "No subnets
     available" only after load.
   Lift the IP/CIDR/port helpers that `ui/sgrulecreate/sgrulecreate.go:583-593` and
   `ui/portcreate/portcreate.go:469-490` already have into `shared/validate.go`
   (`ValidCIDR`, `ValidIP`, `IPInCIDR`, `ValidPort`, `ValidMAC`) and use them. Tests for
   each helper and each form rule.
2. **Only the 5 LB forms sanitize API errors**; the other forms show raw gophercloud text
   inline. Route every form's submit error through `shared.ParseError(err).FriendlyMessage`
   (grep `m.err = err.Error()` and `m.err = fmt.Sprintf(.*err` under `ui/` and list the
   files touched). Test one representative form.

Files: `shared/validate.go` (new), the listed forms, tests.

---

## Task 9: Filter, sort and Esc contract on every list view

1. **Shared helpers first** (own commit): in `shared/` add
   - `SortCycle` helper: `NextSort(cur sortState, cols int) sortState` and
     `RestoreCursor(ids []string, prevID string) int` replacing the six copies
     (`serverlist:229-274`, `volumelist:353-396`, `floatingiplist:158-197`,
     `keypairlist:132-172`, `imageview:646-690`, `lbview:822-860`);
   - `ui/filterbar` component wrapping `textinput` with `Active()`, `Value()`,
     `Match(fields ...string) bool` (case-insensitive substring) and a one-line `View()`
     `"/ <input>"` / `"filter: x (/ edit, esc clear)"`, extracted from serverlist;
   - replace the seven local `truncate()` helpers (`dnslist:325`, `hypervisorlist:229`,
     `usermanagement:460`, `lbview:1952`, `copypicker:206`, `vmpassword:246`,
     `servicecatalog:240`) with `shared.TruncateCells`;
   - move the six `padContent` copies (`imageview:979`, `lbview:1161`, `routerview:674`,
     `networkview:714`, `secgroupview:698`, `serverdetail:843`) to `shared.PadContent`.
   Tests for every helper.
2. **Add `/` filter** to volumelist, floatingiplist, keypairlist, secgroupview (groups),
   networkview (networks), routerview (routers), dnslist (zones) using `filterbar`.
   Filter persists across refresh; cursor restored by ID. Register each view in
   `app.textInputFocused()` so typed keys never reach global handlers. Add the binding
   to each view's `Hints()` and help section.
3. **Add `s`/`S` sort** to secgroupview, networkview, routerview, dnslist (name / status /
   count columns as appropriate) using the shared helper; add to Hints and help.
4. **One Esc contract**, enforced through a view-interface method
   `Escape() (handled bool)` on every list view: clear filter → clear selection → not
   handled. Remove the three per-view Esc special cases in `app/app.go:496-514`
   (and their redundant `|| msg.String()=="esc"`). lbview's Hints must stop saying
   "esc back" on panes where it does not go back (`lbview.go:2135-2143`); imageview and
   lbview hints saying "S sort" become "s/S sort".
5. **Routers: Enter is a no-op** though help (`help.go:255`) and README promise detail.
   Make Enter on a router focus the interfaces pane (the existing detail pane) and Esc
   from that pane return focus to the list (this is the one view where Esc also moves
   focus, after filter/selection). Test.
6. **dnslist Esc "back to serverlist"** (`dnslist.go:119`) is removed in favour of the
   contract (tabs are switched with number keys / arrows like every other tab).

Files: `shared/`, new `ui/filterbar/`, the seven list views, `app/app.go`,
`app/routing.go`, `ui/help/help.go`, tests. Snapshot tests under `__snapshots__` must be
updated deliberately (review each diff).

---

## Task 10: Rebindable keys and the executeAction table

1. **73 hardcoded key strings bypass the keybinding config.** Add to `shared/keys.go` and
   `config.DefaultKeybindings()`: `submit` (ctrl+s), `health_monitor` (ctrl+h),
   `drain_member` (w), `select_all` (x), `download` (ctrl+g), `background` (b),
   `toggle_details` (d, error modal). Replace every `msg.String() == "ctrl+s"` in the
   forms and the LB/image literals in `app/app.go:869-889, :1011` with `key.Matches`.
   Keep literal comparisons only for `"esc"` and `"ctrl+c"`. Add a test that greps the
   source tree (`ui/`, `app/`) for `msg.String() == "` and allows only those two
   literals, so the regression cannot return. `config` tests must cover the new names
   and the reserved-key rejection for them.
2. **`executeAction` is ~555 lines of 25 near-identical closures**
   (`app/actions_server.go:547-1102`), differing only in compute func, audit action and
   label. Replace with a table `var serverOps = map[string]serverOp{…}` where
   `serverOp{run func(ctx, client, id) error; audit string; label string}` and one
   generic closure. Actions with special return values (rescue password, evacuate
   password, resize) keep dedicated code. Behaviour must be identical: existing tests in
   `app/actions_server_test.go`, `app/admin_actions_test.go`,
   `app/complex_actions_integration_test.go` pass unchanged; add a table-driven test that
   every op name in the map maps to the expected audit action.

Files: `shared/keys.go`, `config/config.go`, `app/app.go`, `app/actions_server.go`,
all form files with `ctrl+s`, `ui/modal/error.go`, tests.

---

## Task 11: Help, README and PRD accuracy; lint target

1. **Help overlay contradicts the code** (`ui/help/help.go`): User Management says `d`
   delete (`:333`) but code binds ctrl+d; Security Groups `enter expand/collapse` (`:234`)
   but Enter edits a rule / opens a server, and `r` rename, `c` clone, tab pane cycling
   are missing; Networks `enter expand/collapse subnets` (`:245`) but Enter edits
   subnet/port and the ports pane + ctrl+n create port are missing; Volume List section
   (`:177`) omits sort; `esc back to …` entries (`:237,248,259,270`) must reflect the new
   Esc contract from Task 9; Global must list `0` for tab 10, and every view that gained
   `/` and `s/S` in Task 9. Add help sections for DNS, Hypervisors, User Management and
   Audit Log if absent. Snapshot under `ui/help/__snapshots__` updated deliberately.
2. **README keybinding tables**: fix "read-only port listing" (ports have CRUD); add `O`
   columns, SG `r`/`c`, Networks ports pane, LB detail keys (`o`, `w`, `x`, `space`,
   `ctrl+h`, per-pane `ctrl+n`), Volumes `space` and `s/S`, `/` and `s/S` where added,
   `0` for tab 10, and sections for DNS, Hypervisors, User management, Audit trail; fix
   the Routers / Security groups / Networks `Enter` rows. Mention `none` for no-gateway
   subnets and the new keybinding names from Task 10 in the config example.
3. **PRD.md**: Project Switching is no longer "ALPHA — UNTESTED" (tests exist in
   `app/connection_scope_test.go`); Phase 5 is complete; add a short "Technical debt"
   entry for the deferred `ui/form` and `ui/table` refactors.
4. **Lint target**: add `lint` to both `Makefile` and `src/Makefile`: `gofmt -l` must
   print nothing, `go vet ./...`, and `go run honnef.co/go/tools/cmd/staticcheck@latest
   ./...`. Fix anything staticcheck reports in the tree (list them in the commit body).
   Document `make lint` in INDEX.md next to `test`.

Files: `ui/help/help.go` (+snapshots), `README.md`, `PRD.md`, `INDEX.md`, `Makefile`,
`src/Makefile`.

---

## Task 12: Tests for the untested packages

Packages with no `_test.go`: `ui/lbcreate`, `ui/networkcreate`, `ui/serverrebuild`,
`ui/serverrename`, `ui/serversnapshot`, `ui/sgcreate`, `ui/actionlog`,
`ui/hypervisorlist`, `ui/servicecatalog`. For each, add a focused test file covering:
construction, key navigation (tab/arrow), submit with an empty required field produces a
validation error and no command, Esc produces the cancel message, and the width clamp
at 40 columns renders without panic. Follow the style of `ui/routerview/routerview_test.go`.
One commit per package (`test(<pkg>): …`).

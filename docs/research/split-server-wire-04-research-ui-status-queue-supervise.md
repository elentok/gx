# Movable clusters in `ui`, `ui/status`, `ui/tickets/queue` and `ralphloop/supervise`

Ticket `gx:split-server-wire/04`. Design branch at `31dd2aba6`. Facts come from the Go source;
proposals are for the grilling ticket to decide.

## Method

- Sizes are counted like `sizeguard/sizeguard_test.go`: tracked `.go` files directly in the
  directory (sub-directories are separate packages), all lines (`wc -l`), Test funcs are
  `^func Test[A-Z_]` in `_test.go`.
- 90% targets: at most 45 files, 4,050 source lines, 4,500 test lines, 162 Test funcs.
- A cluster's "uses from the rest of its package" comes from a token scan of each file against the
  package-level declarations of the other files, then read by hand. Names that only collide with a
  method name (`Error`, `Update`, `Tick`) were checked and discarded.
- `sizeguard` says (`packageInstruction`, `sizeguard_test.go:46`): "Trim tests or move code to
  another package". Trimming tests is a sanctioned fix, but this report only proposes moves unless
  a move cannot reach the target.

## Summary

| Package | Files | Source | Test lines | Test funcs | Over 90% | Proposed fix | After |
|---|---|---|---|---|---|---|---|
| `ui` | 47 | 1,844 | 1,295 | 79 | files | `ui/badge` sub-package | 43 files, 1,685 src, 1,192 test, 70 funcs |
| `ui/status` | 45 | 2,609 | 4,991 | 163 | test lines, Test funcs | move `e2e_test.go` to `ui/status/e2e` | 44 files, 2,609 src, 4,201 test, 142 funcs |
| `ui/tickets/queue` | 45 | 2,622 | 4,968 | 161 | test lines | `queue/rows` sub-package plus a test rewrite (no moves-only fix exists) | 43 files, 1,921 src, about 4,450 test, about 143 funcs |
| `ralphloop/supervise` | 35 | 2,465 | 4,768 | 119 | test lines | `supervise/startprompt` sub-package | 30 files, 2,195 src, 3,986 test, 91 funcs |

Only `ui/tickets/queue` has no clean moves-only fix. The other three are real moves with no
import cycle.

---

## 1. `ui`

Current: 47 files (94%, over), 1,844 source lines (41%), 1,295 test lines (26%), 79 Test funcs.
Needs at least 2 fewer files.

`ui` is a leaf toolkit (colors, frame, badges, overlays). It has no sub-package of its own named
after a file cluster; `ui/components`, `ui/keys` and others already exist as siblings. It imports
only `config`, `ralphloop/agent` (`settings.go:4-5`) and `ui/keys` (`chord_overlay.go:8`). 263
files import `github.com/elentok/gx/ui`.

### Clusters

Which `ui` file uses which (token scan, non-test files):

- `badge.go` uses `buttons.go` (`capLeft`, `capRight`, `buttons.go:11-12`) and the `Color*` vars
  in `styles.go`. `buttons.go` uses `badge.go` (`renderPill`) and `ColorGreen`. No other `ui` file
  uses either one.
- `harness_label.go`, `editor_args.go`, `openurl.go`, `columns.go`, `memo.go`, `progress_bar.go`,
  `relative_time.go`, `spinner.go`, `mouse_wheel.go`, `overlay.go`, `icons.go`, `terminal.go`,
  `hover.go`: no non-test use of another `ui` file (only `styles.go` colors for a few).
- `feedback.go` uses `icons.go` and `keyhints.go`. `frame.go` uses `hover.go` and `styles.go`.
  `panel.go` uses `frame.go`. `chord_overlay.go` uses `frame.go` (`injectBorderTitle`) and
  `styles.go`.
- Tangled core to keep: `styles.go` (colors, used by almost everything), `frame.go`, `panel.go`,
  `chord_overlay.go`, `feedback.go`, `keyhints.go`, `settings.go` + `terminal.go`.

### Candidate moves

| Cluster | Files | Src | Test files | Test lines / funcs | `ui.X` uses outside |
|---|---|---|---|---|---|
| `badge` (RenderBadge*, RenderChip, BadgeGroupItem, RenderButton) | `badge.go` 129, `buttons.go` 30 | 159 | `badge_test.go` 75, `buttons_test.go` 28 | 103 / 9 | 11 uses in 7 files (`ui/commit/view.go:190`, `ui/prs/view.go:301`, `ui/status/diffarea/render.go:187`, `ui/worktrees/view.go:114`, `ui/worktrees/model_header_test.go:68`, `ui/log/list_panel.go:309`, `ui/components/modal_confirm.go:41,108,109`) |
| `harnesslabel` | `harness_label.go` 56 | 56 | `harness_label_test.go` | 37 / 2 | 2 uses in 2 files (`ui/tickets/queue/server.go:99`, `ui/agents/terminal/terminal_view.go:115`) |
| `editorargs` | `editor_args.go` 33 | 33 | `editor_args_test.go` | 29 / 1 | 4 uses in 4 files |
| `scrollbar` | `scrollbar.go` 103 | 103 | `scrollbar_test.go` | 107 / 8 | 5 uses in 5 files |

### Import-cycle check

The `badge` cluster needs these parent internals: the exported `Color*` vars from `styles.go`
(`ColorBlue`, `ColorDeepBg`, `ColorGreen`, `ColorMauve`, `StyleHint` and so on). They are exported,
so the child imports `github.com/elentok/gx/ui` and `ui` does not import the child. No cycle. The
test files use only `capLeft`/`capRight` (which move along in `buttons.go`) and no other unexported
`ui` name. `harnesslabel`, `editorargs` and `scrollbar` need nothing (scrollbar needs two
`Color*` vars).

### Proposal

Move `badge.go`, `buttons.go` and their two test files to a new `ui/badge` package (name for the
shape ticket). Result: 43 files (86%), 1,685 source lines (37%), 1,192 test lines, 70 Test funcs.
Cost: 11 `ui.X` uses in 7 files.

For more room, add `harness_label.go` (2 uses in 2 files) and its test: 41 files (82%).

---

## 2. `ui/status`

Current: 45 files (90%, at the limit), 2,609 source lines (58%), 4,991 test lines (111% of the 4,500
target, over), 163 Test funcs (over by 1). Needs 491 fewer test lines and 1 fewer Test func.

### Source: tangled, keep

Every source file is a method set on `Model` (`model_state.go:18`) and they call each other in
loops: `diff_*`, `filetree_*`, `model_*`, `view*` all reference `model_state.go`'s `Model`, and
`model_update.go` / `model_keys.go` call into `diff_runtime.go`, `diff_loader.go`,
`filetree_actions.go` and `diff_actions.go`. For example `diff_keys.go` uses `diff_loader.go`,
`diff_runtime.go`, `diff_state.go`, `diff_view.go`, `filetree_runtime.go` and `model_update.go`;
`model_state.go` uses `diff_actions.go`, `diff_loader.go`, `filetree_keys.go`,
`filetree_runtime.go`, `model_help.go` and `model_keys.go`. Any source cluster that moved would
need unexported `Model` internals, so it would create an import cycle (child needs parent, parent
needs child). The package already has three sub-packages (`diffarea`, `filetree`, `stagemodal`)
for what could be extracted. Source lines are not the problem, so nothing needs to move.

### Tests: one external-package file moves cleanly

Of 18 test files, 17 are `package status` and use unexported `Model` internals. One is
`package status_test`: `ui/status/e2e_test.go:1` (790 lines, 21 Test funcs). It uses only
`status.NewModel` and `status.DefaultSettings` (two uses each), `testutil/teatestv2` and real git
repos. It defines its own helpers (`startStageTUI`, `mustRunGit`, `setupRemoteAndClone` and so on,
`e2e_test.go:29-252`) and needs no helper from the other test files (`helpers_test.go` is
`package status`).

An external test package in the same directory still counts: `sizeguard` counts every tracked `.go`
file in the directory. Moving the file to a sibling directory fixes that.

| Cluster | Files | Test lines / funcs | Uses of unexported internals |
|---|---|---|---|
| `e2e_test.go` (TUI flows over git repos) | 1 | 790 / 21 | none |

### Proposal

Move `ui/status/e2e_test.go` to `ui/status/e2e/e2e_test.go` (package `e2e`, a test-only directory;
`ui/commit/e2e_test.go` and `ui/log/e2e_test.go` show the same file name elsewhere). Result:
44 files (88%), 2,609 source lines (58%), 4,201 test lines (84%), 142 Test funcs (79%).

One edit outside the package: `sleepguard/sleepguard_test.go:172` keys the sleep allowlist by file
path (`"ui/status/e2e_test.go": 1`), so the key changes to the new path. Nothing else references
the file (`rg` over the repo: only `CHANGELOG.md:1378` mentions `TestStageE2E_PushActionWithConfirm`).

Possible duplicates (overlap, not identical): `TestStageE2E_YankFilenameFromStatus` and
`TestYankFilenameWithYFInFiletreeView` (`model_search_yank_test.go`); `TestStageE2E_StageFullNewFileFromSidebar`
and `TestFiletreeSpaceTogglesWholeFile` (`model_staging_test.go`). They test different layers (full TUI
vs `Model.Update`), so none is proposed for deletion.

---

## 3. `ui/tickets/queue`

Current: 45 files (90%, at the limit), 2,622 source lines (58%), 4,968 test lines (110% of the
4,500 target, over), 161 Test funcs (99.4% of 162, under but at the edge). Needs 468 fewer test
lines.

### The blocker: the tests cannot move without a rewrite

All 28 test files are `package queue`. Every test drives `QueueModel` through its unexported
fields and helpers. The test helpers chain across files: `rows_test.go` defines `loadQueueModel`,
`withChecked`, `seedFromDisk`, `selectFirstQueueTicketRow` (`rows_test.go:522-590`), which most
other test files call. A token scan found one Test func that touches no package-level name
(`TestResolveTicketPath`, `ticket_menu_test.go`, 19 lines). So no test file can move to another
directory as it stands. A test-only directory that imports `queue` would need `m.rows`,
`m.server`, `m.srv`, `m.harnessPicker`, `m.keys`, `m.header()`, `m.rowsEnv()`, `clampSelected`,
`queueBody` and others exported, which is a production-code change and not a move.

Also `method_count_test.go:41-50` reads every non-test `.go` file in the working directory and
requires `QueueModel` to have 1 to 50 methods. A move of `QueueModel` methods needs that guard in
mind; moving `Rows` methods (below) does not touch it.

### Source clusters (source is not the problem, but for the record)

`QueueModel` already embeds three self-contained structs and a sub-package:

| Cluster | Files | Src | Uses from the rest of the package | Uses outside |
|---|---|---|---|---|
| `Rows` (tree builder + row renderer) | `rows.go` 492, `rows_view.go` 209 | 701 | none: both import only `wire`, `schedule`, `tickets`, `queue/header`, `ui/tickets/rowfmt`, `ui/tree`, `ui`. Mentions of `QueueModel` are comments (`rows.go:69,89,94`). A local `icons := h.Icons` (`rows_view.go:92,190`) is a variable, not the `QueueModel.icons` method at `model.go:105`. | 0 (`rg` finds no `queue.Row/Node/Rows/RowsEnv` use from another package) |
| `Server` | `server.go` 220 | 220 | none (`server.go` uses no other file) | the `Server` type is embedded in `QueueModel` |
| `HarnessPicker` | `harness_picker.go` 143 | 143 | `Server` (`server.go`, in `ReadCmd`) | none |
| `header` (already moved) | `header/header.go`, `finished.go` | 505 | n/a | n/a |

`Rows` is used by `model.go` (9 refs), `preview.go` (4), `search.go` (2), `menus_host.go` (1) and
the test files. The `header` sub-package is the precedent: it holds its own tests
(`header/header_test.go`, `header/summary_test.go`: 10 funcs, 185 lines) written against
`header.Header`, not through `QueueModel`.

Tangled core to keep: `model.go` (660), `state.go`, `link.go`, `layout.go`, `view.go`,
`preview.go`, `search.go`, `hover.go`, `menus_host.go`, `edit.go`, `clear.go`, `approve.go`,
`server_keys.go`. They are all `QueueModel` methods and call each other.

### Candidate test groups

Test lines that test `Rows` behavior through `QueueModel` (they build a model from a directory, then
read `m.rows`):

| Tests | File | Lines (about) | Funcs |
|---|---|---|---|
| tree order, nesting, ancestors, cycles | `rows_test.go:21-370, 458-504` | 390 | 11 |
| `TestRenderQueueTicketRow_*` (call `m.rows.RenderTicketRow(m.rowsEnv(), ...)`) | `render_test.go:65-205` | 130 | 7 |

The three preview-focus tests in `rows_test.go` (`LOnLeafRowFocusesPreview`,
`EnterOnExpandedParent...`, `HLeftEscReturnFocus...`, lines 371-457) test key handling and stay.

Other groups that look cohesive but are tied to `QueueModel`: header text tests in
`model_header_test.go` (398 lines, 16 funcs; they call `m.header().X` on a model, so a few could
become `header` tests: `TestEpicStatusRow*` 3 funcs about 85 lines, `TestQueueHeaderCostSuffixFormats`
about 18 lines, `TestQueueServerMode_HeaderColourBands` about 28 lines, which already only calls
`header.BudgetTotalStyle`), and the `Server` tests (`server_live_test.go` 351/20,
`server_down_test.go` 179/6, `server_budget_test.go` 68/3, `finished_test.go` 79/4).

### Proposal

A moves-only change cannot reach 4,500. Recommended: two steps.

1. Move `rows.go` and `rows_view.go` to `ui/tickets/queue/rows` (clean move; no cycle; 0 outside
   uses; `model.go`, `preview.go`, `search.go` and `menus_host.go` gain a `rows.` qualifier).
2. Rewrite the `Rows` tests above (about 520 lines, 18 funcs) as `rows` package tests that call
   `Rows.BuildEntries(RowsEnv)` and `Rows.RenderTicketRow` on epics built in code, as `header/`
   does. That is a rewrite, not a move: each fixture is rebuilt from `tickets.Epic` values instead
   of a temp directory plus `QueueModel`. Estimated after: 43 files (86%), 1,921 source lines
   (43%), about 4,450 test lines (89%, only about 50 under the cap), about 143 Test funcs.

The margin is thin because the rewritten tests may be longer or shorter than the originals. If the
grilling wants more margin, add the `header`-testable tests listed above (about 130 lines, 5 funcs).

The other way, if the person rejects a rewrite: trim tests (the `sizeguard` message allows it) or
export a test seam. Both are outside "moves only". No exact duplicate Test names exist in the
package; overlaps worth a look are `TestQueueModel_EditTargetIsTheAddressFile` (`cross_tab_test.go`)
and `TestQueueEpicMenu_EditTargetIsTheEpicFile` (`epic_menu_test.go`), and the `QueueServerMode_*`
groups in `server_live_test.go` and `server_down_test.go`.

---

## 4. `ralphloop/supervise`

Current: 35 files (70%), 2,465 source lines (55%), 4,768 test lines (106% of the 4,500 target,
over), 119 Test funcs. Needs 268 fewer test lines. All 21 test files are `package supervise`.

### Source: tangled core

`launch.go` (`LaunchParams`, `launch.go:21-60`) is used by nearly every file; `waitforfinish.go`
(860 lines) uses `backgroundhold.go`, `longticket.go`, `modelguard.go`, `verifycap_cap.go`,
`launch.go`, `recordlivesession.go`, `personinput.go`, `smartzonerecovery.go` and
`waitforfinish_recovery.go`; `smartzonerecovery.go` and `verifycap_cap.go` call back into
`waitforfinish.go` (`compactBoundarySnapshot`, `errCompactNeverConfirmed`, `smartZoneOccupancy`,
`interruptOrRetry`). The loop `waitforfinish.go` <-> `smartzonerecovery.go` <-> `verifycap_cap.go`
is the tangled core. Keep: `launch.go`, `waitforfinish*.go`, `smartzonerecovery.go`,
`backgroundhold.go`, `longticket.go`, `modelguard.go`, `verifycap*.go`, `ratelimit_hold.go`,
`recordlivesession.go`, `personinput.go`.

### Cluster that moves cleanly: start-and-prompt

`startandprompt.go` (270 lines) is only a consumer. No other non-test file uses a symbol it
declares (`StallNudgeText`, `LaunchFailure`, `LaunchFailureKind`, `Hooks`, `StartAndPrompt`,
`Launched`, `PromptedLaunch`, `AdoptedLaunch`, `noActivitySinceLaunch`; the only hits are
`Error` method-name collisions in `modelguard.go`, `recordlivesession.go` and
`waitforfinish.go`). It uses from the parent only exported names: `LaunchParams`, `EventSink`,
`AlreadyFinished`, `LogAgentStartEvent`, `LogLifecycleEvent` (`launch.go`), `WaitForFinish`,
`EmitContextOccupancy` (`waitforfinish.go`), `ErrBlockedPanePaused`, `PromptIdle`
(`waitforfinish_recovery.go`). It reads no unexported field of `LaunchParams` (no `p.x` lowercase
access). So a child package imports `supervise` and `supervise` does not import the child: no cycle.

| Cluster | Files | Src | Test files | Test lines / funcs | Uses outside |
|---|---|---|---|---|---|
| start-and-prompt | `startandprompt.go` 270 | 270 | `startandprompt_retry_test.go` 355/15, `startandprompt_test.go` 115/6, `startandprompt_vim_test.go` 99/4, `promptedlaunch_test.go` 213/3 | 782 / 28 | 33 uses in 8 files: `StallNudgeText` 18, `StartAndPrompt` 4, `Hooks` 4, `LaunchFailure` 3, `LaunchFailureKind` 2, `PromptedLaunch` 1, `AdoptedLaunch` 1; files `ralphloop/conflict.go`, `ralphloop/lifecycle.go`, `ralphloop/lifecycle_died_test.go`, `ralphloop/lifecycle_wait_test.go`, `ralphloop/finish_unexecuted_tool_call_test.go`, `server/sched/launch.go`, `server/sched/run.go`, `testutil/gxfixture/designflows/stallscenarios_test.go` |

The four test files form a closed set: they share helpers only with each other (`startandprompt_retry_test.go`
helpers used by the other two) and use `noActivitySinceLaunch`, which moves along in
`startandprompt.go`. They use `LaunchParams`, `PersonInput.Sleep` (exported) and fakes.

### Leaf cluster (optional): gate

`gate.go` (146 lines, type `Gate`, `NewGate`, `Pause`, `Paused` and others) uses nothing from the
package. Users: `launch.go` (`LaunchParams.Gate *Gate`), `waitforfinish_recovery.go`,
`waitforfinish_background_test.go`, the `waitforfinish_*` tests. Test: `gate_test.go` 172/7.
`supervise.Gate`/`NewGate`: 8 uses in 8 files outside. Moving it alone does not reach the target
(4,596 test lines). A child package would need `supervise` to import it, which is fine
(no back-reference).

### Proposal

Move `startandprompt.go` and its four test files to `ralphloop/supervise/startprompt` (name for the
shape ticket). Result: 30 files (60%), 2,195 source lines (49%), 3,986 test lines (89%), 91 Test
funcs. Cost: 33 `supervise.X` uses in 8 files; `StallNudgeText` accounts for 18 and could stay
in `supervise` (it is a constant with no dependencies) to cut that to 15 uses.

Add the `gate` cluster only if more headroom is wanted: 28 files, 2,049 source lines, 3,814 test
lines, 84 funcs.

---

## Blockers and open points for the grilling ticket

- `ui/tickets/queue`: no moves-only fix for 468 test lines. Needs a test rewrite (the `rows`
  tests), a test trim, or exported test seams. Decide which.
- `ui/status`: the only move is a test file to a new test-only directory; `sleepguard` allowlist
  key changes.
- No cycles for the three proposed moves (`ui/badge`, `ui/status/e2e`, `supervise/startprompt`):
  each child imports its parent's exported API and the parent never imports the child.
- These moves are in packages with no wire types, so none touches `tygo.yaml`.

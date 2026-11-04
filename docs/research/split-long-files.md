# Long files outside the server, cmd and ralphloop splits

Research for ticket gx:split-server-pkg/13. Measured on branch `research/split-long-files` (from
`main` at e039fe8e8) on 2026-11-04. All paths are repo-relative. Line counts are `wc -l` over
tracked `.go` files (`git ls-files '*.go' | xargs wc -l`). Function and type positions come from
`rg '^(func|type)'` on each file. Nothing is estimated except the split sizes in the tables, which
are read from the function line ranges.

## Summary

- The recount matches the baseline: 22 files over 1000 lines, 15 over 800 (the 800 to 1000 band is
  unchanged). `main` moved from a94b22471 to e039fe8e8 and `server/server.go` went from 1016 to
  1018 lines; no file crossed a limit.
- 14 of the 22 are in packages that other research tickets cover (server, ralphloop, ui/status,
  ui/tickets, nativerunner, testutil/agentfake). This note covers the other 11 files in 9 packages.
  There is no other file over 1000 lines.
- 8 of the 11 are `_test.go` files or test-fixture code. They are one flat list of `TestX`
  functions plus a few helpers, so they cut by topic with no code change: move whole functions to
  a new file in the same package. None of those cuts depends on a package split.
- 3 are different. `apiclient/client.go` is 112 methods on one type, `testutil/gxfixture/main.go`
  has one 507-line `run` function, and `server/servertest/servertest.go` is a harness with ten
  jobs. Their file cuts are free, but their imports (`server.*`) will move when `server` splits.
- Only one file in the 900 to 1000 band is in scope: `testutil/gxfixture/scenarios.go` (987).
  Its cut is by scenario family and should happen with the gxfixture cut.
- Every cut below is a move of whole declarations between files of one package. Go does not care
  which file a declaration is in, so behaviour cannot change. `go build ./... && go vet` after each
  move is the check. Test names stay, so `-run` filters and CI history stay valid.

## How to read the tables

- "Cut" is the proposed new file and the line range it takes from today's file.
- "Moves with a package split?" says whether the cut must wait for, or change with, a split of
  `server` or another package. "No" means it can land first and nothing needs redoing.
- Shared helpers stay in the old file (or a `helpers_test.go`) so the new files need no new
  exports.

## 1. Test files cut by topic

### `config/config_test.go` (1227 lines, 55 funcs)

One flat file of `TestLoad*` for the whole config. The source side (`config/`) is already split by
topic (`agents.go`, `dirs.go`, `disk_space.go`, `input_modal_bottom.go`, `migrate.go`,
`project_overrides.go`, `ticket_store.go`), and some tests already followed (`agents_test.go`,
`dirs_test.go`, `input_modal_bottom_test.go`, `migrate_test.go`, ...), so config_test.go is what is
left. It also repeats topics that already have a file: `TestLoadAgentsConfig*` (895-1053) next to
`agents_test.go`, and the `TestLoadInputModalBottom*` tests (652-760) next to
`input_modal_bottom_test.go`.

| Cut | Lines | Size |
|---|---|--:|
| `budget_test.go` (with `writeBudgetConfig`, `loadLongTicketTurns`, `floatSlicesEqual`, only used here) | 290-476 | 187 |
| `agents_test.go` (append the tier tests) | 895-1053, 1068-1090 | 180 |
| `input_modal_bottom_test.go` (append) | 652-760 | 109 |
| `notifications_test.go` (telegram, slack) | 811-894 | 84 |
| `execution_queue_test.go` | 103-130, 239-262, 1091-1123 | 85 |
| `server_test.go` (log retention, TCP listen, watch, recently finished, web TOTP and dev frontend, CPU watchdog) | 57-79, 1124-1227 | 127 |
| `skills_test.go` (skills block) | 477-549 | 73 |
| stays in `config_test.go`: defaults, Load/Init, subscription, UI keys, aliases | ~380 | |

Moves with a package split? No. `config` is not being split; this is a topic cut inside it.
Do not merge into `agents_test.go` if a function name already exists there (`rg '^func Test'` first).
Result: the largest file left is `config_test.go` at about 380 lines.

### `ui/diffview/model_test.go` (1037 lines, 66 funcs)

Tests of `diffview.Model` (16 methods in `model.go`, 44 in the package) in the order they were
written. Natural groups: render rows (84-226), movement and scroll (227-322), `nearestIndex` and
co-scroll (324-387), scroll viewport snapping (439-550, with `buildScrollTestModel`), search
lifecycle (551-713, 969-1029), key handling (715-967, with `buildNavTestModel`, `pressText`,
`pressCode`, `pressCtrl`).

| Cut | Lines | Size |
|---|---|--:|
| `model_keys_test.go` (with the three press helpers) | 715-967, 1030-1037 | 261 |
| `model_search_test.go` | 551-713, 969-1029 | 224 |
| `model_scroll_test.go` | 227-322, 348-387, 439-550 | 260 |
| `model_render_test.go` | 84-226 | 143 |
| stays: init, data, reflow, settings, `nearestIndex` | ~150 | |

Moves with a package split? No. `diffview` already has `diffcore` and `diffrender` subpackages and
`diffview.Model` has 47 methods in non-test files (the baseline's near-limit case). If the model is
split by type, the key and search tests follow the code they test: `model_keys_test.go` and
`model_search_test.go` map to a future `keys.go` / `search.go`. The topic cut above is the same cut
the model split would need, so doing it first costs nothing.

### `ui/app/model_test.go` (1635 lines, 56 test funcs, 6 stub types)

The shell's tests: tab switching, history, lifecycle callbacks, auto-reload gate, modal blocking.
The stub types are each used in one group, so they move with it. The shared helpers `runAppCmd`
(133), `newAppModel` (1123) and `pressKey` (1137) stay in `model_test.go`.

| Cut | Lines | Size |
|---|---|--:|
| `model_tabswitch_test.go` (digit keys, leader keys, g-mnemonics, chords, label tests) | 219-560, 1613-1635 | ~365 |
| `model_firstclass_test.go` (stash, PRs, tickets tabs; open/back restore; carry worktree) | 601-876, 1142-1193 | ~330 |
| `model_gate_test.go` (autoReloadSpy, reloadCounterModel, `TestGate_*`, `TestE2E_*`) | 985-1122, 1194-1336, 1583-1612 | ~290 |
| `model_modal_test.go` (modalOpenStub, modalKeyRecorder, modal tests) | 1337-1582 | 246 |
| `model_lifecycle_test.go` (lifecycleSpy, deactivate/activate) | 60-218 | ~160 |
| stays: input-focused stub, shell chord overlay, back-quits, view height | ~260 | |

Moves with a package split? No for the cut. `ui/app.Model` has 70 methods in non-test files and
already has siblings (`server_stream_test.go` 789, `switcher_test.go`, `model_mouse_test.go`). If
`Model` is split by type, the gate and modal groups map to separate files in the source; the test
cut already matches. `server_stream_test.go` (789) is the only other file in the package close to
the limit, and is under the 800 band.

### `ui/log/model_test.go` (1234 lines, 61 funcs)

Mixed in one file: commit-row rendering and ref colours (72-182), ref rules and search (183-300),
tag jumps and key dispatch (300-540), the pseudo "working tree" log line (541-645), split and
focus handling (646-1008), fullscreen, orientation and flash (1009-1206), the filtered log
(1207-1234). `newTestModel`, `newTestModelDefault`, `newTestModelFiltered`, `runModelInit`, `runCmd`
and `sendKey` are helpers used throughout; they stay.

| Cut | Lines | Size |
|---|---|--:|
| `model_focus_test.go` (split, Esc/Q/H/L returns, fullscreen, orientation) | 646-1076 | ~430 |
| `model_pseudo_test.go` | 541-645 | 105 |
| `model_search_test.go` (search, n/N, tag jumps) | 208-349 | ~140 |
| `model_dispatch_test.go` (dispatch binding, select ref, mouse, key manager, pending focus) | 350-540 | ~190 |
| `model_rows_test.go` (branch class, row colours, badges) | 72-207 | ~135 |
| stays: helpers, flash, filtered log, worktree status | ~150 | |

Moves with a package split? No. `ui/log.Model` has 76 methods; the baseline lists it over 50. The
package already has `model_split.go`, `model_rebase.go`, `list_panel.go` in source and
matching `model_*_test.go` files, so the tests should follow whichever method files `Model` is
split into. The focus group maps onto `model_split.go`'s topic.

### `ui/commit/model_test.go` (1121 lines, 54 funcs)

Groups: setup and detail loading (23-104), focus and keys (105-266), search (268-404), yank (405-493),
file tree (507-733, `commitModelWithDir`), view and header (733-845), mouse (888-948, 1059-1121),
editor and AI chords (846-887, 949-1058). `ui/commit` already has `model_keys_test.go` (245),
`model_yank_test.go` (138), `view_test.go`, `image_diff_test.go`.

| Cut | Lines | Size |
|---|---|--:|
| append yank tests to `model_yank_test.go` | 405-493 | 89 |
| `model_mouse_test.go` | 888-948, 1059-1121 | ~125 |
| `model_editor_test.go` (edit chords, editor line, AI chord) | 846-887, 949-1058 | ~150 |
| `model_filetree_test.go` | 507-733 | ~230 |
| `model_search_test.go` | 268-404 | 137 |
| append header and view tests to `view_test.go` | 733-845 | ~110 |
| stays: setup, focus, tab, Esc | ~260 | |

Moves with a package split? No. `ui/commit.Model` has 114 methods (non-test); it is a type-split
candidate, and the yank / mouse / editor / search groups line up with `model_yank.go`, `view.go`
and the other source files by name, so the tests already follow the cut a type split would make.

## 2. `testutil/gxfixture`

Package: 18 source files, 16 test files, `package main`. It is a test fixture command.

### `testutil/gxfixture/main.go` (1125 lines)

Three pieces in one file:

- `Output` and `ShellEnv` (60-158): the JSON contract the callers read. 99 lines.
- `run` (166-672): one 507-line function. It parses flags in a hand-written loop (166-222), checks
  the scenario, builds the three binaries, writes the config, seeds, starts the server, waits and
  prints. `watchLifetime`, `serveMock` are small neighbours.
- `fixture` and its helpers (716-1125): `isolatedEnv`, `hostPathWithoutCodex`, `writeConfig`,
  `seed` (155 lines), `initRepo`, `git`/`cli`, `waitReady`, `binary`, `removeAll`.

| Cut | Lines | Size |
|---|---|--:|
| `output.go` (`Output`, `ShellEnv`) | 56-158 | ~100 |
| `env.go` (`isolatedEnv`, `hostPathWithoutCodex`, `playwrightBrowsersPath`) | 732-816 | ~85 |
| `seed.go` (`writeConfig`, `seed`, `initRepo`, `seedPauseRound`, `git`, `linkSession`) | 817-1068 | ~250 |
| `build.go` (`binary`, `removeAll`) | 1104-1125 | ~25 |
| `main.go` keeps: `main`, `run`, `serveMock`, `watchLifetime`, `fixture`, `waitReady`, `serverOutput` | | ~520 |

Moves with a package split? The file moves do not. `run` is the real size problem, and its fix is
a code split, not a file move: pull the flag loop into `parseArgs(args) (opts, error)` (pure, easy
to cover) and the binary build into one function. That is a behaviour-preserving refactor, but not
a pure move, so it needs the existing tests (`TestScenarios_*`, `TestConfigFlag_*`,
`TestStart_*`) as the safety net, and should be its own ticket. The `server.*` references in
`main.go` are only 10 and are type names from the `server` package; they move with whatever
package the API types land in.

### `testutil/gxfixture/main_test.go` (1698 lines, 68 funcs, type `running` with 58 methods)

This is the file the baseline flagged for the test-only 58-method type. It mixes the harness
(`running`, `start`, `launch`, `kill`, `stop`, `gx`, `webClient`, `get`; 130-290), then 60 tests,
each followed by its own `running` helper methods (`waitBlockedSession`, `web`, `post`, `detail`,
`waitPermission`, `issueFiles`, `roundTypes`, `currentStep`, `waitChat`, `waitStep`, `waitIdle`,
`openDesign`, `promptWhenIdle`, `checksOf`, `designSessions`, `launchModels`...). Other scenario
families already have their own `*_test.go` (`codexscenarios_test.go`, `stallscenarios_test.go`,
`limits_test.go`, ...), so the convention exists.

| Cut | Lines | Size |
|---|---|--:|
| `harness_test.go` (`TestMain`, `running`, `start*`, `launch`, `kill`, `stop`, `gx`, `ticketStatus`, `waitStatus`, `webClient`, `get`, `web`, `post`, `try`) | 34-67, 130-288, 650-710 | ~270 |
| `env_test.go` (ShellEnv, isolatedEnv, watchLifetime, never-touches-home, flags) | 446-529, 1309-1430, 1688-1698 | ~215 |
| `design_test.go` | 784-860, 1114-1262, 1641-1687 | ~280 |
| `paused_test.go` (paused, dirty trunk, round types) | 887-1037 | ~150 |
| `agents_test.go` (agents, queue-agents, failures, checks, `checksOf`, `kindOf`) | 1431-1585 | ~155 |
| `ready_test.go` (finishing, refused ask, blocked menu, rate limited, usage gated; `waitBlockedSession`) | 530-649 | ~120 |
| `united_test.go` (united scenarios, many done, grilling stats, `cardsOf`) | 60-129, 711-783, 796-846, 1263-1308 | ~230 |
| `main_test.go` stays: smoke (`TestScenarios_StartAnswerAndStop`, web login tests) | | ~280 |

Moves with a package split? No for the file cut. The `running` type stays one type across files in
the package; the 58-method count is by type, so moving methods between files does not change it.
If the guard counts test methods, `running` needs either an exception or a real type split (a
`web` sub-struct already exists at 650 and could take `post`, `try`, `get`, `webClient`; that
changes call sites in the tests but not the fixture).

### `testutil/gxfixture/scenarios.go` (987 lines, near the limit)

The scenario table is one `map[string]scenario` literal (200-726, 527 lines) plus helpers at the
end. 13 lines from the limit. Other scenario families already have their own source file
(`readyscenarios.go`, `stallscenarios.go`, `commandsscenarios.go`, `limitburn.go`, `migrate.go`),
so the same convention fits: `designscenarios.go` for the design and charting entries (727-838),
`websetup` entries stay. Cutting the map literal itself cannot be done by moving: a Go map literal
cannot span files. The repo's way is a constructor in the family file that the map calls (`"design-two-live": designTwoLive()` at line 487, `limitBurnLive()` in `limitburn.go`, `agentsFailuresScenario()` in `sessions.go`). So move the inline entries into such constructors in a new file and leave one call per key. Moves with a package split? Yes in one respect. `servertest` wraps `server.Config`, `server.Event`
and friends. When `server` splits, `Harness`, `Start*` and `startWith` change with it (which
sub-package's `Config` they build is the open design question for the server research). The file
cut itself does not conflict: `tickets.go`, `wait.go`, `projects.go`, `notify.go` do not touch the
server types and survive any server split unchanged; `Turn` and `startWith` are the parts that
will be edited. Doing the file cut first makes the later server split diff smaller and touches
no caller. `startWith` is 170 lines in one function: extract it only in the server-split ticket.

## 4. `apiclient/client.go` (1016 lines)

One type, `Client`, 112 methods, 227 references to `server.` (types of requests and results).
Almost every method is a 4 to 8 line wrapper around `post` or `get`. Groups by endpoint, in the
current order: core and handshake (26-117), web login (118-182), snapshot and events (183-239),
projects and queries (240-367), queue and ticket writes (368-547), sessions (548-776), settings,
config and health (777-927), `post`, `Follow`, `get` (929-1016).

| Cut | Lines | Size |
|---|---|--:|
| `web.go` | 118-182 | ~65 |
| `events.go` (`Snapshot`, `Events*`, `Follow`, `followOnce`) | 183-239, 960-999 | ~100 |
| `reads.go` (projects, locks, budget, usage, history, explain, detail, epics, stats, iterations, queue lists) | 240-367 | ~130 |
| `tickets.go` (queue and ticket writes, `queueWrite*`) | 368-547 | ~180 |
| `sessions.go` | 548-776 | ~230 |
| `settings.go` (AFK, vi mode, budget, limits, config, restart, disk, checks, harness, health, repair) | 777-927 | ~150 |
| `client.go` keeps: type, constructors, `Negotiate`, `post`, `get` | | ~160 |

Moves with a package split? Yes, and this is the dependency to flag. Every method signature uses
`server.*` types, so when `server` splits, `apiclient`'s imports and most signatures change. The
file split above is independent: it only moves whole methods, so it lands first and each new file
is edited once when `server` splits (the `sessions.go` and `tickets.go` ones are the ones that
change most). `Client` has 112 methods, over the 50-method guard, and a file split does not fix the
count. The correct type fix is sub-clients (`c.Sessions().Stop(...)`) matching how `server` splits
its API; that changes about 100 call sites across `cmd` and `ui`, so it must be planned together
with the server split, not before. A files-only cut is the safe first step, and it also puts the
exception reason ("client mirrors the server API") in one place.

## 5. Files at 900 to 1000 lines, in scope

Only one, from the recount (all 15 in the 800 to 1000 band are listed in
`split-size-baseline.md`; the others are in server, cmd, ralphloop, nativerunner, skills, ui/tree,
ui/prs, recovery, tickets, ui/diffview and out of this note except as follows):

- `testutil/gxfixture/scenarios.go` 987: 13 lines of headroom. Cut with section 2.
- `ui/diffview/model.go` 812 with 47 methods on `Model`: not in the 900 band, but 3 methods from
  the 50-method limit. It is the one source file here that grows into a type split.
- `ui/app/server_stream_test.go` 789 and `ui/tree/model_test.go` 878, `ui/prs/model_test.go` 870,
  `skills/bundle_test.go` 892, `recovery/catalog_test.go` 863 are tests that are close but follow
  the topic recipe in section 1 when they cross; nothing needs doing now.

## 6. Order that avoids rework

1. Test files (section 1): independent of everything. Land them whenever; any order.
2. `gxfixture`, `servertest` file cuts: independent of the server split, small diffs. Do before it,
   so the server-split diff touches few files.
3. `apiclient` file cut before the server split; the sub-client type fix with it.
4. `run()` in `gxfixture/main.go`: own ticket, not a move.

## Method

- Recount: `git ls-files '*.go' | xargs wc -l | sort -rn`.
- Function positions: `rg -n '^(func |type )' <file>` for each file; groups read from names and
  from the neighbouring existing files in the package.
- Method counts for `Client` (112) and `running` (58) are from `split-size-baseline.md` section 3.
- The ranges and sizes are not checked by compiling the cut; line ranges overlap by a few lines
  at group borders (comments above functions).

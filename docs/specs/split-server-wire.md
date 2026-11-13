## Problem Statement

The sizeguard test `TestSizeLimits` fails on `server/wire`. The package has 4,505 source lines and the limit is 4,500. The failure blocks the v0.49.6 release. The tier picker commit added the last lines.

The failure is not a one-off. Fifteen other packages sit at 94% to 100% of a limit, so the next small change in any of them breaks the same test. Limits are package totals, so splitting a file inside a package cannot lower them. Code has to move to other packages.

## Solution

Move code out of 16 packages into new sub-packages. Nothing else changes. No limit is raised, no exception is added, and no behavior changes.

When the epic is done, every package in scope is at or below 90% of each limit: at most 45 files, 4,050 source lines, 4,500 test lines and 162 Test funcs. The `server/wire` work goes first, so `TestSizeLimits` passes and v0.49.6 can ship after it merges. The web UI is untouched, because `make web-types` still generates every moved wire type and `generated/server` still exports it.

Packages in scope, measured on the design branch (sizeguard counting: tracked `.go` files directly in the package directory, all lines):

| Package | Files | Source | Test | Test funcs |
|---|---|---|---|---|
| `server/wire` | 49 | 4,505 | 210 | 3 |
| `tickets` | 50 | 4,500 | 3,596 | 168 |
| `server/sessions/prompt` | 50 | 4,040 | 4,923 | 177 |
| `server/sessions` | 50 | 3,383 | 4,999 | 168 |
| `server/epics` | 50 | 3,206 | 4,999 | 138 |
| `server` | 50 | 3,077 | 3,281 | n/a |
| `cmd/serverapi` | 50 | 3,956 | 3,620 | 146 |
| `server/servertest` | 42 | 4,493 | 797 | n/a |
| `ui/status` | 45 | 2,609 | 4,991 | 163 |
| `ui/tickets/queue` | 45 | 2,622 | 4,968 | 161 |
| `server/sched` | 49 | 3,133 | 4,307 | n/a |
| `server/merge` | 49 | 3,227 | 4,549 | n/a |
| `nativerunner` | 49 | 3,860 | 4,619 | n/a |
| `testutil/gxfixture` | 34 | 4,338 | 1,193 | n/a |
| `ralphloop/supervise` | 35 | 2,465 | 4,768 | n/a |
| `ui` | 47 | 1,844 | 1,295 | n/a |

Test funcs are listed only for packages near that limit. The numbers were re-measured on the design branch and match the map. The tier picker commit is already on this branch.

## User Stories

1. As a release manager, I want `TestSizeLimits` to pass on trunk, so that v0.49.6 can ship.
2. As a maintainer, I want `server/wire` to end at or below 90% of every limit (at most 45 files and 4,050 source lines), so that the tier release and later changes fit without another split.
3. As a maintainer, I want no limit raised and no exception added, so that the guardrail keeps its meaning.
4. As a web developer, I want every wire type that moves to still come out of `make web-types`, so that the web UI keeps compiling without an edit.
5. As a web developer, I want `generated/server` to keep exporting each moved type, so that no import in `webui/src` changes.
6. As a maintainer, I want `TestTygoCoversWireTypes` to check every tygo package under `server/wire`, so that a type in a new sub-package cannot drop out of the generated TypeScript without a failing test.
7. As a reviewer, I want a script that compares the generated TypeScript on a base commit with the working tree, so that I can see the output is unchanged apart from the file split.
8. As an implementer, I want `.ai/index.md` to describe the real tygo layout and the new script, so that I do not follow the stale text about `wire.ts`.
9. As a maintainer, I want all 16 packages in scope at or below 90% of each limit, so that no package is one commit away from failing again.
10. As an implementer, I want one rule for the shape of a new sub-package, so that I do not redesign it in each ticket.
11. As an implementer, I want test-only sub-packages named `<area>flows`, so that I can find them and know what they hold.
12. As an implementer, I want each new package whose tests import `servertest` to have a `TestMain` with both process hooks, and each package moved from a parent with its own `TestMain` to copy it, so that a test binary never re-runs itself and fork-storms the machine.
13. As a maintainer, I want the work to be moves only, so that I can review each ticket by checking that code is identical before and after.
14. As an implementer, I want one clear rule for the few changes that are not moves, so that I know when exporting a name or rewriting a test is allowed.
15. As a reviewer, I want each ticket's Result to list every non-move change, so that I can check each one against the rule.
16. As a release manager, I want the `server/wire` ticket first, so that the failing test is fixed before any other work starts.
17. As an implementer, I want the three large renames (`server/wire`, `servertest`, `tickets`) to run one after the other, so that their merge conflicts never stack.
18. As an implementer, I want the test moves that use servertest fixtures to wait for the `servertest` ticket, so that new test packages use `rigs.X` from the start.
19. As a maintainer, I want one exported copy of the design fixture, so that new test packages do not add another private copy (five already exist: `server/sessions`, `fixagent`, `designs`, `prompt` and `promptflows`).
20. As a maintainer, I want the two dead test files removed, so that no unused file keeps a package over its file count.
21. As a maintainer, I want the merge tests that need shared helpers to import them from one place, so that the five shared helpers are not copied. The one allowed copy is the small `schedFixture` wrapper and its two constructors (about 25 lines) in each of the two test packages.
22. As an implementer, I want each ticket to end with a build, capped package tests and the sizeguard check, so that a ticket cannot merge broken or over a limit.
23. As a developer, I want tests of the moved packages to run only under `scripts/proc-cap`, so that a missing hook cannot crash my laptop.
24. As a maintainer, I want the path-keyed guard keys (`sleepguard`, `oldwords`, `envguard`) renamed with the moved files, so that those guard tests keep passing.
25. As a maintainer, I want each new package listed in `.ai/index.md`, so that agents and people find where the code went.
26. As a developer with a branch in flight, I want the move tickets to start from trunk and merge through the normal queue, so that the epic needs no special process and I resolve conflicts the usual way.
27. As a maintainer, I want `ui/app`, `ui/agents`, `git` and `ui/worktrees` left alone, so that the epic stays limited to packages at 94% or more.

## Implementation Decisions

General rules for all 16 packages.

- Shape. Source moves go into one sub-package per domain. The one exception is `server/wire/ops`, a catch-all for wire types that hold only data, because six small packages would each need a tygo entry and a generated file.
- Test-only sub-packages hold external test files (`package X_test`) and are named `<area>flows`. A move goes into the parent's existing test-only package when one exists (`launchtest`, `rigs`, `askflows`, `promptflows`).
- A sub-package may hold one file. The code in it must be cohesive, and a sizeguard measure must need the move.
- Source sub-package names are one lowercase word for what the code is. Repeat the parent's name only when the bare word is unclear, as in `ticketlog`. Never `util`, `misc`, `common` or `helpers`.
- A new package whose tests import `servertest` gets a `TestMain` that calls `servertest.RunSessionProcesses()` and `herdrfake.RunHelperProcess()`, as `testmainguard` enforces. A package moved from a parent that has its own `TestMain` copies that `TestMain`: `cmd/serverapi/oneoff` calls `cmdtest.Main`, and `supervise/startprompt` copies the `supervise` setup (the `herdrfake` hook, `fakes.StateDir` and goleak). Internal-test packages (`tickets/oneoff`, `proctable`) never import `servertest`, because that would be an import cycle. Any other package, including `ops`, `catalog`, `mergetest`, `ui/badge`, `queue/rows` and `statusflows` unless its tests import `servertest`, gets no `TestMain`.
- Non-move work rule. A change that is not a pure move is allowed only when no file can move as it is, and the production logic stays identical. It covers exporting names, exporting shared test helpers and rewriting tests against an exported API. Renames, refactors and cutting duplicate tests are not allowed. Each ticket lists every such change in its Result.
- No duplicate tests are cut anywhere. The research matched them by name only, and the epic is moves only.
- Each move ticket also renames the moved files' keys in every path-keyed guard (`sleepguard/sleepguard_test.go`, `oldwords/oldwords_test.go`, `envguard/envguard_test.go`) and lists each new package in `.ai/index.md`.
- Each move ticket ends with `go build ./...`, the package's tests run under `scripts/proc-cap`, the sizeguard check for its package, and `go test ./oldwords/ ./envguard/ ./sleepguard/ -count=1`. Gx tests are never run uncapped.
- Branches in flight get no special handling. Each ticket starts from trunk and merges through the normal queue. The merge step resolves conflicts with `gx-resolving-merge-conflicts`.

Ordering. There is one ticket per package, 16 in all.

- `server/wire` first, then `server/servertest`, then `tickets`. Each is blocked by the one before it.
- All 15 other move tickets are also blocked by the `server/wire` ticket, because every ticket ends with the sizeguard test and that test fails on trunk until the wire move merges.
- The test moves of `server`, `server/sessions`, `server/sessions/prompt`, `server/epics` and `server/merge` are blocked by the `servertest` ticket.
- The other tickets (`cmd/serverapi`, `nativerunner`, `testutil/gxfixture`, `ui`, `ui/status`, `ui/tickets/queue`, `ralphloop/supervise`, `server/sched`) are unordered among themselves, and run in parallel after the wire ticket merges.

`server/wire` and the TypeScript types.

- `server/wire/ops` takes the schedules, cleanup, settings page, restart, web info and design drafts types (6 files, 557 source lines). Nothing left in `server/wire` uses them, and they use nothing from it, so the new package has no import cycle. Result: 43 files and 3,948 source lines (88%). Every `wire.X` use of a moved type changes to `ops.X`, in about 60 Go files (33 non-test, about 614 uses).
- The moved types use only `time.Time` and `json.RawMessage` from outside their own package. The new tygo entry therefore carries the `time.Time` and `json.RawMessage` mappings and `enum_style: "union"`. The map said only `time.Time` is needed. That was wrong, because the settings page types hold `json.RawMessage`.
- tygo writes each package to its own file and never merges two packages. The new entry writes `webui/src/generated/ops.ts`. The `server/wire` entry gets `frontmatter: export * from './ops';`, so `generated/server` still exports every moved type. `server/wire` re-exports `ops`, not the other way round. No cross-package `type_mappings` are needed.
- The new entry lists the moved files in `include_files`, except the web info file. Its types are in the `notGenerated` list, and listing the file would add them to the TypeScript. The moved files leave the `server/wire` list.
- `TestTygoCoversWireTypes` reads every tygo package whose path is `server/wire` or under it, and scans each package's own directory. One shared `notGenerated` list stays.
- `TestGeneratedTypesAreUnique` reads the output file of every tygo package under `server/wire` and checks that names are unique across all of them. Without it, a name declared in both `ops.ts` and `server.ts` is hidden by `export *` with no error.
- A new script, `scripts/webtypes-diff.sh <base-sha>`, regenerates the types on the base commit and on the working tree. It compares the declarations of all generated files as one set. The only allowed differences are the re-export line and declaration order. It replaces the script deleted in an earlier consolidation. The wire ticket adds it and runs it.
- The wire ticket also fixes the stale text in `.ai/index.md` about `wire.ts` and `scripts/webtypes-diff.sh`.

`server/servertest`.

- `designs`, `grilling`, `advancing`, `triagesession` and `promptsession` (561 lines) move into the existing `rigs`. Result: 37 files and 3,932 source lines (87%).
- About 223 uses of 27 symbols in 46 files change from `servertest.X` to `rigs.X`. `servertest` cannot forward the names, because `rigs` imports it.
- The moved design fixture is the one exported copy of the design fixture. The five private copies (in `server/sessions`, `fixagent`, `designs`, `prompt` and `promptflows`) move to it only when a ticket already touches that file.

`tickets`.

- `log.go` and its test merge into the existing `ticketlog` (about 377 uses in about 95 files change to `ticketlog.X`). The one-off and slug code, with their tests, move into a new `tickets/oneoff`. Result: 44 files, 3,973 source lines, 3,245 test lines, 153 Test funcs.

`server`, `server/sessions`, `server/sessions/prompt`. Test files only. Source stays, because it is a tangled core.

- `server`: a new `server/serverflows` takes 8 test files (the four dev-mode files, the heartbeat, runner options, unavailable and parallel tests). Result: 42 files, 3,077 source lines, 2,068 test lines, 58 Test funcs.
- `server/sessions`: a new `server/sessions/convflows` takes the conversations trio and the handoff test. The permission and screens trio goes into the existing `askflows`. Result: 42 files, 3,383 source lines, 4,015 test lines, 141 Test funcs.
- `server/sessions/prompt`: the auto-queue tests (4 files), auto-advance tests (3 files) and the triage-advance test go into the existing `promptflows`. Result: 42 files, 4,040 source lines, 3,773 test lines, 138 Test funcs.
- `prompt` source is 10 lines under the 4,050 target and cannot move without splitting `Prompt` into exported seams. Any change that adds 10 source lines to `prompt` breaks the target.
- The dead test files `designsfixture_test.go` in `server` and `sessionadvancing_helpers_test.go` in `server/sessions` have no callers and are deleted.

`server/epics`, `server/sched`, `server/merge`. Test files only, no source moves.

- `epics`: the six epic-detail test files (epic detail, map detail, launch step, joined card, workspaces, follow-up count) move into a new `server/epics/epicflows`. The 5 helpers it needs from the shared helper file get an alias file there. Result: 44 files, 3,669 test lines, 104 Test funcs.
- `sched`: five holder and reattach test files move into the existing `launchtest`, with about 10 alias lines added to its alias file. Result: 44 files, 3,854 test lines, 94 Test funcs.
- `merge`: seven end-to-end test files (epic merge, merge fork, merge ticket, merge finish, friction stamp, worktree removal, worktree removal live) move into a new `server/merge/mergeflows`.
- The merge helpers are shared through a new non-test package, `server/merge/mergetest`. This differs from the map's ticket for the merge move, which listed seven helpers plus the wrapper and said nothing is copied. The person approved the change during the review: five helpers are exported once, and the small `schedFixture` wrapper with its two constructors (`newSchedFixture` and `retroFixture`, about 25 lines) is the one allowed copy, one per test package. This was probed in a scratch copy of the design branch. Only five helpers cross the boundary: pausing the root, asserting the root is queued, reading the epic ticket text, asserting the completed and merged stamps, and finishing a ticket without a commit. They use only exported names of `servertest`, `wire` and `testutil`. `mergetest` exports a `Fixture` type that embeds `*servertest.SchedFixture` and carries the five helpers as exported methods. Each of `merge_test` and `mergeflows` keeps its own `schedFixture` wrapper, which embeds `*mergetest.Fixture` and keeps its local methods. The helpers' call sites change to the exported method names. No production file in `server/merge` changes. In the probe, `go vet` passed and both test packages passed under `scripts/proc-cap`. Result: `server/merge` 42 files, 2,691 test lines, 79 Test funcs; `mergeflows` 9 files and about 1,880 test lines.

`cmd/serverapi`, `nativerunner`, `testutil/gxfixture`.

- `cmd/serverapi`: the one-off verb (3 source files and 3 test files) moves to `cmd/serverapi/oneoff`. Its constructor takes `cmdbase.Env`, and `Commands` calls it with `d.Env`. `DecideOneOff` in `cmd/evolve` changes its import. Result: 44 files, 3,527 source lines, 3,041 test lines, 119 Test funcs.
- `nativerunner`: the process table and exit watcher code (with tests) move to a new `nativerunner/proctable`. It may export the process-table value, `ExitWatcher` with `watch`, and `exitPollInterval`, with no logic change. `pidstatus.go` is removed. Its 29 uses in 6 files become `harness.X`. `TestClaudeSessionsDirIn` and `TestParsePidStatus` move to `harness`, and `TestParsePidStatus` is rewritten from `nativerunner.X` to `harness.X` (a non-move change listed in the Result), so no test is cut. Result: 43 files, 3,634 source lines, 4,479 test lines, 129 Test funcs. This leaves 21 test lines of room. `events`, `launch`, `usage` and `version` do not move.
- `testutil/gxfixture`: the scenario map moves to a new `testutil/gxfixture/catalog`, with the six scenario values from the cache gate, launch state and images scenario files, plus `chatEpicDone` and `oldShapeStatus`. The `fixture` methods stay in `main`. Result: 34 files and about 3,500 source lines (78%). Data only moves.
- `cmd/serverapi` web and cleanup verbs stay.

`ui`, `ui/status`, `ui/tickets/queue`, `ralphloop/supervise`.

- `ui`: `badge.go`, `buttons.go` and their two tests move to a new `ui/badge`. It imports `ui` for the `Color*` vars, so there is no cycle. 11 `ui.X` uses in 7 files change. Result: 43 files, 1,685 source lines, 1,192 test lines, 70 Test funcs. `harness_label.go`, `editor_args.go` and `scrollbar.go` stay.
- `ui/status`: `e2e_test.go` moves to a new test-only `ui/status/statusflows`. Source stays, because every file is a `Model` method set. Result: 44 files, 4,201 test lines, 142 Test funcs.
- `ui/tickets/queue`: `rows.go` and `rows_view.go` move to a new `ui/tickets/queue/rows`. The `Rows` tests (about 520 lines, 18 funcs) and the header-only tests (about 130 lines, 5 funcs) are rewritten as tests of `rows.Rows` and of `queue/header`, against their exported API, with epics built in code. This is a test rewrite only. Result: about 43 files, 1,921 source lines, 4,318 test lines, 138 Test funcs. `method_count_test.go` is not touched.
- `ralphloop/supervise`: `startandprompt.go` and its four test files move to a new `ralphloop/supervise/startprompt`, with `StallNudgeText`. 33 `supervise.X` uses in 8 files change. Result: 30 files, 2,195 source lines, 3,986 test lines, 91 Test funcs. `gate.go` stays. No wire types are involved, so `tygo.yaml` is not touched.

Choices the implementer makes: the names of moved test files in alias files, the names of the exported methods and helpers where the rules above allow exporting, which four dev-mode test files in `server` move, and the order of the move steps inside a ticket. Each choice must keep the production logic identical.

## Testing Decisions

- A good test here checks external behavior. This epic adds no behavior, so the main proof is that the existing tests still pass from their new place and that the size measures fall.
- The seams are the repo's existing guards. No new seam is added.
  - `TestSizeLimits` in `sizeguard`, which proves the limits.
  - `TestTygoCoversWireTypes` in `server/wire`, extended to every tygo package under `server/wire`. This is the one test change that adds coverage.
  - `scripts/webtypes-diff.sh <base-sha>`, which proves the generated TypeScript is unchanged.
  - The `testmainguard` scan and the sleep guard test in `sleepguard`, which catch a missing `TestMain` hook and a stale file key.
  - The moved tests themselves, run under `scripts/proc-cap`.
- Prior art for test-only packages: `banners`, `askflows`, `promptflows`, `launchtest` and `servertest/rigs`.
- Moved tests are not edited, except for imports, alias lines, exported method names and the `queue/rows` rewrite, and each is listed in its ticket's Result.
- The 90% target is not enforced by `TestSizeLimits`, which fails only at 100%. The verify step measures each package with sizeguard's counting (tracked `.go` files directly in the directory, all lines). For Test funcs it may use `rg -c '^func Test'` on the test files, which counts a superset of sizeguard's count, so a pass is safe.

## Verification

All checks run in the repo root of the finished epic branch. "Capped tests" means `go test -exec "$PWD/scripts/proc-cap" -count=1 <packages>`. The epic has no user-visible behavior, so no flow-catalog flow applies and no new flow or scenario needs writing.

1. `go test ./sizeguard/ -run TestSizeLimits -count=1` exits 0 on trunk after the `server/wire` ticket merges. `go build ./...` exits 0.
2. Measure `server/wire`: at most 45 files and 4,050 source lines (expected 43 and 3,948). `TestSizeLimits` exits 0.
3. `rg -n 'exceptions = map' sizeguard/sizeguard_test.go` shows an empty map. `git diff <base-sha> -- sizeguard/sizeguard_test.go` shows no change to the limit constants.
4. `make web-types` followed by `git status --short webui/src/generated` shows the changes expected from the file split only (`ops.ts` added, `server.ts` re-exporting it). `gx brief tsc` exits 0.
5. `rg -n "generated/server" webui/src` shows the same import lines as on the base commit (`git diff <base-sha> --stat -- webui/src` lists no file outside `generated/`). `gx brief tsc` exits 0.
6. `go test ./server/wire/ -run 'TestTygoCoversWireTypes|TestGeneratedTypesAreUnique' -count=1` exits 0. `TestGeneratedTypesAreUnique` covers `ops.ts` and `server.ts` together. To prove coverage, move one exported type into an unlisted file under `server/wire/ops` in a throwaway edit and confirm the test fails. Undo the edit.
7. `scripts/webtypes-diff.sh <base-sha>` exits 0 and reports only the re-export line and declaration order as differences. The base is the commit before the `server/wire` ticket.
8. `rg -n 'wire.ts|webtypes-diff' .ai/index.md` shows text that matches the real layout (`ops.ts`, the script's `<base-sha>` usage). No sentence claims `wire.ts` is generated.
9. For each of the 16 packages, measure files, source lines, test lines and Test funcs. Each is at or below 45, 4,050, 4,500 and 162. `go test ./sizeguard/ -count=1` exits 0.
10. `fd -t d . server/wire server/merge ui testutil/gxfixture tickets nativerunner cmd/serverapi ralphloop/supervise` lists each new source package with the name decided in this spec (`ops`, `mergetest`, `badge`, `catalog`, `oneoff`, `proctable`, `rows`, `startprompt`). `fd -t d '^(util|misc|common|helpers)$'` finds none of them.
11. `fd -t d 'flows$'` lists `serverflows`, `convflows`, `epicflows`, `mergeflows` and `statusflows`. For each, `fd -t f -E '*_test.go' . <dir>` finds no file. The existing `askflows`, `promptflows`, `launchtest` and `rigs` take the other test moves.
12. `go test ./testmainguard/ -count=1` exits 0. `rg -n 'RunSessionProcesses|RunHelperProcess' <dir>/main_test.go` matches both calls in each new package whose tests import `servertest`. `cmd/serverapi/oneoff` calls `cmdtest.Main`. `startprompt` has the `supervise` setup (`herdrfake` hook, `fakes.StateDir`, goleak). `rg -l 'servertest' tickets/oneoff nativerunner/proctable` finds no file.
13. For each ticket, `git diff <trunk-at-ticket-start> --stat` shows renames and import changes. `git diff -M --stat` shows moved files with identical line counts. For `server/merge`, `git diff <base-sha> --stat -- server/merge ':!*_test.go'` shows only the new `mergetest` package and no change to an existing production file.
14. The non-move changes of each ticket are exported names, exported shared test helpers, or the `ui/tickets/queue` test rewrite. `git diff <base-sha> -- '*.go' ':!*_test.go'` for a move ticket shows no changed production logic line other than package clauses, import paths, qualifiers and exported names.
15. `rg -n 'func TestParsePidStatus' harness` finds one definition and `fd pidstatus nativerunner` finds nothing. Each ticket Result has a "non-move changes" list. Compare it with the diff from entry 14. Every non-move change in the diff is in the list. Renames, refactors and removed tests are absent.
16. `git log --oneline` on trunk shows the `server/wire` merge before every other move ticket's merge and before the v0.49.6 release commit. `go test ./sizeguard/ -count=1` exits 0 at the wire merge commit.
17. `gx tickets show` on the `servertest` and `tickets` move tickets shows `servertest` blocked by `server/wire` and `tickets` blocked by `servertest`. Every other move ticket is blocked by the `server/wire` ticket. Their merge commits are in that order in `git log`.
18. `git merge-base --is-ancestor <servertest-merge> <merge>` exits 0 for each of the `server`, `server/sessions`, `server/sessions/prompt`, `server/epics` and `server/merge` test-move merges. `rg -n 'servertest\.(Designs|Grilling|Advancing|TriageSession|PromptSession)' --glob '*.go' .` finds no use of a moved name.
19. `rg -n 'type DesignsFixture\b|func StartDesigns\b' --glob '*.go' .` finds exactly one of each, in `server/servertest/rigs`. `rg -n 'func designsFixture|func startDesigns' server/serverflows server/sessions/convflows server/epics/epicflows server/merge/mergeflows` finds nothing. Private copies may remain only in files no ticket touched.
20. `fd -t f 'designsfixture_test.go|sessionadvancing_helpers_test.go'` finds nothing.
21. `rg -n 'func \(f \*Fixture\) (PauseRoot|AssertStamps)' server/merge/mergetest` finds one definition each. Each of `merge_test` and `mergeflows` has its own `schedFixture` wrapper, the one allowed copy. `rg -n 'func \(f \*schedFixture\) (pauseRoot|assertStamps)' server/merge` finds none. `go vet ./server/merge/...` exits 0 and capped tests of `./server/merge/...` pass.
22. For each ticket, the final commit passes `go build ./...`, capped tests of its package and sub-packages, and `go test ./sizeguard/ -count=1`, all with exit code 0. The ticket Result records the three commands.
23. Every `go test` line in a ticket Result uses `-exec` with `scripts/proc-cap`. A `go test` run without it fails this check.
24. `go test ./sleepguard/ ./oldwords/ ./envguard/ -count=1` exits 0. `rg -n 'ui/status/e2e_test.go' sleepguard/sleepguard_test.go` finds nothing, and the moved file's new path appears instead. `rg -n 'tickets/log|gxfixture/scenarios.go' oldwords/oldwords_test.go` and `rg -n 'servertest/promptsession.go' envguard/envguard_test.go` show only the new paths.
25. For each new package directory, `rg -n '<package import path relative to the repo>' .ai/index.md` finds a line (for example `server/wire/ops`, `ui/tickets/queue/rows`, `tickets/oneoff`, `cmd/serverapi/oneoff`). The new packages are `serverflows`, `convflows`, `epicflows`, `mergeflows`, `mergetest`, `statusflows`, `ops`, `oneoff`, `proctable`, `catalog`, `badge`, `rows` and `startprompt`.
26. `git log --first-parent --oneline` shows each ticket merged through the normal queue as one merge. No ticket Result mentions a pause or a hold for another branch.
27. `git diff <base-sha> --stat -- ui/app ui/agents git ui/worktrees` is empty. `git diff <base-sha> -- sizeguard/sizeguard_test.go` shows no change to a limit.

## Out of Scope

- `ui/app`, `ui/agents`, `git` and `ui/worktrees` (88% to 91% of a limit). The scope is 94% or more.
- Raising a sizeguard limit or adding an exception.
- Renames, refactors and behavior changes beyond moving code, and cutting duplicate tests.
- Splitting the tangled cores of `server/wire` (sessions, session types, epic list, epic detail, detail, index), `server`, `server/sessions`, `server/sessions/prompt`, `server/epics`, `server/sched` and `server/merge` into exported seams.
- The optional second move groups in `server/epics` (ticket detail) and `server/servertest` (paused run, headless fake, sched fixture, merge helpers). The targets are already met without them.
- The health, disk and limits wire clusters (`healthreport`, `diskwatch`, `devstatus`, `limitburn`, `limitsstore`, `limitshistory`, `usagesummary`). `server/wire` meets its target with `ops` alone.
- Special handling for branches in flight.

## Further Notes

- The numbers in each Result above come from research that counted by name and by `rg`. They carry "about" where noted. A ticket's own sizeguard check decides whether it meets the target. Only `server/sessions/prompt` (10 source lines of room) and `nativerunner` (21 test lines of room) are tight.
- The design branch already holds the tier picker commit, so the `server/wire` measures above include the tier fields.
- Several new source sub-packages import their parent (`ui/badge` imports `ui`, `tickets/oneoff` imports `tickets`). The parent must never import the child, or the move creates a cycle. The research found no cycle in any planned move, and the `server/wire` move and the `server/merge` helper export were checked against the code.
- Source: research findings in `docs/research/split-server-wire-01` to `04`, and the map `gx:split-server-wire` with tickets 05 to 11.

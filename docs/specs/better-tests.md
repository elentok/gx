# Better tests: per-project check config, one `gx check` entry point, and a fast, stable gx suite

Source: wayfinder map `gx:better-tests`. Tags like `[05]` point to the map ticket that settled the
point. The spec has two parts. Part A is check config, `gx check`, onboarding and the skills
(tickets 02, 03, 04, 05, 06, 08, 10, 11). Part B is gx's own test suite (tickets 01, 07, 09, 11).

## Problem Statement

gx runs agents in many projects, but it only knows how to verify gx.

**The checks are hardcoded.** The `gx-verify` skill holds a fixed check table, with a special case
for gx's `make test-server`. `gx-implement` and `.ai/index.md` "Testing" hold Go-only rules. A
project with its own `make test`, `npm run lint` or `just check` has no way to tell gx. Agents
guess, or they run the gx table and miss the project's own setup.

**There are many entry points.** gx's Makefile has `test`, `test-server`, `test-race`, `test-short`,
`web-test` and `web-e2e`, plus `scripts/test-shards.sh`. Skills name some of them directly. Agents
pick different ones, pipe them through `tail`, or move them to the background and lose the result.
`gx:follow-ups/07` records agents piping and backgrounding `gx brief go-test`.

**gx's own suite is slow and flaky.** On main `c892f6e6`, 0 of 6 full runs were green, and a full
run took 6.1 to 7.0 minutes [01]. A full `./server` run takes 9 to 10 minutes unsharded and trips
flakes (`gx:follow-ups/06`). Agents call a failure "pre-existing" or "flaky" without checking, and
nothing records it, so the same flake costs time on ticket after ticket.

## Solution

**Part A.** Each project can commit a small `.gx/checks.json` that names its command for each kind
of check: format, lint, typecheck and test [03]. A kind it leaves out runs the gx default. A project
with no file keeps today's built-in checks. Agents run every verification through one command,
`gx check`, which runs the gx defaults and the check config on the changed files or the whole repo
[04]. It records each run, so `gx tickets set --iteration-status finished` can refuse a ticket that
never passed its checks [08, 10]. One shared rule tells agents how to sort a failing test into a
flake, a pre-existing failure or a branch fault [08]. gx-evolve reports checks that got slower and
tests that flaked [10].

When a project has no check config, gx offers onboarding. `gx project add` asks once in a terminal,
`gx project onboard` asks again later, and the Epics page shows a banner with a "Set up checks"
dialog [05, 06]. Both read one shared check in the server, which detects commands from the
project's `Makefile`, mise, just and `package.json` and recommends one per kind [05].

**Part B.** gx becomes the first project on the new config. `make test` runs every non-e2e step
side by side through a new `scripts/test.sh` [09]. The server tests stop changing process env and
run in parallel, so `scripts/test-shards.sh` goes [09]. Timer-bound tests wait for conditions, and
the known flakes are fixed [07]. The suite meets measured targets: `make test` (web unit tests
included) in 4 minutes, `make web-e2e` in 2 minutes, `go test ./server` in 2 minutes, and 20 clean
full runs in a row under agent load [07, 09].

## User Stories

### Check config

1. As a project owner, I want to declare each kind of check in a committed `.gx/checks.json`, so that every clone, worktree and agent runs the same commands. [03]
2. As a project owner, I want a kind I leave out of the file to run the gx default, so that I only write what differs from gx's built-in checks. [03]
3. As a project owner, I want to set a kind to `false`, so that gx never runs that kind in my project. [03]
4. As a project owner, I want a plain string command to run for both changed-files runs and full runs, with `{files}` replaced by the selected files, so that one line covers both. [03]
5. As a project owner, I want a `{"changed": ..., "full": ...}` pair where `changed` is optional, so that a project command that only works on the whole repo still lets changed-files runs use the gx default. [03, 05]
6. As a project owner whose repo can't take a gx file, I want to keep the same shape under `checks` in gx's `project.json`, so that gx still knows my commands. The repo file wins when both exist. [03]
7. As a project owner, I want the file to have a JSON schema, so that my editor flags a wrong key or kind. [03]
8. As a project owner, I want a configured command whose tool is missing to fail, while a gx default skips a missing tool as today, so that a broken setup in my own config is never silent. [03]

### The `gx check` entry point

9. As an agent, I want one command, `gx check`, that runs the gx defaults and the check config on my working changes (files changed against `HEAD` plus untracked files), so that I never pick between Makefile targets. [04]
10. As an agent about to commit, I want `gx check --staged` to check only staged files and re-stage the files the format kind rewrote, so that the commit holds the formatted files. [04]
11. As a merge agent, I want `gx check --since <ref>` to check the files changed since a ref, so that I verify exactly what a merge brings in. [04, 08]
12. As a verify agent, I want `gx check --full` to run each kind's `full` command, with file-based gx defaults and `{files}` getting every tracked file (`git ls-files`), so that a full run covers the whole repo. [04, 11]
13. As an agent, I want `--kind test,lint` to run only the named kinds, so that I can rerun one kind after a fix. [04]
14. As an agent, I want `gx brief`-style output: the summary path first, one line per kind (`pass`, `fail` or `skip` with time and reason), the log path of each failing kind, failing test names for gx-default tools, and the last 20 log lines of a failing configured command, so that I read a short result and never need `tail`. [04]
15. As an agent, I want exit codes 0 (pass), 1 (fail), 2 (gx could not run) and 3 (no applicable checks), so that a skill can act on the code alone. [04]
16. As an agent, I want the format kind to run first and lint, typecheck and test to run side by side after it, in a fixed output order, so that a check never reads files the formatter is still changing. [04]
17. As an agent, I want a kind's changed-files command to be skipped when no selected file exists, and the run to report "no applicable checks", so that an empty change is not a failure. [04]
18. As an agent in a project with no check config, I want `gx check` to run today's `gx-verify` table (shell, Python, Go and JS/TS rows, the lockfile table, the frozen install), without the `make test-server` exception, so that nothing changes for projects that never onboard. [04, 09]
19. As an agent whose `gx check` run moved to the background, I want the summary written next to the logs, so that the result survives. [04]

### Onboarding

20. As a person running `gx project add` in a terminal, I want gx to ask once for the missing check config, showing each kind's recommended command and its source, so that a new project is set up in one step. [05]
21. As that person, I want to accept all, edit the proposed JSON in `$EDITOR` (validated like `gx project edit-config`), or skip, so that I stay in control. Skip saves nothing, and the next run asks again. [05]
22. As that person, I want to choose where the answer goes: commit `.gx/checks.json` on trunk (the default) or keep it in `project.json`, so that a repo I don't own stays untouched. A `vcs: none` project only gets `project.json`. [05]
23. As a script running `gx project add` without a terminal or with `--json`, I want no prompt, only a list of what is missing and the command to run later, so that automation never hangs. [05]
24. As a person, I want `gx project onboard [project]` to run the same flow for a registered project, and print "nothing missing" with exit 0 when the project already has config, so that I can run it any time. [05]
25. As a person, I want "Keep the gx defaults" to save `{}`, so that gx stops asking and keeps its built-in checks. [05, 06]
26. As a person, I want gx to detect candidates from `Makefile` targets, `mise.toml` tasks, `justfile` recipes and `package.json` scripts (root and workspace packages), ranked in that order, without running anything and without reading CI workflows, so that the recommendation is safe and predictable. [05]
27. As a person, I want a kind with no candidate to be recommended as "gx default", and a found command to be saved as that kind's `full` command only, so that changed-files runs keep the gx default. [05]
28. As a person viewing the Epics page filtered to one project, I want a banner while that project has no check config, so that I know agents are running gx's built-in checks. [06]
29. As a person viewing "All projects", I want one banner that lists every missing project, each with its own "Set up checks" button, so that several projects don't stack banners. [06]
30. As a person, I want "Not now" to hide the banner until the next page load and save nothing, so that I can put setup off without a lasting choice. [06]
31. As a person, I want "Set up checks" to open a dialog with one row per kind: a command field, a Use command / gx default / Off switch, and a line naming where the command came from or what the gx default runs, so that I see every kind at once. [06]
32. As a person, I want found commands preset to "Use command" with a "Recommended" chip, kinds with no candidate preset to "gx default" with an empty disabled field ("No command found") and a "Nothing found" chip, and the other candidates offered as suggestions with their sources, so that the usual answer is one click. [06, 11]
33. As a person, I want a "Save to" choice (commit `.gx/checks.json` on trunk, or `project.json`) and a collapsed "Show the file" preview of the exact file, so that I know what gets written. [06]
34. As a person, I want Save to write the file, show a toast ("Committed .gx/checks.json on main") and remove the banner, so that I know it worked. [06]
35. As a person, I want a "Keep gx defaults" button in the dialog that saves `{}`, so that the web matches the CLI. [06]
36. As a person on a phone, I want the dialog rows to stack, so that the dialog fits a 400 px screen. [06]
37. As a person, I want a failed save (for example a commit on trunk that git refuses) to show the error in the dialog and save nothing, so that I never think a failed save worked.
38. As a person, I want the CLI and the web to use the same shared check in the server, with each kind's current state (configured, gx default or off), recommendation, source and other candidates, so that both show the same answer. [05]

### Skills

39. As an implement agent, I want to run one `gx check` (changed files) before commit as the only package-wide run, and `gx brief go-test -run` while iterating, so that I don't spend minutes on repeated whole-package runs. [08]
40. As a skill author, I want `gx-verify` to be a thin call to `gx check` (`staged` maps to `--staged`, `commit-all` and `working-changes` to the default) that reports the exit code, so that the check table lives in one place. [08]
41. As a merge agent, I want gx-merge to run `gx check --since <target>`, and a relaunched merge to rerun the tests that failed before it reports `finished`, so that a merge is verified against what it brings in. [08]
42. As an epic verify agent, I want to run `gx check --full` once and send failures through the shared failing-test rule, with a branch fault as a finding, so that every e2e flow runs once per epic before land. [08, 09]
43. As any agent with a failing test, I want one shared rule (`skills/gx-failing-tests.md`): rerun that test 3 times with `gx brief`; any pass is a flake; 3 failures mean run it on the base; failing the same way there is a pre-existing failure; passing there is a branch fault, so that I sort failures the same way every time. [08]
44. As an agent who found a flake or pre-existing failure, I want to file it through gx-follow-up (whose dedupe searches open follow-ups by test name) and carry on, so that a known problem never blocks unrelated work. [08]
45. As an agent, I want to fix a flake or pre-existing failure inside my ticket when the fix is 30 changed lines or fewer in at most 2 files, record it in the epic's `## Spec deviations`, and file the follow-up as fixed, so that small fixes don't wait for a new ticket. [08]
46. As an epic verify agent, I want a failing flow to be rerun at `git merge-base main HEAD` in a temporary worktree, and a flow that fails there too to read `pre-existing, <follow-up addr>`, so that the epic is not blamed for it. [08]
47. As an implement agent writing a test that waits on time or concurrency, I want a rule to run it with `-count=20` before commit, so that I catch a new flake before it lands. [07, 08]
48. As a gx maintainer, I want a repo test that fails on a new `time.Sleep` in a `_test.go` file, with an allowlist, so that fixed sleeps don't come back. [07]
49. As an agent reading `.ai/index.md` "Testing", I want the run rules to say "verify with `gx check` in the foreground, unpiped; iterate with `gx brief`; never pipe, `tail` or background either", while the gx-specific facts stay, so that the doc matches the tools. [08]
50. As a planning agent, I want gx-to-tickets test seams to name the narrow `gx brief` command as an iteration hint, not as the end-of-ticket check, so that tickets don't invent their own verification. [08]
51. As a research or investigate agent, I want gx-research and gx-investigate to run `gx check` before finishing, so that my docs-only commits pass the finish guard (exit 3 passes). [11]

### Finish guard

52. As gx, I want `gx tickets set <addr> --iteration-status finished` to refuse unless the run record has a run on the ticket's branch and current tree id, with no `--kind` filter and exit 0 or 3, so that no ticket with commits finishes unchecked. [08, 10, 11]
53. As an agent whose check fails on flakes or pre-existing failures, I want `--check-failures <follow-up addrs>`, one per failing test, to let the finish through, so that an unrelated failure is recorded and does not block me. [08]
54. As an agent, I want a commitless finish to skip the guard, so that a ticket with nothing to check can still finish. [08, 11]
55. As an agent, I want a check run before my commit to still count after the commit, because the guard matches the tree id, not HEAD, so that I don't run checks twice. [10]
56. As an agent refused by the guard, I want the message to name the `gx check` command to run and the `--check-failures` option, so that I know the next step. This is a CLI refusal, not a pause. [08]

### Run record and gx-evolve

57. As gx-evolve, I want `gx check` to append one line per run to `~/.local/share/gx/check/<project>.jsonl` with the run id, kind, duration, exit code, scope, any `--kind` filter, branch, worktree path, HEAD, tree id and failing test names, swept after 30 days, so that later reports have the data. [10]
58. As gx, I want `gx check` to pass its run id in `GX_CHECK_RUN` to child processes, and `gx brief` to add the branch, tree id, test names it ran and that id to its index line, so that a run's failing test names come from nested `gx brief` runs without output scanning. [10]
59. As a project owner whose commands don't call `gx brief`, I want the record to hold pass or fail only, so that gx never guesses test names from output. [10]
60. As a person reading gx-evolve's report, I want a "slower" row that flags a check kind when the epic verify ticket's `--full` run is at least 20% and at least 30 s slower than the median of the last 5 `--full` runs recorded before the epic's first ticket claim, so that a slowdown is caught per epic. [10]
61. As a person reading gx-evolve's report, I want a "flake" row listing tests that failed in a `gx check` run and then passed in a later `gx check` or `gx brief` run on the same tree, so that flakes are caught even when an agent did not file them. [10]
62. As gx-evolve, I want to file each row's finding through gx-follow-up, whose dedupe finds the follow-up the failing-test rule already filed, so that one problem gets one follow-up. [10]

### Part B: gx's own suite

63. As a gx maintainer, I want `make test` to call a new `scripts/test.sh` that runs the Go race packages, the other Go packages (not `./e2e`), `./server` and the web unit tests as separate processes side by side, keeps each step's output in a file, and prints one line per step with its time, then the failing steps and test names, so that one command runs everything but e2e. [09]
64. As a gx maintainer, I want `scripts/test.sh` and `make web-e2e` to call `gx brief go-test`, `gx brief vitest` and `gx brief playwright`, so that the run record gets failing test names. [10]
65. As a gx maintainer, I want `test-server`, `test-race`, `test-short` and `scripts/test-shards.sh` removed, and `test`, `web-test`, `web-e2e` and `e2e` kept, so that there are fewer entry points. [09]
66. As a gx maintainer, I want CI to drop its separate `make web-test` step and change nothing else, so that CI does not run the web unit tests twice. [09, 11]
67. As a gx maintainer, I want the test harness to stop changing process-wide env and pass it to the server and its child processes through config, and the server tests to use `t.Parallel`, so that `go test ./server` takes 2 minutes or less under agent load. [09]
68. As a gx maintainer, I want `servertest` to set short poll intervals by default and fixed sleeps to become condition waits, with no fake clock, so that timer-bound tests are fast and stable. [07]
69. As a gx maintainer, I want the store watch (FSEvents) bug fixed and the tests made independent of when a watch event arrives, so that earlier tests in the process can't break later ones. [07]
70. As a gx maintainer, I want Playwright to keep `retries: 0`, flow 24 "Phone, Design page" fixed as a layout bug, and each slow 10 s visibility wait fixed at its cause, so that a pass on retry never hides a failure. [07]
71. As a gx maintainer, I want the named flakes fixed: the StatsPage grilling-order test, the fake-agent races, the ENXIO race (wait until the fake opens its control FIFO), the `ChatLog.test.tsx` focus-mode wait, and `testutil/agentfake` using a per-test temp dir, so that the known failures stop. [07]
72. As a gx maintainer, I want the slowest tests of `testutil/gxfixture`, `cmd` and `nativerunner` (under `-race`) cut, so that the side-by-side run meets 4 minutes. [07]
73. As a gx maintainer, I want gx's own `.gx/checks.json` to set only `test.full`: `scripts/test.sh`, then `make web-e2e` after it, so that flows don't compete with Go tests for CPU. [09]
74. As a gx maintainer, I want `scripts/soak.sh [N]` (default 20) to run the `test.full` command N times, keep going after a failure, and list each failing test with the number of rounds it failed, so that I can measure the flake target. [07, 09]
75. As a gx maintainer, I want the last Part B implement ticket to be done only when `scripts/soak.sh 20` is clean under agent load and the timings meet the targets, with both recorded in its Resolution, so that the epic proves its targets before land. [11]
76. As an epic verify agent, I want to check the one `gx check --full` run's test time against the target and read that Resolution, so that verification confirms Part B without a 2-hour run. [11]

## Implementation Decisions

### Check config [03, 05]

- File: `.gx/checks.json` at the repo root, JSON, with a JSON schema. Keys are the four kinds:
  `format`, `lint`, `typecheck`, `test`. No other kinds.
- A value is a string (both scopes), `false` (off), or an object with an optional `changed` and a
  required `full`. A kind left out, or a pair without `changed` on a changed-files run, uses the gx
  default.
- `{files}` is replaced by the selected files: changed files on a changed-files run, every tracked
  file (`git ls-files`) on a `--full` run [11]. Commands run from the repo root. gx passes every
  selected file; the project's own wrapper filters them. When the expanded command would pass the
  OS argument limit, gx splits the files into batches that fit and runs the command once per batch,
  like `xargs`. The kind fails if any batch fails.
- Resolution order: the repo file, else `checks` in the ticket store's `project.json`, else the gx
  defaults. No `.local` override file.
- A project is missing check config only when it has neither the repo file nor `checks` in
  `project.json`. `{}` counts as config.
- No setup or install field. Dependencies are installed at worktree creation
  (`docs/specs/test-and-worktree-health.md`).

### `gx check` [04, 10, 11]

- A new Go module (package name is the implementer's choice) owns: config loading and validation,
  file selection, the gx defaults, running kinds, output and the run record. `cmd` holds a thin
  `gx check` command. `gx-verify`, the finish guard and `gx evolve inputs` use the module's
  interface, not the record file directly.
- Selection flags: default (changed against `HEAD` plus untracked), `--staged`, `--since <ref>`,
  `--full`, `--kind <list>`. Changed files use today's `--diff-filter=ACMR` rule.
- The gx defaults are today's `gx-verify` table moved into Go, with the `make test-server`
  exception removed. They use `gx brief` inside. On `--full`, every file-based row runs on every
  tracked file.
- Order: format first; then lint, typecheck and test side by side; output in that fixed order.
  With `--staged`, gx re-stages the files format rewrote. Otherwise it stages nothing and reports
  that format changed the working tree.
- Output: summary path first, one line per kind, failing log paths, test names from brief's parsers
  for gx defaults, the last 20 log lines for a failing configured command, and the overall result
  last. The summary is also written next to the logs. No built-in timeout.
- Exit codes: 0 pass, 1 fail, 2 could not run (for example invalid config), 3 no applicable checks.
- Run record: one JSON line per run in `~/.local/share/gx/check/<project>.jsonl`, with a 30-day
  sweep on each write, like `gx brief`'s 14-day sweep. Fields: run id, kind, duration, exit code,
  scope, `--kind` filter, branch, worktree path, HEAD, tree id, failing test names.
- Tree id: a `git write-tree` of the whole working tree (tracked and untracked, minus gitignored)
  built in a temp index, as `git/dirtyland.go` does. Only the tree id decides "same tree".
- `gx check` sets `GX_CHECK_RUN` for its children. `gx brief` adds branch, tree id, test names run
  and `GX_CHECK_RUN` (when set) to its index line. A run's failing test names are the failures of
  the brief lines with its id.
- Ticket link: a `ralph-loop/<epic>-<NN>` branch gives the ticket address. The `project.json`
  `trunk` branch marks a trunk run. Any other branch is "other". No new env vars beyond
  `GX_CHECK_RUN`.

### Finish guard [08, 10, 11]

- `gx tickets set <addr> --iteration-status finished` finds the ticket's iteration worktree from
  its address and computes its tree id. If the ticket has commits, it needs a record line with that
  branch and tree id, no `--kind` filter, and exit 0 or 3. This covers every ticket type with
  commits.
- `--check-failures <addrs>` accepts a failing run when it names one follow-up per failing test.
  When the record names no tests, at least one address is needed. Each address must be an existing
  ticket.
- A commitless finish skips the guard.
- The refusal is a non-zero CLI exit with a message. It adds no pause kind and no pause-to-verb row
  (ADR 0042).
- An agent that is refused and cannot fix the cause (a branch fault it cannot fix) reports
  `asking`, as it does today for a failing test it cannot fix. gx pauses the ticket with reason
  `self-reported`, an existing pause. gx cannot recover on its own, because only a person can decide
  what to do about a fault the agent could not fix. The ticket page's existing answer action ends
  the pause.

### Detection and onboarding [05, 06]

- Detection runs in the server on the project's trunk checkout. It reads `Makefile` targets,
  `mise.toml` tasks, `justfile` recipes and `package.json` scripts (root and workspace packages),
  matching names like `test`, `lint`, `typecheck`/`tsc`, `format`/`fmt`. It never runs anything and
  skips CI workflows. Ranking: Makefile, mise, just, package.json.
- The shared check is one server read route. Per project and kind it returns the current state
  (configured, gx default or off), the recommendation and its source, and the other candidates,
  plus whether the project is missing config. A second, write route saves an answer to the repo or
  to `project.json`. Both routes follow ADR 0028 and the "Adding a server route" checklist in
  `.ai/index.md`: `routeTable`, CLI verbs with parity, the web mux (`webWrites` for the save), and
  the `TestWeb_EverythingElseIs404` allowlist.
- Save to the repo makes one local commit of `.gx/checks.json` on trunk. gx does not push. If the
  commit fails, the route returns the error, nothing is saved, and the dialog shows the error.
  Save to `project.json` writes `checks` there. `vcs: none` projects can only save to
  `project.json`.
- A found command is saved as `{"full": "<command>"}`. "gx default" leaves a kind out. "Off" writes
  `false`. "Keep the gx defaults" saves `{}`.
- `scratch` has no repo and is never missing config.
- `gx project add`: after registering, it calls the shared check. Interactive: one prompt (accept
  all, edit in `$EDITOR`, skip), then a save-target prompt. Non-interactive or `--json`: it prints
  what is missing and the `gx project onboard` command.
- `gx project onboard [project]`: the same flow for a registered project; "nothing missing" and exit
  0 when config exists.
- Web: a banner and a "Set up checks" dialog on the Epics page, as drawn in the stored mock. The
  banner follows the page's project filter. "Not now" is page state only. The dialog's command field
  offers the other candidates as suggestions. The file preview is collapsed by default. Wire types
  are regenerated with `make web-types`.
- An iteration worktree made before the commit on trunk keeps the old config until its branch picks
  up trunk. That is expected.

### Skills and docs [07, 08, 11]

- New shared file `skills/gx-failing-tests.md`, linked from gx-implement, gx-tdd, gx-merge and
  gx-verify-epic. It replaces gx-merge's "diff touches neither the test nor its package" rule.
- gx-verify: mode mapping to `gx check` flags, report the exit code; the check table goes.
- gx-implement: one `gx check` before commit; `gx brief -run` while iterating; the `-count=20`
  rule; the "whole package once at the end", separate gofmt and `make test-server` lines go.
- gx-tdd: the `-count=20` rule and the failing-tests link.
- gx-merge: `gx check --since <target>`; a relaunched merge reruns earlier failures.
- gx-verify-epic: one `gx check --full`; flows failing at HEAD are rerun at the merge-base only on
  failure; the "do not run the whole suite; CI does that" line goes; the Part B check of story 76.
- gx-research and gx-investigate: run `gx check` before finishing.
- gx-to-tickets: test seams name the narrow `gx brief` command as an iteration hint.
- `.ai/index.md` "Testing": the run rules change as in story 49. `claudeenv.Isolate()`, socket path
  length, no live machine state and `-C webui` stay.
- gx-evolve and `gx evolve inputs`: the slower and flake rows from stories 60 and 61. `gx evolve
  inputs` reads both the check record and the brief index.
- No cross-worktree lock on heavy test runs. No known-flakes list.

### Part B [07, 09, 11]

- `scripts/test.sh` runs four steps side by side: Go race packages, other Go packages (excluding
  `./e2e`, which needs herdr), `./server`, and `make web-test`. Each step's output goes to a file.
  It prints one line per step with time, then failing steps and test names. It calls `gx brief`.
- `make test` calls `scripts/test.sh`. `make web-e2e` calls `gx brief playwright`. Removed:
  `test-server`, `test-race`, `test-short`, `scripts/test-shards.sh`.
- CI: drop the separate `make web-test` step. No other CI change.
- Harness env: `testutil`, `testutil/claudeenv`, `testutil/gitenv` and the 24 server test files that
  call `Setenv` stop changing process-wide `HOME`, `PATH`, `XDG_*`, git config and the Claude config
  dir. The values go to the server and its children through config. Server tests use `t.Parallel`.
  The `claudeenv` canary keeps working through config.
- `servertest` sets short `PollInterval`, `BudgetPollInterval` and `ChatPollInterval` by default.
  Fixed sleeps become condition waits. No fake clock.
- `server/storewatch.go`: fix the missed events after earlier tests, and make the tests write again
  until seen (like `581f848b`).
- Playwright: `retries: 0`; fix flow 24's layout; fix each slow 10 s wait at its cause; raise a
  timeout only for a named wait with a known reason.
- gx's `.gx/checks.json`: `{"test": {"full": "scripts/test.sh && make web-e2e"}}` in effect. The exact
  string is the implementer's choice, as long as e2e starts after the other steps finish.
- `scripts/soak.sh [N]`: default 20, no Makefile target, runs every round, lists failing tests with
  round counts. herdr's `make e2e` is not part of the target.
- Targets: `make test` (web unit tests included) at most 4 min; `make web-e2e` at most 2 min;
  `go test ./server` at most 2 min; 20 clean `test.full` runs in a row on the dev Mac with other
  agents running. A pass on retry is a failure. The last Part B implement ticket records the soak
  result and the timings in its Resolution.

### Implementer's choices

- The Go package name for the `gx check` module and the detection code.
- The server route paths and their `gx server` CLI verb names.
- The JSON schema's location and how editors find it.
- Field names in the run record and in the brief index additions.
- The finish-guard refusal text, as long as it names `gx check` and `--check-failures`.
- How the commit on trunk is made (the dirty-trunk handling in `git/dirtyland.go` is prior art).
- The new fixture scenario names and contents.
- How `scripts/test.sh` excludes `./e2e`.
- The `time.Sleep` allowlist format and where the repo test lives.
- The exact phrases the skill text tests match.

## Testing Decisions

- A good test drives the highest seam and checks what a person or agent sees: output, exit code, the
  saved file, the record line, the rendered page. It does not test private helpers.
- **Seam 1: the `gx check` command.** Tests run it against a `testutil.TempRepo` whose configured
  commands are small scripts, and assert output, exit code and the record line. This seam covers
  config resolution, selection flags, `{files}`, `--full` over tracked files, ordering, re-staging,
  exit codes, `GX_CHECK_RUN` and the sweep. Prior art: `cmd/brief_test.go`,
  `cmd/brief_retention_test.go`, `brief/sweep_test.go`.
- **Seam 2: `gx tickets set --iteration-status finished`.** Tests write record lines and assert the
  refusal, `--check-failures`, the commitless skip and tree-id matching. Prior art:
  `cmd/tickets_set_test.go`.
- **Seam 3: the shared check routes.** Server tests with `servertest` fixtures and a project repo
  (`SetProjectRepoNoAutoMerge`) cover detection, ranking, missing state, both save targets, `{}`,
  `false`, `vcs: none` and a refused commit. Route parity and the 404 allowlist come from the
  existing tests. CLI flow tests for `gx project add` and `gx project onboard` (interactive,
  non-interactive, `--json`, idempotent) follow `cmd/project_add_test.go` and
  `cmd/project_editconfig_test.go`.
- **Seam 4: the web page.** Vitest for the banner and dialog states, using `waitFor` for Chakra
  menus. New e2e flows in the catalog (listed under Verification).
- **Seam 5: `gx evolve inputs`.** Tests write check and brief records and assert the slower and
  flake rows. Prior art: `cmd/evolve_inputs_test.go`.
- **Skill text.** Tests in `skills/` match the new rules (failing-tests file and its links,
  gx-verify's thin call, gx-research and gx-investigate running `gx check`). Prior art:
  `skills/mock_rules_test.go`, `skills/gx_design_rules_test.go`, `skills/bundle_test.go`.
- **No-new-sleep test.** Prior art: `oldwords/oldwords_test.go`.
- **Part B.** The suite itself is the test. `scripts/soak.sh 20` and the timings decide the last
  Part B ticket. New or changed tests that wait on time pass `-count=20`.

## Verification

Fixture runs use `testutil/gxfixture` with `--env` for CLI checks. Unless an entry says otherwise,
CLI checks run in a temp repo made for the check.

New fixture scenario to write: `checks-missing`. Projects `demo` and `auto`, both with repos and no
check config. `demo`'s repo has a `Makefile` with a `test` target and a `package.json` with a
`build` script, which detection must not offer for any kind (typecheck shows "Nothing found"). Existing scenarios get `{}` check config, so no current flow shows the banner.

New flows to write in `webui/e2e/CATALOG.md` (next free numbers):

- 63 "Check config banner": `checks-missing`, filter `demo`, banner text and buttons.
- 64 "Check config banner, all projects": `checks-missing`, All projects, one banner listing `demo`
  and `auto`.
- 65 "Not now": the banner hides and the save route is never called.
- 66 "Set up checks, save to repo": rows preset as recommended, preview, Save, toast, banner gone,
  `.gx/checks.json` committed on trunk.
- 67 "Set up checks, save to project.json" and "Keep gx defaults" (`{}`).
- 68 "Set up checks, refused commit": a dirty trunk that refuses the commit shows the error and
  saves nothing.
- 69 "Phone, Set up checks": 400 px, rows stack.

Stories:

1. In a temp repo with `.gx/checks.json` setting `test`, `gx check --kind test` runs that command; a second worktree of the same repo runs it too.
2. Config with only `test`: `gx check` runs the gx default for format, lint and typecheck.
3. `{"lint": false}`: the lint line reads `skip` with reason "off".
4. `{"format": "echo {files}"}`: the changed run's log lists the changed files.
5. `{"test": {"full": "..."}}`: a changed run uses the gx default for test; `--full` runs the command.
6. Remove the repo file and set `checks` in `project.json`: `gx check` uses it. With both, the repo file wins.
7. The schema file validates the gx config and rejects an unknown kind.
8. A configured command naming a missing tool gives `fail`, exit 1; a gx default for a missing tool gives `skip`.
9. `gx check` with one changed `.go` file and one untracked `.sh` file runs Go and shell rows only.
10. `gx check --staged` after staging an unformatted `.go` file leaves the formatted file staged.
11. `gx check --since HEAD~1` checks only that commit's files.
12. `gx check --full` with `{"format": "echo {files}"}` logs every tracked file; with no config, the Go test row covers every package directory. In a temp repo with more tracked files than fit one command line, the command runs in several batches that together list every file, and a failing batch fails the kind.
13. `gx check --kind lint` prints only the lint line.
14. A failing configured command prints the summary path first, the log path and its last 20 lines; a failing gx-default Go test names the test.
15. Exit codes: pass 0, fail 1, invalid JSON config 2, no changed files 3.
16. A format command that sleeps then rewrites a file finishes before lint starts (log timestamps); output order is format, lint, typecheck, test.
17. A clean tree: exit 3, "no applicable checks".
18. In gx with no config, `gx check` on a changed `server/` file runs `gx brief go-test ./server`, not `make test-server`.
19. Start `gx check` in the background with its output discarded, wait for it to exit, then check that the summary file exists at the printed path.
20. Interactive `gx project add` of a repo with a `Makefile` `test` target lists four kinds, `test` recommended from "Makefile · test".
21. "skip" saves nothing and `gx project onboard` asks again; "edit" with invalid JSON is rejected like `gx project edit-config`.
22. Choosing the repo makes one commit of `.gx/checks.json` on trunk; choosing `project.json` writes `checks`; a `vcs: none` project offers only `project.json`.
23. `gx project add --json` prints what is missing and `gx project onboard`, with no prompt.
24. `gx project onboard demo` after saving prints "nothing missing", exit 0.
25. Keep the gx defaults: the file holds `{}` and the shared check reports nothing missing.
26. A repo with `Makefile test`, `justfile test` and a `package.json` `test` script recommends the Makefile target and lists the others; no command runs (a target that writes a file leaves no file).
27. A saved found command reads `{"test": {"full": "make test"}}`; a kind with nothing found is absent.
28. Flow 63. Mock state banner-filtered.
29. Flow 64. Mock state banner-all-projects.
30. Flow 65.
31. Flow 66. Mock state setup-dialog.
32. Flow 66 checks the presets and the suggestions list. Mock state setup-dialog (typecheck row drawn as a disabled lower-ranked candidate).
33. Flow 66 opens the preview before Save. Mock state setup-dialog.
34. Flow 66. Mock state saved.
35. Flow 67.
36. Flow 69. Mock state setup-dialog-phone.
37. Flow 68.
38. The shared check's CLI verb and the web show the same state for `demo` in `checks-missing`.
39. Skill text tests in `skills/` pass with `go test ./skills/ -timeout 60s`.
40. Same as 39 (gx-verify maps modes and holds no table).
41. Same as 39 (gx-merge runs `gx check --since <target>`).
42. Same as 39; the epic's own verify ticket follows it.
43. Same as 39 (`skills/gx-failing-tests.md` exists and is linked from the four skills).
44. Same as 39.
45. Same as 39.
46. Same as 39.
47. Same as 39.
48. Add a `time.Sleep` to a `_test.go` file outside the allowlist: the repo test fails.
49. `.ai/index.md` "Testing" no longer names `make test-server` or the 60s rule, and says to run `gx check` in the foreground, unpiped.
50. Same as 39.
51. Same as 39 (gx-research and gx-investigate run `gx check`).
52. In a fixture ticket worktree with a commit and no record line, `gx tickets set <addr> --iteration-status finished` exits non-zero; after `gx check` passes, it succeeds.
53. With a failing run, `--check-failures <addr>` per failing test succeeds; too few addresses or an unknown address is refused.
54. A ticket with no commits finishes with no record line.
55. Run `gx check`, commit, then finish: accepted.
56. The refusal text names `gx check` and `--check-failures`; the ticket's status is unchanged and no pause appears. In a fixture, an agent that reports `asking` after a refusal leaves the ticket paused with reason `self-reported`, and the ticket page's answer action reopens it.
57. After a run, the project's `.jsonl` has one line with every field; a line older than 30 days is gone after the next write.
58. A configured command that calls `gx brief go-test` on a failing test: the record names the test, and the brief index line carries the run id.
59. A configured command without `gx brief`: the record has exit 1 and no test names.
60. `gx evolve inputs` with 5 earlier `--full` lines and a verify-ticket line 40% and 60 s slower shows the slower row; 10% slower shows none.
61. A test failing in a `gx check` line and passing in a later `gx brief` line on the same tree shows in the flake row.
62. Same as 39 (gx-evolve files through gx-follow-up).
63. `make test` in gx prints one line per step with times and exits 0.
64. After a `make test` with one forced Go failure, the record names the failing test.
65. `make test-server`, `make test-race` and `make test-short` fail as unknown targets; `scripts/test-shards.sh` is gone.
66. `git diff main -- .github/workflows/ci.yml` shows only the removed `make web-test` step.
67. `go test ./server` finishes in 2 minutes or less, and `rg Setenv server testutil` shows no process env changes in the harness.
68. Covered by 75 (the soak and timings).
69. Covered by 75.
70. Flow 24 passes; `webui/playwright.config.ts` has `retries: 0`.
71. Covered by 75.
72. Covered by 63 and 75.
73. gx's `.gx/checks.json` sets only `test.full`, and `gx check --full` starts e2e after `scripts/test.sh` ends.
74. `scripts/soak.sh 2` with one forced failing test runs both rounds and lists that test with count 2.
75. The last Part B ticket's Resolution records a clean `scripts/soak.sh 20` and timings at or under 4 min (`make test`), 2 min (`make web-e2e`) and 2 min (`go test ./server`).
76. The verify ticket's `gx check --full` test kind takes 6 minutes or less (4 plus 2), and it cites story 75's Resolution.

**Mock states**

- `$(gx tickets root)/better-tests/mocks/06-onboarding-web-view.html` · banner-filtered · 1100 px ·
  `checks-missing` filtered to `demo` · top-bar, filter-row, missing-banner, not-now, set-up,
  epic-list
- `$(gx tickets root)/better-tests/mocks/06-onboarding-web-view.html` · banner-all-projects ·
  1100 px · `checks-missing` on All projects · top-bar, filter-row, missing-banner, set-up, not-now,
  epic-list
- `$(gx tickets root)/better-tests/mocks/06-onboarding-web-view.html` · setup-dialog · 1100 px ·
  `checks-missing`, "Set up checks" on `demo`, preview open · top-bar, missing-banner, setup-dialog,
  kind-rows, save-target, file-preview, actions
  One change from the mock, on both setup-dialog states: the typecheck row has an empty disabled
  field ("No command found") like the format and lint rows, not `pnpm -C webui build`.
- `$(gx tickets root)/better-tests/mocks/06-onboarding-web-view.html` · setup-dialog-phone ·
  400 px · `checks-missing`, "Set up checks" on `demo` · setup-dialog, kind-rows, save-target,
  file-preview, actions
- `$(gx tickets root)/better-tests/mocks/06-onboarding-web-view.html` · saved · 1100 px ·
  `checks-missing` after Save to the repo · top-bar, filter-row, epic-list, toast

## Out of Scope

- The server running checks itself, before land or as an epic-verify gate. Land runs no tests on
  purpose, and a server gate would add a new pause kind.
- CI changes beyond dropping the duplicate `make web-test` step. CI does not call `gx check`; it may
  later.
- Onboarding for project data other than check config.
- Onboarding in the TUI.
- Reading mise, Taskfile or lefthook formats as the config itself. Detection only reads them for
  recommendations [02].
- Per-check fields (`glob`, `group`, `dir`, `timeout`, `parallel`) and a `.local` override file [03].
- herdr's `make e2e` in the 20-clean-runs target [09].
- A cross-worktree lock on heavy test runs, and a known-flakes list [07, 08].

## Further Notes

- The decisions behind this spec are the closed tickets `gx:better-tests/01` to `11`. Research:
  `docs/research/test-suite-measurement.md` on branch `research/measure-test-suite`, and
  `docs/research/how-other-tools-declare-checks.md` on branch
  `research/how-other-tools-declare-checks`.
- Absorbed tickets, all closed with a pointer here [07]: test fixes `gx:follow-ups/06`, `/07`, `/11`,
  `/17`, `/18`, `/21` (now `gx:follow-ups-archived/48` to `/53`), `gx:test-suite-performance/01`,
  `gx:flaky-session-test/01`, `gx:test-logging/01`; which tests agents run
  `gx:implement-verify-skills/01`, `/06`, `/08`, `gx:brief-tooling/05`, `/06`; flake handling
  `gx:implement-verify-skills/02`, `/04`, `gx:tickets-authoring/07`, `gx:agent-guidance/05`,
  `gx:loop-health/01`.
- Glossary terms already added: check, gx defaults, check config, onboarding, flake, pre-existing
  failure, branch fault.
- Prior work: `docs/specs/test-and-worktree-health.md` (dependency install at worktree creation,
  hermetic Go tests).
- The mock wins on look and later decisions win on behaviour. The typecheck row in the setup-dialog
  state is a lower-ranked candidate shown while "gx default" is selected [11].

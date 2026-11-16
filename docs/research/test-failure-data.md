# What failure data the gx brief logs hold

Research for `gx:test-suite-stability/02`. Read-only: no tests ran and no code changed. Counts come from
`/Users/david/.local/share/gx/brief/index.jsonl`, the `gx check` records in
`/Users/david/.local/share/gx/check/gx.jsonl`, the raw logs, and `gh run list` for CI. All counts were taken on
2026-11-16.

Terms. A fact is something a file or command showed. A guess is marked "Guess".

## Short answer

- Per-test pass and fail can be counted today for Go, Vitest and Playwright, from the index alone.
- "First attempt" and "rerun" are not recorded. The nearest proxy is the `tree_id` field: a hash of the whole working
  tree. Same `tree_id` plus a fail then a pass means a pass on rerun with no code change.
- Failure signatures are not recorded. They exist only inside the raw logs, and the logs are not indexed.
- Duration p50 and p95 per test can be counted today (Go, Vitest, Playwright).
- The data cannot tell a real red (an agent doing TDD, a guard test that is correctly failing) from a flake, except
  through the same-tree flip.
- Release checks and agent checks are not marked. Load, worker count and commit are missing from the brief index.

## 1. What is kept

### The brief directory

Fact. `/Users/david/.local/share/gx/brief/` holds 21,362 files and 2.9 GB. Retention is 14 days
(`brief/sweep.go`, `Retention`). The oldest log is from 2026-11-05 23:35, so the directory is 12 days old and has not
reached the window yet. It will stop growing near 14 days.

| File kind | Files | Size |
| --- | --- | --- |
| `*-go-test.log` | 16,371 | 1.9 GB |
| `*-vitest.log` | 1,888 | 360 MB |
| `*-tsc.log` | 1,436 | 0.4 MB |
| `*-playwright.log` | 1,640 | 12 MB |
| `*-go-build.log`, `*-go-vet.log`, `*-go-test-2.log` | 23 | small |
| `index.jsonl` | 1 | 674 MB, 21,344 lines |

Files per day run from 292 (Nov 11) to 3,052 (Nov 9).

### What each raw log holds

- Go: `go test -json` events (`run`, `pause`, `cont`, `output`, `pass`, `fail`, `skip`, `start`), each with `Time`,
  `Package`, `Test`, and `Elapsed` on pass and fail. Output lines carry the failure text. Example: the Nov 16 20:25
  log has 3,600 events with `Elapsed` and 12,662 `output` lines.
- Vitest: one JSON report. Per test it has `fullName`, `status`, `duration` and `failureMessages`.
- Playwright: the text `list` reporter (`webui/e2e/playwright.config.ts`: `retries: 0`, `workers: 3`,
  `trace: "retain-on-failure"`). The header says `Running N tests using 3 workers`. Failure bodies keep the error
  text and call log. No JSON.

### The index

Fact. Each `gx brief` run appends one line to `index.jsonl` (`brief/brief.go`, `indexEntry`):

`time, tool, args, dir, exit, units, duration_ms, log, tests, failed, branch, tree_id, check_run, check_kind,
test_durations`.

- `tool` is `go-test`, `vitest`, `playwright`, `tsc`, `go-build` or `go-vet`.
- `tests` lists test names. `failed` lists the failing names. `test_durations` is ms per test, and old lines lack it.
- `dir` is the worktree, so the same test run in different worktrees is told apart by `dir` and `branch`.
- `check_run` and `check_kind` are set only when the run was started by `gx check`.

Run counts in the index:

| Tool | Runs | Exit 0 | Exit 1 | Other |
| --- | --- | --- | --- | --- |
| go-test | 16,372 | 14,294 | 2,064 | 14 with exit 2 |
| vitest | 1,882 | 1,531 | 349 | 2 with exit 143 |
| playwright | 1,638 | 535 | 1,103 | none |
| tsc | 1,436 | 1,350 | 86 | none |

Of the 3,532 failing go-test, vitest and playwright runs, 3,232 name at least one failed test. The other 300 (18 Go, 20
Vitest, 262 Playwright) name none. Guess: the tool failed before any test ran (build or setup error, kill).

Data quality facts found while counting:

- Go and Vitest runs that fail to build or compile record a pseudo test such as
  `github.com/elentok/gx/server/stats build failed` or `<pkg> package failed`. There are 1,919 such names in Go and 24
  in Vitest. They must be filtered out of per-test counts.
- Vitest test names carry the absolute worktree path (`/Users/david/dev/gx/<worktree>/webui/src/...`). Across
  worktrees they look like different tests. The path must be stripped before counting.
- Playwright names differ between fields. `failed` and `tests` use `e2e/<file>.e2e.ts.<flow>`, and `test_durations`
  uses `<file>.e2e.ts.<flow>` without the `e2e/` prefix. For passing Playwright runs, `tests` is empty and only
  `test_durations` holds the names.
- Some Playwright failed names carry a trailing rule of `─` characters (for example `Workspaces ──────`), so one
  failure counts twice in the top lists: once plain, once with the rule.
- A Playwright flow is one test with many steps. There are only 282 distinct Playwright test names, so a failure
  names a whole flow, not the failing step.

### `gx check` records

Fact. `gx check` writes `<data>/check/<project>.jsonl` (`check/record.go`, `Record`) and one directory per run with
`summary.txt` and per-kind logs. Retention is 30 days (`check.Retention`). `gx.jsonl` has 3,765 records, the oldest
from 2026-10-27, and the whole `check` directory is 62 MB with 3,780 run directories.

Each record has: `run` id, `time`, per-kind `kinds` (`status`, `duration_ms`, `failed` names, `warnings`), `exit`,
`scope` (`changed`, `staged`, `since`, `full`), `kind_filter`, `branch`, `worktree`, `head`, `tree_id`, `link`
(`ticket`, `trunk`, `other`), `epic`, `ticket`, `load_start`, `load_end`, `lock_wait_ms`.

This record is the only place that holds load average, the head commit and the ticket link. The brief index joins to
it through `check_run`.

A slow test that fails the slow limit under high load is rerun alone once. That rerun is the only recorded rerun. It
gets the kind `<kind>-rerun` in the index, with the same `check_run`: 20 go-test, 19 vitest and 8 playwright lines.

### `scripts/test.sh` and `make test`

Fact. `scripts/test.sh` runs the race, go and server steps as three `gx brief go-test` calls and the web step as
`make web-test`. These brief calls carry no `check_run`, so they are not linked to a check. They are recognizable by
args (`-timeout=10m -count=1` with the server package, the other packages, or `-race`). Each suite run produces
three lines with the same second in `time`.

Fact. `scripts/test.sh` writes its own step statuses to a temp dir that it deletes on exit (`trap 'rm -rf "$work"'`).
Only the brief lines remain.

### Release checks versus agent checks

Fact. There is no field that marks a release check (`/gx-bump`). Two groups can be told apart only by
inference:

- `gx check` runs have `check_run`, `link` (`ticket`, `trunk`, `other`) and `worktree`. All ticket runs belong to an
  agent.
- Runs of `scripts/test.sh` are all on branch `main` in the index (31 suite runs, Nov 9 to Nov 16). Whether a person,
  an agent or `/gx-bump` started one is not recorded. Guess: most are release checks, because they run the full suite
  on trunk.

### CI

Fact (`.github/workflows/ci.yml`). The `test` job (macOS and Ubuntu) runs `make test` and uploads nothing. The
`web-e2e` job uploads `webui/test-results` (traces) only on failure, as `web-e2e-traces`. The `herdr-e2e` job
uploads the herdr server log only on failure. No job uploads Go `-json` output, a JUnit file or a summary. Playwright
uses only the `list` reporter, so there is no machine-readable CI report.

Fact. `gh run list --workflow ci.yml --limit 100` returned 100 runs, all `push` on `main`, from 2026-10-20 to
2026-11-15: 98 `failure`, 2 `success`. The job that failed in each run was not read, so this does not say which test
failed. `gh api .../actions/artifacts` reports 123 artifacts in total.

## 2. What can be counted today

All counts below use the index (19,892 go-test, vitest and playwright lines) unless noted. A pair is one test on one
`tree_id`.

| Question | Go | Vitest | Playwright |
| --- | --- | --- | --- |
| Test and tree pairs | 1,344,355 | 1,137,650 | 11,829 |
| Distinct tests seen | 14,819 | 4,061 | 282 |
| Distinct tests that failed at least once | 1,687 | 745 | 235 |
| Pairs whose earliest run failed | 5,038 | 1,216 | 2,537 |
| Fail then pass on the same tree | 670 | 115 | 77 |
| Pass then fail on the same tree | 92 | 58 | 39 |

### First-attempt failures per test: partly

Counted: a test's first recorded run on a tree. Not separable: a failure that is an agent's red step in TDD, a
guard test that fails until the change is finished, and a flake all look the same. Example: `sizeguard.TestSizeLimits`
failed in 382 of 1,011 runs, and `TestNoNewSleeps`, `TestNoOldWords`, `TestNoEnvReads` also fail often. These are
guard tests that fail on purpose when the code breaks a rule. They would swamp any first-attempt rate.

The Playwright number (2,537 of 11,829) is biased too: agents run one failing flow again and again with `-g`, and the
index stores passing names only through `test_durations`.

### Pass on rerun: yes, with the same-tree proxy

A fail then a pass on the same `tree_id` means the code did not change. 670 Go, 115 Vitest and 77 Playwright
test-and-tree pairs show it. Not all are flakes: the environment can change between the two runs (load, a stale
fixture, another process). They are a candidate list, not a verdict.

Most frequent same-tree flips, by number of trees (Go): `gxfixture.TestScenarios_StartAnswerAndStop/check-lock` (15),
`server/web.TestWeb_CancelEndsAQueuedOneOffWithoutAStateToken` (12),
`designflows.TestKeepWorktreeScenario_TheAskedFileSurvivesTheAnswerAndASecondTicketRuns` (11),
`gxfixture.TestCodexBlockedScenario_PermissionRequestShowsAPendingCard` (10),
`server/prflow.TestConflict_MergeAgainOnABlockedPREpicForksAPRModeTicketPastTheCap` (9).

Vitest: `shell/useShortcuts.test.tsx` "Alt+1 opens the first own row" (10), `design/focus.test.tsx` "reports on open,
on visibility change and on a session switch" (7), `pageReady.test.tsx` "'epics' ends its span only after its primary
content has painted" (5), `InterruptControl.test.tsx` "Esc again closes the confirm" (4).

Playwright: `workspaces.e2e.ts` Workspaces (15), `menu.e2e.ts` 180 Menu limit burn (10), `modes.e2e.ts` 61 Page modes
at 760px (8).

### Recurring failure signatures: not recorded

The index has no signature. The raw logs hold the text. A count from the raw logs for the 2,078 failing Go logs (an
`rg -l` over them, run once):

| Text in the log | Failing logs with it |
| --- | --- |
| `panic: test timed out` | 61 |
| `no such file or directory` | 42 |
| `resource temporarily unavailable` | 37 |
| `codex-mismatch` | 5 |
| `connection refused` | 4 |

`resource temporarily unavailable` by day: Nov 8 (2), Nov 10 (1), Nov 12 (3), Nov 13 (2), Nov 15 (6), Nov 16 (23).
Fact: it grows sharply on Nov 16. The index lines of those runs do not carry the text. A signature count needs a log
parse each time. These are string counts, not a classification: a log can match several lines, and a text match does
not prove it is the cause of any one failure.

### Duration p50 and p95: yes, per test

`test_durations` gives per-test ms. Over all samples in the window:

| Tool | Samples | p50 | p95 | max |
| --- | --- | --- | --- | --- |
| Go | 1,475,198 | 0 ms | 1.19 s | 116 s |
| Vitest | 1,305,840 | 52 ms | 385 ms | 125 s |
| Playwright | 12,824 | 5.3 s | 51.6 s | 1,230 s |

Go durations come from `Elapsed`, which `go test -json` rounds to 10 ms, so a p50 of 0 means under 10 ms. Samples mix
loaded and idle machines, so a p95 here is not the p95 of a quiet machine. No sample records load.

Slowest Go tests by p95 (5 or more samples): `gxfixture.TestKeepWorktreeScenario_...` (p95 97 s),
`designflows.TestLongVerifyNoteScenario_...` (92 s), `gxfixture.TestDeadRunEmptyReopens` (91 s). Playwright:
`pause.e2e.ts` "every pause reason recovers" (p95 580 s, max 1,200 s).

### A rough count of runs and failing runs

- All runs: 19,892 test runs (go-test, vitest, playwright), 3,532 failed, about 18 percent. Run level is misleading:
  agents rerun one failing test many times.
- `gx check` test kind (the first attempt on the check path): 3,424 records with a test result, 2,293 pass and 1,131
  fail (33 percent). Per scope: `changed` 931 of 3,170 failed, `since` 90 of 138, `staged` 1 of 5, `full` 109 of 111.
- By tree: 3,190 trees with a test result, 1,001 failed on the first check. 92 trees failed then passed with no
  change. 14 passed then failed.
- Full checks: 111 full runs, 109 failed. Failing names per run range from 0 to 30 in most runs, and two Nov 16 runs
  listed 186 and 144. The top names are Playwright flows: `pause.e2e.ts` (three flows, 30 each),
  `cleanup.e2e.ts` 198 (26), `epics.e2e.ts` 2 (24), `codex.e2e.ts` 117 (24), `phone.e2e.ts` 24 (23), plus the guards
  `sleepguard.TestNoNewSleeps` and `sizeguard.TestSizeLimits` (25 each).
- `scripts/test.sh` suite runs on main: 31 runs from Nov 9. 7 passed all steps. 24 had at least one failing step. The
  two Nov 16 runs (18:58 and 20:25) each failed the go step and the server step: server 20 and 14 failing, go step 102
  and 101 failing, race step passed. Note the go step counts include cascade names such as `package failed`.
- Most frequent failing Go tests (failing runs): `sizeguard.TestSizeLimits` 382, `sleepguard.TestNoNewSleeps` 118,
  `oldwords.TestNoOldWords` 115, `gxfixture.TestScenarios_StartAnswerAndStop/check-lock` 35,
  `designflows.TestKeepWorktreeScenario_...` 32, `envguard.TestNoEnvReads` 30,
  `cmd/check.TestCheck_SlowTestRerunNodeTools/playwright` 30.
- Most frequent failing Vitest tests (failing runs): `TriagePage.test.tsx` "a printed round shows the answered
  cards folded..." 6, `AgentsPage.newSession.test.tsx` (several tests, 5 each), `AgentsPage.phone.test.tsx` 5.
  Vitest failures are few. After path stripping the top tests fail 4 to 6 times each.

## 3. What is missing, and the cheapest place to record it

| Missing | Why it matters | Cheapest place |
| --- | --- | --- |
| Attempt number and "is this a rerun" | Separates a first attempt from a rerun | Add `attempt` to `indexEntry`. `gx check` already knows the run id and sets `-rerun`; give the same field to a manual rerun by looking up the last index line with the same `tree_id` and `args`. The proxy in section 2 works without it. |
| Real red versus flake | A TDD red step counts as a first-attempt failure | Count only runs with a `check_run` and `check_kind: test` on a tree that later passes, or only `link: trunk`. Computed at read time, no new field. |
| Failure signature | Groups failures by cause | Store a short text per failed test in the index when `Parse` already has the failure output (`brief/failures.go`). One line per failure, cut to about 200 characters. |
| Load and machine state per run | Separates load failures from logic failures | Add `load_start` and `load_end` to `indexEntry`; `gx check` already reads `loadAvg`. |
| Process count and `GOMAXPROCS` per run | The Nov 16 fork errors | Add the proc-cap value and the process count at start and end to the index line. This links to `gx:test-suite-stability/05`. |
| Commit | Joins a run to a commit | `tree_id` already identifies content. Add `head` (the check record has it) if a commit link is wanted. |
| Release check marker | Tells a release check from an agent check | Set an env var (for example `GX_CHECK_ORIGIN=release`) in `/gx-bump` and write it into the index. Without it `scripts/test.sh` runs on main stay unlabeled. |
| Playwright per-test pass list and step | Failure names a flow, not a step | `tests` is empty for passing runs; fill it from the list reporter. A JSON reporter in the diagnostic lane would give step and retry data. |
| Worker count | Playwright uses 3 workers and Vitest 3 | In the log header for Playwright. Not in the index. Add a `workers` field. |
| CI failure data | 98 of 100 CI runs failed and no test data is kept | Upload `go test -json` output and a Playwright JSON or JUnit report as failure artifacts. Only traces are uploaded today. |
| Stable names | Vitest paths and Playwright prefixes break joins | Strip the worktree path from Vitest names and the `e2e/` prefix from Playwright names when writing the index; drop the `─` rule suffix. |
| Retention for signature analysis | The index is 674 MB | Keep the new fields small. The big part is the `tests` arrays (3,600 names per Go line). Consider storing only failed names plus counts. This is a guess; the split was not measured. |

## 4. Caveats

- The kept window is 12 days, not 14, and it moves forward as the sweep runs. A rate computed today will not match a
  rate computed next week.
- Runs on different worktrees with the same `tree_id` merge in the same-tree count. This is intended, but it also
  merges runs at different loads.
- Agents rerun a single failing test many times, which inflates fail counts and per-run rates. Use trees or check runs
  as the unit.
- `gx check` full-scope runs include Playwright flows, so the 109 of 111 failed number includes e2e failures that
  `scripts/test.sh` does not run.
- A string count in a log shows a symptom. It does not show the cause. Fact: the Nov 16 logs show fork failures.
  Guess: they come from the process cap, as `gx:test-suite-stability/05` will test.

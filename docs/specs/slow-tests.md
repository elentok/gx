# Slow tests

## Problem Statement

gx's test suite got slow and unreliable. Recorded full runs take 5 to 9 minutes, and a `gx check`
on changed files took 10 to 19 minutes 28 times on Nov 1-2. A `--since` run took 30 minutes. Only
2 of 37 recorded full runs passed the test check. Main has been red in every CI job since
a3cad310, and CI was last green on 2026-10-23. About 36 test follow-ups are open across
`gx:follow-ups`, `follow-ups-2026-11-01` and `gx:test-flakes`; they come down to about 10 root
causes.

The measurements show that most of the time goes to failing or hanging tests that wait for a
timeout. The fake `claude` never proves its launch prompt, so each agent launch waits up to 60 s
(about 23% of full-run worker time). Even with no failures, race, server and web unit tests each
take 4.5 minutes or more, and web-e2e is about 25 minutes if everything passes today. Nothing
stops the next epic from making it worse: there is no per-test or suite time limit, no test
review, and the known flake patterns exist only as scattered notes.

## Solution

The full run (`gx check --full`, web-e2e included) finishes in under 4 minutes, and everything is
green. The work has four parts:

- Fix the causes of the waits and the failures, then add parallelism only as far as the
  measurement needs.
- Remove tests that cost time and add no coverage. Replace the rest with cheaper versions where
  one exists.
- Add guard rails: time limits in `gx check`, a `gx-test-review` step, shared test guidelines and
  a gx-implement self-check.
- Keep full runs from fighting for the cores: a machine-wide lock for `gx check --full`, with a
  waiting label in the Queue tab and on the web ticket page.
- Close the epic only on proof: `scripts/soak.sh 10 --fail-fast` on main, run through
  `gx check --full` in this epic's own verify ticket, is 10 of 10 clean, and the 90th percentile of
  those 10 records is under 4 minutes.

## User Stories

Time limits in `gx check`

1. As a gx developer, I want to set `suite`, `slow` and `hang` limits in the `test` entry of
   `.gx/checks.json`, so that each project chooses its own limits.
2. As a gx developer, I want a project with no limits set to have nothing enforced, so that slower
   projects do not start failing.
3. As an agent, I want `gx check` to fail when a single test passes the `slow` limit, and to print
   the test name and its time, so that a slow test is caught on the branch that adds it and I know
   what to speed up.
4. As a gx developer, I want a test that is over `slow` on a loaded machine re-run alone once (the
   re-run time is not counted in `suite`), and failed if it is still over, so that load does not
   cause false failures.
5. As a gx developer, I want `slow` to take one value or a value per tool (go, vitest,
   playwright), so that Playwright flows can have a higher limit than unit tests.
6. As a gx developer, I want a full run to fail when its wall time passes `suite`, so that the
   whole suite cannot creep past 4 minutes.
7. As a gx developer, I want the suite limit to only warn under high load, so that a busy machine
   does not fail a correct run.
8. As a gx developer, I want the run record to keep the load average at the start and end of the
   run, so that I can tell a slow run from a loaded one afterwards.
9. As an agent, I want a hung Go test killed as soon as it passes `hang`, with the test named, so
   that a hang costs about a minute and not the 30 minute package timeout.
10. As a developer of any project, I want per-test limits to apply to every test run through
    `gx brief go-test`, `vitest` or `playwright` under a check run, whatever command started it,
    and a test command that never calls `gx brief` to get only the suite limit, so that I get a
    guard in both cases.
11. As an agent, I want a clear error for a malformed limit value, so that I fix the config and
    do not get silent no-limit behaviour.
12. As a gx developer, I want a second `gx check --full` that selects the test kind to wait for the
    first one and say whom it waits for, so that two full runs do not fight for the cores and time
    out.
13. As a gx developer, I want the lock wait kept out of the suite limit and the proof, so that
    queueing does not look like a slow suite.
14. As an agent, I want changed, staged and `--since` `gx check` runs to take no lock, whatever
    command they run, so that day-to-day checks are not queued.
15. As a gx developer, I want a ticket whose agent waits for the check lock to show that in the
    Queue tab and on the ticket page, so that I can see why it is idle.

Guard rails for agents

16. As a gx-implement agent, I want to check each new or changed test against the shared guideline
    list before I finish, so that known flake and slowness patterns do not reach review.
17. As a gx-implement agent, I want to be pointed to the guideline list at the start of a ticket
    that adds tests, so that I write the test right the first time.
18. As a gx-implement agent, I want a clear rule for a per-test limit failure (speed it up, or ask
    the person), so that I never raise a limit or add an exemption myself.
19. As a gx-implement agent, I want a rule that I run changed-file `gx check` and leave `--full`
    to the verify ticket, the proof and the person, so that many agents do not queue on the lock.
20. As a gx developer, I want a `gx-test-review` step in the default code-review batch, so that
    every project reviews its tests next to the spec and thermo-nuclear reviews.
21. As a reviewer, I want `gx-test-review` to skip when the diff touches no test file, so that it
    costs nothing on non-test work.
22. As a reviewer, I want `gx-test-review` to judge guideline rules, duplicate coverage and test
    level without running anything, so that it stays fast and read-only.
23. As a gx developer, I want its findings to use the existing triage (approved, deferred,
    rejected), so that I learn one process.
24. As an agent writing a Playwright flow, I want a `gx-playwright-e2es` skill with the
    Playwright rules, so that I do not repeat the known state-read mistakes.
25. As an agent writing tickets, I want `gx-to-tickets` to name that skill in every ticket whose
    seams include Playwright, so that the implementer loads it.
26. As an agent, I want gx-implement and `gx-test-review` to load that skill when the ticket or
    diff is Playwright, so that the rules apply even when the ticket forgot to name it.
27. As an agent in any project, I want generic rules in the shared list and gx-only rules in
    `.ai/index.md`, so that other projects get only what applies to them.
28. As an agent, I want the guard tests (`TestNoNewSleeps`, `TestNoEnvReads`, `TestNoOldWords`,
    `TestSchemaSnapshot`) to run in every `gx check`, so that a count drift shows on my branch and
    not after the merge.
29. As an agent, I want a lint that fails a Playwright state read (`evaluate`, `evaluateAll`,
    `getComputedStyle`) not preceded by an `expect` on the same locator, so that this top flake
    pattern is caught in gx's repo.

Speed

30. As a gx developer, I want the fake `claude` to give the ready signal and the launch record by
    default, so that no test waits for a launch prompt that never comes.
31. As a test author, I want to opt out of those signals, so that I can still test the
    "signal missing" path.
32. As a gx developer, I want launch waits to stop when the server context is cancelled, so that
    shutdown never blocks for up to 60 s.
33. As a gx developer, I want the hung `ralphloop` Telegram test fixed at its cause (`Run` stops
    when the watchdog fires, and the empty `git commit` is explained), so that it cannot hang
    again.
34. As a gx developer, I want a lean command scenario of about 6 sessions for the agent command
    flows, so that each flow does not seed 22 live fake sessions.
35. As a gx developer, I want the failing `gxfixture` scenario tests fixed, so that the fixture is
    trusted again.
36. As a gx developer, I want the 5 Codex approval tests in `nativerunner` sped up with `-race`
    kept, so that they stop costing about 82 s.
37. As a gx developer, I want each fake binary built once per package, so that tests do not
    rebuild it.
38. As a gx developer, I want web-e2e to start together with `scripts/test.sh` in a full run, so
    that the two do not add up.
39. As a gx developer, I want vitest to run with 3 workers in `scripts/test.sh`, so that unit
    tests finish sooner.
40. As a gx developer, I want further levers (`./server` at 4 CPUs, then Playwright at 3 to 4
    workers) used only when the measurement still misses the limit, so that I do not add flake
    risk for nothing.
41. As a gx developer, I want the person asked when every lever is used and 4 minutes is still
    missed, so that the limit is never raised alone.

Removing tests

42. As a gx developer, I want the long-ticket tests that duplicate `ralphloop/longticket_test.go`
    removed (G1), so that about 70 s of Go time goes away.
43. As a gx developer, I want the "second long ticket" test replaced by a direct `alertLongTicket`
    call in a `package server` test (G2), so that the same rule is covered in a fraction of the
    time.
44. As a gx developer, I want the three `...PausesBlockedPaneAfterTheDwell` gxfixture tests to
    stop at the blocked state and card (G3), so that the real pause is left to the Playwright
    pause and codex flows.
45. As a gx developer, I want `TestScenarios_StartAnswerAndStop` trimmed to the 12 scenarios with
    no dedicated test plus 2 or 3 covered ones (G4), so that it stops starting every scenario.
46. As a gx developer, I want the four command-menu Playwright flows removed (G5), because vitest
    covers sort order, highlight, keys, loading and failure.
47. As a gx developer, I want the exact duplicate `TestRefreshKeyTriggersRefresh` removed, so that
    no test runs twice for nothing.
48. As a gx developer, I want the vi-editor Playwright flows kept (G6), so that real key events
    and the wrapped-line check stay covered.

Green

49. As a gx developer, I want the root causes still red (guard counts, the fake PTY session
    family, the 30 s ready wait, `gxfixture` EOF, the missing `claude` on CI's PATH) fixed, so
    that main is green.
50. As a gx developer, I want `go.mod`, and so CI, on the latest stable Go, so that reliability is
    judged on the Go we ship with.
51. As a gx developer, I want CI fail-fast off in the test matrix, so that macOS always reports.
52. As a gx developer, I want `herdr-e2e` tests to pin `agent-runner: herdr`, and `TestClaimHold_*`
    to supply its own fake `claude`, so that CI needs no `claude` on its PATH.
53. As a gx developer, I want every test item in `gx:follow-ups`, `follow-ups-2026-11-01` and
    `gx:test-flakes` closed (fixed, or obsolete when its test is deleted), so that no known flake
    is left.

Proof

54. As a gx developer, I want the epic to close only when the 90th percentile of the 10 soak
    records on main is under 4 minutes, so that the claim rests on data.
55. As a gx developer, I want `scripts/soak.sh 10 --fail-fast` to be 10 of 10 clean, run once in
    this epic's own verify ticket, so that reliability is shown, not assumed. The soak runs each
    round as `gx check --full`, still lists failing tests per round, and with `--fail-fast` stops
    at the first failed round and exits 1.
56. As a gx developer, I want a miss caused by another epic's tests to go to the person, so that
    nobody raises the limit or deletes their tests alone.
57. As a gx developer, I want the time limits to ship in the first implementation tickets, so that
    epics landing meanwhile are already checked.

## Implementation Decisions

Ordering (from the landing decision)

- The `gx check` time limits (per-test and suite) ship in the first implementation tickets,
  before the speed work, so epics that land meanwhile are already checked. In `slow-tests-impl`,
  the time-limit tickets block the speed tickets with `blocked_by` edges.
- There is no freeze and no per-epic test budget. The suite limit is the guard. An epic whose tests
  push the suite over a limit speeds them up or asks the person.
- The speed proof is taken last, after every ticket of this epic has merged. This epic's own verify
  ticket (in `slow-tests-impl`) runs `scripts/soak.sh 10 --fail-fast` on main. The generic verify
  ticket template never runs the soak. `scripts/soak.sh` changes to run each round as
  `gx check --full` (test kind), so a round takes the lock, writes a record and sets
  `GX_CHECK_RUN`. It still lists failing tests per round. `--fail-fast` is a new flag: it stops at
  the first failed round, prints that round's failing tests and exits 1. Without the flag it runs
  every round, so flake counting still works. On a failure the verify ticket fixes the cause and
  restarts the 10 rounds. Those 10 records are the sample, whatever landed in them. The metric is
  the whole run's wall time minus the lock wait. A round that started over the high-load threshold
  is left out, and the verify ticket runs one extra `gx check --full` for it. That run must also
  be clean. The sample is the 10 kept records. The 90th percentile must be under 4 minutes (one
  slow run is allowed in 10). Because the verify agent's shell call can time out, it runs the full
  run and the soak in the background and polls them.
- If the proof is over the limit because of tests another epic added, the agent asks the person.
  It never raises the limit or deletes those tests alone.
- The per-test limit that cannot be reached, the spent levers and the proof missed through another
  epic's tests all use the existing ask pause. No new pause kind is added (ADR 0042). gx cannot
  recover alone, because only the person may raise a limit or approve a coverage loss (decisions
  03 and 05). The person's answer on the ticket page in the web UI ends the pause.
- The order of the rest: restore green and remove waits, apply the cheap levers, measure, then add
  further levers only as the measurement needs.

Time limits in `gx check`

- `.gx/checks.json` gets an optional `limits` object inside the `test` entry:
  `{"full": "...", "limits": {"suite": "4m", "slow": "20s", "hang": "60s"}}`. Values are duration
  strings. A missing key means that limit is off. A plain-string `test` entry has no limits. The
  config parser rejects unknown fields today, so it needs to accept `limits`, and the schema file
  and the config docs must follow. A malformed duration is a config error (exit 2, like invalid
  JSON).
- `slow` is a string (one value for all tools) or a map keyed by tool: `go`, `vitest`,
  `playwright`. `hang` and `suite` are single values.
- gx's own values: `suite` 4m, `hang` 60s, `slow` 20s for go and vitest and 45s for playwright.
  After the speed work the implementer lowers each `slow` value to just above the slowest kept
  test of that tool. Nothing is raised without asking the person.
- gx's `.gx/checks.json` also commits its own full command shape that starts web-e2e alongside
  `scripts/test.sh` (see Speed).
- Blocking: the per-test `slow` limit blocks on every run. The `suite` limit blocks on full runs
  only.
- Load rule: a test over `slow` while the load per core is low blocks at once. With high load per
  core, `gx check` re-runs that test alone once. It blocks if the test is still over, and warns
  and records otherwise. The suite limit only warns under high load, because there is no cheap
  re-run. The implementer picks the load-per-core threshold and notes it in the code and docs.
- The run record keeps the load average at the start and end of the run, and the lock wait as its
  own field.
- Per-test times: the brief index (`brief/brief.go` indexEntry) gains per-test durations. Per-test
  limits apply to every test run through `gx brief go-test`, `vitest` or `playwright` under the
  check run (tagged with `brief.EnvCheckRun`), whatever command started it, including a plain
  shell script such as gx's own `test.full`. A test command that never calls `gx brief` gets only
  the suite limit, measured as the wall time of the command. The re-run of a slow test uses the
  index entry's tool, dir and args narrowed to that one test. Its time is not counted in `suite`.
- Hang kill: `gx brief go-test` watches the `go test -json` stream and kills the run when a test
  passes `hang`, naming that test. The package timeout stays as a backstop. The hang limit reaches
  `gx brief go-test` through `gx check`. `gx brief go-test` keeps its 30 minute package timeout
  default.
- Machine-wide lock: `gx check --full` takes a lock that is released when the process dies. A
  second full run waits, and prints that it waits and for which run. The lock is taken only when the
  mode is `--full` and the test kind is selected and not off, since the reason is CPU use.
  Changed, staged and `--since` runs never lock, whatever command they run, and keep the shared
  CPU budget. `scripts/soak.sh` rounds take it because they go through `gx check --full`. The wait is not
  counted in the suite limit or the proof. The implementer picks the lock mechanism and where the record shows the wait.
- Waiting indicator: while `gx check --full` waits for the lock, it writes a live waiting state
  (waiting, and held by which run) that the server reads. The Queue tab row and the web ticket page
  show it as a status label and clear it when the lock is taken. It is a label, not a pause. The
  server maps the run's worktree to the claimed ticket. A run with no ticket shows no label. The
  server shows the state only while the waiting process is alive (a pid or lease), so a killed
  waiter never leaves a stale label. The implementer picks the mechanism.
- The four guard tests (`TestNoNewSleeps`, `TestNoEnvReads`, `TestNoOldWords`,
  `TestSchemaSnapshot`) are included in every `gx check` test row, whatever the scope. A new
  optional generic key in the `test` entry (for example `always`) is a list of
  shell commands. Each runs through the same `gx brief` path as the scoped command, in non-full
  scopes only, because `--full` already covers them. gx's repo commits its four guard tests there.
  The schema file and the config docs follow.
- Schema: `limits`, `always`, the brief index durations and the run-record fields change no ticket
  or epic schema, so no migration step is needed. Readers treat a missing new field as zero or
  unknown. Old brief index entries are never re-run targets.

Guard rails for agents

- A new default skill `gx-test-review` is added to the default `skills["code-review"]` list, so
  every project gets it. It runs in the same parallel batch as the thermo-nuclear and spec
  reviews. It skips when the diff touches no test file. It reads the diff, the shared guideline
  list and `.ai/index.md` (and `gx-playwright-e2es` when the diff touches a Playwright file). It
  runs nothing. It judges guideline rules, duplicate coverage and test level. Its findings use the
  existing consultant triage with no special path.
- One shared guideline list holds the generic rules. gx-implement's self-check and
  `gx-test-review` both point to it. The list lives inside the skill bundle, so other projects read it. It covers all 11
  known patterns: timer-count assertions, product waits with no test override, state reads
  without a wait, host binary assumed, shared ids, exact-count ratchets, generated-file drift,
  hidden failure causes, wall clock, stale assertions, and fail-fast hiding a platform.
- gx-only rules (the ready wait, the `GOMAXPROCS` cap, `claudeenv`, the guard tests) stay in
  `.ai/index.md`.
- gx-implement: at the start of a ticket that adds tests it points the agent to the guideline
  list. Before `--iteration-status finished` it checks each new or changed test against the list,
  next to the existing `-count=20` rule for tests that wait. A per-test limit failure means the
  agent speeds the test up first (fake clock, a test-sized wait override, shared setup, a lower
  test level). If it cannot get under the limit, it pauses with `gx tickets ask` and the person
  decides. There is no exemption list, and the agent never raises a limit. Suite-limit failures
  are not a gx-implement concern. gx-implement adds no per-test timing to the handoff, because
  `gx check` already prints and records it.
- Full runs: ticket agents run changed-file `gx check`. `gx check --full` is for the epic's verify
  ticket, the proof and the person. The verify ticket runs it and the soak in the background and
  polls, so an agent shell timeout does not cut a lock wait or the soak. The rule is written in gx-implement and `.ai/index.md`.
  `gx check` does not enforce it.
- A new skill `gx-playwright-e2es` ships with gx and holds the Playwright-specific rules. It
  points back to the shared list for the generic ones. `gx-to-tickets` names it in any ticket
  whose test seams include Playwright. gx-implement loads it as a fallback when the ticket text,
  seams or touched files are Playwright.
- One automated rule: a Playwright lint in gx's own repo. `evaluate`, `evaluateAll` or
  `getComputedStyle` must follow an `expect` on the same locator. It does not ship to other
  projects in this epic (they get the written rule in the skill). Other scannable patterns become
  follow-ups if they recur.

Speed (in order)

1. Restore green and remove waits.
   - Fake claude (`testutil/agentfake`): gives the ready signal (delay 0 s) and the launch record
     by default. A test opts out to test the "signal missing" path. `gxfixture` and `servertest`
     need no knobs. Product `DefaultReadyWait` stays 30 s, which is right for real claude.
   - Launch waits follow the server context: `awaitReady` and `claudeui.CheckLaunchDelivered`
     stop when the server context is cancelled. This is a product code change.
   - The `ralphloop` Telegram hang is fixed at its cause: `Run` must stop when the watchdog fires,
     and the empty `git commit` is explained. The `hang` limit stays as the safety net.
   - A lean command scenario (about 6 sessions instead of 22) serves the 29 agent command flows.
     No parallel seeding and no seed snapshot cache now. The implementer may try a snapshot only if
     the lean flows are still over budget (untested: the seeded sessions are live fake processes).
   - Fix the failing `gxfixture` scenario tests. Apply removal groups G1 to G5 and the exact
     duplicate. Speed up the 5 Codex approval tests in `nativerunner` with `-race` kept. Build each
     fake binary once per package; the 3 Codex screen-dialog tests skip the `gx` build.
   - Investigate `TestRateLimitHold_...known_reset` and
     `TestBlockedPause_UsageLimitErrorHoldsInsteadOfPausing` (10.5 s, cause unknown). They are not
     removed.
   - The AutoAdvance 1 s countdown stays as is. A fake clock needs product code and a shorter
     delay adds flakes.
   - The `install repo template: rename ... file exists` race is fixed here.
2. Cheap levers, applied now: `gx check --full` starts web-e2e at the same time as
   `scripts/test.sh`, and vitest runs with 3 workers in `scripts/test.sh`.
3. Measure. Apply only if still over the limit, in this order: `GOMAXPROCS` (and so `-parallel`)
   of 4 for `./server` only, then Playwright workers 3 to 4.
4. If 4 minutes is still missed after every lever, the agent asks the person (more removals or a
   higher limit). It never drops a lever or raises the limit alone.

Green

- Upgrade `go.mod` (and so CI, which reads `go-version-file`) to the latest stable Go. Reliability
  is judged on that version. The implementer runs `TestBrief_*` on 1.25.8 once; a real bug in how
  gx reads `go test` output becomes a follow-up.
- Turn fail-fast off in the CI test matrix.
- "Green" means the CI jobs on main (ubuntu and macOS test, `web-e2e`, `herdr-e2e`) plus
  `gx check --full` locally. The soak and the p90 proof stay as stated above.
- Remaining red causes are fixed at the cause: guard counts, the fake PTY session family, the 30 s
  ready wait, `gxfixture` EOF. For the missing `claude` on CI: `herdr-e2e` tests pin
  `agent-runner: herdr`, and `TestClaimHold_*` supplies its own fake `claude`. CI's PATH is
  unchanged. The implementer reads the preflight error of web-e2e restart flows 129, 130, 137 and
  143 first and applies the pin only if it is the same cause.
- A CI-only failure is reproduced first only when its cause is unknown. A known cause is fixed
  directly and the next CI run is the proof.
- All three follow-up lists count. The epic closes only when none has an open test item. Each item
  is closed as fixed, or as obsolete when its test is deleted (including the groups removed
  above). Tickets stay in their lists.

Removing tests

- Exact duplicates go without asking. Any other group that loses coverage needs the person's
  approval; G1 to G5 are approved, G6 is kept. Any further candidate found during the work goes to
  the person first, after the search for a faster or less flaky way to cover it.

## Testing Decisions

- A good test drives the highest seam and checks what a person or agent sees: output, exit code,
  the saved file, the record line, the rendered page. It does not test private helpers.
- **Seam 1: the `gx check` command** (existing, from better-tests). Tests run it against a
  `testutil.TempRepo` whose configured test command is a small script. They cover `limits` parsing
  and its errors, slow-test blocking, the re-run under high load (the load average is injected,
  not read from the machine), suite limit on full versus changed runs, other-tool suite-only
  limits, the lock wait message, the record fields, and the guard tests included in the test row.
  Prior art: `cmd/brief_test.go`, `cmd/brief_retention_test.go`, the `gx check` tests from
  better-tests.
- **Seam 2: `gx brief go-test`** (existing). Tests feed a fake `go test -json` stream or a small
  test binary that hangs, and assert that the run is killed past `hang` and the test is named.
  Prior art: `cmd/brief_test.go`, the `TestBrief_*` tests.
- **Seam 3: skill text** (existing). Tests in `skills/` match the new rules: `gx-test-review` is in
  the default code-review list and skips with no test file, gx-implement's start pointer, self-check
  and ask rule, `gx-to-tickets` naming `gx-playwright-e2es`, and the skill bundle contents. Prior
  art: `skills/bundle_test.go`, `skills/check_rules_test.go`, `skills/mock_rules_test.go`,
  `skills/gx_design_rules_test.go`.
- **Seam 4: the fake agents through the existing fixtures.** The ready signal, launch record and
  opt-out are covered through `testutil/agentfake` and the server tests that already start fake
  agents (`servertest`, `gxfixture`). The context-cancel behaviour is covered by a `nativerunner`
  test that cancels the server context during a launch and asserts a prompt return. Prior art:
  `nativerunner` pty launch tests, `testutil/gxfixture` scenario tests.
- **Seam 6: the waiting label.** A server test writes a waiting state, asserts the ticket's Queue
  row and API label, that it clears when the lock is taken, and that a dead writer's state shows
  nothing. A Queue tab view test covers the TUI row. One web flow is required.
- **Seam 5: the Playwright lint** is a repo test, like `TestNoNewSleeps`
  (prior art: `oldwords/oldwords_test.go`), with a fixture file that violates the rule and one that
  passes.
- **Replacement coverage.** G2's replacement is a direct `alertLongTicket` call in a
  `package server` test.
- **The suite itself** is the test for the speed and green parts. Timings come from the `gx check`
  run record. New or changed tests that wait on time pass `-count=20`.
- The proof is a measurement, not a test: `scripts/soak.sh 10 --fail-fast`, whose 10 run records
  give the timing sample. A test with a fake `test.full` covers `--fail-fast`.

## Verification

The epic's verify ticket runs these against a temporary gx server. Unless an entry says otherwise,
CLI checks run in a temp repo (`testutil.TempRepo`) with a small `.gx/checks.json`. The new flows
below are tests inside the existing suites; the story entries name them.

New flows or scenarios to write:

- A lean command scenario in `testutil/gxfixture` (about 6 sessions) for the agent command flows.
  The existing command flows move to it.
- One new web flow for the lock-waiting label, plus the server and Queue tab tests of Seam 6.
  Otherwise no new web flow: the existing web-e2e flows prove the speed and the fake-agent changes.

Stories:

1. Write `limits` in a temp repo's `.gx/checks.json`; `gx check --full` reads them (a limit of
   `1s` on a script that sleeps 3 s fails). A schema check rejects an unknown key inside `limits`.
2. Remove `limits`: a script that sleeps 3 s passes, exit 0.
3. A test script that runs one test at 5 s through `gx brief go-test` (or vitest, playwright) with
   `slow: 2s`: `gx check` exits 1 and names the test and its time.
4. Same with an injected high load: the test is re-run alone once; if the second run is under the
   limit the output warns and exits 0, and the record keeps the warning. If the re-run is still over
   the limit, exit 1 and the test is named. The re-run time is not in the suite time.
5. `slow` as a map: `playwright` at 45s passes a 30 s Playwright test while `go` at 20s fails a
   30 s Go test.
6. `suite: 1s` with `gx check --full` over a 3 s script: exit 1. The same limit on a changed-file
   run does not fail it.
7. The same with injected high load: a warning, exit 0.
8. After a run, the last line of `~/.local/share/gx/check/<project>.jsonl` (the run record) holds
   the load averages and the lock wait.
9. A Go test that sleeps forever with `hang: 5s`: `gx brief go-test` ends in about 5 s and names
   the test, and `gx check` fails with it.
10. A `test` command that is a plain shell script that never calls `gx brief`: only `suite` applies.
    The same script calling `gx brief go-test` gets per-test limits.
11. `limits: {"suite": "fast"}`: exit 2 and a message naming the key.
12. Start two `gx check --full` runs; the second prints that it waits and for which run. A
    `gx check --full --kind lint` run starts at once.
13. The second run's record shows the wait separately and the suite limit does not count it.
14. A changed-file `gx check` started while a full run holds the lock starts at once. Same for a
    `--since` and a staged run.
15. Hold the lock with a long full run; the waiting ticket's Queue row and web page show the
    waiting label and the holder, and the label clears when it takes the lock. Kill the waiter: the
    label clears. A run with no ticket shows no label.
16. `skills/` tests pass: gx-implement text contains the self-check and the start pointer
    (`go test ./skills/ -timeout 60s`).
17. Same as 16 (the start pointer is checked there).
18. Same as 16 (ask rule, no exemption list, never raise a limit).
19. Skill-text test: gx-implement and `.ai/index.md` say ticket agents run changed-file `gx check`
    and keep `--full` for the verify ticket, the proof and the person. They also say the verify ticket
    runs `gx check --full` and the soak in the background and polls (`go test ./skills/`).
20. `gx project` default config lists `gx-test-review` in `skills["code-review"]`; the skill
    bundle contains it.
21. A skill-text test for the skip rule; run `gx-test-review` against a diff with no test file in a
    temp repo and see it skip (manual check by the verify agent).
22. Same as 21 (it runs nothing: the skill text names no command).
23. Same as 21 (triage wording points to the existing consultant triage).
24. The `gx-playwright-e2es` skill is in the bundle; `go test ./skills/` passes.
25. Same as 24 (`gx-to-tickets` names the skill; text test).
26. Same as 24 (gx-implement fallback and `gx-test-review` read it; text test).
27. The skill bundle contains the shared list with the 11 patterns; `.ai/index.md` keeps the gx-only rules (text
    test or review).
28. In a temp copy of gx with a sleep count drifted by one, `gx check` on an unrelated changed file
    fails `TestNoNewSleeps`. gx's `.gx/checks.json` lists the four guard tests under the `always` key,
    and the schema accepts it. A parse error in `always` exits 2. On a changed-file run the listed
    commands run through `gx brief`; on a `--full` run they do not run twice.
29. The Playwright lint test passes on gx's flows and fails on a fixture with a bare `evaluate`.
30. Run a web-e2e agent launch flow (any flow that starts a fake agent, such as the agent session
    flows) and check the launch takes seconds, not 60 s; the full run needs no launch wait.
31. A `servertest` or `agentfake` test sets the opt-out and sees the "signal missing" path.
32. `gxfixture ready` started, then SIGTERM while a launch is waiting: the process exits in under
    5 s.
33. The `ralphloop` Telegram test passes 20 times (`-count=20`), and a watchdog test shows `Run`
    returning.
34. The agent command flows run against the lean scenario (flow-catalog flows for `agents-commands`
    that stay after G5: 149 refresh, 150, 151, 152, plus the other command flows).
35. `go test ./testutil/gxfixture` passes.
36. `go test -race ./nativerunner` passes and the 5 Codex approval tests together take well under
    82 s.
37. The fake binaries are built once per package (a build counter in a test, or the timing in the
    run record).
38. `gx check --full` in gx's repo runs web-e2e at the same time as `scripts/test.sh` (log
    timestamps overlap).
39. `scripts/test.sh` shows vitest started with 3 workers (its log or process args).
40. After the measurement, record whether `./server` at 4 CPUs and Playwright workers were needed.
    Each is applied only if the run is still over the limit.
41. If the measured p90 is still over 4 minutes with every lever used, the verify agent stops and
    asks the person (`gx tickets ask`).
42. `rg TestLongTicket_ZeroThresholdProducesNoAlert` and
    `TestLongTicket_ANonImplementTicketGetsNoMarkBannerOrDraft` find nothing; `ralphloop` long-ticket
    tests pass.
43. The `package server` `alertLongTicket` test exists and passes.
44. The three gxfixture pause tests pass and stop at the blocked state; Playwright `pause.e2e.ts`
    and `codex.e2e.ts` flows pass.
45. `TestScenarios_StartAnswerAndStop` passes and lists 14 or 15 scenarios.
46. The four removed command-menu flows are gone from `agents-commands.e2e.ts` and from
    `webui/e2e/CATALOG.md`; vitest `match.test.ts` and `CommandMenu.test.tsx` pass.
47. `rg TestRefreshKeyTriggersRefresh ui/prs` finds nothing.
48. The vi-editor flows still pass in web-e2e.
49. `gx check --full` locally is green, and the CI jobs on main are green (`gh run list --branch
    main`).
50. `go.mod` names the latest stable Go; CI reads it; `TestBrief_*` passes on it.
51. `.github/workflows/ci.yml` has `fail-fast: false` in the test matrix, and macOS reports.
52. `herdr-e2e` passes on CI with no `claude` on PATH; `TestClaimHold_*` passes on a machine with no
    `claude` installed.
53. `gx tickets show` for each of the three follow-up lists shows no open test item.
54. The proof: `scripts/soak.sh 10 --fail-fast` on main, each round through `gx check --full`,
    writes 10 records with a lock-wait field in `~/.local/share/gx/check/gx.jsonl`. Their wall
    time minus the lock wait has a 90th percentile under 4 minutes. A round that started over the
    high-load threshold is left out and replaced by one extra clean `gx check --full`.
55. `scripts/soak.sh 10 --fail-fast` reports 10 of 10 clean. A test with a fake `test.full` that
    fails in round 2 stops after round 2, prints the failing test and exits 1. Without the flag it
    runs every round.
56. If the proof is over only because of another epic's tests, the verify agent asks the person.
57. In `slow-tests-impl`, `blocked_by` edges make the time-limit tickets block the speed tickets
    (`gx tickets show slow-tests-impl`).

## Out of Scope

- A CI time limit. Shared runners make a time target noisy. CI only has to be green.
- A built-in time limit for projects that set none. It would fail slower projects for no reason.
- A freeze of other epics, and a per-epic test budget.
- A seed snapshot cache for the command scenario, parallel seeding, and shortening the AutoAdvance
  countdown.
- Removing the vi-editor flows (G6) or lowering `-race`.
- Shipping the Playwright lint to other projects, and automating the other 10 patterns.
- Changing product `DefaultReadyWait` or CI's PATH.
- Raising any limit without the person's approval.

## Further Notes

- Research behind the numbers is on branches `research/slow-tests-measure`,
  `research/slow-tests-root-causes`, `research/slow-tests-audit` and
  `research/slow-tests-speed-facts` (files `docs/research/slow-tests-*.md`).
- Prior work: `docs/specs/better-tests.md` Part B and ticket `gx:better-tests-impl/38b`. That epic
  skipped its soak; this one requires it.
- Under high machine load the `./server` tests take about twice as long, so judge timings under
  normal agent load, as the proof does.
- A "flake" here follows GLOSSARY.md: a test that fails and then passes on a rerun with no code
  change.

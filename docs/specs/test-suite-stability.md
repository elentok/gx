# Test suite stability

## Problem Statement

The release checks on November 16 failed twice, with 23 and 14 failing tests. A full `gx check` fails almost every time: 109 of 111 recorded full runs failed. Developers and agents cannot tell a real regression from a flake. They lose time on reruns, on tickets that name the same test again and again, and on "fixes" that raise a timeout.

Research on trunk found real causes behind most of this:

- Two real product regressions that sat unnoticed on main (the Codex question matcher and the store-commit debounce).
- A process limit that is too small for a full run. A test that starts 240 processes spends the limit of every other test binary and every other session.
- Tests that act before the thing they need is ready: an agent that is not reading input yet, a launch whose fake never runs a turn, a session still in `Launching`, a lock file read while empty.
- Stale Playwright and Vitest expectations after planned product changes, tests that read one async source after waiting for another, a disabled button that gets clicked, and a leaked timer that fails a Vitest run with every test green.
- A real product bug: on phone and tablet widths the empty Agents page covers the sidebar burger.
- A service worker precache that reads 280 response bodies too late, so the install takes 18 seconds.
- Rules for writing reliable tests that live in one repo, so other projects and reviewers do not get them.
- No trusted measure of progress. The `gx brief` index cannot separate a real red from a flake, counts slow tests as failures, and hides Vitest errors that fail a run with every test green.

## Solution

Fix every failure group at its cause, then prove it with a bounded run. The work has these parts:

- The two regressions are fixed in the product.
- The fork errors are fixed with a bigger default process headroom (800). The 240-process burst test moves to its own step that runs after the others.
- The Go, Playwright and Vitest flakes are fixed group by group. Each fix removes the cause. None adds a retry or a longer timeout.
- The test rules move into shared `gx-` skills that other projects can install: framework-free guidelines, and one skill each for Go, Vitest and Playwright. The gx-only Playwright rules become a repo-local skill.
- `gx-test-review` applies those rules to any project.
- Two kinds of guard keep the worst patterns out: the Biome linter for the web UI, and Go rules tests.
- `gx brief` records enough to compute the first-attempt pass rate of full trunk runs and a same-tree flip list.
- Old flake tickets are closed where a fix explains them. The rest move into the `test-health` backlog. New flake reports follow one route.

Done means 20 consecutive full runs on the finished trunk pass, with no retry and no timeout change. Any failure is root-caused and fixed, and the count restarts at zero.

## User Stories

Regressions and fork errors

1. As a person answering a Codex question in the web UI, I want the answer sent when the screen shows the question the card shows, so that a header folded into the card title does not block the answer.
2. As a maintainer, I want gx to compare the raw question text from the screen with a raw question field on the card, so that display text can change without breaking the safety check that stops gx from sending keys to a screen it did not verify.
3. As a developer, I want the Claude dialog path checked for the same title comparison, so that the same commit's other card change cannot hide a second break.
4. As a person who reloads config with a new store-commit debounce, I want a change made after the reload to commit about one debounce later, so that the documented contract holds when a file watcher event is lost.
5. As a maintainer, I want the store watch to call the commit loop's poll check on every poll tick, so that the poll stays the guarantee and the watch only makes it faster.
6. As a person who reloads config, I want a pending quiet timer re-armed when the settings change, so that the new debounce applies even when no further event comes.
7. As a release owner, I want a full capped run to show no `resource temporarily unavailable` errors, so that a fork limit never fails an unrelated package.
8. As a release owner, I want the default process headroom at 800, set in one place, so that the normal ramp of a full run plus one burst from another session fits.
9. As a developer running two sessions on one machine, I want the 240-process burst test to run in its own step after all other steps finish, so that it cannot spend the limit of its siblings or of the other session.
10. As a maintainer, I want the process cap kept, so that a test binary that re-runs itself still fails fast instead of crashing the laptop.
11. As a maintainer, I want the extra `-exec scripts/proc-cap` removed from the report's `gx brief go-test` examples, so that nobody applies the cap twice.

Go readiness

12. As a developer, I want launch and nudge tests to run a real fast turn for the launch prompt, so that they assert a real event and not the startup count.
13. As a developer, I want the test harness to arm a fast turn for a launch prompt by default unless the test asks for a stall, so that new launch tests do not repeat the mistake.
14. As a person launching a real Claude session, I want the first move from "starting" to "idle" not counted as a turn, so that a launch is never reported delivered because of startup, and a real turn is not missed.
15. As a developer, I want a real busy or shell state on the first read to still count as a turn, so that a turn that really started is seen.
16. As a developer, I want tests that type into a fixture agent to wait for the agent to be ready, not for the holder socket, so that keys are not typed before the agent reads input.
17. As a developer, I want every test that waits for the holder and then types audited and fixed the same way, so that the same race has no second copy.
18. As a developer starting two servers on one state, I want the lock file written in one write of fixed width and never truncated to empty, so that a loser never reads `already running (pid )`.
19. As a developer, I want a fixture to find a new session's control file by the session id, so that a late fake cannot be mistaken for the new session.
20. As a developer reading a failed fixture test, I want each fixture child's stderr captured and attached to that test's failure, so that the cause does not land under a neighbouring test.
21. As a developer, I want nested waits in a test to add up to less than the `gx brief` hang limit, so that the test prints its diagnostic before it is killed.
22. As a developer, I want the budget test to wait until no run is `Launching` before it polls, so that the budget kill does not skip a run.
23. As a developer, I want a safe finish helper on the runner fake that ignores a missing session, so that a failed launch fails one test and not the whole package.
24. As a developer, I want all unguarded fake `SetState` cleanups moved to that helper, so that a cleanup can never end the test binary.
25. As a developer, I want the auto-advance countdown test to hold the countdown from the test and not assert against the wall clock, so that a one-second state is not missed by a stalled goroutine.
26. As a developer, I want tests whose cause could not be found to print the state they waited for (ticket file, server log, holder stderr, iteration list), so that a recurrence is diagnosed on its first sighting.

Playwright

27. As a developer, I want flows 199, 155 and 159, 72b, the menu part of epics 2, settings 200, menu 117, 158 and 160 updated to the planned product behaviour, so that stale expectations stop failing every run.
28. As a developer, I want flow 158 and 160 tails rewritten to answer through the question sheet, so that they match the card that hides the blocked notice.
29. As a phone or tablet user, I want the sidebar burger usable on every Agents page state, so that I can open the sidebar when no session is selected.
30. As a maintainer, I want every state of every routed page to have a header registered with the shell, so that no pinned page can cover the burger again.
31. As a maintainer, I want a Vitest test that renders each routed page in its loading, not-found, empty and failed states and checks for a header, with no allowlist, so that the rule is enforced.
32. As a developer, I want a page without its own header to claim one with a small shared helper, with the page's own title from the route in loading and error states, so that the rule is cheap to follow.
33. As a developer, I want the shell's own burger row kept for shell states that mount no page, so that "Can't reach gx" and the lazy-page loader still work.
34. As a developer, I want flow 24 to read the four footer boxes together inside `expect.poll`, so that a reflow after a viewport change is not read frame by frame.
35. As a developer, I want flow 22 to wait for the final settings URL before the next navigation, so that a late redirect cannot overwrite it.
36. As a user installing the web app, I want the service worker to install in well under a second, so that it activates before any flow or person needs it.
37. As a maintainer, I want a Vitest case that the precache reads every response body before it opens the cache, so that the rule is guarded even though the stall only shows in Chrome.
38. As a developer, I want flow 184 to load a real route as a hard load before it reads the web performance counts, so that `app_ready` is queued.
39. As a developer, I want the "Include dev" step of flow 184 diagnosed and fixed at its cause, so that the flow is green end to end.
40. As a developer, I want the Playwright suite to keep `retries: 0` in required CI, so that a pass on retry never counts as a fix.
41. As a maintainer, I want flows with no reproduction (flow 180, 198, the pause "every reason" flow, the "fixture never starts" family) given a diagnostic message that names the last state read, so that a recurrence is root-caused.

Vitest

42. As a developer, I want the EpicPage tests to wait for the epic list and the epic detail before they read card output, so that they do not read one fetch after waiting for another.
43. As a developer, I want the routes "Merge blocked" test to wait for the text that proves the new epic painted, and to clear the page cache between its two renders, so that the cache is not an input.
44. As a developer, I want the ScreenModal Refresh test to wait until the button is enabled before it clicks, so that a click on a disabled button is not silently lost.
45. As a developer, I want the InterruptControl test to keep its focus wait and drop the 5 s overrides, so that it uses the repo's global wait.
46. As a developer, I want the menu's focus-settle timer to return a disposer that the menu calls on unmount, so that no timer outlives jsdom.
47. As a developer, I want test setup to cancel module-level timers after each test, so that a leaked timer cannot fail a run with every test green.
48. As a developer, I want a unit test showing the timer count is zero after a menu item is picked and the menu unmounts, so that the leak stays fixed.

Shared skills

49. As an agent in any project, I want a framework-free `gx-test-guidelines` skill, so that the common rules (waits, isolation, cleanup, diagnostics, stale assertions) apply everywhere.
50. As an agent writing Go tests in any project, I want a Go testing skill that holds the readiness, baseline, cleanup, lock, process-limit and fault-injection lessons, so that I do not repeat them.
51. As an agent writing Vitest tests in any project, I want a Vitest skill moved from the person's dotfiles and expanded with the lessons from the Vitest report.
52. As an agent writing Playwright flows in any project, I want a portable Playwright skill with the lessons from the Playwright report, so that I know about redirects, `expect.poll`, covered controls and error-context first.
53. As an agent in gx, I want the gx-only Playwright rules in a repo-local skill that the index, the skill description and the guard message all point to, so that I load it before I touch a flow.
54. As a person using other projects, I want gx to install only the portable skills, so that gx-only rules do not leak into my projects.
55. As a maintainer, I want a test that fails when a portable test skill names a gx command, so that the shared skills stay portable.
56. As an agent implementing a ticket, I want `gx-implement` and `gx-tdd` to read the guidelines and the framework skill that matches the changed files, so that the rules are applied while I write.
57. As a maintainer, I want the portable Go rules moved out of `.ai/index.md`, with only gx policy left there, so that each rule has one home.

Review

58. As a reviewer in any project, I want `gx-test-review` to judge meaningful coverage, assertions that pass early, timing assumptions, isolation, cleanup and diagnostics against the shared skills and the project's own test docs, so that review catches what the research found.
59. As a reviewer, I want the review to keep its name and its place in `gx-code-review`, so that nothing else changes in the review flow.

Guards

60. As a maintainer, I want the Biome linter on in `gx check` and CI with `recommended` plus the `react` and `test` domains, so that hook misuse, index keys, untyped buttons and focused tests fail early.
61. As a maintainer, I want `noNonNullAssertion` off with a comment, and every other finding fixed with the rules running as errors, so that the linter ships green.
62. As a maintainer, I want a rules test that fails on Playwright fixed sleeps (`waitForTimeout`), so that clock waits do not return.
63. As a maintainer, I want a rules test that fails on a fake `SetState` cleanup with no liveness check, so that the cleanup panic does not return.
64. As a maintainer, I want guards to have no allowlist and to ship green, so that an exemption list cannot grow.

Measurement

65. As a maintainer, I want the `gx brief` index to record the attempt, a short failure signature, load at start and end, process count at start and end, and an origin marker, so that real reds and flakes can be told apart.
66. As a maintainer, I want stable test names in the index (no worktree path in Vitest names, no `e2e/` prefix or `─` suffix in Playwright names), so that one test has one history.
67. As a maintainer, I want the slow-test count fixed so the headline does not call slow tests failures, so that the headline is trusted.
68. As a maintainer, I want a Vitest run that exits 1 with all tests green recorded as a failed entry with a signature, so that a silent red run is visible.
69. As a maintainer, I want `go test -json` and a Playwright JSON report uploaded as CI failure artifacts, so that a CI failure can be classified without rerunning.
70. As a maintainer, I want one command or script to print the first-attempt pass rate of full trunk runs and the fail-then-pass list per test on the same tree, so that progress is a number.
71. As a maintainer, I want the baseline recorded before the first fix, so that the result has something to compare with.
72. As a maintainer, I want p50 and p95 duration per test tracked with no pass or fail bar, so that slow drift is visible.
73. As a maintainer, I want the final proof to be 20 consecutive passing full runs with no retries and no timeout changes, so that "stable" has a definition.

Tickets

74. As a maintainer, I want each fix to close the old flake tickets it explains, in place, with a note that links the impl ticket, so that evidence stays and addresses stay stable.
75. As a maintainer, I want the 7 duplicates and the obsolete tickets closed first with the right reason, so that the list shrinks before the work.
76. As a maintainer, I want every flake ticket that no fix explains moved into `test-health` at the end of the epic, so that old epics end up empty.
77. As a person filing a flake, I want `gx-follow-up` and the flake-ticket rules to check `test-health` for a match and add a "seen again" entry, so that reports stop spreading.

## Implementation Decisions

Principle for every item below: fix at the cause. No retries. No blanket timeout increases. A pass on retry never counts. Where a report offers a quicker symptom fix next to a cause fix, the cause fix wins.

Regressions

- Codex question matcher: add a raw question field to the wire question-card type, set it where the Codex card is built, and compare the overlay's question text with that field (falling back to the title when it is empty). Do not edit the prompt-package fixtures to draw the header, and do not guess where a header ends. The implementer checks whether the web type for this wire struct is generated or hand-written and updates it if needed. The Claude dialog check is read to confirm it has no title comparison.
- Store commit: the watch hook calls the poll check on every poll tick, with the watch live or not. The check restarts the quiet period only when the dirty files change. The implementer picks the poll interval for a live watch. It must be short enough that the existing 10 s test wait is met with margin, and it must cost no more than the existing per-tick rescan. A pending quiet timer is also re-armed when settings change. The test, its debounce and its 10 s wait stay as they are.
- A debug run showed the cause: zero watch events reached the loop in the test and the poll never reached the commit loop. The fix does not depend on why the watcher delivered nothing.

Process limit

- The default headroom in `scripts/proc-cap` is 800. It stays the single place for the default. The cap stays on. Only headroom and concurrency may change.
- The burst test moves out of the shared steps into its own step in the test script that starts after every other step ends. This is a later, separate ticket after the headroom ticket. The implementer picks the mechanism for the separate step.
- The test that raises its own limit keeps a note that its siblings do not get the raise.
- Validation: 0 fork-error lines in the check log and the `go test -json` logs in two full runs, one of them while another session runs a Go test job. The process sampler's peak minus the lowest binary start count must stay under 80 percent of the headroom.

Go readiness (grouped by cause)

- Launch fakes: the harness arms a fast turn for a launch prompt by default, unless a test asks for a stall. The no-hook launch tests (nudge, vim, pasted command, hung pane, in-flight, dropped prompt) assert a real turn.
- Launch baseline (product): the pid watcher does not count the first move from starting to `idle` as a turn. It counts a first read of `busy` or `shell`, and every later status change. The OSC working count and the new session id count stay. `awaitNotBusy` stays as is, and starting is not busy. The launch proof's transcript check remains the second signal. The two harness tests that pin the old count are updated, and one new test covers "a fresh idle file does not move the turn". A real Claude was probed for this: its launch is neither a turn nor busy, in four of four runs.
- Session screen: wait for the agent (an acked control) before typing. Audit every caller that waits for the holder and then types.
- Lock file: write the pid in one write of fixed width and never truncate to empty. Validate with a multi-process loop that must show zero empty reads.
- Seeder: find the control file by the session id (the id is the fake's `--session-id`). Do not diff a directory after a quiet window.
- Fixture stderr: capture per child and attach it to the failing test.
- Nested waits: the fixture start, the launch-slot wait and the part poll fit under the hang limit. The poll prints the missing parts and capture lines. Keep the slot wait out of the poll.
- Budget test: wait for the launching-run count to reach zero (the existing seam) after the sessions appear. Do not widen the 5 s wait.
- Runner fake: add a non-panicking finish helper. Move the 16 unguarded cleanups and the six-label loop to it. Then the guard (below) forbids the raw pattern.
- Countdown test: the test holds the countdown timer, observes the Continue, then releases it. The wall-clock assertion is dropped.
- Tests with no reproduced cause (the `holder.sock never answered` and `claude exited (code 1)` family, the usage gate, late finish and derived-actionable tests, the slow `TestDeadMany`, the nested launch-summary test): diagnostics only. The failure message prints the state read. Survivors go to `test-health`. A recurrence inside the 20-run window is root-caused then. Do not raise the `TestDeadMany` limit.

Playwright

- Stale flows: update 199, 155 and 159, 72b, epics 2 menu, settings 200, menu 117, and the two locators and tail of 158 and the tail of 160, as listed in the Playwright report. Prefer `toContainText` of named items over a count. Review renames in e2e files by diff.
- Burger: the empty and other headerless states of pinned pages (Agents, Design, Triage) get a header that carries the burger. A shared helper, named by the implementer, lets a page without its own header claim one. In loading and error states the title is the page's own title from the route. About 20 other headerless states (Epic, Ticket, Draft, Settings and account loading or not-found states) are covered by the same rule. The Schedules and File viewer pages use their own bars and keep them. Flows 152, 88 and 117 stay as they are and must pass.
- Flows 24 and 22: poll the footer boxes together, and wait for the final settings URL.
- Service worker: read each response body inside the precache map callback, so no response is held unread. The Vitest guard checks the order (all bodies read before the cache opens). Do not raise the 10 s activation poll. Bounded concurrency is not needed.
- Flow 184: before the counts are read, set the hash to the stats route and reload, then wait for the Stats page. Test only. Do not edit the route span code. The "Include dev" step still fails after this. The implementer diagnoses it from the error context and the Stats page code, and fixes it at its cause. If the cause is a product bug, the product is fixed so that the dev checkbox toggles. No wait, retry or timeout change.
- Flow 180 and the "no repro" flows get diagnostics only.
- macOS storm and burst flows need the cgo `gx` build. Keep each `expect` timeout below its test timeout so a miss names the step.

Vitest

- EpicPage tests: the render helper returns after both the list and the detail fetch settled, or waits on the card-dependent output.
- Routes test: wait on the text that proves the new epic painted, and clear `sessionStorage` between the two renders.
- ScreenModal: wait for the first load to show, then click.
- InterruptControl: keep the focus wait, drop the 5 s overrides.
- Menu timer: the focus-settle function returns a disposer. The menu calls it on unmount. Test setup cancels leftover module timers after each test.
- Closed without a test change: the ArchivePage and RestartDialog "spinner" and "once" tests (no repro), webui-flakes/01 (obsolete), test-health/03 (stale), follow-ups/518 (fixed).

Skills

- `gx-test-guidelines` becomes a skill directory with a description and no framework content. The portable skills are `gx-go-tests`, `gx-vitest-tests` and `gx-playwright-tests`. The implementer may pick different names within the `gx-` prefix. They hold no gx command. A new test enforces this.
- Content comes from the "Lessons" sections of the root-cause reports. Examples: wait on the signal that means ready, a counter must come from a real event, baseline timing, cleanups tolerate "already gone", inject a delay to prove a race, read error-context first, never wait a long time on a lost event.
- The Vitest skill is a copy of the person's dotfiles `vitest` skill, expanded. Deleting the dotfiles copy is a manual step (see Further Notes).
- The gx-only Playwright rules move to the repo-local skill `gx-only-e2es`. It is one real directory under `.claude/skills`, symlinked from `.agents/skills`, because Codex reads only `.agents/skills` in a repo. gx stops installing it into other projects. Its description names `webui/e2e/*.e2e.ts`, `playwrightguard` and `CATALOG.md`. The Testing section of `.ai/index.md` says to read it before touching a Playwright file. The `playwrightguard` failure message names it. A repo rules test guards the index line, the skill path and the symlink. The `gx-` prefix is not enforced on repo-local skills.
- `.ai/index.md` keeps gx policy and helpers (the process cap and headroom, `GOMAXPROCS`, `claudeenv`, `envguard`, `servertest`, `shortTempDir`, `gx brief` and `gx check`, the flow budget). The portable Go rules move out (state from `t.TempDir` and `t.Name`, short socket paths, injected clock, no real HOME or live machine state, no fake that hangs on idle paths). The extra `-exec scripts/proc-cap` text is removed where `gx brief` already applies the cap.
- `gx-implement`, `gx-tdd` and `gx-test-review` read the guidelines plus the matching framework skill and point to `.ai/index.md`. They stop naming `gx-playwright-e2es`. `skills/test_rules_test.go` gets new anchors. The guidelines heading test keeps its Why and Fix check but reads the rule count from the file.
- `gx-failing-tests.md` and `gx-flake-tickets.md` stay plain gx files because they name the tracker.
- `gx-test-review` rules: meaningful coverage, assertions that pass early, timing assumptions, isolation, cleanup, diagnostics. It judges against the shared skills and the project's test docs. The dotfiles `code-review` skill gets no Tests axis.

Guards

- Biome linter: `recommended` plus the `react` and `test` domains, run in `gx check` and CI. A trial found 270 errors and 760 warnings. `noNonNullAssertion` is off with a comment. Every other finding is fixed in the epic. No ratchet.
- Go rules tests (the `envguard` pattern, run in `gx check`): Playwright fixed sleeps, and fake `SetState` cleanups with no liveness check. The implementer picks whether they extend an existing guard package. Neither has an allowlist. Both ship green.
- Left to review: Go `time.Sleep`, negative Playwright assertions with no positive barrier, one-time `evaluate` reads, Vitest waits on the wrong fetch, leaked timers.

Measurement

- New index fields: attempt, a failure signature of about 200 characters, load at start and end, process count at start and end, origin (release or agent). Stable test names as in story 66.
- The headline is the first-attempt pass rate of full `gx check --full` runs on trunk. Guard tests (`sizeguard`, `sleepguard`, `oldwords`, `envguard`) are excluded. A pass on rerun counts as a failure. The same-tree fail-then-pass list per test is the diagnostic.
- Numbers are computed when read, from the index and `gx check` records. No new store or dashboard. A short script or a `gx brief` view prints the headline and the list. The implementer picks which.
- Slow tests no longer count as failures in the headline. A Vitest run that exits 1 with all tests green is recorded as a failed entry with a signature (for example the unhandled error text).
- The first impl ticket records the baseline (109 of 111 full runs failed) before any fix.
- Final validation: 20 consecutive full passes on finished trunk under normal load. Any failure is root-caused and fixed, and the count restarts.
- CI uploads `go test -json` and the Playwright JSON report as failure artifacts.

Tickets

- A fix closes the tickets it explains in place with `gx tickets set --status done` (map and backlog tickets) or `gx tickets close --reason fixed` (follow-ups), with a note linking the impl ticket. Duplicates close with `--reason duplicate` and the kept address. Obsolete tickets close with `--reason obsolete`.
- At the end, unexplained tickets move to `test-health` with `gx tickets move`. A move renumbers, so it waits until fixes have closed their tickets. The old map epics keep `destination_unconfirmed: true`.
- `gx-follow-up` and `skills/gx-flake-tickets.md` also search `test-health` and append a "seen again" entry to a match. New flakes go to follow-ups, then triage, then `test-health`.

Order

1. Baseline and measurement fields.
2. Close duplicates and obsolete tickets.
3. Regression fixes and the headroom change.
4. Go, Playwright and Vitest cause fixes, with the skill and guard work beside them.
5. The burst test step.
6. Moving survivors to `test-health`.
7. The 20-run proof.

## Testing Decisions

A good test here checks behaviour a person or the next test run sees. It does not check how the fix is written. The proof for a race fix is a delay injected at the seam that fails before the fix and passes after it, not a count of lucky runs.

Seams (existing ones, highest first):

- `gx check --full` on trunk, as measured by the `gx brief` index. This is the proof of the epic.
- Go package tests at each fixed behaviour: the Codex question tests (8 existing), the store-commit debounce test, `storecommit`, the launch and nudge tests, the budget tests, the pid watcher tests, the fixture package tests.
- Vitest files for each fixed Vitest group, the page-header test over the routes, the service worker precache test, the menu timer test.
- Playwright flows 152, 88, 117, 102, 24, 22, 184, 199, 155, 159, 72b, 158, 160, settings 200, menu 117, dev-mode and epics 2.
- Rules tests in `skills/` and the guard packages for the skills, the guards and the index line.

Rules:

- Always run Go tests capped and with `-count=1`. Use `gx brief` runs. Never run `go test ./...` locally.
- For a flake fix, show the failure by injection or by `--repeat-each` 18 to 20 at load 3 or more on the old code, and show 0 failures on the new code under the same load. One rerun of a rare flake proves nothing. Say so when the failure rate is too low to compare.
- The existing Codex and debounce tests are not changed. They already encode the contract.
- Prior art: the `envguard` and `sleepguard` rules tests, `playwrightguard`, the injected-delay runs in the root-cause reports, the existing guard in `skills/test_rules_test.go`.

## Verification

Regressions and fork errors (stories 1 to 11)

- Flows: none (no web change).
- Checks: `gx brief go-test ./server/sessions/prompt -run TestSessionAnswer_CodexQuestion -count=1` exits 0 with all 8 entries passing. `gx brief go-test ./server/lifecycle -run TestConfigReload_StoreCommitDebounceApplies -count=20` exits 0. `gx brief go-test ./storecommit ./server/settings ./server/index ./server/lifecycle -count=1` exits 0. `rg "resource temporarily" ` over the logs of two full `gx check --full --kind test` runs finds nothing, one run taken while another session runs a Go test job. `scripts/proc-cap --help` and the script show a default of 800. The burst test appears in its own step that starts after the others end.
- New to write: the "fresh store poll catches a lost watch event" Go test if the existing debounce test does not cover the re-arm (story 6).

Go readiness (stories 12 to 26)

- Flows: none.
- Checks: `gx brief go-test ./server/sched/launchtest -count=10` exits 0. `gx brief go-test ./harness -count=1` exits 0 and includes the new "fresh idle does not move Turn" test. `gx brief go-test ./testutil/gxfixture ./server/lifecycle ./server/budget -count=1` exits 0. The delay-injection runs from the reports pass with the fix. The guard test for unguarded `SetState` cleanups passes (see guards).
- New to write: tests named in the decisions (pid watcher, lock write, seeder by id, budget wait, countdown hold).

Playwright (stories 27 to 41)

- Flows: `gx brief -C webui playwright agents-commands.e2e.ts 152`, `agents-create.e2e.ts 88`, `codex.e2e.ts 117`, `login.e2e.ts 102`, `phone.e2e.ts 24`, `jump.e2e.ts 22`, `stats.e2e.ts 184`, then the stale flows (199, 155, 159, 72b, 158, 160, settings 200, menu 117, dev-mode, epics 2). All exit 0. The same flows pass 20 times with `--repeat-each` at load 3 or more for 24 and 22.
- Checks: `gx brief -C webui vitest src/pwa` exits 0 with the new body-read test. The route page-header Vitest test exits 0.
- New to write: the route-header Vitest test (story 31), the precache order test (story 37).

Vitest (stories 42 to 48)

- Flows: none.
- Checks: `gx brief -C webui vitest src/epic src/routes src/ui` exits 0. Ten loaded full Vitest runs print no `Errors` line. The new timer-count test passes.

Shared skills and review (stories 49 to 59)

- Flows: none.
- Checks: `gx skills list` shows the new portable skills and does not list `gx-only-e2es`. `gx brief go-test ./skills -count=1` exits 0 (no gx command in the portable skills, new anchors, heading count). The `.agents/skills/gx-only-e2es` symlink resolves to the `.claude/skills` directory. `.ai/index.md` has the Testing line that names it.
- New to write: the rules tests in the decisions.

Guards (stories 60 to 64)

- Flows: none.
- Checks: `gx check` runs Biome lint and exits 0 on trunk. Adding a `waitForTimeout` or a raw `SetState` cleanup in a scratch copy makes the matching rules test exit 1 and name the file.

Measurement (stories 65 to 73)

- Flows: none.
- Checks: after a `gx brief` run, the newest line of the brief index holds `attempt`, `signature`, load, process count and `origin`. A Vitest run with an injected unhandled error is recorded as failed. The measurement command prints the first-attempt pass rate and the flip list. The slow-test count no longer appears as a failure in the headline. After the 20-run proof, the command shows 20 consecutive passes.
- New to write: the measurement command or view.

Tickets (stories 74 to 77)

- Flows: none.
- Checks: `gx tickets show` of a fixed flake ticket shows a done status and the impl link. After the final step, the old flake epics list no open tickets and `test-health` holds the survivors. `skills/gx-flake-tickets.md` and `gx-follow-up` name `test-health`.

## Out of Scope

- The dotfiles `code-review` skill. It is deprecated in favour of `gx-code-review` and gets no Tests axis.
- Raising timeouts across the board, or running whole suites one at a time, without evidence for each case.
- Retries in required CI. A diagnostic lane with retries may come later.
- A fix for the `holder.sock never answered` family, the Go Group F tests, the slow `TestDeadMany`, Playwright flow 180 and the InterruptControl lost Esc. They get diagnostics only until one recurs.
- Changing the number of Playwright workers, quarantine lanes, or adopting `testing/synctest` across the suite.
- A gx verb for `proc-cap`. A follow-up idea in `gx:follow-ups` is filed instead.
- Closing the old map epics. gx has no verb for it and an empty epic costs nothing.

## Further Notes

- Manual step: after `gx-vitest-tests` ships, the person deletes the `vitest` skill from the dotfiles. The epic does not touch the dotfiles.
- The research reports under `docs/research/test-*.md` hold the evidence and the exact failure text for each cause. The impl tickets link to them.
- Findings and hypotheses stay apart. A cause is called proven in a ticket only after the run that shows it. The reports mark which causes are proven and which came from a code read.

# gx:test-suite-stability/10 - Playwright root causes

Question: for each of the 15 Playwright tickets in `test-flake-inventory.md`, what is the root cause and what is the fix?

Date of runs: 2026-11-17, worktree `test-suite-stability-design`, HEAD `10bf928a5`. Merge base with main: `74bfde98f`. Nothing in the tree was changed. Every fix below was tried on a copy in the scratchpad.

## Answer in short

- Today a full run (`gx brief -C webui playwright --all`, 3 workers, load 2 to 4) gives 15 failed, 137 passed, in 11 minutes. The suite has 152 flows, not 310. (The brief headline said "24 failed"; the log says 15 failed. The 9 extra are slow-test lines, not failures.)
- Of the 15 failures: 8 are **stale expectations** (199, 155/159, 72b, settings 200, menu 117, 158, 160, and the menu step of epics 2): a product change was on purpose and the flow was not updated. Six of these are verified green with a scratch edit; 158 and 160 need a rewrite of a long tail.
- 3 flows (152, 88, 117) share **one real product bug**: on phone and tablet width the "Pick a session" Agents page covers the sidebar burger.
- 3 flows (login 102, dev-mode, and the PWA step of epics 2) share **one unresolved cause**: the service worker never activates.
- 2 flakes reproduced under modest load and have a proven cause and a validated fix: flow 24 (state read without a wait, 3 of 20 fail, 0 of 50 after the fix) and flow 22 (navigation overwritten by an async redirect, 2 of 18 fail, 0 of 36 after the fix).
- The two older pause tickets are **obsolete**: the storm flow was fixed by `ca62bbde0`. I proved the cause by undoing it. "Every pause reason" does not reproduce (13 of 13 pass, up to load 28).
- Not explained: flow 184 (deterministic, counts `[1,1,1,0,0]`), flow 180 (failed once in the full run, 6 of 6 pass alone) and the fixture-never-starts family (test-health/04).

## Method

- Read `skills/gx-playwright-e2es`, `gx-ui-check`, the inventory, and the two Go readiness reports.
- Ran the whole suite once at low load, then each failing flow alone, then read `error-context.md` before anything else (the skill's own rule).
- Reproduction and fixes: a copy of `webui/e2e` in the scratchpad (`pw/`), with `src` and `node_modules` symlinked, `global-setup.ts` pointed at the worktree and `run.sh` as the runner. It builds the binaries once per run like the real suite. The copy must run with cwd at the repo root, or the `dev-mode` scenario cannot `go build` its fake `pnpm` (that failure prints `gxfixture exited with 1` and then a `sessionStorage` SecurityError; see Lessons).
- Load: `GX_PROC_HEADROOM=600` throughout. Machine load average at each step is in the table. Synthetic load for the last two runs: 16 busy loops on 12 cores (load 22 to 28).
- Bisect: not done by building old trees (a build costs about 2 minutes per commit). First bad commit is named from `git log -S` plus the code and the snapshot. Each is marked "code read".
- Scratchpad: `/private/tmp/claude-501/-Users-david-dev-gx-test-suite-stability-design/d1536974-9324-48ad-950d-7804404bd1b2/scratchpad/` (`full1.out`, `tickets.txt`, `pw/run.sh`, `pw/e2e/*`, `pw/load1.out`, `pw/load2.out`).

## Facts: today's full run

| Run | Result |
| --- | --- |
| Full suite, 3 workers, load 2 to 4 | 15 failed, 137 passed, 11.0 min. Log `20261117T073300.488564000-playwright.log` |
| Failed: 155/159, 152, 88, 160, 158, 117 (codex), 72b, menu 117, menu 180, settings 200, 199, 184, dev-mode, epics 2, login 102 | |
| Passed today (they have tickets): 24, 22, 198, pause "process storm", pause "every pause reason", 118, 76, bulk, 189, newdesign 26, phone 41 | |
| Slow only: pause "live task" 68 s, "live task dirty worktree" 152 s | |

## Table

| Ticket | Cause | Evidence | Fix | Validation |
| --- | --- | --- | --- | --- |
| webui-test-flakes/01, follow-ups/519 (flow 199) | **Stale expectation**, 4 places. `a67e45462` (Nov 16 12:24, code read; it edited the flow in part) removed `RoundCard`. The decided list and the Route summary now live in the Chat tab; the page opens on Questions. The question text also gained a `Q1 - ` prefix. | Alone: fails at line 73, snapshot shows the Questions tab and "Q1 - Fix it in the parser or the caller?". The three later failures show only after the earlier ones are fixed. | Click the `Chat` tab before the three `Decided unless you object` / `move ... -> backlog` asserts and the `Questions (` tab after. Match the card by `/^Q1 - Create the backlog "tooling"\?$/`. | Scratch copy: 1 of 1 pass, 53 s, load 2.4 |
| webui-test-flakes/02, webui-flakes/03 (flow 24) | **State read without a wait.** After `setViewportSize(390x844)` the test reads four footer boxes one after the other (`boundingBox`). The footer reflows on the next frame, so one button can still sit on a second row (centre 32 px lower). | 3 of 20 failed alone-ish at load 2.5 to 5 (`toBeCloseTo` 816 vs 784 at `phone.e2e.ts:271`). Matches the ticket (32 px). Passes in the full run. | Read all four boxes together inside `expect.poll` until the centre spread is under 0.5 px, then assert. | Same load, `--repeat-each`: base 3 of 20 failed; fixed 0 of 20 and 0 of 30. One extra failure in the first fixed batch was a `toBeVisible` I did not capture (not repeated in 50 runs) |
| webui-test-flakes/03 (pause "a process storm recovers") | **Obsolete (fixed).** On macOS the process table is read through cgo only. A `gx` built with `CGO_ENABLED=0` cannot see the storm, so "Too many processes" never shows. `ca62bbde0` (Nov 16) builds a cgo `gx` for `*-storm` scenarios. All the ticket's sightings are Nov 10 and 11. | Passes 3 of 3, 16 s. Undo test: scratch `fixture.ts` forced to the non-cgo binary: fails with `getByText('Too many processes')` not visible, the ticket's exact signature. | None. Close as fixed by `ca62bbde0`. Optional: make the `expect` timeout shorter than the test timeout (150 s against 60 s) so a future miss names the step. | Alone x3 pass; undo test fails |
| webui-test-flakes/04 (flow 158, "empty chat after answered card") | **Not reproduced; likely obsolete.** The Agents page chat was rewritten to the question sheet (`ff2d23337`, Nov 11). Visits to later session pages render in today's run up to line 376. Hypothesis only: the empty chat came from the old card path. | 158 reaches line 376 with correct pages after two stale locators are fixed (see 158 below). | None proven. Do not add a flow. | n/a |
| webui-test-flakes/05 (flow 22 jump) | **Navigation overwritten by an async redirect.** The palette loop ends on `goto("#/settings")`. The Settings page redirects to `#/settings/s/<first>` after it loads. The flow then sends `goto("#/d/demo:design-live")` without waiting, and the late redirect wins. The next click (`Composer` > `Note`) never finds the Design page. | 1 of 6 and 2 of 18 failed at load 4. Snapshot of the failure is the Settings "Agents" page. | Before the next `goto`, `await expect(page).toHaveURL(/#\/settings\/s\//)`. | Base 2 of 18; fixed 0 of 18 twice (0 of 36), load 3.4. Also 4 repeats under load 22 to 28 pass |
| webui-flakes/02 (flow 198) | **Not reproduced.** The ticket's sightings are Nov 9 and the flow shared a fixture failure with other tickets. Passes in the full run, alone and 3 of 3 under load 22 to 28. | See Validation. | None. | Alone 17.7 s; full run; load 22 to 28 x3 |
| test-flakes/07, gxfixture-load-flakes/01 (pause "every pause reason recovers") | **Not reproduced.** The Nov 4 failure (240 s on the `Reason` textbox) predates the flow's own limits (120 s test, 45 s Approve wait). 3 of 3 alone, 3 of 3 under load 3.5, 3 of 3 under load 22 to 28 (about 50 s each). | See Validation. | None. Duplicate: close one of the two. | Same as evidence |
| test-flakes/08 (flows 155 and 159) | **Stale expectation (bad rename).** `210fa8c64` (Nov 16) renamed the Agents form button "New session" to "New chat" and also changed two lines of this flow that check the **failure banner**, whose button is still "New session" (`FailureBanner.tsx:45`). The ticket's own text (`never reached Unread`, a `sessionStorage` error) did not recur. | Full run: line 168 `New chat` not found; snapshot shows `button "New session"` in `Session failure`. | Lines 125 and 168: `New session`. | Scratch: 2 of 2 pass (155/159 and the 12-line flow) |
| test-health/04 (flow 158 fixture never starts) | **Fixture or readiness, not reproduced.** 0 of 5 full and load runs today show `never reached a agent message`. The Go readiness reports show the same family in Nov 6 to 15 logs, before the launch cap and parallel cap. The `sessionStorage` SecurityError is a side effect: when `gxfixture` exits 1 the page is still `about:blank`. | I produced the pair on purpose by starting the fixture from a dir with no `go.mod` (see Lessons). | None proven. | n/a |
| test-health/08 (flow 117 codex) | **Real product bug**, same as 152 and 88. At 600 px the "Pick a session" page covers the burger (see group below). The first click lands before the page is pinned, so the sidebar toggles late or not at all. | Snapshot at failure: only the `Show sidebar` button and `Pick a session`. Probe at 600 px: covered by the page's `DIV`. | Product fix, below. | Probe: covered at 390 and 600, free at 1000 and 1280 |
| test-health/08 (Dev mode PWA step) | **Unresolved** (service worker, see group below). | `workerState` is `none` at `dev-mode.e2e.ts:117` | see below | see below |
| test-health/17 (25 flows) | A list. Today: 152, 88, 117 (product bug); 72b, epics 2 menu, bulk (pass), 76 (pass), 189 (pass), phone 24 (flake, fixed above), 41 (pass), newdesign 26 (pass), 118 (pass), dev-mode, epics 2 PWA (service worker). | Full run. | Split by cause as in this table: stale (72b, epics 2 menu), product bug (152, 88, 117), service worker (dev-mode, epics 2, login). | n/a |
| test-health/19 (many flows) | A list. Today: 160, 158, menu 117, 180, settings 200, 184, 199, 155, 72b, login, dev-mode, epics 2, 152, 88, 117. The load part: 24 and 22 are the only flows that failed from timing at load 2 to 5. | Full run plus repeats. | Same split. Do not raise timeouts. | n/a |

## Groups with a shared cause

### Group A: the Agents "Pick a session" page covers the burger on phone and tablet (flows 152, 88, 117) - real product bug

- Fact: `AppShell.tsx:364` renders the burger (`MascotToggle`) in its own 48 px row when no page header is present (`headers === 0`). `AgentsPage` with no session shows only an `EmptyState` and no header.
- Fact: `DetailPage` with `fill` is `position: fixed` on phone and tablet (`c6787d0b8`, Nov 11, "Pin fill pages to the window"). Its `top` is `--shell-strips-height`, which does not include the burger row.
- Fact: probe at 390 and 600 px after 1.5 s: `elementFromPoint` at the burger centre returns the page's `EmptyState` div; at 1000 and 1280 px there is no burger. Immediately after load the burger is still on top, which is why the first click sometimes works (117 fails on the next step instead).
- Fact: Playwright's own call log says `<div class="css-5ptl4y"> from <div class="css-6a99bx"> subtree intercepts pointer events`.
- Hypothesis: a person on a phone cannot open the sidebar from `#/agents` with nothing selected. Not tried by hand.
- Fix (product, not test): include the burger row in the pinned page's top offset, or give `EmptyState` a header that carries the burger. Add a Vitest test on the page's pinned offset.
- Validation not run (needs a product change and a rebuild). After the fix: flows 152, 88, 117 alone, then the full run.

### Group B: stale expectations after on-purpose product changes

| Flow | Product change (first bad commit, code read) | Fix |
| --- | --- | --- |
| 199 | `a67e45462`, see table | see table |
| 155/159 | `210fa8c64`, over-wide rename | `New session` at lines 125 and 168 |
| 72b | Archive added to the item menu `438c82bb7` (Nov 11) | `toHaveCount(5)` |
| epics 2 (menu part) | same | add `"Archive"` to the expected list |
| settings 200 | `245984e5c` adds the Mental health category | count 7, add `Mental health` |
| menu 117 | `612e6cab2` (Nov 12): each bar shows the tighter of the 5h and weekly windows | Fresh shape: C bar is `waiting`, 81. After `payload(55, 96)` it is `deadlocked`, 96. The old last step (`85, 90` expects `waiting`, 85) cannot happen; assert the 96 bar |
| 158 (two locators) | The question card's button name is `Note on question <number>`, not the question title | `Note on question 2` and `Note on question 1` |
| 158 tail (line 374 on) and 160 (line 240) | `f53612a34` (Nov 12): when a card has its own Dismiss and terminal buttons, the blocked notice with Deny and Allow once is hidden (`sheetCoversBlock`). A readable Bash prompt now yields a card (Yes, No, Submit) and no notice | Rewrite the tail to answer through the sheet; keep the notice asserts only for prompts with no card (unreadable text, Codex). About 100 lines; not done |

Validation: scratch runs, 117 (menu) 1 of 1 pass, 72b and 200 pass, epics 2 passes its menu step, 158 reaches line 376 where the tail starts. 160 and the 158 tail are not verified.

### Group C: service worker never activates (login 102, dev-mode, epics 2) - unresolved

- Fact: all three use `serviceWorkers: "allow"` and wait for `registration.active.state === "activated"`; it stays `none`.
- Fact (CDP `ServiceWorker` events): `register("/sw.js")` succeeds, the worker reaches `running/installing`, hangs for longer than 8 s, then `event.waitUntil Promise rejected` and `Uncaught (in promise) TypeError: Failed to fetch` at `sw.js:125` (the precache).
- Fact: the same 280 URLs fetched later, in parallel, return 200 in 26 to 57 ms from the page, from a dedicated worker and from the service worker itself once it exists. The server is HTTP/1.1 over TLS (`nextHopProtocol`). `requestfailed` events show nothing.
- Fact: `precache` (`workerCore.ts:38`) holds every response and fails the install on one `TypeError`.
- Not shown: which URL fails, and why only during install. Tried and ruled out: concurrency of 280, unread bodies held in an array, certificate flags.
- Fix to try: log the URL that throws in `precache` (one temp edit) and look at the server's log for the same instant. Then decide between bounded concurrency and a server fix. Do not add a wait.

### Group D: unexplained

- Flow 184 (stats): deterministic alone (68 s). Web-performance counts after the hide event are `[1, 1, 1, 0, 0]` for LCP, FCP, CLS, app_ready, INP. Only one sample (`route_ready`, trigger click) was POSTed before the hide. So no `app_ready` sample and no INP sample ever reach the server. Hypotheses: the hard-load `app_ready` is dropped (`hardDropped` or `skipFirst` in `routeSpans.ts`, refactored in `45ab6e5d4`), or INP needs a real input event the synthetic click does not produce. The code review of telemetry/17 already flagged this flow as unverified. Next step: run `vitest src/routeSpans` and `webPerf` with a hard-load case, then log the samples POSTed.
- Flow 180 (menu burn): failed once in the full run (60.9 s, Codex weekly 55% not shown). 6 of 6 pass alone in 12.6 s, 3 workers. Not explained. The wait is event-based (Codex scan saves a sample); the scan's period is not read.

## Lessons for the portable Playwright skill

- Read `error-context.md` first, and fix the **first** failure only as a hypothesis: a long flow stops at its first stale line, so the next three are invisible. Flow 199 had four stale steps and flow 158 about a hundred lines more. Keep a flow short enough that one failure does not hide the rest.
- A hash change or `goto` is not "done" when the call returns. A route that redirects after its data loads (`<Navigate replace>`) can overwrite the next navigation. After navigating, assert the final URL or a landmark of the destination before the next action.
- Reading several boxes one after another after a viewport change reads different frames. Read them together inside `expect.poll` (a poll is a retrying assertion, not a retry of the test).
- A test that fails after a rename was often fixed by a search-and-replace that also hit a different control with the same old word. Review renames in e2e files by diff.
- Visible-and-enabled is not clickable: `position: fixed` pages and overlays can cover a control that is in the accessibility tree. Playwright's call log names the interceptor; report that as a product bug, do not wait or `force`.
- `gxfixture exited with 1 before it printed its output` followed by `sessionStorage` SecurityError is one failure, not two. The second is the page still at `about:blank`. Read the line above it.
- When a test is fixed by a product change that is on purpose, the same commit must update the flow (search `webui/e2e/` for the old text). Four of today's stale flows were hit by changes one to five days old.

## Lessons for `gx-playwright-e2es`

- Add: "A navigation helper returns before the page settles. After `goto(page, hash)` to a route that redirects (`#/settings`), wait for the final URL before the next `goto`." (flow 22).
- Add: "Several `boundingBox` reads after `setViewportSize` go in one `expect.poll`." (flow 24).
- Add: "A control covered by a fixed page is a product bug. Do not use `force` or a wait." (flows 152, 88, 117).
- Add: "When a new menu item, tab or settings category lands, search `webui/e2e` for the counts of its siblings (`toHaveCount(4)`, the category list)." (72b, epics 2, settings 200). Prefer `toContainText` of the named items over a count.
- Add: a note that macOS storm and burst scenarios need the cgo `gx` build (`GX_E2E_GX_CGO`). Without it the watchdog cannot read the process table and the flow fails with a 150 s wait (storm). Keep `expect` timeouts below the test timeout.
- Add: how to run a scratch copy of a flow: copy `webui/e2e` to the scratchpad, symlink `src` and `node_modules`, patch `global-setup.ts`, and run with cwd at the repo root (the `dev-mode` scenario builds a fake `pnpm` relative to it).
- Existing rule "an unrelated failing flow: rerun once" held up: of the 15, 8 failed the same way when run alone. None of the 15 was a plain load flake that a rerun would have cleared; the two real flakes needed 18 to 20 repeats to show. Add: "a flake ticket needs `--repeat-each` 18 to 20 at load 3 or more; one rerun of a rare flake proves nothing".
- Add the "brief headline counts slow tests as failures" note, or fix the headline: `24 failed` in the summary against `15 failed` in the log cost time here.

## Fix list for the implementation epic

1. Update flows 199, 155/159, 72b, epics 2 (menu), settings 200, menu 117, 158 (locators and tail), 160 (tail). Test only.
2. Group A product fix (burger row under the pinned page) plus a Vitest test, then flows 152, 88, 117 stay as they are.
3. Flow 24: poll the footer boxes. Flow 22: wait for the settings redirect.
4. Service worker install: diagnose with the failing URL logged (Group C).
5. Flow 184 and flow 180: diagnose (Group D). Do not raise either timeout.
6. Close as fixed or obsolete: webui-test-flakes/03, webui-flakes/02, test-flakes/07 and its duplicate, webui-test-flakes/04 (no repro). Close nothing from here; the epic decides.
7. Do not add retries or longer timeouts. None of the causes above was slowness except possibly 180.

## Hypotheses (not proven)

- H1: the service worker install fails only for one precache URL or only while the page is still loading. Not tested.
- H2: flow 180's Codex scan period is long enough that a loaded run misses the 60 s wait. Not tested; 6 of 6 alone pass.
- H3: flow 184 loses `app_ready` through `hardDropped`. Not tested.
- H4: webui-test-flakes/04's empty chat came from the old card code and is gone. Not tested by hand.
- H5: the older flow 198, pause "every reason" and test-health/04 failures were load plus the pre-cap fork storm that the Go reports describe. Not tested; zero recurrences in 5 runs today, 2 of them at load 22 to 28.

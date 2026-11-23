## Problem Statement

A developer runs `gx check --full` on main and the Playwright step is red or fragile. Measured on main (three full runs, 157 flows, none over 45 s):

- Six flows fail. Four fail every run: health 81 and 82 (Tray popover, Tray offline), united 16 (Needs you), epics 2 (Epic looks right) and chat-draft 209. Two fail under load: agents-session 75 and chat-images 189.
- The causes are in the tests and the fake agent, not in the product:
  - Health and united look for footer `img` roles that a status menu replaced.
  - Epics 2 never gets gx's own summary entry, because the fake agent's notes lack the `Red-first` bullet that the finish guard demands.
  - Chat-draft 209 fails when the test process runs inside an agent session. Its `GX_AGENT_SESSION` leaks into the fixture's CLI, and the CLI hides the server's reason behind `refused (bad-request)`.
  - Flows 75 and 189 wait for the session to be `working`. The fake is working for only about 500 ms, so under load the poll never sees it. Flow 189 has a second cause: the fake starts no turn for `/compact foo`, although the server and real claude treat it as a turn.
- A fresh worktree has no Playwright browsers, so the first e2e run fails.

## Solution

After this work, `gx check --full` on main passes its Playwright step three runs in a row, with Go and vitest running at the same time as today. Every flow finishes under 45 s in those runs. Retries stay off. Each failing flow is fixed at its cause. No flow is deleted.

A fresh iteration worktree installs the Playwright browsers on its own, for projects that declare it. `go test` and the e2e run need no other setup: `webui/dist` is built by the e2e global setup, and `go test` compiles with the tracked placeholder.

## User Stories

1. As a developer, I want the Playwright step of `gx check --full` to pass three runs in a row on main, so that a red check means a real regression.
2. As a developer, I want every flow to finish under 45 s in a full run, so that no flow counts as not done.
3. As a developer, I want retries to stay off, so that a flaky flow stays a hard fail and is fixed at its cause.
4. As a developer, I want the health Tray popover flow (81) to pass against the footer status button, so that the footer geometry and health snapshots stay covered by a real browser.
5. As a developer, I want the health Tray offline flow (82) to assert the status button's accessible name before and after the server stops, so that the offline state is checked together with high health.
6. As a developer, I want the united Needs you flow (16) to find the paused-queue state on the status button by name, so that the real `queue pause` over SSE stays covered.
7. As a developer, I want the status-button checks to match by regex where health can add text to the name, so that the flows do not break when both states are shown.
8. As a developer, I want the epics flow (2) to see gx's own summary entry, so that the Latest summary shows all eight terms.
9. As a developer, I want the fake agent's notes to include a `Red-first` bullet, so that the finish guard lets the iteration settle in every flow and Go test that uses the fake.
10. As a developer, I want the fake agent's own test to assert the `Red-first` bullet, so that the fake cannot drift from the guard again.
11. As a developer, I want the chat-draft flow (209) to pass whether or not the test process runs inside an agent session, so that agents can run `gx check --full`.
12. As a developer, I want the fixture to drop the test process's `GX_AGENT_SESSION` once, for every flow, so that no flow needs its own patch.
13. As a developer, I want flows that need a source session to keep setting `GX_AGENT_SESSION` explicitly for one CLI call, so that the fork-draft and step flows still work.
14. As a user of the CLI, I want a refused draft command to print the server's reason, for example `refused (bad-request): the source session was not found`, so that I can tell what to fix.
15. As a user of the CLI, I want the same reason for a refused draft delete, so that both draft commands tell the same story.
16. As a developer, I want the `ask` helper in the agents flows to wait for the fake's turn end, so that agents-session 75 and every other `ask` caller stops depending on a 500 ms window.
17. As a developer, I want the fixture to expose a turn-ended check that reads the fake's pid file for a chat's conversation, so that flows wait on the same signal as the Go seeder.
18. As a developer, I want the chat-images flow to wait for the turn end before it counts images, so that a stale `idle` from before the turn cannot pass the wait.
19. As a user, I want `/compact foo` to start a turn in the fake agent like a bare `/compact`, so that the fake matches the server and real claude.
20. As a developer, I want flows 189 and agents-commands-send to wait for the turn end after a `/compact` send, so that they stop polling for a `working` state that may never be seen.
21. As a developer, I want those flows to keep their input-count assertion, so that a double send still fails.
22. As a developer, I want the composer-images "keep busy" step to use the slow marker, so that the turn stays busy and the `working` poll has no race.
23. As a developer, I want a fake-agent test that `/compact foo` at idle goes busy like `/compact`, so that the gap stays closed.
24. As a developer, I want a fresh iteration worktree to install the Playwright browsers, so that the first e2e run does not fail on a missing executable.
25. As a developer, I want a project to declare its worktree setup as a `setup` list of shell commands in its checks config, so that only projects that need setup pay for it.
26. As a developer, I want gx's own list to hold the Chromium install command, so that gx worktrees get the browser they test with.
27. As a developer, I want a project without the list to add no start time to its worktrees, so that setup stays cheap.
28. As a developer, I want the setup to run after the dependency install in the same prepare step, so that the install tools exist when setup runs.
29. As a developer, I want a failed setup command to fail the prepare step like a failed dependency install, so that the worktree is discarded and its log is kept.
30. As a developer, I want design and hand-made worktrees to keep using `pnpm run e2e:install`, so that setup runs only where it is declared.
31. As a developer, I want `go test` to compile in a fresh worktree with no `webui/dist` build step, so that no setup builds the web app for Go tests.
32. As a developer, I want the e2e global setup to keep building a stale or missing `dist`, so that no setup builds the web app for e2e runs.
33. As a developer, I want the flow budget (`maxFlows`) and the flow catalog to stay as they are, so that no coverage is dropped to get green.

## Implementation Decisions

- Footer flows (health 81 and 82, united 16): keep all three flows and update their selectors. The status button replaces the retired `img` roles.
  - United 16 matches the paused-queue state with a button whose name matches `/Server queue is paused/` as a regex, because the name can carry "; System health ..." when health is also high.
  - Health 81 changes only at its offline step.
  - Health 82 does not assert that the status button is absent or count zero. `setHealth("cpu=75 mem=85")` already makes health high, so the status button exists before the stop. The flow asserts the accessible name on the status part: "System health is high or critical" before the server stops, then "Disconnected from the server; System health is high or critical" after. The workspaces-touch flow already asserts this way.
  - The component's vitest test stays as the mocked-props check.
  - The flow catalog needs no edit.
- Epics 2: the fix is in the fake agent, not in the product and not in the test. The fake's notes writer emits `- Red-first: n/a: fake agent` so the finish guard accepts the summary. The worker test for the notes-then-finish order asserts the bullet. Other users of the same fake (the `verify-notes` flow in the pause flows and the progress scenario Go test) get the fix with the fake.
- Fixture env: the fixture builds the CLI env from the test process env plus the fixture's own entries. It drops `GX_AGENT_SESSION` from the base env, once. Flows that need a source session already pass it per call through the fixture's env-override CLI helper (the workspaces, design and task-list flows). Flow 209 is never patched alone.
- CLI refusal message: the server already sends the reason text in the draft result's message, and the CLI drops it. The refusal accessor on draft results returns the refusal, the reason and the message, the same shape as the web-info and hosted-resolve results. The result-print path for draft create and for draft delete prints `refused (<reason>): <message>`. The draft-result interface, both result types and their tests change.
- Fixture turn-ended check: a new fixture method, the TypeScript twin of the Go `agentfake.TurnEnded`. It reads the fake's pid files under the fixture HOME's `.claude/sessions` and returns true when the file whose `sessionId` equals the chat's `conversation_id` has status `idle`. The fixture's server and fake env drops every `CLAUDE*` variable, so the pid files always land under the fixture HOME and the fixture HOME path is the right place to read.
- `ask` helper: replace the `working` poll with a poll on the turn-ended check. Keep the final poll that waits for the server to show `idle`. The `ask` helper already waits for the reply count, and the fake appends the reply after it sets busy, so the turn has provably started. The change covers every caller (agents-create, composer, host-files, context-window, agents-session). The chat-images flow's wait near its image count also uses the turn-ended check.
- Fake `/compact`: the TUI fake treats `/compact` and `/compact <args>` as plain prompts, mirroring the server rule for single-send commands. A `/compact foo` at idle plays a normal chat turn.
- Flows after a `/compact` send (chat-images 189, agents-commands-send): drop the `working` poll. After the send, wait on the turn-ended check, then keep the existing input-file assertions. An input count of 1 still proves there was no double send.
- Composer-images "keep busy": send `keep busy [slow]` (the slow marker). The turn stays busy, so the `working` poll has no 500 ms window. No finish control is needed, because the fixture is torn down after the flow.
- Worktree setup:
  - A project declares a `setup` list in its checks config: shell commands that gx runs in the worktree, in order, after the dependency install.
  - A project without the list gets no setup and no added start time.
  - Only iteration worktrees run it, in the same prepare step as the dependency install. Design and hand-made worktrees keep `pnpm run e2e:install`.
  - gx's own list holds `pnpm -C webui exec playwright install chromium`.
  - There is no extra skip check: `playwright install` skips a browser build already in the cache.
  - A failed setup command fails the prepare step: the worktree is discarded and the log is kept, as for a failed dependency install.
  - The config key is `setup`. The implementer picks the loader and the type names.
- `webui/dist`: no setup step. The tracked `.gitkeep` lets `go test` compile with the embed's placeholder. The e2e global setup already builds a stale or missing `dist` before every run.
- Flow count: no flow is deleted, so `maxFlows` (157) stays.
- Retries stay off in the Playwright config.

## Testing Decisions

- A good test checks external behavior: what a flow shows in the browser, what the CLI prints, what the fake does when prompted. It does not check how a helper is built.
- Seams, highest first:
  - The Playwright flow catalog, run through `gx check --full`. It covers stories 1 to 13, 16 to 18, 20 to 22 and 33.
  - The fake agent's Go tests: the worker notes test (Red-first bullet) and a new TUI fake test (`/compact foo` goes busy at idle).
  - The CLI command tests for draft create and delete: refusal prints the reason and the message. One new test covers the message.
  - The ralph-loop install test for the setup step: runs the list after the dependency install, fails the prepare step on a failed command, runs nothing when the list is absent, and does not run for design or hand-made worktrees.
- Prior art: the go-test-flakes work for the turn-end wait (`TurnEnded`), the workspaces-touch flow for status-button name asserts, the fork-draft flows for per-call `GX_AGENT_SESSION`, and the existing install tests for the dependency step.
- Go tests run through `scripts/proc-cap`, and follow the Go test rules (readiness waits, no fixed sleeps). Playwright changes follow the Playwright test rules.

## Verification

Flows run against a temporary gx server. Stories that share a flow are grouped.

Gate (stories 1 to 3, 33):
- Run `gx check --full` three times on the finished branch with Go and vitest running at the same time. The Playwright step passes all three runs, and no flow reports "slow" over 45 s.
- Confirm the Playwright config sets no retries. Confirm `maxFlows` is still 157 and the flow guard test passes.
- Out of the check: the `format` step and the Go step's flaky tests are not part of this map (see Out of Scope).

Footer flows (stories 4 to 7):
- Run catalog flows health 81, health 82 and united 16. All pass.
- Flow changes: the selector and name asserts above.

Epics (stories 8 to 10):
- Run catalog flow epics 2. It passes with eight terms in Latest summary.
- Run the fake agent worker notes test and the pause flows' `verify-notes` flow. Both pass.
- Run the progress scenario Go test. It passes.

Chat draft and CLI message (stories 11 to 15):
- Run catalog flow chat-draft 209 from a shell with `GX_AGENT_SESSION` set to a value that is not in the fixture. It passes.
- Run the workspaces, design and task-list flows. They still pass, so the per-call override still works.
- In a fixture, run `GX_AGENT_SESSION=missing gx agents drafts create --project demo --name remove-me --brief -` (stdin empty or a short brief). Expect exit 1 and output containing `refused (bad-request): the source session was not found`.
- Run a draft delete that the server refuses. Expect its refusal output to carry the reason and the message too.
- New tests: one for the refusal message in the agents and designs draft command tests.

Turn waits (stories 16 to 18):
- Run catalog flows agents-session 75, agents-create, agents-composer, host-files, context-window and chat-images 189, each three times in a full run. All pass.
- New code: the fixture turn-ended check.

Compact (stories 19 to 23):
- Run catalog flows chat-images 189, agents-commands-send and composer-images "keep busy". All pass.
- Run the new TUI fake test: `/compact foo` at idle goes busy. It passes, and the bare `/compact` test still passes.

Worktree setup (stories 24 to 32):
- Create an iteration worktree for a gx epic ticket in a project whose Playwright cache lacks the browser build (or point `PLAYWRIGHT_BROWSERS_PATH` at an empty directory). The prepare step runs the Chromium install and the first `gx check --full` e2e run does not fail on a missing executable.
- Create an iteration worktree for a project with no `setup` list. The prepare step runs no extra command and its time matches the dependency install alone.
- Set a `setup` entry that exits non-zero in a scratch project. The prepare step fails, the worktree is discarded and the log file is kept.
- Create a design worktree. It runs no `setup` list.
- In a fresh worktree, run `go test` on a package that embeds the web app with no `webui/dist` build. It compiles and runs. Run an e2e flow. The global setup builds `dist` first.
- New tests: the ralph-loop install test cases above.

No mock states: this map has no stored mock.

## Out of Scope

- A scheduled full check on trunk that reports a red run. Keeping main green after this map ships is a separate feature.
- Playwright retries for flaky flows. A flaky flow stays a hard fail.
- The `format` check failure (Biome wants a vitest file reformatted) and the flaky Go tests seen in the full runs. The destination covers only the Playwright step. go-test-flakes covers the Go seeder.
- The shared Playwright cache pruning a browser mid-run when worktrees pin different Playwright versions.
- Building `webui/dist` in worktree setup, and setup for design or hand-made worktrees.
- Tickets 01, 02 and 04 to 10 of this map: their flows already pass three of three under 45 s, so they were closed as stale.

## Further Notes

- Measurements are from main at baf0493e6 on 2026-11-23 (ticket 12 of the map). Worst flow time was Workspaces at 43 s, so the 45 s limit has little room. A flow that goes over it counts as not done.
- Go tests in this work run only through `scripts/proc-cap`.

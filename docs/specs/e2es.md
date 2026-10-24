## Problem Statement

Nobody can check the web UI's epic behavior without a person clicking through it. There is no browser test anywhere in the repo: web tests are vitest and jsdom only. A change to the epics page, the queue epic card, the epic page or the design flow can break the real UI and nothing notices until a person looks.

Agents have the same gap. An agent that finishes a ticket cannot show that the feature works in the running UI. A design agent that wants to know how the UI behaves today has to read code.

Three more things get in the way:

- The epic page shows no pause reasons. A person sees that a ticket is stuck, but not why, unless they open the ticket page.
- A design epic's Progress step is guessed from files in the trunk checkout and from git refs. Tests would depend on spec drafts in trunk, and the guess is hard to seed.
- The `Next:` line in the design agent's run-end message carries phase words (`map`, `review`, `implement`) that drive the Continue button. Those words duplicate what Progress already says.

## Solution

Four parts that depend on each other.

1. A coded Playwright suite for the epic flows of the web UI. It runs without an agent, locally and in CI. One flow catalog (a markdown file) lists the flows. The suite and the agents both read it.
2. A shared fixture. A Go program starts an isolated gx server with seeded epics and a fake agent. The suite, the verify step and design agents all use it.
3. A new `verify` ticket type in ralph-loop. It runs after code review. An agent starts a temp gx server on the fixture and checks that the implementation works through the web UI and through CLI and server verbs.
4. An explicit `design_step` field on map epics, set with a CLI verb. It replaces the file-based step guess and the `map`/`review`/`implement` phase words. The epic page also shows pause reason labels.

This spec designs these parts. It does not build them.

## User Stories

1. As a gx maintainer, I want a browser test suite for the epic flows, so that a UI regression fails CI before a person sees it.
2. As a gx maintainer, I want the suite to run with no agent in the loop, so that results are repeatable and cheap.
3. As a gx maintainer, I want each flow to start from a fresh server with known data, so that flows cannot affect each other.
4. As a gx maintainer, I want the suite to take about 3 minutes or less, so that it stays in the CI path.
5. As a gx maintainer, I want a flaky flow to fail hard with no retries, so that flakiness gets fixed and not hidden.
6. As a gx maintainer, I want `make web-e2e` to run the suite locally, so that I can reproduce a CI failure.
7. As a gx maintainer, I want a `web-e2e` CI job on Ubuntu, so that the suite runs on every change.
8. As a person queueing work, I want a test that clicks Queue on the epics page and sees the epic on the queue, so that the first action in the UI is always covered.
9. As a person watching the queue, I want a test that checks an epic's state, counts and ticket lines on the epic page and on the queue card, so that the two views never disagree.
10. As a person with a stuck ticket, I want to see why it is paused on the epic page, so that I do not have to open every ticket page.
11. As a person with a stuck ticket, I want the epic page row to link to the ticket page, so that I can recover from there.
12. As a person with a repair-class pause, I want the epic page to say "Fix it in the TUI", so that I know the web cannot fix it.
13. As a maintainer, I want one test row per recovery action (Answer, Retry, Choose base, Resolve conflict, Approve), so that every web recovery path is covered.
14. As a maintainer, I want a display-only repair row in the pause flow, so that the "Fix it in the TUI" label is covered.
15. As a maintainer, I want each recovery action to clear the pause in the test, so that I know recovery works end to end.
16. As a maintainer, I want the Approve row to run against a fake agent that draws a Claude-style permission screen, so that the pty runner's real code path is tested.
17. As a person designing an epic, I want a test that starts a design session and shows the first step, so that the entry to design is covered.
18. As a person answering a grilling round, I want a test that checks question numbers render as `Q8` or `Q8b` and that my answer reaches the agent, so that rounds work.
19. As a person who answered the last question, I want a test that checks the Continue button and a fresh scripted round, so that the design loop keeps going.
20. As a person following a design, I want a test that walks Progress through every step to Done, so that each step renders.
21. As a person who edits tickets through the CLI, I want a test that shows the change on an open page without a reload, so that live updates are covered.
22. As a maintainer, I want a test where the last ticket merges and the epic shows done and leaves the queue, so that completion is covered.
23. As a maintainer, I want edge-state tests (no epics, zero tickets, a ticket blocked by an unmerged dependency), so that empty and odd states do not crash the UI.
24. As a maintainer, I want no flow to reload the page after an action, so that live updates are exercised in every flow.
25. As an agent finishing a ticket, I want written instructions for checking my change in a browser, so that I do not invent a harness each time.
26. As an agent, I want those instructions to share the flow catalog with the suite, so that humans and agents check the same thing.
27. As a design agent, I want to start a fixture and drive the UI, so that I can see how existing behavior works before I design on it.
28. As a maintainer, I want one command to start a named scenario and print its web URL, state dir and hold-file path, so that tools can use it.
29. As a maintainer, I want CLI and server verbs to work against the fixture's socket, so that flows can mix CLI actions and UI checks.
30. As a maintainer, I want fake-agent behavior declared per ticket (finish, ask, fail), so that a scenario controls what the agent does.
31. As a maintainer, I want a hold file that delays only the finish and always times out, so that a flow can freeze a ticket as running without hanging the launch.
32. As a maintainer, I want paused tickets seeded straight into the store, so that I do not have to reach every pause reason through a live agent.
33. As a maintainer, I want the fixture to stop on SIGTERM, so that no server leaks.
34. As a maintainer, I want the fixture not to touch my real gx state, so that tests cannot corrupt my queue.
35. As a ralph-loop operator, I want a verify step after code review, so that a feature is checked in a running gx before the epic lands.
36. As a ralph-loop operator, I want the verify ticket to be one per epic and blocked on code review, so that it runs after review and does not wait on other verify tickets.
37. As a ralph-loop operator, I want the code-review ticket to ignore verify tickets in its "all others done" rule, so that the two do not wait on each other.
38. As a ralph-loop operator, I want an epic with no web, CLI or server changes to close its verify ticket at once, so that I do not pay for a pointless run.
39. As a spec author, I want a Verification section in each spec, so that verify knows which flows and checks to run per user story.
40. As a design reviewer, I want to check that every user story has a Verification entry, so that no story goes unchecked.
41. As an agent running verify, I want a backstop that maps the diff to the flow catalog, so that I add flows the spec missed.
42. As an agent running verify, I want one pass or fail line per story written into the ticket body, so that the result is readable.
43. As an agent that finds one small problem, I want to fix it inline and re-check, so that a tiny fix does not cost a whole ticket.
44. As an agent that finds several problems or a large one, I want to open child tickets and a blocked next-round verify ticket, so that verify works the way code review does.
45. As an agent, I want a harness failure to count as a finding, so that a broken fixture is fixed and not ignored.
46. As a ralph-loop operator, I want verify to stop after 3 rounds and ask with `gx tickets ask`, so that a loop that never gets clean does not run forever.
47. As an agent, I want a clean round to close the ticket commitless, so that no empty commit is made.
48. As an agent, I want to stop at 90K tokens and open tickets for what is left, so that I stay inside the 150K smart zone.
49. As a design agent, I want to set the design step with `gx tickets set <epic> --design-step <step>`, so that Progress is explicit.
50. As a design agent, I want a backward move to need an explicit flag, so that an accidental regression is refused.
51. As a design agent, I want the flag used only when a review sends tickets back, so that backward moves mean something.
52. As a person looking at Progress, I want a map with status `done` to always show Done, so that a map closed outside gx-design is right.
53. As a person looking at Progress, I want a map without `design_step` to show Chart, so that an unset field has one clear meaning.
54. As a person with existing maps, I want `gx tickets migrate` to write `design_step` for them, so that nothing changes on screen after the upgrade.
55. As a person designing, I want Continue at every step except Branch and Done, so that I can always go on.
56. As a person at Branch or Done, I want Progress to show "Open implementation epic", so that I can go on to the implementation epic.
57. As a person looking at Continue, I want its subtitle to be the step label, or the next ticket's title during Resolve tickets, so that I know what it will do.
58. As a maintainer, I want the `map`/`review`/`implement` phase words gone, so that there is one source of design state.
59. As a maintainer, I want the `Next:` line to be only a run-end marker, so that nothing reads phase words from it.
60. As a test author, I want to seed any Progress step with one CLI verb, so that tests need no spec files in trunk.

## Implementation Decisions

### Web e2e suite

- Tool: Playwright Test (Node), living with the web UI. No agent is involved.
- The suite runs against the fixture's `web_url`. Each test opens the URL once to get the web cookie, then drives the UI.
- Locators use roles and visible text. A `data-testid` is allowed only where text is ambiguous. Each one is listed in the flow catalog. The implementer picks the names.
- No flow reloads the page after an action. Flows 1 and 2 only pass if the UI updates live.
- A flaky flow is a hard fail with no retries. It gets a follow-up ticket.
- CI: a new Ubuntu-only `web-e2e` job, and `make web-e2e` locally. The suite stays small: one flow per journey, about 3 minutes in total. Edge cases go to verify or vitest.
- The suite waits on the pty spike (see Testing Decisions) for the Approve row.

### Flow catalog

One markdown file holds the flows (scenario, steps, pass condition). The suite implements them. Agents read the same file through an agent skill (see below). Nine flows, in this order:

P1:

1. Queue an epic. Scenario `ready`. Click Queue on the epics page. The epic appears on the queue.
2. Epic looks right. Scenario `mixed`. State, counts and ticket lines are correct on the epic page and on the queue card.
3. Pause reasons and recovery. Scenario `paused`. One row per recovery action: Answer (`self-reported`), Choose base (`ambiguous-base`), Resolve conflict (`merge-conflict`), Retry (one generic reason), Approve. Plus one display-only repair row. The reason shows on the queue card, the ticket page and the epic page. Each action clears the pause.
4. Start a design session. Scenario `design`. A design epic opens a session and shows the first step.
5. Answer a design round. Scenario `design`. Question numbers render correctly, the answer reaches the agent, Continue shows, and a fresh scripted round appears.
6. Outside changes show up. Scenario `ready`. A change made through the CLI appears on an open page.

P2:

7. Epic completion. Scenario `finishing`. The last ticket merges, the epic shows done and leaves the queue.
8. Edge states. Scenario `edge`. No epics, zero tickets, a ticket blocked by an unmerged dependency.
9. Design progress to the end. Scenario `design`. Progress walks every step to Done, set with the same `design_step` verb.

Cut from the catalog:

- Dequeue and reorder. The web has no remove or move verbs.
- Separate resume and failed-ticket flows. They are rows of flow 3.
- Server restart. It is tracked as follow-up gx:follow-ups/141.

### Scenarios and fixture

- Six named scenarios, one fresh server each: `ready`, `mixed`, `paused`, `design`, `finishing`, `edge`.
- The fixture is a Go program (name is the implementer's choice) built on the in-process server test helpers. It starts one server per scenario. The web listener binds a free port on 127.0.0.1. The state dir is short, because Unix socket paths are limited to about 100 bytes.
- It prints JSON with `web_url` (including the token), the state dir and the hold-file path. It stops on SIGTERM. CLI and server verbs work against its socket.
- Runner: `agent-runner: pty`. herdr is not used.
- Isolation: the fixture never reads real gx state. Config comes from a temp `HOME`, and state and data come from temp XDG dirs.
- Fake agent: a `fakeclaude` binary, installed as `claude` on the server's PATH. One program covers three jobs: the pty screen, the ticket worker (commit, report finished) and the scripted design chat. It reads a committed config in the worktree.
- Control: each scenario declares per-ticket behavior (finish, ask, fail). A hold file controls timing. A hold blocks only the finish, never the launch, and always times out. This follows the lesson from the live-agent Stop test fix.
- Pause reasons are seeded straight into the store as paused tickets. Only `self-reported` and one failing launch are reached live. The Retry row's seed reason is the implementer's choice.
- Design scenario: a seeded map epic with open tickets, `design_step` set by the verb, and a scripted transcript. The transcript holds the Q ids (including a lettered one such as `8b`), one Send, the run-end marker and the Continue button. The scenario needs no spec file in trunk.
- How the fixture starts `agent-holder` for design sessions is the implementer's choice.
- Risk: the pty runner reads a Claude-style screen, and `fakeclaude` must draw one. This is untested. A spike comes first and gates the Approve row.

### Agent instructions

- A skill (name is the implementer's choice, for example `gx-ui-check`) tells an agent how to start a fixture scenario, open the web URL with `playwright-cli`, run catalog flows and stop the fixture.
- Agent instructions run on demand only. They are not part of CI.
- Design agents use the same skill to research how existing UI behaves.

### Verify step in ralph-loop

- A new ticket type, `verify`, run in its own session after code review. The skill name is the implementer's choice (`gx-verify` is taken).
- Placement: every epic gets one `verify` ticket, created the way the code-review ticket is, and blocked on it. The code-review rule "every other ticket done" ignores verify tickets, so the two cannot wait on each other.
- `gx-to-tickets` skips the verify ticket for epics with no user-visible behavior. An epic with no web, CLI or server changes closes the ticket at once.
- Verification section: each spec gets one. Per user story it lists the catalog flows to run, the CLI and server verb checks, and the new flows or scenarios to write. `gx-to-spec` drafts it, the design review checks that every story has an entry, and grilling settles gaps. `gx-to-tickets` turns new flows into implementation tickets.
- Run:
  - The agent starts a temp gx server on the fixture and works through the Verification section.
  - It runs only the touched catalog flows plus the feature checks. It does not rerun the whole suite, because CI does that.
  - A backstop maps the diff to the catalog and adds flows the section missed.
  - It writes one pass or fail line per story into the ticket body.
- One small finding: exactly one finding whose fix is about 30 changed lines or less in at most 2 files is fixed inline and re-checked. A small missing flow or scenario is committed to the suite under the same rule.
- Several findings, or a larger fix: the agent opens child tickets (`parent` is the verify ticket) and a next-round verify ticket, blocked on them. This works like code review.
- Harness failures count as findings like any other.
- Rounds: 3 in total. The round counter marker is the implementer's choice. If round 3 still finds problems, the agent pauses with `gx tickets ask`. A clean round closes the ticket commitless.
- Smart zone: drive the browser by hand only for gaps, with text snapshots. At 90K tokens, stop and open tickets for what is left. How the 90K is measured is the implementer's choice.
- The TUI is not verified.

### `design_step` on map epics

- New field on map epics. Values are the eight Progress steps: Chart, Resolve tickets, Spec, Review, Grill findings, Create tickets, Branch, Done.
- CLI: `gx tickets set <project:epic> --design-step <step>`. The server validates the name and refuses an unknown one.
- Backward moves (for example Review back to Resolve tickets when a review reopens tickets) are refused unless the call passes an explicit flag. The flag name is the implementer's choice. gx-design passes it only when a review sends tickets back.
- Who writes it: gx-design sets the step each time it moves. Charting a map sets Chart. gx-design sets Branch, and Done only when it closes the map itself.
- Reading rules:
  - A map with status `done` always shows Done, with no write. This covers a map closed when its implementation epic merges, outside gx-design.
  - Otherwise the step is `design_step`.
  - A map without the field is treated as Chart: Chart is current, other steps are todo, Continue shows.
  - The server never derives a step from files or branch refs.
- Step notes (" · confirm destination", " · fog left", " · N of M", " · N tickets published") still come from the epic body and tickets.
- Removal: the file-based guess and the `ProgressPhase` type go now, with no server fallback. The `map`/`review`/`implement` phase words go from the gx-design skill, the run-end parser and the web.
- Migration: `gx tickets migrate` runs the old guess once and writes `design_step` on existing map epics.
- Run-end marker: the `Next:` line stays only as a marker that a run ended. Its exact text is the implementer's choice. Nothing reads a phase from it. A Grilling round still beats a run-end marker.
- Continue and Progress:
  - Continue shows at every step except Branch and Done. At those two, Progress shows "Open implementation epic".
  - The subtitle is the step label, or the next ticket's title during Resolve tickets.
  - Continue still runs `/clear` then `/gx-design <epic>`.
- The epics page, the design picker and the session banner read the step from `design_step` instead of the old phase.

### Pause reasons on the epic page

- Each paused ticket row on the epic page shows the same plain-words reason label as the ticket page, plus a link to the ticket page.
- Display only. Recovery stays on the ticket page, and flow 3 clicks through to it.
- Repair-class reasons show "Fix it in the TUI", as on the queue card.
- The epic detail payload gains a reason field on its rows. Map epic tickets are never Paused.

### Product changes this spec asks for

- Pause reason labels on the epic page.
- The `design_step` field, its verb, its validation, the migrate backfill and the removal of the file-based guess.
- The run-end marker change and the removal of phase words.
- Any `data-testid` attributes, listed in the flow catalog.
- The `verify` ticket type, its frontier rules and its skill.
- The Verification section in the spec template.

## Testing Decisions

A good test checks external behavior: what a person sees in the UI, or what a verb returns. It does not check internal state or call order.

Proposed seams, as few as possible:

1. **Playwright flow suite against the fixture's web URL.** The highest seam. It covers the nine flows, the pause labels on the epic page, Continue and Progress as rendered, and live updates. Prior art: none in the repo (no browser tests exist). The fixture builds on the existing in-process server test helpers and the pty worker fake that is built as `claude`.
2. **The existing server test seam and the API client** for `design_step` validation (unknown name, backward move with and without the flag, status `done` showing Done, missing field showing Chart) and for the epic detail pause labels. Prior art: the existing Progress tests and epic detail tests.
3. **Tickets and ralph-loop frontier tests** for the `verify` type: blocked on code review, ignored by the code-review rule, not waiting on other verify tickets, child tickets and the next-round ticket. Prior art: the existing code-review frontier tests.
4. **Vitest** for the Continue and Progress view logic, since the Playwright suite covers it only at the happy path. Prior art: the existing web view tests.
5. **A fakeclaude pty spike**, as a prerequisite. It shows that the pty runner reads `fakeclaude`'s screen and can answer its permission prompt. The Approve row of flow 3 is blocked until the spike passes.

The migrate backfill is tested through the same server test seam as item 2.

## Out of Scope

- Testing the TUI. Herdr-based TUI testing is a separate effort, and verify skips it too.
- Flows for pages other than epics (tickets, follow-ups, agents tab). Later specs add them on the same fixture.
- Dequeue and reorder flows. The web has no verbs for them.
- Server restart flows. They are tracked as follow-up gx:follow-ups/141.
- Recovery actions on the epic page. Pause reasons there are display only.
- Building the harness, the tests or the verify step. This spec ends at the design.
- Moving where gx-design writes the spec, ADRs and glossary drafts (now trunk, uncommitted). `design_step` removes the tests' dependency on them. The move needs its own follow-up.

## Further Notes

- Implementer's choices, not settled here: the verify skill name, the round counter marker, the backward-move flag name, the `Next:` marker text, the Retry row seed reason, the `data-testid` names, the fixture program name, the agent skill name, how the 90K stop is measured, and how the fixture starts `agent-holder`.
- Live agents hang tests. The fake agent holds only the finish step, never the launch, and every hold has a timeout.
- The glossary entry for Progress says it is derived from the tracker, the trunk checkout and the repo's refs. It must change to say it comes from `design_step`, and "Design step" should be added as a term.
- A new ADR is worth writing for "the server never guesses a design step from files or refs", next to ADR 0036 (trunk resolved live, never guessed).

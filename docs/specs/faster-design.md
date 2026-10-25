# Faster design: fewer presses, fewer fresh sessions

Source: the map `gx:faster-design` and its tickets 01 to 10. Evidence from 318 `/gx-design` runs
is in `docs/research/faster-design-evidence.md`. Where tickets disagree, the later one wins: 08
removes the "map runs never chain" rule that 05 kept; 09 settles Codex, the busy-turn display and
`--allow-backward`; 10 settles the spinner source, defaulted questions, prototype and HITL task
tickets, and the Codex toggle.

## Problem Statement

A person designing an epic with `/gx-design` presses Continue over and over, and most presses add
nothing. In 318 runs, 23% had no human input at all, and 54% from the spec step on. The median gap
before the next Continue press was under a minute. Each press also starts a fresh session that
reloads about 47K tokens of context.

The agent may chain steps in one session, but the rule is a fixed 50K ceiling. The first turn alone
is a median 47K, so the check passed 1 time in 52. Map runs never chain at all, even though 186 of
378 human answers in map runs were a plain "yes" or "agree".

The spec step is the loop that repeats. When the draft finds design questions, it files them as a
map ticket and stops. The next run resolves the ticket, the run after that redrafts, and the cycle
repeats: about 30 times on 14 maps, up to 4 rounds on one map.

Three bugs make it worse:

- An answer typed on the design page leaves the grilling Actionable in place, and the server never
  overwrites a set Actionable. So the run-end Continue does not show after a grilling round.
- The server raises Continue after the Branch and Done steps. Only the web Composer hides it, so
  the TUI still offers it.
- `gx agent context-window` fails with "no claude transcript found" when the session started in
  another directory (`~/dev/gx` or `.bare`) and later moved into the checkout. It failed 9 times.

## Solution

The agent chains steps in the same session while the next step fits a per-step token budget. When
it does not fit, the run ends with its run-end line, and the server sends Continue by itself after
a 10-second countdown the person can cancel. A per-session switch turns auto-advance off. With it
off, chaining also stops at the next step, so the person can step through a design by hand.

Map runs chain too: after a map ticket closes, the run moves to the next frontier ticket while the
budget allows. Each grilling round marks low-risk questions "decided unless you object", so one
reply covers a round of easy answers.

The spec step grills its design questions in the same session and redrafts. After 2 grilling
rounds, the next draft decides new questions by itself and lists those choices for the review to
check. The review runs a second time at most once, and only when a fix added or removed user
stories or changed scope.

While the agent works, the design page shows a spinner row with the step it is running.

## User Stories

Actionable fixes

1. As the person running gx, I want an answer I send from the design page to clear the grilling Actionable, so that the page stops asking a round I already answered.
2. As the person running gx, I want the server to recompute the design Actionable from the last assistant turn on every idle of a Design session, so that a run-end line shows Continue even after an earlier grilling round.
3. As the person running gx, I want a turn with neither a grilling round nor a run-end line to clear the server-derived Actionable and keep an agent-raised or gx-raised one, so that a stale prompt never lingers and a real one is never lost.
4. As the person running gx, I want the run-end line at the Branch and Done steps to raise an Actionable with a banner and a sound but no Continue, in every UI, so that no UI offers a step that does not exist.
5. As the person running gx, I want the web Composer to show Continue only when the Actionable carries an action, so that the web and the TUI agree.
6. As a developer, I want a contract test that runs the gx-design and gx-grilling templates and the fake agent's run-end line through the design detector, so that a wording change that breaks detection fails a test.

Auto-advance

7. As the person running gx, I want the server to send Continue by itself when a run ends with a run-end line that carries an action, so that I do not press it.
8. As the person running gx, I want a "Continuing in 10s" countdown with a Cancel button on the design page and in the Agents tab, so that I can stop a send I do not want.
9. As the person running gx, I want Cancel, any input from me, or turning the switch off to stop that one send and leave Continue raised, so that I can still press it later.
10. As the person running gx, I want an auto-advance switch per Design session, on by default, on the design page and in the Agents tab actions menu, so that I choose per session.
11. As the person running gx, I want sessions that existed before this change to count as auto-advance on, so that they behave like new ones.
12. As the person running gx, I want a grilling round, the Branch and Done steps, and a turn with no run-end line to never auto-advance, so that the flow waits only when it needs me.
13. As the person running gx, I want a step whose command failed but which still printed the run-end line to advance, so that the run-end line stays the single contract.
14. As the person running gx, I want auto-advance to stop after 5 auto-sends in a row with no input from me and leave Continue raised, so that an unattended loop cannot run forever.
15. As the person running gx, I want no banner or sound when a countdown starts, and a banner and sound when auto-advance does not send, so that I am called only when I am needed.
16. As the person running gx, I want turning auto-advance on while Continue is raised to start the countdown, so that I do not have to press Continue as well.
17. As the person running gx, I want a Continue press with auto-advance off to run one step and stop, so that I can step through a design by hand.
18. As the person running gx, I want a server restart during a countdown to leave Continue raised and send nothing, so that a restart never sends a step I did not see coming.

Chaining budget

19. As the design agent, I want `gx agent context-window --fits <step>` to answer chain or stop from a per-step growth table and a 130K ceiling, so that I chain only when the next step fits.
20. As the person running gx, I want `--fits` to print the tokens used, the step's growth and the ceiling, so that I can see why a chain stopped.
21. As the person running gx, I want `--fits` to report stop when auto-advance is off for the session, so that I can stop a chain without interrupting the agent.
22. As the person running gx, I want the context-window lookup to find the transcript when the session started in another directory, so that a chain does not fail with "no claude transcript found".
23. As the design agent, I want a failed check to stop the chain, so that the server's auto-advance starts the next step in a fresh session.
24. As the person running gx with Codex, I want chaining inside one session to work for Codex Design sessions, and the Agents tab switch to stop it, so that a Codex chain stops the same way as a Claude one.

Map runs

25. As the person running gx, I want a map run to chain to the next frontier ticket while `--fits` passes, so that I resolve more tickets per session.
26. As the person running gx, I want each ticket to keep its own grilling round, its own Resolution and its own Decisions so far line, so that the record stays one decision per ticket.
27. As the person running gx, I want a chain never to enter a prototype or HITL task ticket, and the run to end with the run-end line just before it, so that heavy work and work outside the chat start in a fresh session.
28. As the person running gx, I want a run that starts on a prototype or HITL task ticket to end after it, so that it never chains on from one.
29. As the person running gx, I want `/gx-design <epic>/NN` to pick only the first ticket and then chain to the frontier, so that a ticket address does not limit the run to one ticket.
30. As the person running gx, I want parallel sessions to skip claimed tickets, so that two sessions never work the same ticket.
31. As the person running gx, I want the last map ticket closing to chain into the spec step when the budget allows, so that no step boundary forces a fresh session by rule.

Low-risk defaults

32. As the person running gx, I want the agent to mark low-risk questions "decided unless you object" and ask the rest, so that one reply covers the whole round.
33. As the person running gx, I want a question to count as low-risk only when it changes no user story, no scope and not the Destination, is cheap to reverse and has one clear answer, so that the agent asks me whenever in doubt.
34. As the person running gx, I want defaulted questions listed as plain text under their own heading after the asked questions, with no question card, so that I can object in the message box.
35. As the person running gx, I want a round where every question is defaulted to show one yes/no card, "Accept these defaults?", so that every round still raises the grilling Actionable and waits for me.
36. As the person running gx, I want the Resolution to list the defaulted answers under their own heading, so that I can tell what I accepted without answering.
37. As the person running gx, I want the defaults rule to apply to map tickets, the spec step's design questions and review findings alike, so that every grilling round works the same way.

Spec step

38. As the person running gx, I want the spec step to grill its design questions in the same session and then redraft, so that a question does not cost a map run.
39. As the person running gx, I want each spec grilling round recorded as one closed `grilling` map ticket plus a Decisions so far line, so that the answers stay on the map.
40. As the person running gx, I want the round count to survive a `/clear`, an auto-advance and a research or prototype detour, so that the cap cannot reset.
41. As the person running gx, I want the third and later drafts to decide new design questions themselves and list each with its question, choice and reason in a spec section, so that the loop ends after 2 rounds.
42. As the person running gx, I want the review to check each self-decided choice and file a finding only when it disagrees or the choice changes user stories or scope, so that I see the choices that matter.
43. As the person running gx, I want only a question that needs research or a prototype to become a map ticket, filed after the rest of its batch is grilled and recorded, so that the map resumes with everything else settled.
44. As the design agent, I want to run `--fits spec` before each redraft and end the run if it fails, so that auto-advance resumes at the spec step with the answers already on the map.

Second review

45. As the person running gx, I want the review to run a second time at most once, and only when a fix added or removed user stories or changed scope, so that a spec is not reviewed twice for nothing.
46. As the person running gx, I want the agent to mark a story or scope change on the checked finding it came from, so that the reason for a second review is on the map.
47. As the person running gx, I want the second review to get the first checklist with its resolutions plus the full spec, check only the changed stories and scope, and write `## Re-review findings`, so that it checks the change instead of starting over.
48. As the person running gx, I want the second review's findings to get one grilling round and then create-tickets, with never a third review, so that the design always ends.
49. As the person running gx, I want Progress to move back to Review for the second review, so that the page shows what is running.

Busy display

50. As the person running gx, I want a row with a spinner and the current step at the bottom of the design page chat on every busy turn, plus the ticket title during Resolve tickets, so that I know what the agent is doing.
51. As the person running gx, I want that row to name the step the agent is running before `design_step` moves, so that it does not read "Resolve tickets" while the spec is drafted.

Skill rules

52. As the design agent, I want gx-design to tell me to set `--design-step` before printing the run-end line, so that the server reads the right step at idle.
53. As the design agent, I want `--allow-backward` allowed only for a review finding sent back to the map and for the move back to Review, so that Progress never moves back by accident.
54. As a skill author, I want gx-wayfinder's rule to read "one ticket per session unless the calling skill allows more", so that gx-design can chain map tickets and other callers keep one ticket.

## Implementation Decisions

### Design Actionable (tickets 01, 02, 03)

- `SessionActionable` gains a `Derived` flag. `detectDesignActionable` sets it on the grilling and
  run-end Actionables it raises. No matching on reason text.
- On every idle of a Design session, the server recomputes from the last assistant turn. The
  grilling marker wins over the run-end line. A grilling turn sets the grilling Actionable, a
  run-end turn sets Continue, and a turn with neither clears a `Derived` Actionable.
- A derived Actionable replaces an agent-raised one, because the newer turn supersedes it. It never
  replaces a `GxRaised` one, and a turn with neither marker leaves agent-raised and `GxRaised` ones
  alone.
- A prompt the design page types (`typeSessionPrompt`) clears any Actionable that is not
  `GxRaised`, the same as pane input. Continue already does this.
- At the Branch and Done design steps, the run-end line raises a `Derived` Actionable with the line
  as its reason, a banner and a sound, and no action. The server reads the map's `design_step` once,
  at idle, when it finds the run-end line. The recompute on every idle covers a later step change.
  When the step cannot be read, the server keeps today's behaviour and offers Continue.
- The web Composer drops its own step list (`NO_CONTINUE_STEPS`) and shows Continue only when the
  Actionable carries an action, as the TUI does. The Progress link to the implementation epic stays.
- `runEndLine` keeps its current pattern: `✅ <label>:` followed by `Next:`. It needs no em dash, so
  `Next: map - ...` and `Next: map.` already match. No other variants.
- Rejected: shared constants between the skill and the detector. A markdown skill cannot import a
  Go constant. The contract test (Testing Decisions) catches drift instead.

### Auto-advance (tickets 04, 09, 10)

- Trigger: the idle detection above. When it raises a run-end Continue that carries an action and
  the session's auto-advance is on, the server sends that action (`/clear` then `/gx-design <epic>`)
  itself. The skill gets no new step.
- Countdown: 10 seconds before the send. The design page shows "Continuing in 10s" with Cancel. The
  TUI shows the countdown and Cancel in the Agents tab terminal info row. Cancel, any input from the
  person, or turning the switch off stops this one send. Continue stays raised for a press.
- Switch: per session, saved in the session meta, on by default. The meta stores an "off" flag so
  older sessions read as on; the field name is the implementer's choice. It shows on the design
  page's Composer next to Continue, and as an item in the Agents tab actions menu
  (`ui/agents/actions.go`), only for Design sessions. Ticket 09 has the person flip it during a
  chain, so the Composer shows it whether or not Continue is raised.
- Codex: the Agents tab menu item also shows for Codex Design sessions, and `--fits` honours it.
  On Codex it only stops chaining. The server never auto-sends for Codex.
- Endings that never auto-advance: a grilling round (the grilling marker wins), Branch and Done
  (no action), and a turn with no run-end line, which includes an interrupt mid-step. A step whose
  command failed but still printed the run-end line advances. A HITL map ticket as the next step
  also advances, so its questions are ready when the person returns.
- Runaway guard: after 5 auto-sends in a row with no input from the person, the server stops
  auto-advancing and leaves Continue raised. The person's input resets the count. A switch flip
  counts as input.
- Banner and sound: none when a countdown starts. They play only when auto-advance does not send:
  the switch is off, the person cancelled, or the guard stopped it. A grilling round in the next
  run still plays its own.
- Turning the switch on while Continue is raised starts the 10-second countdown. A Continue press
  with the switch off runs one step and stops again, because `--fits` reports stop.
- A server restart during a countdown drops the countdown and leaves Continue raised.
- A countdown starts only after a turn that ends while the server is running. A session that is
  already idle when the server starts or reconnects never starts a countdown, even if detection
  raises Continue again. Its Continue stays raised for a press.
- New server routes: flip the switch and cancel a countdown. Each needs a CLI verb, the route
  table, the web mux and the web allowlist (repo rule). Route, verb and field names are the
  implementer's choice.
- `servertest` gets an off switch for auto-advance that is off by default, so fake agents that go
  idle do not trigger sends in unrelated tests.

### Chaining budget and the lookup fix (ticket 05)

- Rule: chain when tokens used now plus the next step's expected growth stay under 130K. One
  ceiling for every model, the same as ralph-loop's smart zone (`defaultSmartZone`). It replaces the
  50K rule in gx-design's "After every run".
- Growth table, padded medians, in Go: spec 25K, review 20K, map ticket 50K, chart 60K, and
  grill-findings through branch 150K as one block. Steps 6 to 8 always chain with each other, so a
  check that lands on step 6 covers the whole block, and that block always starts in a fresh session
  (47K + 150K > 130K).
- Command: `gx agent context-window --fits <step>`. It exits 0 for chain and 1 for stop, and prints
  the used tokens, the step's growth and the ceiling. Without `--fits` the command keeps printing
  the token count. Step names are the implementer's choice (the `design_step` values are the obvious
  pick). The `--fits` names for create-tickets and branch map to the grill-findings growth. Step 9
  always fits.
- `--fits` reports stop when the session's auto-advance is off. It reads the switch at every check,
  so turning it back on makes the next check chain. With no gx session, it treats auto-advance as
  on. How it reads the switch is the implementer's choice.
- Next step: the agent runs detection first, then calls `--fits` with the step it found.
- A failed check (stop, or the command errors) stops the chain. The server's auto-advance starts a
  fresh session, so a failed check costs no press.
- Lookup fix: try the transcript path built from the current directory first. If that file is
  missing, find the session id across all project folders and take the newest match.
- `--fits` reads Codex sessions the same way the command does today.

### Map runs and low-risk defaults (tickets 08, 10)

- After a map ticket closes, the run chains to the next frontier ticket while `--fits` passes with
  the map-ticket growth. Each ticket keeps its own grilling round. No combined round over several
  tickets.
- Grilling, research and AFK task tickets chain. A chain never goes into a prototype or HITL task
  ticket: the run ends with the run-end line just before it, and auto-advance starts it in a fresh
  session. A run that starts on such a ticket ends after it.
- Each ticket gets its own Resolution and its own Decisions so far line.
- gx-design drops "Never chain out of steps 1 to 3". Every step uses the same `--fits` check.
- A ticket address (`/gx-design <epic>/NN`) picks only the first ticket. After it, the run chains to
  the frontier like any other run. Parallel sessions skip claimed tickets.
- gx-wayfinder's "never resolve more than one ticket per session" becomes "one ticket per session
  unless the calling skill allows more". gx-design sets the chain rules above.
- Low-risk defaults live in gx-grilling, so they cover map tickets (step 3), the spec step's design
  questions (step 4) and review findings (step 6). In each round the agent marks low-risk questions
  "decided unless you object" and asks the rest. A question is low-risk only when it changes no
  user story, no scope and not the Destination, is cheap to reverse, and has one clear answer. When
  in doubt, the agent asks.
- Format: defaulted questions come after the asked ones, in plain text under their own heading,
  with no `❓` marker and no `gx-question` block. The person objects in the message box. A round
  where every question is defaulted prints one yes/no question card, "Accept these defaults?", so
  the round still carries the grilling marker, raises the grilling Actionable and waits. An
  objected default is asked like any other question. The heading text is the implementer's choice.
- The Resolution lists the defaulted answers under their own heading.
- A review finding (step 6) has no ticket. A defaulted finding's one-line resolution gets a
  "(default)" tag, and the agent checks it off after the person's next reply if that reply does not
  object. An objected finding stays unchecked and is asked in the next round.

### Spec step grills in place (ticket 06)

- The spec subagent drafts and returns its design questions with suggested answers. The main
  session grills the person in the same session, then dispatches the subagent again with the
  answers. Continuing the same subagent where the harness allows it is the implementer's choice.
- Each grilling round becomes one closed `grilling` map ticket (Question is the questions,
  Resolution is the answers) plus a Decisions so far line. The 2-round cap counts these tickets.
  The slug prefix or marker that tells them apart is the implementer's choice. The next spec
  dispatch gets them through the closed-ticket gists it already receives. No new map field.
- Before each redraft, the agent runs `--fits spec`. On stop, it ends the run with the run-end
  line, and auto-advance resumes at step 4 with the answers on the map.
- From the third draft on, the subagent decides new design questions itself and lists each in a
  new spec section with the question, the choice and the reason. The section name is the
  implementer's choice (for example `## Decided without the person`).
- The design-review rubric gains a check on that section: file a finding only when the reviewer
  disagrees with a choice or the choice changes user stories or scope.
- Only a question that needs research or a prototype becomes a map ticket. The agent grills and
  records the rest of that batch first, then files the ticket and stops without a redraft. Step 3
  resumes the map. A question too big for one round becomes the next round.
- A research or prototype detour never resets the round count.
- The spec step sets `--design-step spec` only after the draft is written, so it never moves
  backward.

### Second review (ticket 07)

- During grill-findings the agent writes a one-line resolution on each checked finding, plus a
  marker when the fix added or removed user stories or changed scope. The marker text is the
  implementer's choice. No diff check.
- Step 6 no longer continues straight to step 7. When every finding is checked, the agent goes back
  to "Detecting the next step".
- A new detection entry right after step 6 matches when every review finding is checked, at least
  one carries the marker, and the map has no `## Re-review findings` section. It runs the second
  review into that section. The section existing is the "at most once" rule.
- The second review's subagent gets the first checklist with its resolutions and the full spec. It
  checks only the changed stories and scope and what they affect. It may write `- [x] No findings`.
  No spec snapshot or diff: the spec is uncommitted until step 8.
- Its findings get one grilling round, the same as step 6, through a second new detection entry:
  unchecked `## Re-review findings`. Then create-tickets, even if that round changes stories again.
  Never a third review.
- Step 7 (create-tickets) matches only when neither `## Review findings` nor `## Re-review
  findings` has an unchecked finding.
- `design_step` moves back to `review` with `--allow-backward`, then to `grill-findings`. Before
  the second review the agent runs `--fits review`. On stop, the run ends and auto-advance starts
  the second review in a fresh session.
- `--allow-backward` is allowed in two cases only: a review finding that becomes a map ticket
  (steps 5 and 6), and the move back to `review` for the second review.

### Busy display (tickets 09, 10)

- At each detection, gx-design reports the step it starts, plus the ticket title during Resolve
  tickets, to the session meta through a `gx agent` call. This needs a server route and its CLI
  verb; names are the implementer's choice.
- The server clears the reported step when the session goes idle. The next busy turn shows
  `design_step` until gx-design reports a new step.
- The design page chat shows a row at the bottom with the spinner and "Working: <step>" (plus the
  ticket title during Resolve tickets) on every busy turn, not only during a chain. It reads the
  reported step and falls back to `design_step`.
- Progress and the Continue subtitle keep following `design_step`. The Agents tab gets no new
  display beyond the countdown row and the menu item.

### gx-design skill text

- "After every run" replaces the 50K rule with the `--fits` rule, and drops "Never chain out of
  steps 1 to 3".
- A rule: set `--design-step` before printing the run-end line. Today's steps already do this.
- Step 3 loses "resolving exactly one ticket" and gains the map-run chain rules.
- Step 4 replaces "file the questions as map tickets and stop" with the in-place grilling, the
  round cap and the self-decided section.
- Step 6 goes back to detection instead of continuing to step 7.
- The detection list gains two entries after step 6: the second review, then unchecked
  `## Re-review findings`. Step 7 waits until both sections are fully checked.
- The `--allow-backward` rule names its two cases.

### Glossary

- GLOSSARY.md gains "Auto-advance" (the server sends a Design session's Continue by itself after a
  countdown) and "Chain" (a design run moving on to the next step or map ticket in the same
  session). Exact wording is the implementer's choice.

## Testing Decisions

A good test checks external behaviour: the session meta and Actionable a server read returns, the
prompts a fake agent receives, the exit code and output of a CLI verb, the rendered page. It does not
check internal calls. Every seam below already exists.

1. Server harness with the fake design agent (`server/servertest`, `testutil/agentfake`). Covers the
   `Derived` recompute (grilling turn, page answer, run-end turn shows Continue; the same through
   pane input), the turn with neither marker, `GxRaised` kept, Branch and Done raising no action,
   an unreadable step offering Continue, the auto-send after the countdown, Cancel, input and the
   switch stopping one send, the 5-send guard and its reset, switch-on starting a countdown, the
   restart dropping a countdown, and the reported step in the session meta. Prior art:
   `sessions_actionable_test.go`, `sessionactionable_internal_test.go`,
   `sessions_banner_design_test.go`.
2. Skill contract test in `server`. It reads gx-design and gx-grilling from the embedded skill
   bundle and runs the run-end template, its examples and the question-round template (including
   an all-defaulted round) through `designActionable`. It checks that `agentfake.DesignRunEnd` has
   the same shape as the skill's template.
3. CLI tests for `gx agent context-window` (`cmd/agent_test.go`) and transcript tests
   (`transcript/transcript_test.go`, `occupancy_test.go`). Cover `--fits` exit codes and output per
   step, the grill-findings block mapping, step 9 always fitting, the switch off reporting stop, no
   gx session treated as on, and the lookup fallback across project folders.
4. Route and CLI parity test (`cmd/server_parity_test.go`) and the web allowlist test. Cover the new
   routes and verbs.
5. Web UI Vitest (`Composer.test.tsx`, `ChatLog.test.tsx`, `DesignPage.test.tsx`). Cover Continue
   shown only with an action, the switch, the countdown with Cancel, the "Working: <step>" row and
   its fallback, defaulted questions rendered as plain text, and the "Accept these defaults?" card.
6. TUI tests (`ui/agents/actions_test.go`). Cover the menu item for Claude and Codex Design sessions
   only, and the countdown with Cancel in the terminal info row.
7. Skill text checks (`cmd/skills_test.go` style). Cover the gx-design, gx-grilling and gx-wayfinder
   rules listed above.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture design`) and a temp
`CLAUDE_CONFIG_DIR`. Catalog flows are in the web e2e flow catalog.

1. Story 1: flow 5 (Answer a round). After Send, `gx agents list` shows no grilling Actionable.
2. Story 2: flow 5. Continue shows after the run-end line that follows the answered round.
3. Story 3: new scenario: the fake agent ends a turn with plain text after a grilling round. `gx agents list` shows no Actionable; a session with an agent-raised Actionable keeps it.
4. Story 4: new scenario: the map at `branch`, the fake agent prints a run-end line. `gx agents list` shows the Actionable with the line as reason and no action; the Agents tab offers no Continue.
5. Story 5: flow 9 (Design progress to the end) plus a session at Branch: Continue is absent and "Open implementation epic" shows.
6. Story 6: the contract test passes; edit a copy of the run-end template to drop `Next:` and the test fails.
7. Story 7: new flow "Auto-advance": with the switch on, the run-end turn is followed by a fresh round with no press.
8. Story 8: new flow "Auto-advance": "Continuing in 10s" and Cancel show on the design page; the TUI terminal info row shows the countdown.
9. Story 9: new flow "Auto-advance, cancel": Cancel stops the send and Continue stays; a second case types a note during the countdown.
10. Story 10: new flow: the switch shows on the Composer while the agent works and while idle; the Agents tab menu lists the item for the Design session and not for a plain session.
11. Story 11: start the fixture with a session meta written without the new field; `gx agents list` reports auto-advance on.
12. Story 12: flow 5 (a grilling round does not auto-send); the Story 4 scenario (Branch does not auto-send); the Story 3 scenario (no run-end line does not auto-send).
13. Story 13: new scenario: the fake agent prints an error then the run-end line; the countdown starts.
14. Story 14: new scenario: the fake agent ends 6 runs with run-end lines and no person input; the fifth send happens, the sixth does not, and Continue stays raised.
15. Story 15: in the Story 7 and Story 9 flows, the server log shows no banner when the countdown starts and one after Cancel. In the Story 16 flow, a run that ends with the switch off shows one banner. In the Story 14 scenario, the sixth run-end turn shows one banner.
16. Story 16: new flow: turn the switch off, let a run end, turn it on; the countdown starts.
17. Story 17: with the switch off, press Continue; one run happens and `gx agent context-window --fits <step>` inside it exits 1.
18. Story 18: start a countdown, restart the fixture server; Continue stays raised, no countdown starts and no prompt reaches the fake agent.
19. Story 19: `gx agent context-window --fits spec --agent claude --session <id>` against a transcript at 100K exits 0; at 110K exits 1.
20. Story 20: the same commands print used tokens, the step's growth and 130K.
21. Story 21: flip the switch off with the new CLI verb; `--fits spec` inside that session exits 1 at any token count.
22. Story 22: put the transcript under a project folder for another directory; `gx agent context-window` from the checkout prints the token count.
23. Story 23: read gx-design "After every run": a stop or an error ends the run with the run-end line.
24. Story 24: `--fits spec --agent codex --session <id>` against a Codex session exits 0 or 1 by tokens; the Agents tab menu lists the item for a Codex Design session; with it off, `--fits` exits 1.
25. Story 25 to 31: read gx-design step 3 and "After every run" and check each rule; read gx-wayfinder's Invocation rule (also Story 54).
26. Story 32 to 33, 36 to 37: read gx-grilling and check the low-risk rule, the format, the Resolution heading and the "(default)" tag on review findings.
27. Story 34: new flow "Defaulted questions": a round with one asked question and two defaulted ones shows one card and the defaults as plain text; Send asks nothing about the defaults.
28. Story 35: new flow "Defaulted questions": an all-defaulted round shows one "Accept these defaults?" card and raises the grilling Actionable.
29. Story 38 to 44: read gx-design step 4 and the review rubric and check each rule (in-place grilling, one closed ticket per round, the cap, the self-decided section, research and prototype tickets, `--fits spec`).
30. Story 45 to 48: read gx-design's detection list and check that step 6 returns to detection, the second-review entry, its input, its section, the re-review grilling entry and that step 7 waits for both sections.
31. Story 49: `gx tickets set <map> --design-step review --allow-backward` from `grill-findings` succeeds; flow 9 shows Review as current again.
32. Story 50: new flow "Working row": while the fake agent is busy, the chat shows the spinner and "Working: Resolve tickets" with the ticket title.
33. Story 51: report the step `spec` with the new CLI verb while `design_step` is `resolve-tickets`; the row reads "Working: Spec". After the session goes idle and turns busy again, the row reads "Working: Resolve tickets".
34. Story 52, 53: read gx-design's `--design-step` rules and check both.
35. Story 54: see entry 25.

New flows and scenarios to write (implementation tickets): "Auto-advance", "Auto-advance, cancel",
the auto-advance switch, "Defaulted questions", "Working row", and fixture scenarios for a plain-text
turn after a round, a run-end line at Branch, an error before the run-end line, and repeated
run-end turns for the guard. These need the fake design agent to script more than one run.

## Out of Scope

- Shrinking the ~47K start-up context (skill re-reads, MCP config). `context-budgets/01` owns it.
- rtk: removed from the toolchain; no design work left.
- Auto-advance for Codex sessions (reading Codex sessions in the idle detection and sending `/new`).
  Filed as a follow-up.
- Chat history lost after `/clear` (`gx:design-session/02`), which auto-advance makes more frequent.
- The idle-read flush race that could stall auto-advance silently (`gx:design-detection/02`).
- A combined grilling round over several map tickets.
- New display in the Agents tab beyond the countdown row and the menu item.

## Further Notes

- Measured step growth behind the table: spec +13K, review +10K, spec with questions +20K, map
  ticket +35K, chart +40K, grill-findings through branch about +60K to +180K. Peaks: spec 75K,
  review 73K to 125K, map run 85K, create-tickets 201K (max 276K).
- In-skill chaining is the first choice. The server's auto-advance exists because an agent cannot
  `/clear` itself.
- Names left to the implementer: the session meta field for the switch, the `Derived` field's JSON
  name, route and CLI verb names for the switch, the countdown cancel and the step report, `--fits`
  step names, the marker for spec-round tickets, the story-or-scope marker on findings, the
  self-decided section name, the defaulted-questions heading, and the glossary wording.

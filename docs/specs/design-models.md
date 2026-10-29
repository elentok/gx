# Design models

## Problem Statement

A gx agent session runs on whatever model the agent CLI picks. Design sessions pass no `--model`, so they run on the CLI default, which is often the most expensive model. Implementation tickets already run on `agents.claude.model` (default `sonnet`). Nothing tells the design work which steps need the strongest model and which do not.

Three things are missing:

- Design sessions cost too much. Most steps are fine on a cheaper model.
- The session that charts a map (the first design step) needs the strongest model. Nothing gives it one.
- Only Claude has any launch args. Codex gets none, so Codex has no model choice at all.

The person also cannot see which model a session runs on.

## Solution

gx gets one shared concept: a model tier. There are three tiers: `light`, `worker` and `strongest`. Each agent (Claude, Codex) maps each tier to a model and an effort in config.

- Only the `chart` design step runs on `strongest`. Every other design step runs on `worker`.
- The chart step never chains. The design skill ends the run after charting, so Continue or auto-advance restarts the agent inside the same gx session on the `worker` tier, then starts the next design step.
- Skills name a tier, not a model. A new verb, `gx agent model <tier>`, tells a skill which model to pass to its subagent.
- The fix agent runs on `strongest`. A new Agents tab session runs on `worker` unless the person picks another tier in the new-session dialog.
- The Agents tab shows the model and effort of each session in the session header.

## User Stories

1. As a gx user, I want every design step except charting to run on the worker tier, so that most design work costs less.
2. As a gx user, I want the chart step (including Destination confirmation) to run on the strongest tier, so that the map is drawn with the best reasoning.
3. As a gx user, I want a stopped and retried charting session to stay on the strongest tier, so that a retry does not lower the quality of the chart.
4. As a gx user, I want New Design to count as a charting session, so that a map I just created starts on the strongest tier.
5. As a gx user, I want a design session launched on a map with an empty `design_step` to count as a charting session, so that it starts on the strongest tier. To chart directly from an Agents tab session, I pick strongest in the new-session dialog.
6. As a gx user, I want Continue after charting to move the session to the worker tier, so that the later steps cost less.
7. As a gx user, I want auto-advance after charting to move the session to the worker tier in the same way, so that hands-free runs also get cheaper.
8. As a gx user, I want the design skill to end the run right after charting and after Destination confirmation, so that the strongest tier never chains into later steps.
9. As a gx user, I want the handoff to keep the session's ticket, name, auto-advance setting and web page, so that nothing I set up is lost.
10. As a gx user, I want the handoff to end the old conversation like an agent exit, so that its totals and grilling count are recorded and the new conversation has its own id.
11. As a gx user, I want a failed handoff restart to leave the session Exited (crashed), so that I fix the config and press Resume in the web UI.
12. As a Codex user, I want a Codex session to show the saved `meta.json` values in its header, so that gx shows only what it knows. The Codex handoff and the reported model wait for Codex design sessions to get Continue.
13. As a gx user, I want a resumed session to keep the tier it last ran on, so that a restart of gx does not change the model.
14. As a gx user, I want a resume of a session with no saved tier to pass no model or effort flag, so that old sessions behave as they do today.
15. As a gx user, I want a Continue on a session that already runs on the worker tier to restart nothing, so that I do not lose the conversation for no reason.
16. As a skill author, I want to name a tier in a skill instead of a model, so that the skill works for every agent and survives model renames.
17. As a skill author, I want `gx agent model <tier>` to print the configured model for the session's agent, so that I can pass it to a subagent call.
18. As a gx user, I want both design-review subagents to run on the strongest tier, so that reviews are as careful as possible.
19. As a gx user, I want the gx-to-spec and gx-to-tickets subagents, and the gx-research subagents that gx-wayfinder fires, to run on the worker tier, so that writing and reading work stays cheap.
20. As a gx user, I want subagents spawned during a charting session to follow the same tier rules, so that a strongest session does not make every subagent expensive.
21. As a Codex user, I want a subagent to fall back to the session model when the spawn call has no model field, so that a design run never fails because of a missing option.
22. As a gx user, I want `gx agent model` outside a gx session to print the configured Claude value for the current directory's project, so that I can try the verb in any shell and see my overlay.
23. As a skill author, I want `gx-code-review`, `gx-evolve` and `gx-triage-follow-ups` to ask `gx agent model <tier>` instead of a hard-coded alias, so that no gx skill names a model.
24. As a gx user, I want configured `agents.claude-args` that hold `--model` or `--effort` to win over the tier, so that my explicit flags are never overridden.
25. As a gx user, I want the fix agent to run on the strongest tier, so that investigations get the best model.
26. As a gx user, I want a new Agents tab session to run on the worker tier by default, so that day-to-day sessions cost less.
27. As a gx user, I want a tier picker in the new-session dialog with all three tiers, so that I can pick a model level for one session.
28. As a gx user, I want each picker label to show the configured model, for example "Strongest (opus)", so that I know what I pick.
29. As a gx user, I want the picker labels to follow the harness the dialog launches (Claude today), so that a label names the model the session will run on.
30. As a gx user, I want a session I create with a tier to keep it on resume, so that the choice is stable.
31. As a ralph-loop user, I want evolve and investigate tickets to stay on the worker tier, so that loop tickets behave as they do today.
32. As a gx user, I want the tiers configured in the `agents.<agent>` block of config, so that I find them next to the model and effort I know.
33. As a gx user, I want `agents.<agent>.model` and `effort` to stay the worker tier, so that existing config and implementation tickets keep working.
34. As a gx user, I want a project overlay to override each field of a tier block, so that one project can use another model.
35. As a gx user, I want an empty `model` or `effort` to mean "use the agent CLI's own setting", so that I can leave a knob alone.
36. As a Codex user, I want default slugs for all three tiers, so that Codex sessions work without any config.
37. As a Codex user with an explicit `agents.codex.model`, I want my value kept, so that an upgrade does not change my setup.
38. As a gx user, I want the session header to show `<model> · <effort>` next to the state chip, so that I can see what a session runs on.
39. As a gx user, I want a tooltip on that label to name the tier, so that I can tell which tier the session has.
40. As a gx user, I want the model the Claude agent itself reports to win over the launch value, so that a change made inside the session shows up.
41. As a gx user, I want a session on the CLI default to show "<Tier> · CLI default" until its first reply, so that gx never invents a model name.
42. As a gx user, I want the header to show nothing when tier, model and effort are all empty, so that old sessions do not show a wrong label.
43. As a gx user, I want the label for design, fix-agent and interactive sessions on Claude, and for sessions from `gx agents new --harness codex` (saved values only), so that every session gx launches is covered.
44. As an API client, I want the new `SessionInfo` fields to be additive, so that existing clients keep working.
45. As a ralph-loop user, I want a skill that runs in a loop ticket to get the model of that run's agent from `gx agent model <tier>`, so that a Codex run does not get a Claude alias.
46. As a skill author, I want `gx agent model <tier>` to print nothing and exit 0 when the tier's `model` is empty, and the skill convention to say "leave out the `model` field on empty output", so that a subagent never gets an invalid alias.
47. As a gx user, I want a map with `destination_unconfirmed: true` to count as `chart`, whatever its `design_step`, so that Destination confirmation always runs on the strongest tier.
48. As a gx user, I want only sessions launched by a design path (Start design, New Design, Continue or auto-advance restart) to get the step-based tier and the handoff, so that a session I linked to a design later keeps its launch tier and is never restarted.
49. As a Codex user, I want `gx agents new --harness codex` to start the session with tier launch args and take an optional `--tier` flag, so that Codex sessions get a model choice.

## Implementation Decisions

### Tiers and config

- Three tiers: `light`, `worker`, `strongest`.
- The existing `agents.<agent>.model` and `effort` are the worker tier. Their keys do not change.
- The other tiers are nested blocks, `agents.<agent>.light` and `agents.<agent>.strongest`. Each has `model` and `effort`.
- An empty field inherits the agent CLI's setting. gx passes no flag for that knob.
- A project's `agents.<agent>` overlay merges each field of the tier blocks, as it does for `model` and `effort` today.
- Defaults:
  - Claude: light `haiku` with no effort; worker `sonnet` / `medium` (unchanged); strongest `opus` / `high`.
  - Codex: light `gpt-6-luna` / `medium`; worker `gpt-6.1-sol` / `medium`; strongest `gpt-6-astra` / `high`.
- The Codex worker default moves from `gpt-5.6-sol`. Implementation tickets on Codex move with it. An install with an explicit `agents.codex.model` keeps its value.
- gx does not validate a slug at config load. A stale slug fails at launch, as the worker tier does today.
- There is no setting per session kind. The tier blocks are the only tier config.
- The implementer picks the Go type shape for a tier block and how the overlay merge code reuses the existing merge.

### Launch args

- Every launch, resume and handoff restart builds the agent's args from the session's tier: model and effort for that tier and that agent.
- Claude takes the model and the effort through its own flags. Codex takes the model through its model flag and the effort through a `-c` config override (the loop code already does this for effort).
- Codex gets launch args for the first time, for the sessions started by `gx agents new --harness codex` and for ralph-loop tickets. Every server launch path (web create dialog, fix agent, Start design, New Design) uses Claude today, and a Codex harness for them is out of scope. `gx agents new` takes an optional `--tier`, default worker. Claude's existing extra args (design disallowed tools, remote control name, configured Claude args) stay.
- Configured `agents.claude-args` that already hold `--model` or `--effort` win. gx adds no tier flag for that knob.
- A resume of a session whose `meta.json` has no saved tier passes no model or effort flag, as today.
- The implementer picks where the tier-to-args logic lives and its name. It must serve both the session launch and the ralph-loop path, so the worker tier is resolved in one place.

### Which session runs on which tier

| Session | Tier |
| --- | --- |
| Design session launched by a design path, while the map's `design_step` is `chart` or empty, or the map has `destination_unconfirmed: true` (Destination confirmation, step 2 of gx-design) | strongest |
| Design session launched by a design path, at any other step | worker |
| Session linked to a design later (`gx agent link`), for example a direct `/gx-design <epic> <idea>` in an Agents tab or plain CLI session | its launch tier, never restarted |
| Fix agent (`/gx-investigate` session started by the fix-agent code) | strongest |
| Agents tab new session | worker, or the tier picked in the dialog |
| Evolve and investigate tickets run by ralph-loop | worker (unchanged) |
| Implementation tickets | worker (unchanged) |

Per design step, the main session and its subagents run on these tiers:

| Design step | Main session | Subagents |
| --- | --- | --- |
| `chart` (with Destination confirmation) | strongest | research: worker |
| `resolve-tickets` | worker | research: worker |
| `spec` | worker | gx-to-spec: worker |
| `review` | worker | design review: strongest |
| `grill-findings` | worker | none |
| `create-tickets` | worker | gx-to-tickets: worker |
| `branch` | worker | none |

- `design_started_at` is not used to find the charting session. The `design_step` of the map is the marker.
- A design session is one launched by a design path: Start design, New Design, or a Continue or auto-advance restart. `meta.json` saves this as the launch kind (the implementer picks the field name). `gx agent link` sets the design flag on any session, but does not set the launch kind. A linked session keeps the tier it was launched with, whatever the step, and is never restarted by the handoff. A direct `/gx-design <epic> <idea>` typed into a running session is such a case: to chart on strongest, the person picks strongest in the new-session dialog.
- An empty `design_step` counts as `chart`, so it runs on strongest. Every design launch path already has a map.
- A map with `destination_unconfirmed: true` counts as `chart`, whatever its `design_step` (gx-design checks it before `design_step`). Seeded maps and maps moved back with `--design-step` can have both fields disagree.
- A charting session that is stopped and retried is launched again while the step is still `chart`, so it stays on strongest.
- New Design writes the map at `design_step: chart`, so it counts as a charting session with no extra rule.
- The light tier is used by no design or fix-agent session. Only the picker can choose it.
- The saved tier decides a resume. A resume never recomputes it from `design_step`.

### Charting handoff

- Continue and auto-advance share one handoff path.
- When a Continue or auto-advance fires on a design session (launch kind design) that runs on the strongest tier, and the map's `design_step` is not `chart` and `destination_unconfirmed` is not set, gx:
  1. stops the agent process and ends the old conversation exactly like an agent exit (`saveConversationTotals`, grilling recount, `recordConversation`),
  2. gets a new conversation id and records it,
  3. starts a fresh conversation in the same gx session under the worker tier's launch args,
  4. types `/gx-design <epic>`.
- The chart step never chains. `skills/gx-design/SKILL.md` must end the run with the run-end line after charting and after Destination confirmation, so the next Continue or auto-advance restarts on worker. The skill never moves on to a later step in the strongest session.
- The gx session is the same, so ticket, name, auto-advance and web page stay. No new session is created.
- When the session already runs on worker, Continue keeps today's behaviour: it types the reset command and `/gx-design <epic>` into the running agent.
- The handoff applies to sessions with the design launch kind only. A fix agent, an Agents tab session on the strongest tier, or a session linked to a design later is never restarted.
- The handoff saves the new tier, model and effort in `meta.json`.
- If the restart fails (for example a stale slug), the session becomes Exited (crashed), as any failed start does. The person fixes the config and presses Resume in the web UI. There is no new pause kind.
- The handoff uses launch flags, not an in-session command. This epic builds it for Claude only. Codex has no design Continue today: `sessionPrompt` refuses non-chat harnesses, and the design Actionable reads only the Claude transcript. The Codex handoff takes effect once Codex design sessions get Continue. That work is not in this epic.
- Rejected: typing `/model` (Claude's version rewrites the user's default model setting, and keeping it across `/clear` is unverified) and opening a second gx session (it would have to copy ticket, name, auto-advance and web page).
- The implementer picks how the restart is sequenced with the session's state (idle, starting) and what the session shows while the process restarts.

### Subagent models

- Skills name a tier. `gx agent model light|worker|strongest` prints the model configured for the current agent. For worker it reads `agents.<agent>.model`. It prints the value as written. A skill passes it as the `model` of its subagent call.
- Finding the agent: in a server session the verb uses the session's agent (`GX_AGENT_SESSION`). ralph-loop agents get no `GX_AGENT_SESSION`, so ralph-loop puts the run's agent into the agent's env (for example `GX_AGENT=codex`, next to `GX_INVESTIGATE_TICKET`), and the verb reads it before it falls back to Claude. `gx-evolve` and `gx-code-review` run mostly in loop tickets, so a Codex run must get Codex slugs from the verb.
- Empty model: when the tier's `model` is empty, the verb prints nothing and exits 0. The skill convention says: on empty output, leave out the `model` field, so the subagent runs on the session model.
- No validation. A full model ID on Claude is the user's mistake, as with slugs.
- Tier use:
  - strongest: both design-review subagents.
  - worker: the gx-to-spec and gx-to-tickets subagents, and the gx-research subagents. gx-research is dispatched from `skills/gx-wayfinder/SKILL.md` ("Fire the research subagents"), so gx-wayfinder is the call site that asks `gx agent model worker`.
  - Nothing in the design run uses light.
- The charting session runs on strongest, but its subagents follow the list above. Research tickets spawned while charting use worker.
- Outside a gx session and a loop run, with no `GX_AGENT`, the verb prints the configured Claude value for the current directory's project (overlay applied), and falls back to the built-in default. The implementer picks how the verb finds the session's agent and the exact output format.
- The skill convention in `skills/gx-local-tracker.md` ("Skill conventions") changes from hard-coded aliases to "ask `gx agent model <tier>`". It also lists the `light` tier name. It says: on empty output, leave out the `model` field. The call sites in `skills/gx-design/SKILL.md` and `skills/gx-wayfinder/SKILL.md` follow it.
- Three other skills hard-code a model today. This epic moves them to the verb, each on its current tier (opus becomes strongest, sonnet becomes worker):
  - `skills/gx-code-review/SKILL.md`: the review subagent, `model: "opus"`, becomes `gx agent model strongest`.
  - `skills/gx-evolve/SKILL.md`: the per-ticket `Agent` calls, `model: "sonnet"`, become `gx agent model worker`. The read-only review subagent, `model: "opus"`, becomes `gx agent model strongest`.
  - `skills/gx-triage-follow-ups/SKILL.md`: the per-item `Agent` calls, `model: "sonnet"`, become `gx agent model worker`.
- After this epic no gx skill hard-codes a model alias.
- Skill changes in `skills/gx-design/SKILL.md`: the subagent call sites ask the verb, and the run ends with the run-end line after charting and after Destination confirmation.
- Codex: the skill asks for the tier's model in the spawn call. If the spawn tool has no model field, the subagent runs on the session model. gx turns on no spawn-override flag. A Codex review may therefore run on the worker tier. This is accepted.

### Agents tab: new-session dialog

- The create-session request takes an optional `tier`. Empty means worker.
- The dialog shows a tier picker with `light`, `worker` and `strongest`. Each label shows the agent's configured model, for example "Strongest (opus)".
- The picker labels follow the harness the dialog launches, which is Claude today.
- The session's `meta.json` saves the tier, and a resume keeps it.

### Agents tab: model display

- The session header shows `<model> · <effort>` next to the state chip. The effort is left out when empty. The switcher row does not change.
- A tooltip names the tier, for example "Strongest tier".
- Saved value: at every launch, resume and handoff restart, gx saves the tier, the resolved model and the effort in `meta.json`. This is the fallback.
- Reported value: the model the agent reports wins whenever it is present. A change in the middle of a run shows on the next reply. A reported ID is shown as is, for example `claude-sonnet-5-5`.
  - Claude: the model ID on assistant messages in the session transcript. The transcript has no effort, so the saved effort is shown. It reads from the same server code that reads the transcript for the design Actionable, on the same refresh. No new hook or poll.
  - Codex: this epic builds no Codex rollout reader. The Codex header shows the saved `meta.json` values only. The reported Codex model takes effect once Codex design sessions get Continue and a rollout reader.
- CLI default: a tier with an empty `model` has no saved model. Until the first reply the header shows the tier label and "CLI default", for example `Worker · CLI default`. The reported model replaces it after the first reply. gx never guesses a model name.
- `SessionInfo` gains three additive fields: tier, model and effort. The header shows nothing when all three are empty. No new API version.
- A session created before this change shows the reported model once it replies, and nothing before that.
- Every session kind shows it: design, fix agent and interactive, for Claude and Codex. Codex shows saved values only.
- The implementer picks the JSON field names, the tier label wording beyond the examples above, and the component layout.

### Prototype

A Claude-only prototype exists in the worktree `design-sonnet-default`. It passes `--model <agents.claude.model>` to design sessions, runs the first session on `opus` and pins the review subagents to `opus`. It leaves the session on Opus after charting. This spec replaces it. It is input only.

## Testing Decisions

- A good test checks behaviour a person or a skill sees: the args a launch gets, the tier a session ends on, the text of `gx agent model`, the header label. It does not check private helpers.
- Highest seam: the server session API against a fake agent, through the gx fixture server. It covers launch args, the handoff restart, the saved tier and the `SessionInfo` fields.
- Config: unit tests for the defaults, the overlay merge of tier blocks, the Codex worker default move and an explicit `agents.codex.model` kept.
- Launch args: table tests for tier and agent to args, including empty model and empty effort (no flag). Prior art: the existing launch args tests in the server package and the agent flag tests in the ralph-loop package.
- Handoff: a scenario with a design session at `chart` that moves to a later step, then Continue. Check the process restarted under worker args, the session id and name did not change, the old conversation was recorded, a new conversation id exists and `/gx-design <epic>` was typed. A second scenario makes the restart fail and checks the session is Exited (crashed) and Resume works.
- Launch args: also a table case for `agents.claude-args` that hold `--model` or `--effort` (no tier flag for that knob) and a resume with no saved tier (no flag).
- `gx agent model`: CLI test for each tier, an overridden config, a run outside a session (project overlay applied, built-in default as fallback), a run with `GX_AGENT=codex` (prints the Codex slug), and a tier with an empty model (prints nothing, exit 0).
- Launch kind: server tests that a linked-later session is never restarted by the handoff, and that a map with `destination_unconfirmed: true` and `design_step: resolve-tickets` launches on strongest.
- Model display: server tests for the Claude reported model beating the saved one, the CLI-default case and an old session meta. Web unit test for the header label. Prior art: the existing Actionable transcript-reading tests.
- Skills: a contract check that the skill text calls `gx agent model` and that no gx skill hard-codes a model alias. A second check that `skills/gx-design/SKILL.md` ends the run after charting. A third check that the skill convention says to leave out the `model` field on empty output. Prior art: the existing run-end template contract test.

## Verification

A verify ticket runs these against a temporary gx server (`gxfixture design`) and a temp `CLAUDE_CONFIG_DIR`. The fake agent records the args it was started with, so the checks read them from its log. Catalog flows are in the web e2e flow catalog. There is no stored mock for this map, so there is no Mock states list.

1. Story 1: new scenario "Design model tiers": a design session at a step after `chart`. The fake agent log shows the worker model and effort in its args (Claude `sonnet` / `medium`).
2. Story 2: same scenario with a map at `design_step: chart`, and again with a map that has `destination_unconfirmed: true` and `design_step: resolve-tickets` (Destination confirmation). The log shows the strongest args (Claude `opus` / `high`). `gx agents list` shows tier strongest.
3. Story 3: same scenario. Stop the charting session and resume it. The log shows strongest args again and `gx agents list` shows tier strongest.
4. Story 4: new flow "New Design": create a design through New Design. The session starts with strongest args.
5. Story 5: same scenario with a map whose `design_step` is empty: the log shows strongest args. Then a new Agents tab session with strongest picked, and `/gx-design <epic> <idea>` typed into it: it keeps strongest and is not restarted.
6. Story 6: new scenario "Charting handoff": the map moves from `chart` to the next step, then press Continue (flow 9, Design progress to the end, covers the progress). The fake agent restarts with worker args, then receives `/gx-design <epic>`.
7. Story 7: same scenario with auto-advance on and no press. The restart and the typed command happen after the countdown.
8. Story 8: skill contract test: `skills/gx-design/SKILL.md` ends the run with the run-end line after charting and after Destination confirmation, and does not continue to a later step in the same run.
9. Story 9: in the Story 6 scenario, `gx agents list` shows the same session id, ticket and name, auto-advance still on, and the design page still opens.
10. Story 10: in the Story 6 scenario, the old conversation has its totals saved and is recorded (as after an agent exit), and the restarted session has a new conversation id that is recorded before the launch.
11. Story 11: in the Story 6 scenario, make the restart fail (a stale slug in the worker tier). The session shows Exited (crashed), no new pause kind appears, and Resume in the web UI works after the config is fixed.
12. Story 12: a session started with `gx agents new --harness codex` shows the saved `meta.json` tier, model and effort in its header. The fake Codex agent reports a different model and the header does not change.
13. Story 13: in the Story 6 scenario, restart the server. The resumed session args match its saved tier.
14. Story 14: a session whose `meta.json` has no saved tier is resumed. The log shows no model or effort flag.
15. Story 15: a design session at a later step on the worker tier, press Continue. The fake agent process is not restarted and receives the reset command and `/gx-design <epic>`.
16. Story 16: `rg -n "sonnet|opus" skills/gx-design/SKILL.md skills/gx-wayfinder/SKILL.md skills/gx-local-tracker.md` finds no hard-coded subagent model, and the skill text calls `gx agent model`. The skill contract test passes.
17. Story 17: in a session on the Claude fixture, `gx agent model strongest` prints `opus`, `gx agent model worker` prints `sonnet`, `gx agent model light` prints `haiku`. Exit code 0.
18. Story 18: the design-review call sites in `skills/gx-design/SKILL.md` ask for `gx agent model strongest`. A check in the skill contract test covers it.
19. Story 19: the gx-to-spec and gx-to-tickets call sites in `skills/gx-design/SKILL.md`, and the gx-research call site in `skills/gx-wayfinder/SKILL.md` ("Fire the research subagents"), ask for `gx agent model worker`. Same contract test.
20. Story 20: the charting-session text in the skill keeps the subagent tiers of Stories 18 and 19. Same contract test.
21. Story 21: skill contract check: the skill text tells Codex to fall back to the session model when the spawn call has no model field.
22. Story 22: run `gx agent model worker` in a shell with no gx session, in a directory whose project overlay sets another worker model. It prints the overlay value and exits 0. In a directory with no config it prints `sonnet`. The same for `strongest` (`opus`) and `light` (`haiku`).
23. Story 23: skill contract test: `rg -n 'model: ?"|"(sonnet|opus|haiku)"' skills/` finds no hard-coded model alias, and `gx-code-review`, `gx-evolve` and `gx-triage-follow-ups` call `gx agent model` (strongest for the opus calls, worker for the sonnet calls).
24. Story 24: launch args test: configured `agents.claude-args` that hold `--model` or `--effort` get no tier flag for that knob, and the configured value reaches the agent.
25. Story 25: start a fix agent from the fixture. The log shows strongest args and `gx agents list` shows tier strongest.
26. Story 26: new flow "New Agents tab session": create a session without touching the picker. The log shows worker args.
27. Story 27: same flow. The dialog lists light, worker and strongest. Pick strongest. The log shows strongest args.
28. Story 28: same flow. The picker labels read "Light (haiku)", "Worker (sonnet)" and "Strongest (opus)". With a config that sets `agents.claude.strongest.model` to another alias, the label follows it.
29. Story 29: same flow. The labels show the Claude slugs, the harness the dialog launches. `gx agents new --harness codex --tier strongest` starts with the Codex strongest args.
30. Story 30: same flow. Create a session on strongest, stop it and resume it. The args stay strongest and `meta.json` holds the tier.
31. Story 31: the existing ralph-loop e2e for evolve and investigate tickets still shows the worker args. Run `go test ./e2e/...` and the ralph-loop agent flag tests.
32. Story 32: config unit test: a config file with `agents.claude.strongest.model` and `effort` loads into the tier block.
33. Story 33: config unit test: a config with only `agents.claude.model` and `effort` gives the same worker args as before, and an implementation ticket launch is unchanged.
34. Story 34: config unit test: a project overlay that sets only `agents.claude.strongest.effort` changes only that field.
35. Story 35: launch args test: a tier with an empty `model` or `effort` adds no flag for it.
36. Story 36: config unit test: the Codex defaults are `gpt-6-luna` / `gpt-6.1-sol` / `gpt-6-astra` with `medium`, `medium` and `high`.
37. Story 37: config unit test: a config with `agents.codex.model` set keeps that value as the worker model.
38. Story 38: new flow "Session model label": open a session in the Agents tab after its first reply. The header shows `<model> · <effort>` next to the state chip.
39. Story 39: same flow. Hover the label. The tooltip names the tier.
40. Story 40: same flow on Claude. The fake agent reports a model different from the launch model. The header shows the reported one. Then the agent reports another model on the next reply and the header follows.
41. Story 41: same flow. A session on a tier with empty `model` shows `Worker · CLI default` before the first reply and the reported model after it (Claude).
42. Story 42: a session meta written without the new fields: `gx agents list` shows empty tier, model and effort, and the header shows no label before the first reply.
43. Story 43: run the Story 38 flow for a design session, a fix agent and an interactive session on Claude, and for a `gx agents new --harness codex` session. Each shows the label. Codex shows the saved values only.
44. Story 44: an old API client call that reads `SessionInfo` still works, and the server JSON adds only the three new fields. The typed client test (`tygo` output) is regenerated and unchanged except for those fields.
45. Story 45: CLI test: `gx agent model worker` with `GX_AGENT=codex` set and no `GX_AGENT_SESSION` prints the Codex worker slug. ralph-loop sets the variable for a Codex run (check the loop agent env in the ralph-loop launch test).
46. Story 46: CLI test: with an empty tier `model`, `gx agent model worker` prints nothing and exits 0. The skill contract test checks that the convention says to leave out the `model` field on empty output.
47. Story 47: see Story 2, second case.
48. Story 48: a session launched from the Agents tab on strongest is linked with `gx agent link`, the map moves to a later step, press Continue. The process is not restarted. A session started by Start design restarts as in Story 6.
49. Story 49: `gx agents new --harness codex` without `--tier` logs worker args, and with `--tier strongest` logs strongest args.

New flows or scenarios to write (each becomes an implementation ticket): "Design model tiers", "Charting handoff" (Claude, plus the failed-restart case), "New Design" (launch args), "New Agents tab session" (tier picker, resume), "Session model label". The fake agent must log its launch args and report a model in its transcript.

## Out of Scope

- Implementation tickets' model. They keep using the worker tier, which is `agents.<agent>.model`.
- Cost reporting by tier.
- Per-project tier overrides in the web UI. Overrides stay in the project's config overlay.
- Slug validation at config load.
- A per-session-kind tier setting.
- Typing `/model` into a running session.
- Agents other than Claude and Codex. gx supports only these two.
- A tier choice for evolve and investigate tickets in ralph-loop.
- The Codex design handoff, Codex design Continue and a Codex rollout reader. They wait for Codex design sessions to get Continue.
- A Codex harness for web-created, fix-agent and design launches. Only `gx agents new --harness codex` and ralph-loop start Codex sessions.
- A new pause kind for a failed handoff restart.

## Further Notes

- Claude facts, from the research ticket: `--model` takes `sonnet`, `opus`, `haiku`, `fable`, `default`, `opusplan` or a full ID. Effort is `--effort low|medium|high|xhigh|max`. An Agent tool call takes a per-call `model` (alias only).
- Codex facts: model flag `-m/--model`; effort has no flag, use `-c 'model_reasoning_effort="low"'`. Subagent `spawn_agent` has `model` and `reasoning_effort` only when `expose_spawn_agent_model_overrides` is on. gx does not turn it on.
- The Codex model catalog is server-side and old slugs migrate (`gpt-5.4` to `gpt-5.5`). A pinned slug can go stale. This is why the spec does not validate slugs.
- The Codex `/model` live switch and the Codex rollout format were not checked against a live TUI. Nothing in this epic depends on them.
- Research notes live on branches `research/claude-model-switching` and `research/codex-model-switching`.

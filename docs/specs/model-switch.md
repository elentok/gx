# Model switch

## Problem Statement

A chat or design session keeps the model it started with. The person cannot change the model or the effort of a live session without leaving gx. Claude's `/model` is not an option: it saves the model as the default for new sessions (see `docs/specs/design-models.md`).

Models also get outdated. A session can run on an old model while a newer one of the same family exists, and nothing tells the person.

## Solution

One switch action, two entry points.

- `{leader} m` on a chat page opens a picker. The person chooses a model and an effort of the session's own harness. gx stops the idle session and resumes the same conversation with the new flags.
- In chat and design footers, the model name turns red when a newer model of the same family exists. It opens a popover with one button, "Switch to <latest>", which runs the same switch.
- gx learns the models from the harnesses themselves (Codex `model/list`, Claude `initialize`). It ships no model list and no successor table.
- A chat that already has history shows a token cost warning in the picker and in the popover.

## User Stories

1. As a chat user, I want `{leader} m` to open a model picker, so that I can switch the model without leaving the page.
2. As a chat user, I want the shortcut to work on chat pages only, so that design pages and other pages keep their own keys.
3. As a chat user, I want the filter focused when the picker opens, so that I can type to find a model.
4. As a chat user, I want typing to filter the models fuzzily, so that I find a model fast.
5. As a chat user, I want up and down to change the model, so that I can choose with the keyboard.
6. As a chat user, I want left and right to change the effort, so that I can set both without the mouse.
7. As a chat user, I want Enter to switch and Esc to dismiss, so that the dialog follows the keys I expect.
8. As a chat user, I want the list to show every visible model of the session's own harness, grouped by family and newest first, so that I see my real options.
9. As a chat user, I want Claude aliases shown as the model they resolve to, and the `default` alias and hidden models left out, so that the list has no duplicates or internal entries.
10. As a chat user, I want outdated models dimmed with a "newer available" tag and the current model tagged "current", so that I see where I stand.
11. As a chat user, I want the current model always listed, so that the picker is never empty.
12. As a chat user, I want models whose id gx cannot parse in a last group called "Other", so that I can still pick them.
13. As a chat user, I want each model to offer only the efforts it supports, and no effort control for a model that has none, so that I cannot pick an invalid effort.
14. As a chat user, I want Switch disabled until my choice differs from the current model or effort, so that I do not restart for nothing.
15. As a chat user, I want everything disabled with a note while the agent works, so that a switch cannot cut a turn.
16. As a chat user, I want a stopped or exited session to say "Resume the session to switch.", so that I know what to do.
17. As a chat user, I want a chat with history to warn me that a switch costs tokens because the whole chat is sent again, so that I decide knowing the price.
18. As a chat user, I want that warning hidden for a chat with no turns yet, and never blocking the switch, so that it only appears when it matters.
19. As a chat user, I want the picker to show only the current model when model discovery failed, so that the failure does not break the dialog.
20. As a chat user, I want the chat to log "Switching to <model>…" and the footer to show the target with a "switching" hint, so that I see progress.
21. As a chat user, I want the prompt box disabled until the new agent is idle, and the log line to become "Switched to <model>.", so that I know when I can type.
22. As a chat user, I want a failed switch, whether the process exits at start or my first message gets `model_not_found`, to leave the old model running and log "Switch to <model> failed. Still on <old model>.", so that a bad model does not cost me the session.
23. As a chat user, I want the footer to show the chosen model and effort right after the switch, so that it does not show the old model until my next message.
24. As a gx user, I want a server crash during a switch to leave the old model in the record, so that a resume works.
25. As a chat or design user, I want the footer model name red, bold and dotted-underlined when a newer model of its family exists, so that I notice an outdated model.
26. As a chat or design user, I want that name to open a popover with the reason and one button "Switch to <latest>", so that I upgrade in one click.
27. As a chat or design user, I want the popover to say it keeps my effort and changes only this session, and to say so when the effort must change, so that nothing surprises me.
28. As a chat or design user, I want the popover to note it when Settings name the outdated model for a tier, and to link "Open Models settings", so that I can fix the default too.
29. As a chat or design user, I want the popover to say that the switch does not change Settings, so that I know my defaults stay.
30. As a chat or design user, I want the popover button disabled with a wait note while the agent is not idle, and the stopped or exited note when the session is gone, so that it follows the picker.
31. As a chat or design user, I want the popover to show the same token warning as the picker, so that the one-click switch is not blind.
32. As a gx user, I want gx to take the harness's named successor as the latest model, else the highest parsed version of the family among visible models, so that "latest" is never a guess from list order.
33. As a Claude user, I want a lagging alias never to count as latest, so that `fable` resolving to 5.1 shows red when 5.2 exists.
34. As a gx user, I want no warning when the model list is missing or the id cannot be parsed, so that the warning never lies.
35. As a gx user, I want the switch to keep my effort when the new model supports it, and otherwise to use the Codex model's default effort or the nearest lower Claude level (the lowest if none is lower), so that the effort is always valid.
36. As a Claude user, I want a hand switch to beat flags in `agents.claude-args`, so that the switch really changes the model.
37. As a gx user, I want a hand-switched session marked "switched by hand" in the session menu, cleared when I pick the tier's own model again, so that the menu stays true.
38. As a design user, I want Continue and auto-advance to leave a hand-switched session alone, so that the charting handoff does not undo my choice.
39. As a gx user, I want the model list refreshed at server start, when a picker opens and every 6 hours, in the background, so that the footer and picker never wait.
40. As a gx user, I want a failed refresh to keep the last good list and log on the server only, so that a bad query shows no error in the UI.
41. As a Settings user, I want a read-only note under a Claude alias that resolves to a non-latest model, so that I know to enter the full id.
42. As a keyboard user, I want `{leader} m` listed in the chord help, so that I can find it.
43. As a gx user, I want a switch to cancel a running auto-advance or auto-queue countdown first and log it, so that the session does not move on during the switch.
44. As a chat user, I want a click on the plain footer model name to open the picker, so that I can switch without the shortcut.
45. As a chat user, I want the red popover to have a "Choose another model" link that opens the picker, so that I am not limited to the latest model.

## Implementation Decisions

### Model discovery

- The model read goes on the existing probe code, with no parallel probe. A discovery module caches the result in memory, per harness.
  - Codex: `model/list` with hidden models included, added to the `codex app-server` handshake that the skills probe already runs (`skills/list` in `commandlist`). It follows pagination until there is no next cursor.
  - Claude: the existing Claude probe (`commandlist.ProbeClaude`) gets one `initialize` control request and also reads the `models` array (value, resolved model, supported efforts). No model call is made, no SDK is used, and nothing else from the reply (account, commands) is kept.
- Both reads run in the server's directory with the Claude launch env, like the skills probes do, and reuse their timeout and kill-on-reply pattern. A missing `models` array or missing resolved model means "no data".
- Refresh runs at server start, each time a picker opens, and on a 6 hour timer. All refreshes are background. A refresh already running is not started twice. The picker shows the cached list at once and updates when the query returns.
- A failed refresh keeps the last good list and logs on the server only. No UI message.
- Open footers re-evaluate on their next render. There is no push event and no toast.

### Family, version and latest

- Family and version come from strict id patterns: `gpt-<major>[.<minor>]-<family>` and `claude-<family>-<major>[-<minor>]`, with a trailing `-YYYYMMDD` stripped. A missing minor part is minor 0. There is no shipped family list.
- Latest of a family: the harness's named successor wins (Codex `upgrade`, or `upgradeInfo.model`). Without one, the highest parsed version among visible models of that family wins. A successor that is not a visible model is ignored. Hidden models never count as latest.
- Claude aliases never decide "latest". Only resolved ids are compared.
- An id that does not parse has no family and no warning.
- List order and the harness's default marker are never used as recency.

### Picker contents

- The picker lists visible models of the session's own harness. Switching between Claude and Codex is not possible.
- Families are ordered by their newest parsed version, highest first, ties alphabetical. Inside a family, models go newest first. Unparseable visible models form a last group "Other" in harness order, never dimmed and never tagged.
- Aliases show as their resolved id. The `default` alias and hidden models are left out. The current model is always listed, even when absent from the list.
- Efforts per model come from that model's own list. A model with no effort control hides the effort control.
- Outdated models are dimmed with a red "newer available" tag. The current one carries a "current" tag.
- With no list (discovery failed), the picker lists only the current model.
- The picker passes the resolved full id to the switch, not an alias.

### Switch action

- One server action serves the picker and the popover. It takes the session, a model and an effort. The implementer picks its route and verb names.
- Allowed only when the session is idle with a live holder. Any other state is refused. The web disables the controls in those states, with the busy note, or "Resume the session to switch." for stopped and exited sessions.
- The switch stops the idle session and relaunches the same conversation (a resume, not a fresh conversation) with new launch flags, using the existing stop and holder start path. The conversation id and the gx session do not change.
- The record (model, effort, tier) changes only after the new launch is alive. Alive means the agent reaches idle. The new process exiting before that is a failed switch (a crash or other failed start).
- A bad model does not exit in either harness: idle is reached at launch for good and bad models alike, and the error shows only on the first message (map ticket 08). So a switch is also failed when the first message after it ends in `model_not_found`: the Claude transcript error line (`error: model_not_found`) or the Codex rollout `task_complete` with `model_not_found` (the Blocked error). That is the only error that rolls back. Map ticket 10 (`docs/research/model-switch-unavailable-model.md`) found that a real model the account cannot use (a retired Claude id, an API-only Codex id) gives the same `model_not_found` in both harnesses, so one code covers an unknown and an unavailable model. gx matches the code, never the message text. Every other code leaves the switch in place, including the transient ones (Claude `rate_limit`, `overloaded`, `server_error`, `authentication_failed`, `billing_error`; Codex `rate_limit_exceeded`, `server_overloaded`, `http_connection_failed`, `unauthorized`, `usage_limit_exceeded`) and the shared codes (Claude `invalid_request` and `unknown`; Codex `bad_request` and `other`), because the old model would fail the same way. A plan-gated Claude model (Opus on Pro, the `[1m]` entitlement) could not be tried on this Max account and its code is unknown; if it is not `model_not_found`, it does not roll back. gx adds no check of the model name and no discovery-list check: the harness decides what is valid. The first message is spent and the person resends it.
- A failed switch relaunches with the old flags, restores the whole record from before the switch (model, effort, tier and the "switched by hand" flag; it never just clears the flag) and logs "Switch to <model> failed. Still on <old model>." If the old relaunch fails too, the session ends as any failed start does.
- Rollback state: a rollback writes a new switch time (the time of the old-model relaunch). The failed turn is then older than it, so the shown-model rule uses the restored record until the next good turn, and the footer, the red compare and the popover never show the bad model (Claude's `<synthetic>`, or the bad id from the Codex `turn_context`). `chat.ModelSwitched` takes the shown model from the same rule, not the raw reported one, so the session menu shows no "switched by hand" after a rollback. After a Codex rollback the error reader (`codexsession.LastTurnErrorIn`) ignores a `model_not_found` turn older than the switch time, so only the fail line shows and no "Agent hit an error" failure stays.
- Pending switch: the record stores the previous model, effort and flag, plus an "unconfirmed switch" mark that the first model turn clears. These are new optional `Meta` fields (`omitempty`; the implementer picks the names). An old meta reads as "no switch pending", so no migration step is needed. The pending data survives a server restart, a stop and a resume between idle and the first message, and the rollback still happens. A switch made while an earlier one is unconfirmed keeps the earlier rollback target (the last confirmed record).
- Check window: the check covers the first turn after the switch time that calls the model (a Claude assistant line, or a Codex `task_complete`). Input that reaches no model (`/clear`, the reset command that Continue types) keeps the window open. Any other completed model turn closes it, with or without another kind of error. A prompt queued during the switch is the first message and spends the check.
- Rollback feedback: the rollback reuses the switch feedback. It logs "Switching back to <old model>…" and disables the prompt box from the moment gx detects the error until the old model is idle. It runs from Idle (Claude) or Blocked (Codex). If a turn has already started, it waits for that turn to end.
- A server crash mid-switch leaves the old record, so a resume uses the old flags.
- A successful switch keeps the tier and sets the record's model and effort. It also sets a stored "switched by hand" flag (the implementer picks the field name). Picking the tier's own model again clears the flag. The tier's model is compared as configured, and a Claude alias counts as equal to its resolved id. A session with no tier never clears the flag.
- The session menu shows the flag OR the old launch-versus-reported comparison (`chat.ModelSwitched`).
- Launch args: when the flag is set, the record's model and effort win over `--model` and `--effort` in the configured Claude args. When it is not set, a configured flag still wins, as today.
- Effort on switch: the effort stays when the new model lists it. If not, Codex uses the new model's default effort, and Claude uses the nearest lower supported level (the lowest if none is lower). The popover note says when the effort changes. An empty record effort stays empty until the person picks one.
- Charting handoff: when the flag is set, Continue and auto-advance do not restart the session and do not change its model. They behave as for a session already on the worker tier (type the reset command and `/gx-design <epic>`).
- A switch cancels a running auto-advance or auto-queue countdown first, the way typing does, and logs that it did. The switch then goes on. A refused switch leaves the countdown running: it is cancelled only when the server accepts the switch.
- Queue (ralph-loop) agents are not switchable and show no warning.

### Feedback

- The chat logs "Switching to <model>…". The footer shows the target model with a "switching" hint. The prompt box stays disabled until the new agent is idle. The line becomes "Switched to <model>." when the agent is up.
- The record stores the switch time. One shown-model rule serves the footer text, the red compare and the popover: while the newest reported turn is older than the switch time, they use the record's model and effort (the transcript reports the new model only after the next turn). Otherwise they use the reported model. After the next turn the effort shown is the reported one where the harness reports one (Codex), else the record's effort (`cmp.Or(turn.Effort, m.Effort)`).
- gx does not hide the harness's own resume warning.

### Token cost warning (added beyond the mock)

- A chat with history (at least one completed turn) shows one warning in the picker and in the popover, with the same text: "Changing the model costs tokens. The whole chat is sent again at the new model's price." It never blocks the switch and is hidden for a chat with no turns yet. It replaces the plain picker note from ticket 05.
- This warning is not in the stored mock. The mock states below do not show it. Its look follows the note style of the popover ("note" text) and the busy note of the picker.

### Red footer and popover

- The server evaluates the warning and sends it with the session info: the latest model, and which tiers of the session's harness name the current model (the implementer picks the field names). The web decides nothing from the model list itself.
- The compared model is the shown model (see Feedback). Aliases resolve through the cached list. Settings are never compared. A lagging Claude alias session shows red at once.
- The footer name is red, bold and dotted-underlined when a newer model exists, and is a button that opens the popover. Unmarked footers stay plain. The footer name appears in chat and design footers.
- Popover (from the mock): heading "A newer <family> model is available", a reason line, one button "Switch to <latest>", and a note "Keeps effort <x>. Only this session changes." This is a copy change from the stored mock, which says "Only this chat changes." The token warning shows when the chat has history.
- When a tier's configured model, resolved through the list, equals the current model, a note names every matching tier, links "Open Models settings" and says the switch does not change Settings. The popover never edits Settings.
- In a chat, the popover also has a "Choose another model" link that opens the picker. It is added beyond the stored mock, so no Mock states line shows it. Design sessions have no picker, so their popover has no link.
- In a chat, a plain (not red) footer name is a button that opens the picker. In a design session a plain name stays plain text.
- Busy: the button is disabled and the note says to wait until the agent is idle.
- An empty record effort: the popover note reads "Uses the model's default effort." and Switch treats the empty effort as valid.

### Decided by default

- The list is deduplicated by resolved id: an alias and its resolved id show once.
- Date strip, missing minor equal to 0 and an invisible successor being ignored are the rules in "Family, version and latest".

### Models settings note

- Under a Claude alias that resolves to a model that is not the latest of its family, Models settings shows a read-only note, for example "`fable` resolves to claude-fable-5-1. claude-fable-5-2 is newer. Enter the full id to use it." Codex needs no note.

### Web

- The chord `{leader} m` is added in the chord table for the chat page only. It appears in the chord help list.
- The picker is a centered dialog, built like the mock. The popover reuses the footer popover pattern of the context badge.

## Testing Decisions

- A good test checks what a person sees or the args a launch gets. It does not check private helpers.
- Highest seam: the gx fixture server with fake agents, through the web e2e flows and the server session API. The fake `claude` and `codex` must answer the model query (a list for each harness, with a lagging alias, a hidden model, a model without effort and an unparseable id), log their launch args and report a model in the transcript or rollout. The fixture scenario is new. The implementer picks its name.
- Go table tests: id parsing and latest rule (successor wins, highest version wins, alias never trusted, hidden ignored, unparseable gives nothing), effort carry-over, launch args with and without the flag (prior art: `sessionlaunchargs_test.go`).
- Go tests for refresh: not started twice, failed refresh keeps the last list.
- Go tests for the switch: success records after alive; failure (exit at start, or `model_not_found` at the first message) keeps the record and relaunches with the old flags; other errors do not roll back; refused when not idle; the flag set and cleared; handoff leaves a flagged session alone. Prior art: the existing resume and handoff tests in `server/sessions`.
- Vitest: picker keys and filter, ordering and groups, disabled states, footer red state, popover notes and the token warning rules. Prior art: the context badge tests.
- Flow budget: add steps to existing flows on the same scenario where possible (see `skills/gx-playwright-e2es`). A new flow only where a real round trip needs it.

## Verification

A verify ticket runs these against a temporary gx server with the new fixture scenario. The scenario seeds a Codex chat and a Claude chat with history, a chat with no turns, a design session, and the fake model lists above. Fake agents log their launch args.

1. Story 1: on the chat page press `{leader} m`. The picker opens. Mock states `a-open`.
2. Story 2: press `{leader} m` on a design page and on the Agents list. Nothing opens. Vitest on the chord context.
3. Story 3: Mock states `a-open` (filter focused, placeholder shown).
4. Story 4: type "sol" in the picker. Only matching models stay. Mock states `a-changed`.
5. Story 5: press down. The selected model changes. Mock states `a-changed`.
6. Story 6: press right. The selected effort changes. Vitest.
7. Story 7: press Esc: the dialog closes and nothing changes. With a changed choice, press Enter: the switch starts. Mock states `a-changed` for the Switch button.
8. Story 8: Codex chat: families grouped, newest first. Claude chat: the same. Mock states `a-open`. Vitest for the ordering rule.
9. Story 9: the Claude list shows `claude-opus-5-5`, not `opus`. The `default` alias and the hidden Codex model are absent.
10. Story 10: Mock states `a-open` (dimmed outdated model, "newer available" tag, "current" tag).
11. Story 11: a session whose model is not in the list still shows it as current.
12. Story 12: the unparseable id sits in "Other", last, not dimmed.
13. Story 13: pick a Claude 4.6 model: no `xhigh` button. Pick Haiku 4.5: no effort control. Vitest.
14. Story 14: with the current choice, Switch is disabled. Mock states `a-open`.
15. Story 15: while the agent works, open the picker. Mock states `a-busy`. The API route refuses the switch for that session (check the HTTP status). There is no CLI check.
16. Story 16: stop the session and open the picker. The note reads "Resume the session to switch." Switch is disabled.
17. Story 17: open the picker in a chat with history. The token warning shows with the text above. New flow step. Added beyond the mock, so no Mock states line applies.
18. Story 18: the chat with no turns shows no warning. In the history chat, Switch still works with the warning visible.
19. Story 19: make the fake model query fail. The picker lists only the current model.
20. Story 20: start a switch with a held fake agent. The log shows "Switching to <model>…" and the footer shows the target with "switching".
21. Story 21: the prompt box is disabled during the hold. After release the log reads "Switched to <model>." and the box works.
22. Story 22: (a) make the fake agent exit at start for the new model; (b) make the fake agent reach idle, then answer the first message with `model_not_found` (Claude transcript line and Codex rollout form). Mock states `a-failed`. In both the fake log shows a relaunch with the old flags, `gx agents list` shows the old model, and the footer shows the old model. A fake error of another kind leaves the switch in place. After the rollback the footer, the red compare and the popover show the old model, the session menu shows no "switched by hand", a Codex chat shows no error failure, and the flag is as before the switch (test with an earlier successful switch: its flag stays). A second switch before the first message rolls back to the last confirmed record. Restart the server between idle and the first message: the rollback still happens. `/clear` before the first message keeps the window open. The prompt box is disabled during the rollback, which also runs from Blocked.
23. Story 23: after a successful switch and before any new turn, the footer, the red compare and the popover use the record's model and effort. After the next turn they use the reported model.
24. Story 24: kill the server mid-switch (held launch), then restart it. The record and the resume flags are the old ones.
25. Story 25: Mock states `footer-red` and `footer-normal`.
26. Story 26: Mock states `popover-open`. Click the button: the switch runs.
27. Story 27: Mock states `popover-open` (effort note). In a case where the effort is missing, the note says the effort changes. With an empty record effort, the note reads "Uses the model's default effort." and Switch is enabled.
28. Story 28: Mock states `popover-settings`.
29. Story 29: Mock states `popover-settings` (no-change sentence). `gx config` shows Settings unchanged after the switch.
30. Story 30: Mock states `popover-busy`. A stopped session shows the "Resume the session to switch." note.
31. Story 31: open the popover in a history chat. The token warning shows with the same text as in the picker. Added beyond the mock.
32. Story 32: Go table tests for the latest rule, plus the fixture Codex list where `upgrade` is set on one model.
33. Story 33: Claude fixture with `fable` resolving to 5.1 and 5.2 listed: the footer is red at once. Mock states `footer-red`.
34. Story 34: empty list: the footer is plain. Id `claude-weird`: plain.
35. Story 35: Codex switch to a model without the current effort: the fake log shows the model's default effort. Claude switch from `xhigh` to a model without it: the log shows the nearest lower level. Table test for "none lower".
36. Story 36: Claude fixture with `--model x` in `agents.claude-args`: after a hand switch the fake log shows the picked model. Before a switch it shows `x`.
37. Story 37: the session menu shows "switched by hand" after a switch, and not after picking the tier's own model again.
38. Story 38: hand-switched design session at a later step, press Continue. The fake agent is not restarted and its model is unchanged.
39. Story 39: the fake model query log shows one query at start, one on picker open and none duplicated. A timer test with a fake clock covers the 6 hours.
40. Story 40: fail a refresh after a good one. The picker still shows the old list. The server log has the failure, the UI has none.
41. Story 41: open Models settings with the lagging `fable` alias. The note shows. With a Codex model, no note.
42. Story 42: open the chord help. `{leader} m` is listed.
43. Story 43: with an auto-advance countdown running on a design session, run a switch. The countdown stops, the log says so, and the switch goes on. No handoff fires afterwards. A refused switch (agent busy) leaves the countdown running. Repeat with an auto-queue countdown.
44. Story 44: in a chat, click the plain footer model name (`footer-normal`). Added beyond the mock: the name keeps the plain look and gains a pointer cursor and a hover underline, so no Mock states line shows it. The picker opens. In a design session the plain name is text and a click does nothing.
45. Story 45: open the popover in a chat and click "Choose another model". The picker opens. In a design session the popover has no such link. Added beyond the mock, so no Mock states line applies. Mock states `popover-open` for the rest.

**Mock states** (stored mock `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html`; scenario is the new model switch scenario; story entries refer to these by state name):

- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · a-open · 760 px · Codex chat, idle, `{leader} m` pressed, outdated current model · picker-dialog, search, effort, key-hints
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · a-changed · 760 px · same chat, "sol" typed, new model chosen · picker-dialog, search, effort, switch-button
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · a-busy · 390 px · Codex chat with the agent working, picker open · picker-dialog, busy-note
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · a-failed · 390 px · Codex chat after a failed switch (fake agent exits at start, or answers the first message with `model_not_found`) · fail-line
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · popover-open · 760 px · chat, idle, outdated model, popover open · model-name, popover
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · popover-settings · 390 px · design session whose worker tier names the outdated model, popover open · model-name, popover, settings-link
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · popover-busy · 390 px · chat with the agent working, popover open · popover, switch-disabled
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · footer-red · 360 px · chat with an outdated model, popover closed · model-name
- `$(gx tickets root)/model-switch/mocks/04-picker-and-popover-look.html` · footer-normal · 360 px · chat on the latest model · model-name

New flows or scenarios to write (each becomes an implementation ticket): the model switch fixture scenario (model lists and fake agents as above), picker steps (stories 1 to 16, 19), switch feedback and failure steps (stories 20 to 24, 43), popover and footer steps (stories 25 to 31, 44, 45), the token warning steps (stories 17, 18, 31), and a Models settings note step (story 41). The token warning has no mock state, so its look is checked by the story entries.

## Out of Scope

- Switching between Claude and Codex. A conversation cannot move between harnesses.
- Queue (ralph-loop) agents. Only chat and design footers show the warning.
- `{leader} m` on design pages. Design footers have the popover only.
- Editing Settings from the popover. It only links to Models settings.
- A shipped model list, family list or successor table.
- A push event or toast when the model list changes.
- Hiding the harness's own resume warning.

## Further Notes

- Facts come from `docs/research/resume-with-new-model.md` (a resume with new flags keeps the conversation, the reported model lags until the next turn, a bad model does not exit: it reaches idle and fails only on the first message, see `docs/research/model-switch-bad-model-through-holder.md` and the alive rule above) and `docs/research/model-discovery.md` (query shapes, no successor field in either harness).
- Claude and Codex both pay again for the history after a switch (cache loss, resent history). That is the basis of the token warning.
- "Tier" means gx's light, worker and strongest. "Family" means a model line such as Sol, Luna, Opus or Sonnet.

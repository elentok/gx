# Agent launch and recovery 2

## Problem Statement

gx launches and relaunches loop agents, and five faults in that path end as a false failure. A person sees a pause or a wrong result that gx caused.

- A long launch note makes Claude Code treat the typed keys as a paste. The skill command becomes plain text, the skill does not run, and launch proof never sees a match.
- Launch proof resends the prompt into an agent that is already working. A resume appends to an old transcript, so the proof reads the old first record and never matches the new one. Each resend queues a duplicate message, and gx then kills a working agent as a "hung pane".
- A hand trim of a verify ticket's `## Verified` shrinks the relaunch note. The agent verifies only the listed stories and finishes with the rest unchecked.
- The process-storm watchdog kills at 200 processes. A normal `gx check --full` peaks at 234 to 239 and stays over 200 for 11 to 13 seconds, so every verify collect ticket storms. The pause also records only the count.
- When the agent does not know a skill (a fresh worktree, a stale install), the launch fails the same way twice and pauses. The error text names the fix and a person has to run it.

## Solution

gx keeps the launch command bare, proves the launch from the record this launch produced, and never resends into a working agent. It keeps the verify story list in the ticket. It lets a normal test run finish and still stops a runaway fork. It installs a missing skill itself.

- Every non-empty launch note goes to a file in the ticket store. A one-sentence system-prompt addition tells the agent to read it first (for Claude, inside one per-launch prompt file). The typed prompt is always `<skill command> <ticket address>`. Claude and Codex share this path.
- Launch proof counts only user records written after the prompt was first sent. It treats a pasted record as delivered but not run. It never resends while the runner reports the agent working.
- `verify-plan` writes a strict `## Stories` section. The relaunch note is that list minus the units verified at HEAD. A finish with unchecked units is blocked and resent up to a small cap, then pauses as `iteration-error`.
- The watchdog default limit is 500. It kills only after 10 consecutive samples over the limit. A new key, `watchdog.hard-max-tree-processes` (default 1000), kills on the first sample over it. The log line and the pause text name the top processes.
- On "unknown skill" the launcher refreshes the remembered skill install in process and retries once. A second failure, or a failed refresh, pauses as `agent_prompt_stalled` ("Could not start the agent"), as today.

## User Stories

Launch note in a file

1. As a person, I want a long launch note never to turn the skill command into pasted text, so that the skill runs.
2. As a person, I want the typed launch prompt always to be only the skill command and the ticket address, for Claude and Codex, so that no launch depends on Claude Code's unknown paste cutoff.
3. As a person, I want every non-empty launch note (verify relaunch note, iteration summaries, trunk-conflict note, investigate note) written to a file in the ticket store, so that one path carries every note.
4. As a person, I want gx to rewrite the note file on every launch (first launch, Retry, Start over, relaunch, resume) and delete it when the ticket closes, so that the file never goes stale and never lingers.
5. As a person, I want the note file outside the worktree, so that it can never land in a commit.
6. As an agent, I want a per-launch system-prompt sentence (for Claude, in the one per-launch `--append-system-prompt-file`), only when a note exists, telling me to read the note file before the skill's first step and treat it as part of my launch prompt, so that I follow the note.
7. As a person, I want a launch with no note to add no sentence and leave no file, so that nothing changes for plain launches.
8. As a verify agent on a relaunch, I want the note to say "verify every story in the ticket; the lists below only adjust that", so that I never take a short list as my whole scope.

Launch proof

9. As a person, I want launch proof to accept only a user record written after the launch prompt was first sent (a count taken before the send), for Claude and Codex, so that the old first record of a resumed session never proves a new launch.
10. As a person, I want a pasted record that holds the launch command to fail as delivered-but-not-run with its own reason, not as a hung pane, so that the pause names the real cause.
11. As a person, I want launch proof never to resend while the runner reports the agent working, in every launch path that uses it, so that no duplicate message is queued into a busy agent.
12. As a person, I want a launch whose record lands to log no `prompt-nudged` event, and a working agent to log none either, so that the run log shows only real resends.
13. As a Claude agent, I want a prompt line that says to wait for a background task's completion notice and not to poll it (no `ListAgents` loops), so that polling stops costing turns and tokens.

Verify scope

14. As a person, I want `verify-plan` to write a `## Stories` section into each lone verify ticket and each verify part, with one strict line per unit, so that the ticket owns its story list.
15. As a person, I want each Mock states line to be a unit with its own `## Stories` line and its own `## Verified` verdict, so that mock checks are never skipped.
16. As a person, I want the relaunch note to be the `## Stories` list minus the units whose latest `## Verified` entry stands at HEAD, so that a hand trim of `## Verified` cannot shrink the scope.
17. As a person, I want a finish with unchecked units blocked and a note naming them resent to the live session, so that the agent finishes the job.
18. As a person, I want the block to stop after a small cap of resends, counted per launch and reset by Retry, Start over and relaunch, and pause the ticket as `iteration-error` naming the unchecked units, with Retry and Start over on the web, so that a stubborn agent cannot loop forever.
19. As a person, I want tickets without `## Stories` and the collect ticket exempt from the block and the new note source, so that old tickets and the collect step behave as before.
20. As a person, I want gx's "Went wrong" iteration entry to list the ticket's event counts (resends, relaunches, launch failures, recovery escalations, finish blocks, skill refreshes, skill retries), and say "none recorded" only when all are zero, so that a resend storm is visible.

Process-storm watchdog

21. As a person, I want a normal `gx check --full` in an agent's tree to finish without a kill, so that verify collect tickets can run.
22. As a person, I want the default `watchdog.max-tree-processes` to be 500 and a kill to need 10 consecutive samples over it, so that a short peak is not a storm.
23. As a person, I want a sample at or under the limit to reset the count, so that only a sustained run of samples kills.
24. As a person, I want a new key `watchdog.hard-max-tree-processes`, default 1000, that kills on the first sample over it, so that a fast fork storm still dies at once.
25. As a person, I want the hard key on the settings page with the other watchdog keys, so that I can change it.
26. As a person with an explicit `watchdog.max-tree-processes`, I want it to stay exactly as I set it, and the effective hard ceiling never to sit below it, so that raising the limit in config cannot make the new default ceiling kill earlier.
27. As a person, I want interactive sessions to use the same limit, streak and ceiling, so that one rule covers every tree gx watches.
28. As a person, I want the `process storm` log line and the pause text to list the top process names with counts and say whether the streak or the hard ceiling fired, so that I can diagnose the tree.

Unknown skill

29. As a person, I want gx, on an "unknown skill" launch failure, to run the in-process refresh of the remembered skill install and launch once more, so that a missing skill needs no person.
30. As a person, I want a second "unknown skill" failure to pause the ticket as `agent_prompt_stalled` ("Could not start the agent") with the launch error, with Retry and Start over on the web, so that a skill gx cannot install is still recoverable.
31. As a person, I want a failed refresh to skip the retry and pause at once as `agent_prompt_stalled` with both errors in the pause text, so that I see why the install failed.
32. As a person, I want the refresh and retry to cover Claude and Codex launches, so that neither harness needs a person.
33. As a person, I want the refresh and the retry listed in gx's "Went wrong" line, so that the record shows what gx did.

Stored data

34. As a person, I want every stored-data change here to be additive, so that tickets, run logs and configs written before the upgrade load unchanged.

## Implementation Decisions

Note file and bare command

- The launch prompt builder takes no note. It returns the skill command and ticket address. `launchNote` keeps building the note text and the launcher writes it to a file in the ticket store, derived from the ticket address. The implementer picks the file name and the delete hook. A launch with an empty note removes any stale file.
- The system-prompt sentence is one line (herdr refuses launch arguments with a newline): "Before the skill's first step, read `<note file path>` and treat it as part of your launch prompt." Claude gets one per-launch prompt file, written next to the note file, that holds the loop prompt plus this sentence. It is the only `--append-system-prompt-file` on the command (no `--append-system-prompt`), on launch and on resume. This replaces the shared loop prompt file (`harness.GXSystemPromptFile`) for launches with a note; launches without a note keep it. Codex has no prompt-file flag. It keeps `developer_instructions`, which holds the loop prompt plus the sentence; a Codex resume does not repeat it (the instruction survives a resume), and the note file path is the same on every launch. No skill edits and no path in the command.
- The stall nudge already resends the bare launch command (it never carried the note), so it needs no change.
- Probe evidence (ticket 01): the sentence worked 4 of 4 times on Claude and 3 of 3 on Codex 0.169.0. A path in the command worked 0 of 4 times on Claude, so it is not used. No length threshold: one path for every note.
- Both loop launch callers share the start-and-prompt step, so the conflict-resolution agent's bare command gets the proof changes below too.

Launch proof

- One shared proof function serves the herdr runner, the PTY runner (Claude and Codex) and the interactive first-prompt path. The changes live there.
- The first-user input becomes "the first matching person record after the count taken before the first send". Codex already does this with its resume offset. Claude gets the same: a fresh session counts 0, a resumed one counts its existing person records. With a bare command the old first record equals the new text, so without the count a resumed launch would be "proven" by the old record. Representation of the count is the implementer's choice.
- A new input tells the proof whether the runner reports the agent working (the runner's own status reading). While it does, the proof does not resend. The grace clock keeps running. If the grace ends with no record, the existing not-delivered recovery applies unchanged (stop, confirm gone, relaunch once, then pause as `agent_prompt_stalled`). With the count above and a bare command, a working agent without a record means a paste or a lost send, not a resume.
- Pasted record: a person record after the count that carries Claude Code's `<pasted_content` wrapper around the command returns its own error, wrapping the not-delivered error and not the hung-pane error. It follows the same recovery (one relaunch, then `agent_prompt_stalled` worded "the launch command was pasted as plain text and the skill did not run"). This check is Claude-only: the wrapper is a Claude Code transcript format seen only in Claude transcripts (ticket 01 evidence). Codex gets the same protection from the note file, which removes the long text.
- The hung-pane error stays for a pane whose input still holds gx's text with no record. No new pause kind and no new way into one: `agent_prompt_stalled` is reached the same way as today, and Retry and Start over end it.

Wait-for-notice prompt line

- One sentence added to the Claude loop prompt, next to the ask and files sentences, saying to wait for the task completion notice and not to poll or call `ListAgents` in a loop. The copy between the `gx-system-prompt` markers in the local-tracker skill is updated with it (a test keeps them equal).
- Claude only. Codex gets no change: a Codex session has no completion notice for a background task (gx reads only a title heuristic there, see the background-task research), so telling it to wait would stall it.

Verify story list and finish block

- `verify-plan` writes `## Stories` as it builds each lone verify ticket and each verify part. The line format is strict, one unit per line, parsed by a strict parser like `## Verified`. A unit is a numbered story or a Mock states line. The unit key in `## Stories` and in `## Verified` must be the same string; the exact line grammar is the implementer's choice. The agent never edits the section.
- The relaunch note source is `## Stories` minus units whose latest live `## Verified` entry equals HEAD. Units with no entry or another commit go under "check these". Tickets without `## Stories` keep today's source (`## Verified` only) and still get the scope sentence from story 8.
- Finish check: when a lone verify ticket or a verify part reports finished with unchecked units, gx does not finish it. It resends a plain-text message naming the units to the live session. This is a typed message in the running session, not a slash command, so paste handling does not matter. After a small capped number of resends (the implementer picks 2 or 3) the ticket pauses as `iteration-error`. The count is kept per launch and resets on Retry, Start over and relaunch. Before it sends a resend, gx clears the agent's `finished` iteration status, so the agent's next finish is checked again. The cause names the unchecked units. The collect ticket and tickets without `## Stories` are exempt. The implementer picks the hook point in the finish path and the name of the run-log event for a block.
- Why gx cannot recover alone at the cap: the agent ignored the units after resends, and another resend would repeat them. The web buttons that end the pause are Retry (relaunch on the same branch, keeping commits and verdicts) and Start over. This path is the one new entry into the existing `iteration-error` kind (ADR 0042).
- gx's "Went wrong" lists counts of resends, relaunches, launch failures, recovery escalations, finish blocks and skill refreshes from the run log. It says "none recorded" only when all are zero.

Process-storm watchdog

- The watcher counts consecutive samples per root. A sample over the limit adds one, a sample at or under it resets the root. A root that leaves the watched set drops its count. The report fires when the count reaches the streak constant, 10 (a code constant, not a config key). A sample over the hard ceiling reports at once with no streak.
- Config: `max-tree-processes` default 200 becomes 500. New key `watchdog.hard-max-tree-processes`, default 1000, equal to the `watchdog.ulimit-headroom` default (1000). The effective hard ceiling is the larger of the two keys, so an explicit higher limit is never undercut by the default ceiling. A value of 0 or less is ignored and the default is used, like `max-tree-processes` (`config/config.go`). It does not turn the ceiling off.
- Relation to the per-agent ulimit: with both at 1000, the ceiling kills on the first sample over 1000 and the ulimit is the backstop for a fork that outruns the sampling interval. Explicit values in existing configs stay as set, so no config migration.
- New key checklist (`.ai/index.md`): a registry entry with category, section and group in the settings tree, regenerated schemas (`docs/config-schema.json`), and `TestSettingsGuard` green.
- Interactive sessions go through the same watcher, so they get the same rule. The kill path is unchanged.
- The foreign-tree banner keeps its one-sample rule (it kills nothing). It reads the new default limit and ignores the hard ceiling.
- The report carries the top process names (by `Comm`) with counts and which rule fired. The number of names listed is the implementer's constant. The `process storm` log line and the pause text show them. The pause reason keeps its existing wording that a retry would storm again, and adds the names and the rule. Pause kind `process-storm` and its Retry and Start over are unchanged; only the reason text changes.

Unknown skill

- The refresh and retry sit in the shared start-and-prompt step through a hook the launcher fills, because that step cannot import the skills command. Both loop callers use it.
- Flow: the existing two in-session attempts end with the unknown-skill error. The hook runs the in-process refresh (the same function the server uses for the remembered install; it writes every agent root, so Claude and Codex are covered), then the step starts once more. Same error again: the launch fails with the launch error and the existing skill hint, and the ticket pauses as `agent_prompt_stalled` ("Could not start the agent"), as today. Refresh failure: no retry, pause at once with both errors, same kind. The cherry-pick conflict agent keeps its existing child-ticket repair pause with the same text.
- `cmd/skills.Refresh` prints and records its problem and returns nothing today. It changes to return the problem as an error. The server's existing caller ignores the return value; the launcher uses it.
- The refresh is serialized in the launcher hook (single-flight). A launch that arrives while a refresh runs waits for it and shares its result.
- Why gx cannot recover alone after the retry: the refresh already ran and the agent still does not know the skill, so the install is broken in a way gx did not detect. Retry and Start over on the web end the pause, and each launch runs the refresh again. The pause kind is the existing `agent_prompt_stalled`, so this adds no pause entry.
- The skill hint on the final failure says gx already refreshed the install. The refresh and the retry are run-log events (names are the implementer's) that "Went wrong" counts.

Stored data and schema

- `## Stories` is a new optional ticket body section. It is a section-name change, so it needs a new named migration step in `migrations/steps.go`: `verify-stories`, a no-op (`Run` returns no changes) like `model-fields`, because tickets without the section stay valid and exempt. Never rename it. Then update `migrations/testdata/schema.snapshot` with `go test ./migrations -run TestSchemaSnapshot -update-schema-snapshot` if the guard asks.
- New run-log event kinds (finish block, skill refresh, skill retry) are additive. Old logs never carry them.
- The hard key is a new optional config key; an absent value, or a value of 0 or less, takes the default. No migration.
- No pause reason, class or verb table changes: `iteration-error`, `agent_prompt_stalled` and `process-storm` each keep their row and web verbs (Retry, Start over). The unknown-skill failure keeps `agent_prompt_stalled`; the finish-block cap (stories 17-18) is the only path that newly reaches `iteration-error`.

## Testing Decisions

- A good test drives external behaviour: what the fixture server, run log, pause, card, settings page and CLI show. It does not assert on private helpers.
- Highest seam: the gx fixture server with fake agents (the PTY and herdr-fake runners, the fake Claude TUI with a transcript, the fake Codex with a rollout directory), driven through the web e2e flow "Every pause reason recovers" (`pause.e2e.ts`) and `gx server` verbs. Lower seams only where the fixture cannot produce the fault: launch-proof tests over fake input funcs, watcher tests over a fake process table, and `verify-plan` parser tests.
- Prior art: the launch-proof and hung-pane tests, the `hung-pane-twice` and `process-storm` scenarios (the storm scenario sets its limits through `watchdog.*` config), the watcher poll tests with a table reader, the verified-entry parse tests and the settings-guard test.
- Web flows follow the e2e budget: add steps to existing flows on the same scenario before adding a flow, and keep the catalog and `maxFlows` in step.
- Needed fake-agent knobs: a Claude TUI knob that records typed input over a set length as a `<pasted_content>` record with `queueSkipSlashCommands`, a Claude transcript with a `<pasted_content>` first record, a resumed transcript that already holds an earlier launch record, a working agent that writes no record, a fake skill registry that learns the skill after a refresh, a verify agent that finishes with unchecked units, and a fork that spikes over the limit for fewer than 10 samples and falls back.

## Verification

Entry N verifies story N. "Pause flow" is the web e2e flow "Every pause reason recovers" in `pause.e2e.ts`. "Settings flow" is the settings page flow in the catalog. No stored mocks exist for this map, so there is no Mock states list. Scenarios and knobs named "new" become implementation tickets.

1. Fixture verify ticket with a long relaunch note, run with the new fake Claude TUI knob on (typed input over a set length becomes a `<pasted_content>` record with `queueSkipSlashCommands`): the first user record is a `<command-name>` record, not `<pasted_content>`, and the skill ran. The scenario fails before the change. New scenario.
2. Same scenario, once with a Claude fake and once with a Codex fake: the typed launch prompt (the fake's received input) is only the skill command and the address.
3. For a verify relaunch, a summaries ticket, a trunk-conflict ticket and an investigate ticket: a note file exists in the ticket store with the note text before the agent starts.
4. Retry and Start over rewrite the file with the new note; closing the ticket removes it. A launch with an empty note after a note launch leaves no file.
5. After the ticket's worktree commits, `git -C <worktree> status` shows no note file and no commit holds it.
6. The fake's launch arguments carry one `--append-system-prompt-file` for Claude (no `--append-system-prompt`), and the file holds the loop prompt and the sentence with the note file path; `developer_instructions` carries it for Codex on launch. Claude checks launch and resume.
7. A ticket with no note: the arguments hold no sentence and the store has no note file or per-launch prompt file.
8. `rg` over the note text in the fake's read file: it contains "verify every story in the ticket" and the lists; the fake verify agent that follows it verifies all stories.
9. Resumed Claude transcript whose first record equals the bare command: the old record does not prove the launch; a new record after the count does. Same table test for Codex with its offset.
10. Fake Claude that records the command as `<pasted_content>`: the launch fails with the pasted-command reason, not the hung-pane text, relaunches once, and the pause flow shows `agent_prompt_stalled` with Retry and Start over; Retry ends it. A Codex fake is not tested for this error (Claude only).
11. Fake that works without writing a record: the fake's received input holds the launch prompt once; no resend. Table test over the proof function for the herdr runner, PTY Claude, PTY Codex and first-prompt callers.
12. `gx server logs` for stories 1, 9 and 11 shows no `prompt-nudged` event; a fake that drops the prompt while idle still shows resends.
13. `rg` over the Claude loop prompt and the local-tracker skill's `gx-system-prompt` block finds the wait-for-notice sentence in both, and the prompt-equality test passes. The Codex prompt does not carry it.
14. `gx tickets verify-plan` on a spec with stories and a Mock states line: each verify ticket and part has `## Stories` with one strict line per unit. Parser test with a bad line.
15. Same output: the Mock states line is a unit; a fake verify agent writes a `## Verified` entry for it with the same key.
16. Hand-trim `## Verified` of a relaunching verify ticket: the note still lists every unit not verified at HEAD (the fake's read note file).
17. Fake verify agent finishes with two unchecked units: the ticket does not finish, the fake receives a message naming them, and the run log shows the block event.
18. Fake keeps finishing unchecked past the cap: the pause flow shows `iteration-error` naming the units with Retry and Start over; Retry relaunches on the same branch, keeps commits and verdicts, and gets a fresh resend cap (the fake blocks and resends again up to the cap). The run log shows the `finished` status cleared before each resend. New pause-flow row.
19. A verify ticket without `## Stories` and a collect ticket finish with no block; `gx tickets migrate` reports nothing to migrate and the `verify-stories` step is registered.
20. After a run with a resend, a relaunch, a finish block, a skill refresh and a skill retry, the gx entry in `## Iteration summaries` lists each count; a clean run says "none recorded".
21. A fake agent that runs a tree of about 240 processes for 13 seconds (the measured shape of `gx check --full`) under default config: not killed, the ticket finishes. New knob. `gx config get watchdog.max-tree-processes` prints 500.
22. A fake that stays over a test limit: killed on the tenth consecutive sample and not before (watcher test with a table reader; fixture scenario with the interval at 200ms). The `process-storm` scenario still pauses with the card "Too many processes".
23. Watcher test: nine samples over, one under, nine over: no report. A fake with a spike shorter than 10 samples is not killed.
24. `process-storm` scenario with the hard key below the fork size: killed on the first sample over it, with no streak wait. `gx config get watchdog.hard-max-tree-processes` prints 1000.
25. Settings flow: the hard key shows with the watchdog keys and saves. `TestSettingsGuard` and `TestGeneratedSchemas` pass.
26. Config with `max-tree-processes=3000` and the default hard key: the effective ceiling is 3000 (watcher test); an old config with an explicit limit loads unchanged. A config with `watchdog.hard-max-tree-processes=0` (or negative) loads with the default 1000 as the key, so the ceiling stays on (config test).
27. An interactive session whose tree spikes under 10 samples survives; one over the hard ceiling dies at once (fixture session scenario or watcher test over the session roots).
28. After a storm kill, `~/.local/state/gx/server.log`'s `process storm` line and the pause card list the top process names with counts and the rule that fired. Pause flow step on the `process-storm` row.
29. Fixture launch where the fake registry lacks the skill until a refresh: the run log shows the refresh and the retry, the second launch succeeds and no pause appears. New knob.
30. Fake that never learns the skill: extend the existing "Unknown skill" flow in `webui/e2e/pause.e2e.ts`, which already asserts "Could not start the agent", the "does not know the skill" text and the `gx skills install` hint, so it keeps showing `agent_prompt_stalled` with the launch error and the skill hint, now also checking that gx refreshed first; Retry and Start over stay, and Retry runs the refresh again and ends the pause once the skill is known.
31. Fake whose refresh fails: no retry, `agent_prompt_stalled`, and the pause text holds both errors. Two launches that overlap share one refresh (launcher test).
32. Stories 29 and 30 run once with a Claude fake and once with a Codex fake.
33. The `## Iteration summaries` gx entry for stories 29 to 31 lists the refresh and the retry.
34. The load tests pass on fixtures written before the change: a ticket without `## Stories`, a run log without the new events, a config without the hard key. `gx tickets migrate` reports nothing.

New flows or scenarios to write (these become implementation tickets): the pasted-record, resumed-record and working-without-record fake knobs; the long-note launch scenario for Claude and Codex; the unchecked-units verify agent; the spike fork and the hard-ceiling storm; the skill-registry fake with a refresh outcome. Steps go into existing flows first, per the e2e budget.

## Out of Scope

- A filtered mode for `ListAgents` (Claude Code owns the tool).
- Finding Claude Code's exact paste cutoff. The fix does not depend on it.
- The wait-for-notice prompt line for Codex (no completion notice there).
- The pasted-record check for Codex (the wrapper is a Claude Code transcript format).
- The foreign-tree banner's rule (it kills nothing) and a config migration for the watchdog default.
- The cherry-pick conflict agent's pause kind and an unknown-skill fix for interactive sessions, which have no ticket to pause. Interactive sessions keep showing the error at once.
- Finding why a fresh worktree lacked the skill.

## Further Notes

- Research files: `docs/research/pr-flow-49-prompt-nudged.md` (the 18 nudges were launch resends after a resume, not stall nudges) and `docs/research/process-tree-peak-gx-check-full.md` (peak 234 to 239, over 200 for at most 13 s in a row, never over 300).
- Derived choices, with their reason: the effective hard ceiling is the larger of the two keys, because decision 04 says explicit configs stay as set; the unknown-skill hook sits in the shared start-and-prompt step, because the fix belongs where the launch fails and not in one caller; the foreign banner is unchanged, because it kills nothing.
- Why the count in story 9 matters beyond Codex: once the command is bare, a resumed Claude session's old first record equals the new text. Prefix matching alone would then call every resume delivered.

- Confirmed design choices (decided during design, confirmed with no change needed):
  - Question: what does the proof do when the grace ends and the agent is working with no record? Choice: the existing not-delivered recovery. Reason: the map forbids resends, not the relaunch; a working agent with no record after the count is a paste or a lost send.
  - Question: does the verify finish-block message have to be a bare command? Choice: no, it is plain text in the live session. Reason: paste handling only breaks slash commands.
  - Question: does the wait-for-notice line apply to Codex? Choice: no. Reason: Codex gets no completion notice, so the line would stall it.

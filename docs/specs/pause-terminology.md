## Problem Statement

gx uses "park", "parked" and "unpark" for tickets and epics that stop and wait. Users must be told what these words mean. The same state covers very different causes: a question for a person, a stuck agent, a merge conflict, a budget limit. Other words are unclear too:
- "resume" means lifting the queue pause, relaunching an interactive session, and reusing an agent's last Claude session on a claim;
- "land" and "reset" do not say what they do;
- "pause" and "drain" differ only in whether they survive a restart, which nobody can guess.

## Solution

One state, **Paused**, for the queue, epics and tickets. Every Paused thing carries a **pause reason** that says why. The button on a paused item is named after what the person does ("Answer", "Retry", "Resume"). **Resume** is shown only for a pause that a person chose.

The related words change as well:
- **land** becomes **merge**;
- **reset** becomes **start over**;
- the interactive session relaunch becomes **Restore**;
- the queue gets two controls that end in the same saved Paused state: **Finish and pause** now, and **Pause now** later.

The event log, the code and the docs use the new words. Old words survive only in one read shim and in a few hidden aliases (the `park` and `unpark` verbs, the `D` key, the `drain` verb and the `resume-on-start` key), which a follow-up removes.

## User Stories

1. As a person using gx, I want a stopped ticket to show "Paused" with a reason, so that I understand it without reading docs.
2. As a person, I want the reason shown as plain words (for example "Waiting for your answer" or "Merge conflict"), so that I know what is wrong.
3. As a person, I want the explanation text beside the reason, so that I see the question or the failure without opening logs.
4. As a person, I want a ticket that paused to ask me a question to show an "Answer" button, so that the label says what I do.
5. As a person, I want my answer to resume the ticket by itself, so that I do not press a second button.
6. As a person, I want a ticket that paused for a fault to show "Retry", so that I can send it back to the queue after I fix the cause.
7. As a person, I want "Resume" shown only on a pause I chose myself (my own ticket pause, or the queue pause), so that the word never hides a required action.
8. As a person, I want a ticket that paused on an ambiguous base to show "Choose base", and a ticket with a land conflict to show "Resolve conflict", so that each button says the exact action.
9. As a person, I want to pause a ticket that is not running by hand, with a required one-line reason and the label "Paused by you", so that I can hold it back. Pausing a running ticket is still refused, and a done ticket cannot be paused.
10. As a person, I want an epic with no runnable work and a paused ticket to show as Paused with the first paused ticket's reason and "+N more", so that I know where to act.
11. As a person, I want a merge-blocked epic to show "Paused: merge blocked" with its existing merge action, so that I do not learn three words for "stuck".
12. As a person, I want a deadlocked epic to show "Paused: dependency deadlock", so that it uses the same word as every other stop.
13. As a person, I want a draft ticket to show as "Draft", never as Paused, and an epic with only drafts left to keep its current state.
14. As a person, I want a broken ticket (unparseable file, bad dependency graph, unknown status) to keep showing as "Error", never as Paused, because nothing can be resumed until the file is fixed.
15. As a person, I want map-epic tickets never to be Paused, so that `pause` refuses them and `migrate` skips them.
16. As a person, I want the queue to have **Finish and pause**: no new tickets start, running tickets finish, then the queue stays Paused, so that I can prepare a server restart.
17. As a person, I want the Paused queue to survive a server restart, so that a restart never starts work I held back.
18. As a person, I want the queue header to say what is happening ("Paused. 3 tickets still running." and "Paused. No tickets running."), so that I never check by hand.
19. As a person, I want the queue pauses that lift themselves (rate limit, smart zone) to keep their current wording and show no Resume button, so that Resume is never offered for a stop that ends on its own.
20. As a person, I want the budget limits to show as queue pauses with the reasons "budget soft limit" and "budget hard limit", so that nothing I know changes.
21. As a person, I want "resume" to mean only "lift a pause", so that the word is never ambiguous.
22. As a person, I want relaunching an Exited interactive session to be called "Restore session", and its config key to be `restore-sessions-on-start`, so that it is not confused with resuming a pause.
23. As a person, I want the action `land` renamed **merge** (shown with its target, for example "Merge into the feature branch"), so that the word needs no explanation.
24. As a person, I want the action `reset` renamed **start over**, with the help text "keeps a backup branch", so that I know what it does and what it keeps.
25. As a person with existing tickets, I want `gx tickets migrate` to rewrite old statuses and `park_kind` to the new form, so that my store moves in one step, and to print each chosen reason in its dry run.
26. As a person whose store is not yet migrated, I want gx to still read the old statuses, fields and log words, so that gx works while servers and stores move at different times.
27. As a person with scripts, I want `gx server tickets park` and `unpark` to keep working as hidden aliases for one release, so that my scripts do not break.
28. As a person who runs `gx server tickets` with `--wait`, I want the exit codes 3 and 4 to stay tied to the answer class and the repair class, so that my scripts do not change.
29. As a person, I want notification headlines to keep distinguishing "needs your answer" from "needs repair", so that I can tell at a glance what to do.
30. As a maintainer, I want one follow-up that removes the old names and the read shim (`gx:follow-ups/70`), so that the shims do not live forever.
31. As a maintainer, I want the event log, the recovery catalog and the code to use only the new words, so that no conversion logic spreads through the code.
32. As an investigator, I want a note in the gx-investigate skill that lists the old words and their new names, so that I can read old logs and retros.
33. As an agent that asks a question, I want `gx tickets ask` to work as before, so that the question flow is unchanged.
34. As a person, I want `gx server tickets merge` and `gx server tickets start-over` to do what `land` and `reset` did, so that only the names change.
35. As a reader of old specs and ADRs, I want the GLOSSARY entries for the retired words rewritten to point to the new ones, so that I can map old text to the new names.

## Implementation Decisions

- **Status.** A ticket gets one new stored status, `paused`. It replaces the stored statuses `needs-answer` and `needs-repair`. `draft` stays its own status. `error` is not stored in any file: the loader derives it for a broken ticket, and it stays a derived "Error" state outside Paused.
- **Fields.** One stored frontmatter field, `pause_reason` (the enum), which replaces `park_kind`. `pause_reason_text` is a derived, read-only value (an API field): the question's summary line, or the first line under the ticket body's `## Paused` heading, read fresh from disk. It is never written to frontmatter, so it cannot go stale (ADR 0026). The body headings `## Needs Repair` and `## Needs Answer` become one heading, `## Paused`. The read shim accepts the old headings, and `migrate` rewrites them.
- **Reason enum.** The enum is the set of existing failure and recovery kinds, with the new words (for example the manual pause is `manual`, and the epic kind `all-parked` becomes `all-paused`). The implementer chooses the exact new spellings, and no kind keeps "park" in it.
- **Pause class.** Each reason has a class, `answer` or `repair`, derived from the reason. Every rule that used to key on the stored status keys on the class instead: which verbs work, the unpark liveness and commits-ahead rules, the late-finish adoption (including a merge ticket whose agent stopped without a report), `--wait` exit codes 3 and 4, notification wording, counts, and the evolve exports. Behaviour does not change. The class table is part of the implementation and has a test.
- **Answerable.** Only a Paused ticket with reason `self-reported` takes an answer, and only while it has an open question. **Answered** stays derived: a ticket that is `open` again after an answer and that gx has not claimed.
- **Resume dispatch.** The CLI and the server expose one `resume` verb (the old unpark) and one `pause` verb (a manual pause). `resume` dispatches by reason: it keeps the base choice for an ambiguous base, the prompt to a live zero-commit agent, the late-finish adoption, the staleness check, and the liveness refusal for a live `self-reported` ticket. `merge` and `start-over` stay their own verbs. A manual `pause` needs a one-line reason, which is written under `## Paused`. It is refused for a running ticket, a done ticket and an already paused ticket. Stopping a running ticket is "Pause now" (`gx:follow-ups/71`).
- **UI labels.** The UI shows the action by reason, not a generic button: "Answer" for `self-reported`, "Choose base" for an ambiguous base, "Resolve conflict" for a land conflict, "Merge commits" or "Start over" for stranded commits, "Retry" for every other fault, and "Resume" only for `manual` and for queue pauses a person lifts.
- **Epics.** The epic states `parked`, `merge-blocked` and `deadlocked` collapse into Paused with a derived reason. When any ticket is paused, the epic shows the reason of the first paused ticket in the epic's ticket order (by ID) and "+N more". Otherwise it shows its own reason ("Dependency deadlock" or "Merge blocked"). For a merge-blocked epic the action is its existing merge action. A person resumes the ticket, not the epic. There is no manual epic pause. Epics with only drafts or only blocked-but-open tickets keep their current states. The `EpicSummary.State` values and the ticket status values change on the wire, so the generated TypeScript types and the API client change with them, and the API version that `Negotiate` checks is bumped. A TUI and a server from different builds then fail cleanly instead of mis-rendering.
- **Map epics.** Tickets of map epics are never Paused. `pause` refuses them and `migrate` skips them.
- **Queue.** The queue has two states of interest, Running and Paused. Its controls:
  - **Finish and pause** is today's pause under a new name, and today's drain is dropped. No new tickets start, and running tickets finish. The queue stays Paused, saved across restarts, until Resume. The server drops its `draining` mode. The TUI `D` key and `gx server queue drain` stay for one release as aliases of Finish and pause, and `gx:follow-ups/70` removes them. The state file does not change. The scheduler's own "drain complete" notification is checked: the implementer finds out whether anything relies on it and removes it if not.
  - The queue header shows the live state ("Paused. N tickets still running." and "Paused. No tickets running."). The "Safe to restart" wording comes later (`gx:follow-ups/71`).
  - The budget limits and the person's pause stay as queue pauses with their reasons, lifted by Resume. The implementer checks whether a budget pause also lifts itself at the daily budget reset. If it does, it belongs with the self-lifting pauses below and shows no Resume button, or Resume is shown only as an early override. The rate-limit and smart-zone pauses lift themselves, keep their current wording, and show no Resume.
  - **Pause now**, the confirm dialogs and the "Safe to restart" indicator come later (`gx:follow-ups/71`). Until then the Queue tab shows only "Finish and pause".
- **Verb renames.** `land` becomes `merge` and `reset` becomes `start-over` (the CLI verb is `gx server tickets start-over`, the UI says "Start over"). The run-log events follow (for example `manual-land` becomes `manual-merge`, and `ticket-reset` becomes `ticket-started-over`). Epic-level merge states keep their names. The glossary says that a ticket's commits are merged, and that a merge ticket is a ticket that merges.
- **Session relaunch.** Relaunching an Exited interactive session is renamed **Restore**, in the UI, the docs and the config key `restore-sessions-on-start`. The agent session reuse on a claim stays internal and keeps the Claude `--resume` flag.
- **Events and logs.** New writes use the new words everywhere: event types, kinds, notification kinds and run-log lines. The recovery catalog and its tests are updated to match, so they are part of this refactor. Old log lines and old tickets stay readable through one read shim at the single place where logs and tickets are loaded. No other code may use an old word.
- **Agent-facing status.** The `iteration_status` value `needs-answer`, written by `gx tickets ask` and accepted by `gx tickets set --iteration-status`, is renamed `asking`. Agents do not type it, since `gx tickets ask` writes it. Existing tickets are read through the read shim. The gx system prompt and the skills are checked for the old value.
- **Other readers of the old words.** These are in scope and move to the new words: the stats script, the evolve export, the config schema descriptions, the API client method names (`TicketPark` becomes `TicketPause`, `TicketUnmute` stays unless it names an old word), the generated web types and the canned fixtures. One test fails if any old word is written outside the read shim.
- **Migration.** `gx tickets migrate` rewrites the stored statuses `needs-answer` and `needs-repair` to `paused` with a reason. The reason comes from the old `park_kind` for both statuses (a `needs-repair` park stamps it too). With no `park_kind`, `needs-answer` defaults to `zero-commit`, and `needs-repair` takes the reason of the latest pause event in the event log, else `iteration-error`. The stored status wins: when the class of the recorded reason disagrees with the stored status, the reason becomes the default for the status class (`zero-commit` or `iteration-error`), and the dry run flags the mismatch. It also rewrites the old body headings to `## Paused`. It skips map-epic tickets, is idempotent, runs after the existing park-section migration, and its dry run lists each chosen reason. Markdown stays the source of truth (ADR 0026).
- **Compatibility.** The loader accepts the old statuses and `park_kind` until `migrate` has run, and writes only the new form. This is a one-way door: an older gx rejects `status: paused`. All gx binaries must be updated together before `migrate` runs, and the changelog says so. The old `park` and `unpark` verbs and the `resume-on-start` key stay as hidden aliases for one release.
- **Labels.** The UI shows a plain label for each reason, in these groups. The implementer may adjust the wording, not the grouping.
  - Waiting for a person: `self-reported` "Waiting for your answer", `ambiguous-base` "Choose a base".
  - Agent stopped or stuck: `zero-commit` "Agent stopped without a result", `merge-unreported` "Merge agent stopped without a result", `blocked-pane` "Agent stuck", `spinning` "Agent looping", `retry-exhausted` "Retries used up", `handle-mismatch` "Session mismatch", `duplicate-live` "Already running", the three herdr launch faults "Could not start the agent".
  - Background task: `background-task-gate` "Background task still running".
  - Merging: `land-conflict` "Merge conflict", `ambiguous-land` "Unclear what to merge", `dirty-trunk` "Trunk has local changes", `trunk-missing` "Trunk not found", `stranded-commits` "Commits left in the worktree", `land-error` "Merge failed".
  - Ticket graph: `blocked-cycle` "Dependency loop", `parent-defect` "Parent ticket missing or wrong".
  - Limits and other: `budget-killed` "Daily budget reached", `iteration-error` "Run failed", `manual` "Paused by you".
  - These names are the old kind spellings shown for readability. The enum itself follows the "Reason enum" decision, and the implementer renames the ones that contain old words (`land-*`, `*park*`).
- **Docs.** `GLOSSARY.md` is rewritten, not only annotated. New entries: Paused, Pause reason, Pause class, Resume, Restore, Merge (ticket level), Start over, Finish and pause. The Parked, Unpark, Park kind, Park reason, Park round, Deadlocked, Answered, Gate and Orchestration write entries are updated to use the new words. The old statements "Avoid Paused" and "Not resume" are removed. No skill keeps the words "land" or "drain", or any form of "park", even as plain English. These skill files are rewritten: `gx-local-tracker.md`, `gx-investigate` (`SKILL.md` and `gotchas.md`), `gx-implement`, `gx-evolve`, `gx-tdd`, `gx-resolving-merge-conflicts`, `gx-merge`, `gx-code-review`, `gx-cleanup`, `gx-to-tickets`, `gx-one-off`, `gx-wayfinder`, `gx-resolve-follow-ups`, `gx.md` and `gx-prototype/UI.md`. Plain-English uses are reworded too, for example "empty the follow-ups epic" for "drain", and "merged" or "applied" for "landed". The git command `git reset` may stay. The skills bundle tests that name the old words (`bundle_test.go`) are updated to match. `gx-investigate` also gets a short mapping of old to new words for reading old logs. The docs under `docs/` that describe the queue and recovery use the new words too. Old specs, ADRs and the changelog are left as written.
- **Naming of internals.** Identifiers in code, such as `isParked`, `parkTicket`, `UnparkForm`, `Gate.Drain` and the park timers, are renamed to match. The implementer picks the names, and none may contain an old word outside the read shim. The Go `Gate` type may keep its name, since it is internal.

## Testing Decisions

- A good test checks external behaviour: what a loaded ticket reports, what the CLI prints and writes, what the UI shows. It does not check internal names.
- **Seam 1, ticket store.** Load and write a ticket in its old and new forms, and check status, reason, class and the derived text. This also covers the loader read shim and the rejection of an unknown status. Prior art: the ticket loader and schema tests.
- **Seam 2, `gx tickets migrate`.** Run it on a store with `needs-answer` and `needs-repair` tickets (with and without `park_kind` and events), a map-epic ticket and a broken ticket. Check the rewritten files and the dry run, and run it twice to check it is idempotent. Prior art: the existing migrate tests, including the park migration test.
- **Seam 3, server verbs and epic summary.** Pause, resume, answer and retry through the server for each class, and check the epic state and reason for the paused-ticket, merge-blocked, deadlocked and drafts-only cases. Check `resume` for an ambiguous base, a live zero-commit agent, a late finish, and a live `self-reported` ticket. Check `--wait` exit codes 3 and 4 and the notification wording by class. Check the manual `pause` refusals (running, done, already paused, map-epic ticket), the new `merge` and `start-over` verbs, the hidden `park` and `unpark` aliases, and the `Restore` config key with its `resume-on-start` alias. Prior art: the server repair, answer and epic summary tests.
- **Seam 4, queue control.** Finish and pause with running tickets: the queue stays Paused afterwards, survives a restart, and the header text changes with the running count. Rate-limit and smart-zone pauses show no Resume. Prior art: the server pause tests and the queue screen tests.
- **Seam 5, web and TUI views.** Check the label and the action shown for each reason group and for the epic and queue levels. Prior art: the web view tests, the queue screen tests and the canned snapshot.
- **Old words list.** The words are: any form of `park` (parked, unpark, `park_kind`, `park.*` events), `needs-answer`, `needs-repair`, `land` as a verb or in an identifier or event name (`land-conflict`, `manual-land`), `drain` and `draining`, the ticket verb `reset` (not `git reset`), `resume-on-start`, and the body headings `## Needs Repair` and `## Needs Answer`. The test may allow them only in: the read shim, the hidden aliases, tests of the shim and the aliases, the old-to-new mapping in gx-investigate, the notes on retired terms in `GLOSSARY.md`, and old specs, ADRs and the changelog.
- **Seam 6, no old words.** A test searches the repo's code, skills and generated files and fails on any old word written outside the read shim (statuses, kinds, event and notification names, `iteration_status`, API client names). For the skills it also fails on any form of "land", "drain" and "park" in plain text, so the words cannot come back in prose.
- The recovery catalog tests are updated to the new words, and still prove that the same rules fire for the same causes.

## Out of Scope

- A manual pause for a whole epic.
- **Pause now** (Ctrl-C to running agents with a grace period), the confirm dialogs and the "Safe to restart" indicator (`gx:follow-ups/71`).
- Removing the old names, aliases and the read shim (`gx:follow-ups/70`).
- Renaming epic-level merge states.
- Rewriting old specs, ADRs and the changelog.
- Any change to the Claude `--resume` flag or to how a claim reuses a session.

## Further Notes

- Related follow-up: `gx:follow-ups/58` (unpark, resume and reopen are confusing). This spec removes the confusion it describes.
- The test seams follow the existing test layers and were not checked one by one with the user.

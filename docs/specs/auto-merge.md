# Auto-merge

Handoff spec from the `gx:auto-merge` map (`auto-merge/ticket.md` in the ticket store). Decisions
come from the map's brief and its closed tickets 01–10; this spec gathers them into one buildable
whole. Terms follow `GLOSSARY.md`: **Merge ticket**, **Complete epic**, **Merged epic**, **Dirty
trunk**, **Trunk**, **Base** / **Target**, **Blocker** / **Blocking**, **Parked**, **Park kind**,
**One-off**, `gx merge` and `gx-merge`. See ADR 0015 for the `gx merge` / `gx-merge` split and ADR
0036 for trunk resolution.

## Problem Statement

A person turns on `auto-ff-merge` in `project.json` so that finished epics land on trunk without
them. Today that only works when the epic's branch fast-forwards onto trunk. As soon as trunk has
moved (another epic landed, someone committed on trunk), the server parks the root with "needs
rebase: … run gx-merge" and waits. The person has to notice, open a shell, run `gx-merge` and then
clean up. With several epics running at once, the second one to finish nearly always hits this, so
"auto" merge is not automatic in practice.

Other problems around landing make it worse:

- If trunk is checked out in a worktree with local changes, `git merge --ff-only` fails, the server
  logs an error, and the root sits in the queue all-done, neither parked nor dequeued. It looks
  stuck and nothing tells the person why.
- Nothing records whether an epic has landed. "Epic complete" fires even when `auto-ff-merge` is
  off and nothing landed, and the Tickets tab cannot tell a complete epic from a merged one.
- A dependent can be claimed between the moment its blocker epic is all-done and the moment it
  lands. Under `on-landed`, dependents do not wait at all; the policy only picks the base.
- A ticket can block on one commitful ticket in another epic, but it cannot use that ticket's code
  until the whole epic lands, and the epic's rebase would strand it anyway.
- Two epics landing on one trunk at the same time are not serialized; the loser gets a git error
  instead of a clean "needs rebase".

## Solution

When `auto-ff-merge` is on and the server's fast-forward land fails because trunk has moved, the
server forks a **Merge ticket** (`type: merge`) into the epic. The merge ticket is a normal
scheduled ticket. It runs `/gx-merge --ticket <addr>` in the epic's existing feature worktree, which
rebases the epic onto its target, resolves conflicts, runs and repairs checks, and reports
`finished`. The epic is all-done again, and the server's normal landing step fast-forwards trunk.
The epic lands with no person involved, or the merge ticket (or the root) parks with a clear reason.

From the person's view:

- A finished epic whose branch has fallen behind trunk shows one more ticket, "(merging...)", in
  the Queue tab ("12 of 13"). When it finishes, the epic lands and chat says "🎉 epic merged".
- If the merge needs a person (an unresolvable conflict, a real test failure), the merge ticket
  parks `needs-answer` with the reason, like any other ticket.
- If trunk keeps moving, the server tries one more merge ticket, then parks the root ("needs
  rebase … then merge again").
- A dirty trunk checkout, or any other land error, parks the root with a reason that names the fix.
  Nothing sits in the queue looking stuck. The epic shows "Merge blocked" with that reason, and
  "Merging" while a merge ticket is open (see `epic-land-visibility.md`).
- Each epic records `merged_at` when it lands. Dependents wait for the land, the Tickets tab shows
  merged epics apart from complete-but-unmerged ones, and chat tells the two apart.
- A person can ask for a merge at any time with `gx server epics merge <project:epic>`, the
  `m` menu or the web Merge button, even with `auto-ff-merge` off. The same three ways merge again
  after a failed land.
- Cross-epic `blocked_by` waits on a whole epic. A reference to one commitful ticket in another epic
  is widened to that whole epic.

## User Stories

### Merge ticket on non-ff

1. As a project owner with `auto-ff-merge` on, I want a finished epic that cannot fast-forward onto
   trunk to get a merge ticket forked into it, so that it lands without me running `gx-merge`.
2. As a project owner, I want the merge ticket to be claimed through the normal queue and to count
   against the concurrency cap, so that a merge never jumps ahead of the scheduler.
2a. As a project owner, I want a crashed or budget-killed merge ticket to be reclaimed into the
   same feature worktree, so that a retry continues the same rebase instead of a fresh branch.
3. As a project owner, I want the merge ticket to be a top-level ticket in the epic (no `parent`),
   born `open`, so that it lives in the epic it is merging.
4. As a project owner, I want the merge ticket to run in the epic's feature worktree with no
   iteration branch, so that it rebases the epic's real branch.
5. As a project owner, I want the merge ticket's finish to mean "the epic can now fast-forward", so
   that a finished merge ticket always leads to a land.
6. As a project owner, I want the root to stay queued and unparked while the merge ticket runs, so
   that I do not see a "needs rebase" park for something gx is already handling.
7. As a project owner, I want the server, not the agent, to move trunk, so that trunk only ever
   changes through one guarded path.
8. As a project owner, I want a finished merge ticket to make the epic all-done again and land
   through the normal landing step, so that there is one landing path.
9. As a project owner, I want the root dequeued once the land succeeds, so that the queue only
   holds unfinished work.

### Retry cap

10. As a project owner, I want a second merge ticket if trunk moved again during the first, so that
    one unlucky race does not need me.
11. As a project owner, I want the root to park ("needs rebase … then merge again") once the epic has
    two merge tickets and still cannot fast-forward, so that gx never loops forever.
12. As a project owner, I want a non-ff caused by another epic's land to count against the cap like
    any other non-ff, so that the cap is simple and predictable.

### Merge ticket agent behaviour

13. As a project owner, I want the merge ticket to run `/gx-merge --ticket <addr>`, so that manual
    and ticket merges use one skill.
14. As a project owner, I want `gx-merge` in ticket mode to skip its final `gx merge` and report
    `finished` after rebase and checks pass, so that the server does the ff.
15. As a project owner, I want an unresolvable conflict to abort the rebase and park the merge
    ticket `needs-answer` with the conflicting files named, so that the feature worktree is clean
    when I look at it.
16. As a project owner, I want a typecheck or lint failure after the rebase fixed once by a fixer
    subagent that commits on the rebased epic branch, so that trivial breakage does not need me.
17. As a project owner, I want all checks rerun from the start after the fixer commits, and the
    merge ticket parked `needs-answer` if they still fail, so that the fixer gets one attempt only.
18. As a project owner, I want a failing test rerun three times by a triage subagent, so that a
    flaky test does not block the land.
19. As a project owner, I want a test treated as flaky only if it passes at least once and the
    epic's diff does not touch the test file or any non-test file in the test's Go package, so
    that real breakage is never waved through.
20. As a project owner, I want a flaky test to let the merge continue and get one follow-up filed
    through `gx-follow-up`, listed in the merge ticket's `## Resolution`, so that flakiness is
    tracked without blocking.
21. As a project owner, I want an epic-fault test failure to keep the rebased branch and park the
    merge ticket `needs-answer` with the check output, so that I can fix it from where it stopped.
22. As a project owner, I want a test that fails on trunk too to park `needs-answer` with a reason
    that says "fails on trunk too", so that I know the epic is not at fault.
23. As a project owner, I want a `gx merge --json` error or a dirty feature worktree to park the
    merge ticket `needs-answer`, so that every agent-visible stop has a reason.
24. As a project owner, I want a merge ticket that reports `finished` but fails the finish check,
    dies, or is budget-killed to park `needs-repair`, so that server-detected failures look like
    any other ticket's.
25. As a project owner, I want fixer commits to land with the epic, so that the merge ticket can
    stay commitless.
26. As a person running `/gx-merge` by hand, I want the same fixer and flaky-test triage, so that
    manual and ticket merges behave alike.

### Dirty trunk and other land errors

27. As a project owner whose trunk checkout has local changes that block the ff, I want the root
    parked with "uncommitted changes on `<target>` (`<dir>`) clash with `<files>`; commit or
    discard them, then merge again", so that I know exactly what to fix.
28. As a project owner, I want local changes that do not overlap the land to be ignored, so that a
    dirty trunk only matters when git actually refuses.
29. As a project owner, I want the file list in the dirty-trunk reason capped at about five, then
    "+N more", so that the reason stays readable.
30. As a project owner, I want no auto-retry after a dirty-trunk park, so that gx never touches my
    local changes.
31. As a project owner, I want a dirty trunk hit after a merge ticket to leave the merge ticket
    `done` and not use up the retry cap, so that the cap only counts real rebases.
32. As a project owner, I want any other land error to park the root with "land failed: `<git
    error>`; fix it, then merge again", so that no root ever sits all-done in the queue
    looking stuck.
33. As a Queue or chat reader, I want `dirty-trunk` and `land-error` event kinds on the root-park
    event, so that they are told apart from "needs rebase".

### Merged state and dependents

34. As a project owner, I want the server to stamp `merged_at` on the epic when its land succeeds,
    and `gx merge` to stamp it too, so that "merged" is recorded, not guessed.
35. As a project owner with `auto-ff-merge` on, I want `completed_at` stamped at the epic's final
    outcome (the land, or a root park), never when a merge ticket is forked, so that `gx evolve`
    always sees the merge tickets and still sees an epic that parked.
36. As a project owner with `auto-ff-merge` off, I want `completed_at` stamped at all-done, as
    today, so that nothing changes for me.
37. As a project owner, I want a whole-epic blocker to block until every ticket in it is terminal
    and the epic has `merged_at` (`on-landed`) or `completed_at` (`on-done`, plus `merged_at` when
    `auto-ff-merge` is on), so that there is no timing window between all-done and land, and a
    root park never releases dependents.
37a. As a project owner, I want an epic with nothing to land (no commits ahead of its target, or
    no branch) stamped `merged_at` at all-done, whatever `auto-ff-merge` says, so that its
    `on-landed` dependents are not blocked forever.
38. As a project owner using `on-landed` with `auto-ff-merge` off, I want dependents to wait for
    `merged_at`, so that `on-landed` actually waits for a land.
39. As a project owner, I want dependents to stay blocked if a ticket is added to a completed
    epic, so that reopening an epic is safe.
40. As a project owner who lands an epic outside gx (`gx merge` or a plain `git merge`), I want the
    server to stamp `merged_at` when it sees the epic's branch is an ancestor of its target, and
    dequeue a parked root with "epic merged", so that dependents are released and the queue clears.
    The check runs only for epics with a queued root or a waiting dependent, to keep idle cost low.

### Cross-epic `blocked_by`

41. As a ticket author, I want `other-epic` or `project:other-epic` in `blocked_by` to mean the
    whole epic, so that I can wait for an epic's land.
42. As a ticket author, I want a token in ticket-id form (`\d+[a-z]?` or `epic/NN`) to stay a
    ticket, and an all-numeric epic name to need `project:epic`, so that tokens are unambiguous.
43. As a ticket author, I want a whole-epic token naming another project, my own epic or a map
    epic rejected, so that I cannot write a meaningless dependency.
44. As a ticket author, I want `epic2/b` to block on `b` alone only if `b` is commitless, and
    otherwise be read as the whole `epic2`, so that I never depend on code that has not landed.
45. As a ticket author, I want the file to keep `epic2/b` and the widening to happen at resolve
    time, so that it narrows back if `b` becomes commitless.
46. As a ticket author, I want `gx tickets add` and `set --blocked-by` to print "`epic2/b` needs
    commits; waiting for all of `epic2`", so that I see the widening when I write it.
47. As a Queue or `show` reader, I want a widened blocker shown as "`epic2` (via `epic2/b`)", so
    that I understand what the ticket waits for.
48. As a ticket author, I want `add`, `set --blocked-by` and `epic set --blocked-by` to refuse
    dangling refs and cycles on the widened graph and write nothing, so that bad graphs never land
    in the store.
49. As a ticket author, I want cycle errors to name my original tokens and say they were widened,
    so that I can find the edge to remove.
50. As a person who hand-edits tickets, I want the loader, server, `validate` and `move` to run the
    same check and show a bad ticket blocked with the reason, so that hand edits behave like CLI
    edits.
51. As an epic author, I want whole-epic tokens allowed in an epic's own `blocked_by`, checked by
    `gx tickets validate`, and settable with `gx tickets epic set <project:epic> --blocked-by …`, so
    that a whole epic can wait on another.
52. As a project owner, I want a whole-epic blocker to count like a commitful ticket in that epic
    for base derivation, so that `on-done` dependents base on its branch and retarget to trunk once
    it lands, and `on-landed` dependents base on trunk.
53. As a project owner, I want an unknown epic token to error and never resolve, and an empty epic
    to block until it has `completed_at` / `merged_at`, so that edge cases fail safe.

### Queue, chat, counts and Tickets tab

54. As a Queue reader, I want a claimed merge ticket to show a "(merging...)" live phase with its
    own spinner, keyed on `type: merge`, so that I can see a merge in progress.
55. As a chat reader, I want "🎉 epic merged" when the land succeeds and "✅ epic complete (not
    merged)" when `auto-ff-merge` is off or the root parks at the retry cap, so that chat never
    claims a land that did not happen.
55a. As a chat reader, I want a dirty-trunk or land-failed park to send only the root-park notice,
    with no epic notice until the land succeeds and no `iteration-failed` event, so that one
    problem gives one message.
56. As a chat reader, I want no extra notice when a merge ticket is forked, and the usual park and
    done texts for it, so that chat stays quiet.
57. As a Queue reader, I want the merge ticket counted like any other ticket ("12 of 13"), so that
    the count reflects real remaining work.
58. As a gx-evolve user, I want merge tickets kept in evolve's input, so that merge trouble is
    signal for improvement.
58a. As a gx-evolve user, I want the epic's retro submitted only at its final outcome (the land or
    a root park), never when a merge ticket is forked, so that the retro sees the merge ticket.
59. As a Tickets tab reader, I want sections in the order Open epics → Open maps → Complete epics
    (unmerged) → Merged epics → Complete maps → Archived epics, so that I can tell what still needs
    landing.
60. As a Tickets tab reader, I want the three complete sections to collapse and expand like "Closed
    epics" does today, empty sections hidden, and zero-ticket epics kept open, so that the tab stays
    tidy.
61. As a project owner with already-landed epics, I want `gx tickets migrate` to set `merged_at =
    completed_at` once for each loop epic whose branch is gone or is an ancestor of its target, so
    that old epics show up as merged.

### One-offs with `--commits`

62. As a person who submits a `--commits` one-off, I want a non-ff land to fork a merge ticket with
    the same retry cap and dirty-trunk park as an epic, so that one-offs land the same way.
63. As a chat reader, I want one-offs to keep skipping epic-level notices (including "epic merged"
    and "epic complete (not merged)") and retros, but still get the park notice when their merge
    ticket parks, so that I only hear about one-offs when they need me.

### Manual trigger

64. As a project owner, I want `gx server epics merge <project:epic>` to fork a merge ticket, so
    that I can ask for a merge without running `gx-merge` myself.
65. As a TUI user, I want a "merge" item in the `m` menu on a parked root (Queue tab) and on an
    epic in "Complete epics (unmerged)" (Tickets tab), so that I can trigger it without a shell.
66. As a project owner with `auto-ff-merge` off, I want the manual trigger to re-add a dequeued root
    and mark the merge ticket `manual: true`, so that the epic lands once when it finishes, even if
    trunk moved and a normal retry merge ticket finishes last.
66a. As a project owner with `auto-ff-merge` off, I want a manual merge allowed even when `on-done`
    dependents already build on the epic branch, so that I am not blocked; their own merge tickets
    rebase them onto trunk later.
67. As a project owner whose root parked at the cap, I want a manual merge ticket to reset the cap
    and lift the park by itself (the epic is no longer all-done), so that I get a fresh try: the
    manual ticket plus one retry.
68. As a project owner, I want the manual trigger to land directly and report `merged` when the ff
    is already possible, so that no ticket is created for nothing.
69. As a project owner, I want the trigger refused with a code and nothing written for `map-epic`,
    `not-complete`, `already-merged`, `merge-pending`, `branch-missing` and `server-not-running`, so
    that it never does something surprising.
70. As a project owner whose epic worktree was removed but whose branch still exists, I want the
    server to recreate the worktree from the branch, so that the merge can still run.

### Concurrent lands

71. As a project owner running several epics, I want lands into one trunk serialized by a
    per-trunk lock, so that the second epic sees a clean non-ff and gets a merge ticket instead of
    a git error.
72. As a project owner, I want a second land of an already-merged epic to be a no-op that removes
    the root once and sends one "epic merged" notice, so that races cannot double-land or
    double-notify.
73. As a project owner, I want an ff error caused by trunk moving outside the server to be
    re-checked for ancestry and sent down the merge-ticket path, so that only real errors park as
    `land failed`.

### Worktree and branch after land

74. As a project owner, I want the server never to remove the epic's feature worktree or branch
    after any land, so that `on-done` dependents, re-opened epics and the manual trigger can reuse
    them.
75. As a project owner, I want a merge ticket's end-of-run to stop only the agent session, on
    finish, park or discard, so that the feature worktree and branch survive.
76. As a person running gx-cleanup, I want an epic with `merged_at` treated as merged (branch
    ancestry as fallback), so that a done epic with a deleted branch is not offered for merging.
77. As a project owner, I want `retargetIfLanded` and `rootRefs` to treat a blocker with
    `merged_at` as landed (ancestry as fallback), so that cleaning up a landed blocker's branch
    cannot break an `on-done` dependent that has not launched.
78. As a person running gx-cleanup, I want to keep its single up-front confirm and no per-merge
    diff review, so that cleanup stays one step.

## Implementation Decisions

### Merge ticket type and iteration mode

- New ticket type `merge`. Add it to `schema.IsCommitless` and to `commitlessByDesignTypes`
  (`tickets`), so "(commitless)" stays hidden in the Queue tab.
- The server forks it with `writeNewTicket(epic, "", …)`: a top-level ticket in the epic, no
  `parent`, status `open`. Follows the `conflict-resolution` precedent
  (`forkConflictResolutionTicket`) for system-generated tickets, but is scheduled, not run inline.
- New iteration mode in `iterationModeFor`: a `merge` ticket runs in the feature worktree
  `<worktreeDir>/<epic>` with no iteration branch. `PrepareIteration` already recreates a missing
  feature worktree from the branch, so "missing worktree" cannot happen in ticket mode.
- Finish check replaces the commits-ahead check for this mode: no rebase in progress in the feature
  worktree **and** the target is an ancestor of `<epic>`. Pass → `done`. Fail → park `needs-repair`
  with the reason.
- End-of-run stops the agent session only. Unlike iteration `finishCleanup`, it never removes the
  feature worktree or deletes `<epic>`, on finish, park or discard.
- Frontmatter: `manual: true` marks a merge ticket forked by the manual trigger.

### Landing flow (`completeRootIfDone` → `landRoot`)

- No new "needs merge" state. On non-ff, `landRoot` forks a merge ticket; the epic is no longer
  all-done, so the root stays queued and unparked. Merge ticket done → `settleRun` →
  `completeRootIfDone` → `landRoot` again. This replaces today's "needs rebase" root park except at
  the cap.
- Retry cap, derived from the store: count `type: merge` tickets in the epic from the latest
  `manual: true` merge ticket onward, that ticket included (all of them if none is manual). Order
  is ticket ID (allocation order). 0 or 1 → fork one. 2 or more → park the root as today ("needs
  rebase … rebase it, then merge again"). So every run gets 2 merge tickets at most.
- Retro: `submitRetro` runs only at the epic's final outcome, the land or a root park. Forking a
  merge ticket is not an outcome, so no retro goes out then.
- Per-trunk land mutex in the server, keyed by repo + trunk, held around the whole `landRoot`.
  Today only `landGuard` (a stop WaitGroup) exists.
- Inside the mutex, `landRoot` checks `merged_at` first. Set → return success with no git work.
- `landRoot` lands an epic when `auto-ff-merge` is on, or when a `manual: true` merge ticket exists
  in the epic and `merged_at` is unset. The rule reads the store, not the ticket that triggered the
  pass, so a normal retry inside a manual run still lands. Inside a manual run, a non-ff forks a
  normal merge ticket that counts against the cap.
- Nothing to land: at all-done, if `<epic>` has no commits ahead of its target or has no branch,
  stamp `merged_at` (and `completed_at`) with no git work, whatever `auto-ff-merge` says.
- `completed_at` marks the epic's final outcome, the same point as the retro:
  - `auto-ff-merge` on (or a manual run): stamped at the land, together with `merged_at`, or at a
    root park (cap, dirty trunk, land failed). Never when a merge ticket is forked.
  - `auto-ff-merge` off: stamped at all-done, as today.
  - This reverses ticket 01's "`completed_at` only at the land": `gx evolve all` skips epics with no
    `completed_at`, so a cap-parked epic must have one; and stamping it before the fork would let
    evolve take the epic before the merge ticket ran.
- Land errors, after git refuses the ff (no pre-checks):
  1. Re-check ancestry. Trunk no longer an ancestor of `<epic>` → merge-ticket path (counts against
     the cap).
  2. Else, trunk checked out somewhere and `git.UncommittedChanges` reports changes → **dirty
     trunk** park.
  3. Trunk missing → existing `trunk-missing` handling (configurable-trunk spec).
  4. Anything else → `land failed` park.
- Root parks (cap, dirty trunk, land failed) all use today's root-park path: a `root-parked` event,
  chat park as needs-answer, reason text, root stays queued. No ticket gets a status or
  `park_kind`. New event kinds on `root-parked`: `dirty-trunk` and `land-error`.
- Reason texts:
  - Dirty trunk: `uncommitted changes on <target> (<dir>) clash with <files>; commit or discard
    them, then merge again`. Only clashing files are listed, capped at about 5, then `+N more`.
  - Other: `land failed: <git error>; fix it, then merge again`.
  - "Merge again" means `gx server epics merge <project:epic>` or the web Merge button.
- A merge ticket never touches trunk. If the land after it hits a dirty trunk, the merge ticket
  stays `done`, the root parks as above, and no merge ticket is forked (cap untouched).
- The server never removes `<worktreeDir>/<epic>` or the `<epic>` branch after any land (merge
  ticket, direct ff, manual trigger). gx-cleanup is the only remover.

### Merged state

- `merged_at` is a new epic frontmatter stamp. **Merged epic** is derived from it; no status enum.
- `merged_at` is server-owned: add it to `tickets.EpicServerStamped` and to the `gx tickets set`
  docs, so `gx tickets epic add` / `set` can't write or clear it by hand. Only the server, `gx
  merge` and `gx tickets migrate` write it.
- `gx merge` stamps `merged_at` when it lands an epic branch, never `completed_at`. A parked or
  all-done epic already has `completed_at`; for an epic merged early by hand, the server stamps it
  at all-done.
- Poll-time ancestry check: each server poll, for epics with `completed_at`, no `merged_at`, **and**
  either a queued root or a non-terminal ticket blocked on them, the server runs the ancestry check
  `retargetIfLanded` already does (`<epic>` in its target). Pass → stamp `merged_at`; if the root is
  still queued (parked), dequeue it and send "🎉 epic merged". The scope keeps idle cost flat as
  unmerged epics pile up under `auto-ff-merge` off. An epic merged by plain `git merge` with no
  queued root and no waiting dependent stays in "Complete epics (unmerged)" until `gx tickets
  migrate` or the manual trigger (which reports `merged`).
- Leaving a root park: the dirty-trunk, land-failed and cap reasons all point to `gx server epics
  merge <project:epic>`. Ff already possible → it lands, dequeues the root and sends "epic merged";
  otherwise it forks a manual merge ticket. A person who runs `gx merge` or `git merge` instead is
  caught by the poll-time check above.
- `retargetIfLanded` and `rootRefs` treat a blocker with `merged_at` as landed and retarget to
  trunk; branch ancestry is the fallback.
- gx-cleanup's archive check treats `merged_at` as merged; `IsCommitMergedToTrunk` is the fallback.
- `gx tickets migrate` backfills once: for each loop epic with `completed_at` and no `merged_at`
  whose branch is gone or is an ancestor of its target, set `merged_at = completed_at`.

### Whole-epic `blocked_by`

- Syntax: a token that is a CLI epic address (`epic` or `project:epic`, same project only) names a
  whole epic. A token in ticket-id form (`\d+[a-z]?`, or `epic/NN`) stays a ticket. An all-numeric
  epic name needs `project:epic`. Another project, the ticket's own epic, or a map epic → rejected
  at write time, each with its own code.
- Resolution: a whole-epic token is blocking until every ticket in the epic is terminal **and** the
  epic has `merged_at` (`on-landed`), or `completed_at` (`on-done`) plus `merged_at` when
  `auto-ff-merge` is on. The `merged_at` part matters because a root park stamps `completed_at`.
  Owned by the shared resolver (`projectGraph.resolve` / `checkBlockedBy`).
- Widening at resolve time: a cross-epic ticket token `epic2/b` blocks on `b` alone only when `b`
  is commitless (`schema.IsCommitless`; prototype counts as commitful). Otherwise it resolves as
  the whole epic `epic2`. The file keeps `epic2/b`.
- Graph checks: a whole-epic token (written or widened) is an edge to every ticket in that epic.
  Cycle errors name the original tokens and say they were widened.
- Write-time: `gx tickets add`, `gx tickets set --blocked-by` and the new
  `gx tickets epic set <project:epic> --blocked-by …` run the project-wide check (dangling targets
  + cycles on the widened graph) before writing. Fail → refuse, nothing written. `add`/`set` print
  the widening notice; `validate` passes silently.
- Hand edits: loader, server, `validate`, `move` run the same check; a bad ticket shows blocked with
  the reason via `BlockedByErr`.
- Epic-level `blocked_by` in an epic's `ticket.md` accepts whole-epic tokens. `ValidateProject` now
  checks epic-level refs.
- Display: Queue tab and `gx tickets show` render a widened token as `epic2 (via epic2/b)`.
- Base derivation: a whole-epic blocker counts like a commitful ticket in that epic. `on-done` →
  that epic's branch, then `retargetIfLanded` → trunk once landed. `on-landed` → trunk. An epic with
  only commitless tickets gives no branch. Two or more candidate branches → existing
  `AmbiguousBaseError` park.
- Edge cases: unknown epic → error, never resolves. Empty epic → blocking until it has the stamp
  its policy needs.
- No store migration: the only existing cross-epic tokens target a commitless, done ticket.

### Merge ticket agent contract (`gx-merge` skill)

- Add a `merge` case to `launchSkill` (server) → `gx-merge`, prompt `/gx-merge --ticket <addr>`.
  The skill reads the epic branch from the ticket and reports `iteration-status` on it.
- Ticket mode skips gx-merge's final `gx merge`: after rebase + checks pass it reports `finished`.
- Stops and their status:
  - Agent-visible stops → `needs-answer` (self-reported), reason in the ticket: unresolvable
    conflict, checks it can't clear, `gx merge --json` error, dirty feature worktree.
  - `finished` but finish check fails, agent dies, budget kill → server parks `needs-repair`.
- Unresolvable conflict → `git rebase --abort` first, so the worktree is clean on the pre-rebase
  branch; reason names the conflicting files.
- Checks step (typecheck, tests, lint), in the skill itself so manual runs get it too:
  - Typecheck/lint failure → one fixer subagent fixes and commits on the rebased `<epic>`
    (`<epic>: Fix typecheck/lint after rebase onto <target>`), then all checks rerun from the start.
    Still failing → `needs-answer`.
  - Test failure → a triage subagent reruns only the failing tests 3 times. Flaky = passes at least
    once **and** the epic's diff doesn't touch the test file or any non-test file in the test's Go
    package. Otherwise (fails every
    time, touches epic code, unsure) → epic's fault → keep the rebased branch, `needs-answer` with
    the check output. Fails on trunk too → still `needs-answer`, reason says "fails on trunk too".
  - Flaky → continue; one follow-up per flaky test via `gx-follow-up` (dedupes); list them in the
    merge ticket's `## Resolution`.
- Fixer commits do not make the merge ticket commitful: its finish check is ancestry, and the
  commits land with the epic's ff.

### Manual trigger

- New verb `gx server epics merge <project:epic>` in a new `gx server epics` group (every `gx server tickets` verb takes a ticket address). The server does the work.
- TUI: "merge" item in the `m` menu on a parked root (Queue tab) and on an epic in "Complete epics
  (unmerged)" (Tickets tab). No new top-level key.
- Behaviour:
  - Ff already possible → land directly, no ticket, report `merged`.
  - Else re-add the root if it was dequeued, then fork one merge ticket with `manual: true`.
  - Feature worktree gone, branch present → recreate `<worktreeDir>/<epic>` from the branch.
  - Dirty trunk on the land → the dirty-trunk park.
  - `--commits` one-offs are accepted like epics.
- Refusals, each with a code and nothing written: `map-epic`, `not-complete` (non-terminal tickets
  remain), `already-merged` (`merged_at` set), `merge-pending` (a merge ticket is open, claimed or
  parked), `branch-missing`, `server-not-running`.

### One-offs

- No special case. A `--commits` one-off is an epic with one ticket and its own feature branch and
  worktree; it lands through the same path and gets merge tickets, the cap and the dirty-trunk park.
  `IsOneOffEpic` stays true (it only checks for the one-off ticket file).
- `notifyDone` keeps skipping epic-level notices for one-offs, including both new epic notices. A
  merge ticket park inside a one-off still sends the normal park notice. No retro for one-offs.
- Non-`--commits` one-offs are commitless, never land, and are unaffected.

### Queue, chat, counts, evolve, Tickets tab

- Queue: a claimed `type: merge` ticket gets a "(merging...)" live phase with its own spinner,
  keyed on type (unlike `markConflictResolution`, which keys on `parent`). No other glyph.
- Chat: no notice on fork. Park and done use existing texts. Epic notice splits into "🎉 epic
  merged" (land succeeded) and "✅ epic complete (not merged)" (`auto-ff-merge` off, or root parked
  at the cap). Root-park chat text renders the `dirty-trunk` and `land-error` kinds. A dirty-trunk
  or land-failed park sends the root-park notice only: no epic notice until the land succeeds. A
  land error no longer publishes `iteration-failed`.
- Counts: a merge ticket counts like any ticket.
- Evolve: no skip; the existing conflict-resolution filter only matches code-review fix children,
  which a parentless merge ticket never is.
- Tickets tab sections, in order: Open epics (loop, not all-done) → Open maps (`kind: map`, not
  all-done) → Complete epics (unmerged) (loop, all-done, no `merged_at`) → Merged epics (loop,
  `merged_at`) → Complete maps → Archived epics. Zero-ticket epics stay open. The three complete
  sections collapse/expand like "Closed epics" today; empty sections are hidden. "Closed epics" is
  removed.

### Suggested ticket order

1. `merged_at` stamp, `completed_at` timing, poll-time ancestry stamp, `gx tickets migrate`
   backfill, `retargetIfLanded` / `rootRefs` / gx-cleanup reading `merged_at`.
2. Whole-epic `blocked_by`: syntax, widening, resolution rule, write-time checks, `epic set
   --blocked-by`, epic-level validation, base derivation, display.
3. Land errors: per-trunk mutex, `merged_at` no-op, ancestry re-check, dirty-trunk and land-error
   parks with event kinds.
4. Merge ticket type, iteration mode, finish check, fork on non-ff, derived retry cap.
5. `gx-merge` ticket mode, fixer and flaky-test triage, `launchSkill` case.
6. Manual trigger verb, `manual: true` handling, `m` menu items.
7. Queue live phase, chat notice split, Tickets tab sections.

## Testing Decisions

- Good tests drive gx from its outside: a real temp git repo, a real ticket store with a real
  `project.json`, and the public entry point (server runner pass, CLI command, TUI model update).
  They assert on what a person sees (where trunk points, ticket status and park kind, `merged_at` /
  `completed_at` stamps, events and their kinds, chat notices, command output, rendered rows), not
  on which helper was called.
- Seams, highest first, confirmed in the design review. Function and file names in this spec are
  pointers as of fcf11aa, not contracts; follow the behaviour if a name has moved.
  1. **Server runner** (existing seam): non-ff → merge ticket forked, root queued and unparked;
     merge ticket done → land → `merged_at` + `completed_at` → root dequeued → "epic merged"; cap
     (third non-ff parks the root); dirty trunk and other land errors park with the right event
     kind and reason; dirty trunk after a merge ticket does not consume the cap; concurrent lands of
     two epics on one trunk; double land is a no-op; `--commits` one-off gets a merge ticket and no
     epic notice; merge-ticket finish check pass / fail; manual trigger (each refusal code, direct
     land, `manual: true` resets the cap and lands with `auto-ff-merge` off, worktree recreation);
     feature worktree and branch survive every land and merge-ticket end-of-run; a fake agent per
     merge-ticket stop → status mapping (each agent-visible stop self-reports `needs-answer`;
     finish-check fail, agent death and budget kill park `needs-repair`); a reclaimed merge ticket
     reuses the feature worktree. The map names two
     server tests: (a) `auto-ff-merge` on, non-ff → merge ticket forked and a dependent still
     blocked; merge ticket done + land → dependent claimable; (b) `on-landed`, `auto-ff-merge` off,
     blocker complete → dependent blocked; `merged_at` stamped (by `gx merge` or by poll-time
     ancestry) → claimable with base trunk. Prior art: the server landing tests (`landRoot`,
     trunk landing, trunk-missing landing, land recovery, `--commits` one-off, retarget and root-ref
     tests).
  2. **Ticket store resolution** (existing seam, `tickets` package): whole-epic token blocking until
     terminal + stamp, for both policies, including the reopen guard and the empty epic; the map's
     named unit test (blocker all-done without `completed_at` → blocked, stamp → unblocked; same
     under `on-landed` with `merged_at`); token syntax; widening and narrowing; cycle detection on
     the widened graph with the widened-token message; epic-level `blocked_by` validation; base
     derivation with whole-epic blockers, including `AmbiguousBaseError`. Prior art: the existing
     `blocked_by`, cycle and base-derivation tests.
  3. **CLI commands** (existing seam): `gx tickets add` / `set --blocked-by` / `epic set
     --blocked-by` refusing dangling refs and cycles and printing the widening notice; `gx merge`
     stamping `merged_at`; `gx tickets migrate` backfill; `gx server epics merge` argument and
     refusal output. Prior art: the existing tickets CLI and server repair verb tests.
  4. **TUI model update** (existing seam): Queue "(merging...)" phase and counts; widened blocker
     display; Tickets tab section order, collapse and hidden empty sections; `m` menu "merge" item
     on a parked root and on an unmerged complete epic. Prior art: the Queue live-phase tests for
     conflict-resolution and the Tickets tab grouping tests.
- The `gx-merge` skill text (ticket mode, fixer, flaky triage) has no automated test; it is checked
  by running a merge ticket against a temp repo with a real conflict and a real check failure.
- Run tests per package with `rtk go test ./<pkg>/...`; never the whole repo at once.

## Out of Scope

- Merge commits or squash merges. Landing stays ff-only.
- Pushing to a remote.
- gx-cleanup running `git branch -d` before `git worktree remove` (git refuses for a checked-out
  branch); filed as a follow-up.

## Further Notes

- State at the start (fcf11aa): landing is gated only by project.json `auto-ff-merge`; the global
  `server.auto-merge-epic` was removed in 4db1dd0. `landRoot` does an ff-only merge of `<epic>` into
  its target via `git.MergeBranchInto`.
- The brief said a ticket-level cross-epic ref would be "rejected at validation". Ticket 03
  replaced that with widening; this spec follows ticket 03.
- Rewriting `<epic>` history during the merge: with `auto-ff-merge` on, nothing outside the epic
  builds on its old commits, because the epic is all-done and `on-done` dependents also wait for
  `merged_at`. With `auto-ff-merge` off, `on-done` dependents start at `completed_at` and build on
  `<epic>`; a later manual merge rebases it under them. This is allowed. The dependent's own merge
  ticket rebases it onto trunk later; git skips commits already upstream, and a conflict goes
  through the normal merge-ticket path.
- Glossary: **Merge ticket**, **Complete epic**, **Merged epic** and **Dirty trunk** are already in
  `GLOSSARY.md`, and **Blocker** / **Blocking** already cover whole-epic blockers and widening.

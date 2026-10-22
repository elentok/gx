# Epic land visibility: show why a land failed, merge again from the web

Source: the map `gx:epic-land-visibility`, decided in one session with no decision tickets, and its
review findings. It started from an investigation of `gx:vendored-skills-impl`, which sat in
"Awaiting merge" after its merge ticket finished.

## Problem Statement

An epic finishes and its merge ticket rebases the branch. Then the server's land fails. In the
case that started this spec, `main` was checked out with an uncommitted edit to `GLOSSARY.md`, a
file the branch also changes, so git refused the fast-forward.

The person sees none of this:

- The epic shows "Awaiting merge", the same label as an epic that hasn't tried to land yet. It
  looks like gx is still working.
- The reason exists only as a `server.log` line and a `root-parked` event. A server restart loses
  it.
- The reason lists every local change in the target's checkout (9 files here), not the one file
  that blocked the land.
- The only way to retry is `gx server epics merge <project:epic>` in a terminal.
- While a merge ticket's agent rebases the branch, the epic shows "Running" or "Waiting", so the
  person can't tell a merge is under way.

## Solution

A failed land puts the epic in a new state, **Merge blocked**, which shows a short reason and the
fix. For a dirty trunk the reason names only the files that clash. The state is stored on the
epic, so it survives a restart. The existing chat notice for a failed land carries the same
reason.

The web UI gets a **Merge** button on the epic page and on a Merge blocked epic's card on the
queue page. It does what `gx server epics merge` does. A refused or failed merge shows its reason
next to the button.

While a merge ticket is open or claimed, the epic shows a new state, **Merging**, in the web UI and
the TUI, with the merge agent shown live as for a running epic.

## User Stories

1. As a project owner, I want an epic whose land failed to show "Merge blocked", so that I can tell it apart from an epic that hasn't tried to land.
2. As a project owner, I want a Merge blocked epic to show the reason, so that I know what to fix without reading `server.log`.
3. As a project owner, I want a dirty-trunk reason to say "uncommitted changes on `<target>` (`<dir>`) clash with `<files>`", naming the branch the epic lands on and its checkout folder, so that I can read it at a glance, including for a stacked epic.
4. As a project owner, I want a dirty-trunk reason to list only files that I changed and the land also changes, so that I don't hunt through unrelated edits.
5. As a project owner, I want an untracked file at a path the branch adds to count as a clashing file, so that the list explains every refusal.
6. As a project owner, I want the clashing-file list capped at five, then "+N more", so that the reason stays readable.
7. As a project owner, I want a land-error park to show "Merge blocked" with the git error, so that every failed land looks the same.
8. As a project owner, I want a needs-rebase park after the merge-ticket cap to show "Merge blocked" with its reason, so that a capped epic doesn't look like it's waiting.
9. As a project owner, I want a missing-trunk park to show "Merge blocked" with a reason that says to fix the trunk first, so that every root-park kind is covered.
10. As a project owner, I want the Merge blocked reason to survive a server restart, so that I can still read it the next day.
11. As a project owner, I want the reason cleared whenever the epic's land step runs again, on every path, so that a merged or re-finished epic never shows a stale reason.
12. As a project owner, I want a new merge trigger to clear the reason only once it passes every refusal and error check, so that a refused or failed click keeps the reason I need.
13. As a chat reader, I want the failed-land notice to carry the same short reason, so that I know from my phone why the epic didn't merge.
14. As a web user, I want a Merge button on the epic page when the epic is Awaiting merge or Merge blocked, so that I can retry without a terminal.
15. As a web user, I want a Merge button on a Merge blocked epic's card on the queue page, so that I can retry from the queue.
16. As a web user, I want no Merge button while the epic is Merging, so that I can't start a second merge.
17. As a web user, I want no Merge button on running, waiting, parked, deadlocked or finished epics, so that it only shows where it can help.
18. As a web user, I want no Merge button and no Merging state on a map epic or on the one-offs epic, so that it never offers an action the server refuses.
19. As a web user, I want the button disabled while its request runs, so that I can't send it twice.
20. As a web user, I want a refused merge to show its reason next to the button, so that I see the outcome at once.
21. As a web user, I want a merge request that fails with a server error to show "Merge failed: `<error>`" next to the button, so that a missing trunk or a failed worktree setup is visible too.
22. As a web user, I want a merge that forks a merge ticket to switch the epic to Merging, so that I see it was accepted.
23. As a web user, I want a merge that lands at once to show Finished on the epic page and move the epic to "Recently finished" on the queue page, so that I see it landed.
24. As a web user, I want no confirmation dialog before a merge, so that a retry is one tap.
25. As a web user, I want the merge request to carry the epic state my page shows, and the server to refuse it as a stale page when the epic's state has changed, so that I never merge from an out-of-date view.
26. As a project owner, I want an epic whose only unfinished tickets are open or claimed merge tickets to show "Merging", so that I know an agent is rebasing it.
27. As a project owner, I want a Merging epic to show its merge agent live (running line, animation, permission prompt), as a Running epic does, so that I can watch and approve it.
28. As a project owner, I want a Merging epic whose merge ticket parks to show "Parked", so that the park and its question stay visible as today.
29. As an Epics page user, I want Merge blocked and Merging epics to show those states on the Epics page, so that it agrees with the queue page.
30. As a TUI user, I want the Queue tab to show "Merge blocked" with its reason and "Merging", so that the TUI and the web agree.
31. As a TUI user, I want no new Merge action in the TUI, so that this change stays small.
32. As a project owner, I want no new notice for Merge blocked beyond the existing failed-land notice, so that I don't get two messages for one failure.

## Implementation Decisions

- **Two new epic states.** The queue's epic state (`EpicState`) gains `merge-blocked` and
  `merging`. The one function that decides an epic's state decides both. The Epics page's wider
  list state (`EpicListState`) gains the same two values, and the function that maps a queue
  state to a list state passes them through. The two state lists stay separate. The generated
  TypeScript types pick up the new values.
  - `merge-blocked`: the epic is done, not merged, and has a stored land-block reason.
  - `merging`: every ticket is done or cancelled except one or more merge tickets that are open or
    claimed and not parked. A parked merge ticket keeps today's Parked state. The one-offs epic is
    never Merging.
  - `awaiting-merge` keeps its meaning: done, not merged, no stored reason.
- **Live agents in Merging.** The epic summary keeps its active-ticket list in the Merging state, as
  it does for Running. The merge agent's running line, live animation and permission prompt show on
  the card.
- **Stored land-block reason.** A failed land writes the park kind and the reason onto the epic's
  `ticket.md`, next to `completed_at` and `merged_at`. The field names are the implementer's choice.
  - The epic's land step clears both on every path before it decides, so a later finish, a land or
    a default-path completion (auto-ff-merge off) never leaves a stale reason. A new failure writes
    them again.
  - A new merge trigger clears them only after every refusal and error check has passed, right
    before it lands or forks a merge ticket. A refused or failed trigger leaves the old reason.
  - The epic summary, the epic list row and the epic detail carry the kind and the reason to
    clients. The epic page reads the epic detail.
- **Every root-park kind is covered:** dirty trunk, land error, missing trunk, and needs-rebase at
  the merge-ticket cap. Each keeps its existing event kind on the `root-parked` event.
- **The dirty-trunk reason lists only clashing files.** A clashing file is a tracked file that has
  local changes and that the land changes, or an untracked file at a path the branch adds. The land
  changes are the files that differ between the target's tip and the branch tip. Keep the existing
  cap of five files. If the overlap comes out empty (git refused for another reason), fall back to
  a land error with git's message.
- **Reason wording, per kind.**
  - Dirty trunk: "uncommitted changes on `<target>` (`<dir>`) clash with `<files>`; commit or
    discard them, then merge again". `<target>` is the branch the epic lands on, which is a parent
    epic's branch for a stacked epic, and `<dir>` is that branch's checkout.
  - Land error and needs-rebase: today's wording, with "then merge again" in place of "run gx
    server epics merge".
  - Missing trunk: "the trunk branch is missing: `<error>`; fix the trunk, then merge again". This
    replaces today's "run gx-merge `<epic>`".
- **Chat notice.** The existing failed-land notice carries the stored reason. No new notice type is
  added.
- **Merge refusals.** The manual merge refuses the one-offs epic with its own reason code, next to
  the existing map-epic refusal.
- **Web write route.** The web listener's allow-list gains the epic merge route, same-origin like
  the other web writes. The request carries the `EpicState` the page shows. The server works out the
  epic's current `EpicState` and refuses a mismatch with a stale-page reason code. This compare is
  new: today's web writes only check that a state is present. The response is the existing merge
  result: merged, merge ticket forked (with its address), parked with a reason, or refused with a
  reason code. A server error comes back as an HTTP error.
- **Web button.** The epic page shows a Merge button when the epic is `awaiting-merge` or
  `merge-blocked`. The queue page's epic card shows it only for `merge-blocked`: an Awaiting merge
  epic leaves the queue (auto-ff-merge off) or lands at once (auto-ff-merge on). The button follows
  the existing queue-add and unpark buttons: disabled while the request runs, the result shown
  inline, then the page re-reads its data. An HTTP error shows "Merge failed: `<error>`". A missing
  trunk keeps the button; clicking before the trunk is fixed shows the error inline.
- **TUI.** The Queue tab's epic label table gains "Merge blocked" and "Merging". A Merge blocked epic
  shows its reason on the status line. No TUI action is added.
- **Glossary.** `GLOSSARY.md` already has **Merge blocked** and **Merging**, and **Dirty trunk**
  points at Merge blocked (committed on trunk during design).

## Testing Decisions

Test external behaviour: the state a client reads, the reason text, and what the button does. Don't
test how the reason is stored.

- **Server epic state.** Table tests on the epic state function cover `awaiting-merge`,
  `merge-blocked` and `merging`, including a parked merge ticket staying Parked, the one-offs epic
  never Merging, and a Merging summary keeping its active tickets. Prior art: the existing epic
  summary tests.
- **State list guard.** A test that every `EpicState` value maps to its own `EpicListState`, so a
  future queue state can't fall through to "queued" on the Epics page.
- **Server land path.** Extend the land-error tests:
  - A dirty trunk with one clashing file and several unrelated changes stores a reason that names
    only the clashing file.
  - An untracked file at a path the branch adds counts as clashing.
  - A stacked epic's reason names its parent epic's branch and checkout.
  - A later successful land clears the reason. So does a re-finish that completes on the default
    path.
  - A merge trigger that is refused, or fails, keeps the reason. One that lands or forks clears it.
  - Prior art: the dirty-trunk cases in the land-error tests, and the merge-fork tests.
- **Merge refusals.** The manual merge refuses the one-offs epic with its reason code.
- **Web route.** A merge whose state no longer matches is refused as a stale page. A merge from a
  foreign origin is refused. This needs new tests: the queue-add route tests cover the origin check
  but not a state compare.
- **Web UI.** Component tests on the epic page and the epic card: the button shows only in its
  states, is disabled while the request runs, and shows a refusal reason or an HTTP error inline.
  Prior art: the queue-add and unpark component tests (Vitest).
- **TUI.** The queue header test covers both new labels and the Merge blocked reason. Prior art: the
  existing "Awaiting merge" header test.

## Out of Scope

- Stashing or otherwise touching the person's uncommitted changes. gx still never touches them.
- A Merge action in the TUI.
- New notice types, or marking the epic Actionable.
- Changing what a merge ticket does. It still only rebases; the server still does the land.
- Showing a land that is in progress. The land itself takes milliseconds.
- Merging `EpicState` and `EpicListState` into one list. A follow-up weighs it.

## Further Notes

- The `gx-merge` skill and `docs/specs/auto-merge.md` name "run gx server epics merge" as the fix.
  Update both to say "merge again", naming both the CLI and the web button.
- The investigation that led here added an entry to the gx-investigate `gotchas.md` file. Once this
  lands, that entry should point at the fixing commit.

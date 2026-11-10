# Bulk actions

Epic: `gx:bulk-actions`

Web UI only. The TUI sidebar and its read-only Archived section stay as they are.

## Problem Statement

The web sidebar acts on one row at a time. A person who wants to stop six agents, move four epics
to a workspace or put away a dozen finished epics opens a row menu, and then another, and another.

There is also no way to archive from the web UI, and no way to see what is archived. Archiving is
a CLI verb that the Cleanup page reaches for one epic at a time, and an archived epic disappears
from every list. A person cannot find it again, read it, or bring it back.

Last, the sidebar, the Epics page and the future Archive page cannot be driven from the keyboard
beyond what the sidebar does today.

## Solution

A select mode. A "Select" button in the sidebar header and in each page header (or the `s` key)
puts the area into select mode. Rows get a checkbox. A bar shows "n selected", Select all or
Select none, Done, and one button per action that fits at least one selected row ("Archive 2 of
6"). The action runs on the rows it fits, each row on its own. Rows the server refuses stay
selected and a toast gives each one's reason.

The actions are Move, Hold, Take off hold, Stop, Remove and Archive. The Epics page offers Move,
Hold, Take off hold and Archive.

Archive is new in the web UI. It fits a finished epic, or a Design at `design_step: done`, and
moves it into the project's `.archive`. A new Archive page, reached from the sidebar footer's Epics
menu or the "Archive" link in the Epics page header, lists archived epics and Designs. A "← Epics"
link in its header goes back. A row opens a read-only detail view. Unarchive, from the page's select
mode, a row menu or the detail view, moves the epic back with its agents and its workspace.

Every bulk action has a CLI verb that takes many targets (ADR 0028). Arrow keys and h/j/k/l move
between items on the sidebar, the Epics page and the Archive page, and Space toggles selection.

The approved look is variant A of mock 03 and variant B of mock 04, both stored under the map's
`mocks/` directory.

## User Stories

Select mode on the sidebar

1. As a person using the sidebar, I want a "Select" button in its header, so that I can enter select mode with a click.
2. As a keyboard user, I want the `s` key to enter select mode in the area that has focus, so that I never need the mouse.
3. As a person in select mode, I want a checkbox on every row that has a "..." menu, nested agent rows and the Other section included, so that I can pick exactly what I mean.
4. As a person, I want Queue agents to show no checkbox and Space on one to say why, so that I do not try to act on work the queue manages.
5. As a person, I want selected rows tinted and the sidebar footer replaced by a bar with "n selected", Select all or Select none, and Done, so that I always see what I am about to act on.
6. As a person, I want section checkboxes on On hold and Other, with a mixed state, so that I can pick a whole section at once.
7. As a person, I want Select all to cover only the visible, selectable rows of the area I am in, so that it never reaches rows I cannot see.
8. As a person, I want Done, Escape and a run that leaves no selected rows to end select mode and clear the selection, so that I am never left in a mode by accident.
9. As a person, I want one button per action, shown only when it fits at least one selected row and labelled "Archive 2 of 6" (or "Archive 2" when all fit), so that I see what will run.
10. As a person, I want a tooltip on each button that says what the action runs on and lists the skipped rows with reasons, so that a count below the total is never a mystery.

Actions

11. As a person, I want Move to open a workspace menu above the bar and move every selected row that can move, so that reorganising takes one step.
12. As a person, I want Hold to ask for confirmation only when it would end live Design Sessions, and to list them, so that I am not asked when nothing is lost.
13. As a person, I want Take off hold to ask nothing and to skip rows that are not on hold with the note "not on hold", so that it is safe on any selection.
14. As a person, I want Stop to confirm once and list the rows, and to skip an agent that already Exited with "not live", so that a mixed selection still works.
15. As a person, I want Remove on a live agent to stop it and remove it in one confirm that says which rows are live, so that I do not stop and remove in two passes.
16. As a person, I want Stop and Remove in the web UI to refuse a Queue agent ("managed by the queue") and a row whose state changed after I selected it ("changed, reselect"), so that a stale click never hits the wrong state.
17. As a person, I want Archive to open one confirm that lists the rows, the live Sessions it will stop and a warning line for each epic gx cannot see on trunk, so that I know what I am agreeing to.
18. As a person, I want Archive to accept a finished epic whether or not gx sees it on trunk, so that epics from before the PR flow, which were squashed into trunk, can still be put away.
19. As a person, I want Archive to accept a Design only at `design_step: done`, to move a Design and its active `-impl` epic as one row, and to move a Design with no active `-impl` epic alone, so that a Design never splits from its implementation.
20. As a person, I want an archived name that already exists in `.archive` to get the first free `-2`, `-3` suffix, and the toast to say "Archived as epic-2", so that an Archive never fails on a clash.
21. As a person, I want refused rows to stay selected and the toast to list each with its reason, and rows an action did not fit to stay selected with a note, so that I can fix the cause and run again.
22. As a person, I want a row that leaves the list to leave the selection, and a page's select mode to end when I navigate away while the sidebar's survives, so that the selection never holds rows that are gone.

Epics page

23. As a person on the Epics page, I want a "Select" button in its header, a checkbox in the top-left corner of each card and a bar sticky under the header, so that bulk work looks the same as on the sidebar.
24. As a person on the Epics page, I want only Move, Hold, Take off hold and Archive, so that agent-only actions do not appear where cards are not agents.
25. As a person on the Epics page, I want an "Archive" link in the page header that opens the Archive page and shows no count, so that I can reach archived epics from the page I already use.

Archive page

26. As a person, I want the sidebar footer's Epics icon to open a menu with Epics and Archive, so that Archive is two clicks away without a wider footer.
27. As a person on the Archive page, I want a "← Epics" link in the page header, so that I can go back to the Epics page in one click.
28. As a person, I want the Archive page to list every archived epic and Design, newest first, with its title, project, kind, archive date and workspace, so that I can find what I put away.
29. As a person, I want epics archived before this feature, which have no date, to sort last and show no date, so that old archives still appear.
30. As a person, I want an empty Archive page to say nothing is archived, so that an empty list is not mistaken for a failure.
31. As a person, I want the Archive page to have its own select mode whose only action is Unarchive, so that I can bring back many epics at once.
32. As a person, I want Unarchive to put an epic back in the workspace it had, or in Default when it had none or the workspace is gone, and not to restore a hold mark, so that the epic returns where I left it.
33. As a person, I want Unarchive of a name that clashes with an active epic to rename on the way out with the first free suffix and to report the new address, so that it never refuses for a clash.
34. As a person, I want Unarchive to bring the epic's agents back with it, so that its conversation history is not lost.
35. As a person, I want a click or Enter on an Archive row, outside select mode, to open a read-only detail view of the epic, so that I can read what was done before I decide to bring it back.
36. As a person in the detail view, I want the summary, tickets, cost and times of the epic and one action, Unarchive, so that nothing there can change the archived epic.
37. As a person viewing an archived Design, I want its `-impl` epic reachable from it, so that I can read both halves.
38. As a person, I want the Archive page to update after any Archive or Unarchive, from any client, without a reload, so that it never shows stale rows.

Keyboard

39. As a keyboard user, I want arrows and h/j/k/l to move one focused item on the sidebar tree, and h and l to collapse and expand a sidebar row, so that I can walk the tree without the mouse.
40. As a keyboard user, I want arrows and h/j/k/l to move across the cards of the Epics page and the rows of the Archive page by position, so that pages behave like the sidebar.
41. As a keyboard user, I want Space to toggle selection in select mode and do nothing outside it, Enter to open the item (or toggle it in select mode) and Escape to leave select mode, so that each key has one meaning.
42. As a keyboard user, I want each area to be one Tab stop, so that Tab and Shift+Tab move between sidebar, page and controls and not through every row.
43. As a person typing in a field or a dialog, I want these keys to leave my typing alone, so that a name with an `s` or an `h` never moves focus.

CLI and server

44. As a script author, I want `gx server hold` and `unhold` to take many targets, and `gx server hold` to take `--dry-run`, so that the CLI does what the web button does.
45. As a script author, I want `gx agents stop` and `gx agents remove` to take many session ids, so that I can clean up in one call.
46. As a script author, I want `gx server workspaces move` to take many keys and the workspace last, so that Move has a CLI twin.
47. As a script author, I want `gx server epics archive` and `unarchive` to take many `project:epic` targets, with `--yes` for the one confirm and `--dry-run` to list what would happen, so that I can script a cleanup.
48. As a script author, I want an exit code of 1 when any target is refused and `--json` to always print `{"results":[...]}`, so that a script can tell a partial run from a full one.
49. As a script author, I want `gx server epics archived` to list archived epics and `gx server epics archived-detail <address>` to print one, so that the Archive page's endpoints have CLI twins.
50. As a person using the Cleanup page, I want `gx server cleanup-archive` to keep working as a one-epic alias that shares the new code, so that the Cleanup page keeps its Archive button and its results match.

Data

51. As a person, I want Archive to stop the epic's live Sessions (own, Fix and Design) after the one confirm, so that no agent keeps running on a hidden epic.
52. As a person, I want Archive to clear the epic's hold mark and to stamp the epic with `archived_at` and its workspace, so that Unarchive can restore it.
53. As a person with a narrow window, I want the same bar and checkboxes at narrower widths, so that bulk actions work on a phone.

## Implementation Decisions

**Select mode model.** A pure model owns select mode for one area: the mode flag, the selected
keys, the fitting rules per action and the "n of m" counts. The sidebar and each page hold their own
instance, so the sidebar's selection survives navigation and a page's ends with the page. A key is
the row's address, or the session id for an agent row. A key whose row leaves the list leaves the
selection. Components render the model and own no selection state.

**Fit rules in the client.** The client decides which rows an action fits, for the button counts
and the tooltips, from data it already has: the row menu already knows whether Move, Hold, Stop and
Remove apply to a row. Archive fits an epic card in Finished that is a loop epic, or a Design at
`design_step: done`. The server re-decides every row, so the client rule can be wrong without harm.
The result of the run is what the toast reports.

**Layout.** The sidebar bar replaces the footer while the mode is on. A page bar is sticky under the
page header. Both show "n selected", Select all or Select none, Done and the action buttons. The
sidebar footer's Epics icon opens a menu with two entries, Epics and Archive. The Epics page header
has an "Archive" link and the Archive page header a "← Epics" link. Neither shows a count, and
`/v1/epics` gets no archived count.

**Keyboard.** One roving focus item per area, one Tab stop per area. Arrows and h/j/k/l move the
item. On the sidebar, h and l collapse and expand. Pages move by card or row position. Keys are
ignored while a text field, a menu or a dialog has focus. `s` enters select mode in the focused
area. The focus ring is the existing blue inset ring.

**Server verbs and routes.** ADR 0028 applies: each route has a CLI verb with `--json`. These
existing verbs and their routes take a list of targets: Hold, Take off hold, Stop, Remove and
Move. The request carries `targets`. The old single-target field is dropped. No TUI code calls these
routes. Only `cmd/serverapi`, `cmd/agent` and the web do, and this epic moves all three to
`targets`. Two new routes: `POST /v1/epics/archive` and `POST /v1/epics/unarchive`, with
the verbs `gx server epics archive` and `gx server epics unarchive`. `gx server cleanup-archive`
keeps its name and its route as a one-epic alias, gains `--yes`, and calls the same code. `gx
agents remove --exited` stays and takes no targets.

**Results.** Each row runs on its own, in argument order, and a refused row blocks no other. Every
multi-target route answers 200 with `{"results":[...]}`, even for one target (a refusal is a
result, ADR 0028). A result has `target`, `status` (`done` or `refused`, and `fits` on a dry run), `reason` (a stable code,
only when refused), `message` (human text), `new_address` (only after a rename) and `sessions` (the
live Session ids a Hold or Archive stopped, or on a dry run would stop). The CLI prints
the same JSON under `--json`. Without `--json`, one target prints what the verb prints today. Two
or more targets print one line each (`done gx:a`, `refused gx:b: tickets not done`, `renamed gx:c
-> gx:c-2`) and then `n done, m refused`. The exit code is 1 when any row is refused, otherwise 0.
Existing reason codes keep their names, including `queue-session`, `stale` and
`session-not-live` on the web stop and remove routes. One code is renamed. `hold-live-sessions`
becomes `needs-yes`, because Hold, Remove and Archive now share it. The web caller that matches the
old code, `webui/src/workspaces/itemActions.tsx:148`, changes with it. New codes: `needs-yes`,
`not-finished`, `impl-not-archivable`, `not-on-hold`, `rename-failed`.

**Confirm and `--yes`.** One `yes` covers the whole request. A row that needs a confirm (Hold with
live Design Sessions, Archive with live Sessions, Remove on a live agent) is refused with
`needs-yes` when the request has no `yes`, and the other rows run. A `dry_run` flag on Archive and on Hold
(`gx server epics archive --dry-run`, `gx server hold --dry-run`) runs every rule and writes nothing.
Its results carry `status` `fits` or `refused`, `sessions` with the live Session ids that would stop,
and, for Archive, whether the epic is on trunk and the `new_address` a clash would produce. The web
Archive confirm is built from a dry run, and it always shows, so the not-on-trunk warning lines have
somewhere to appear. The web Hold runs a dry run first and shows its confirm only when a result has
`sessions`. It then sends one real call, with `yes` after a confirm and without it otherwise.

**State token.** Only the web route sends one, for Stop and Remove. It is the session id, its
`RunStartedAt` and whether it is live or Exited, as the row showed when selected. A mismatch with
the server's current value refuses that row with the existing `stale` code and the message "changed,
reselect". The single-row `SessionStateToken` (`id:state:StatusChangedAt`) changes on every
Working/Idle flip and would refuse working agents again and again, so Stop and Remove stop using it.
The web routes `/v1/sessions/stop` and `/remove` have one check, `checkAgentWrite`
(`server/sessions/sessionstate.go`), and it now compares only the new token. The route does not
accept both formats. Every web caller of the two routes sends the new token: `useSessionManage.tsx`,
`sessionWrites.ts` (the row menus) and `design/SessionControls.tsx` (the Design page Stop).
`stateToken` and `SessionStateToken` stay for resume, interrupt and design open. The CLI sends no
token, so a CLI row is never stale.

**Rows.** Move fits the rows whose menu offers Move today (epics, Designs, chats). Hold fits the
rows whose menu offers Hold. Hold keeps its refusals (queued, not holdable, unknown) and stops
sessions only for a Design. Take off hold skips a row that is not on hold. Stop and Remove fit agent
rows. The web routes refuse a Queue Session with `queue-session`, as today. The CLI does not, so a
stuck Queue agent can still be stopped by hand. Stop skips an agent that already Exited, with
`session-not-live`. Remove on a live agent stops it first.

**Archive rules.** An epic is fit when it is done and not queued. It need not be on trunk: epics
from before the PR flow were squashed into trunk and gx cannot see them as merged. The confirm adds
a warning line per epic gx does not see on trunk. Queued, tickets not done and unknown stay
refused. A Design is fit only at `design_step: done`; before that it is refused with `not-finished`.
A Design and its active `-impl` epic archive as one row, and the row is refused with
`impl-not-archivable` unless both are archivable. A Design with no active `-impl` epic archives
alone. A target given as the `-impl` address resolves to its Design's row, and its result is keyed
by the address given.

**Archive effects.** In this order: stop the epic's live Sessions (own, Fix and Design) through a
stop path shared with Hold, which replaces Hold's inline Design-only loop; for a Design, run the
Design cleanup (`CleanupDesign`) when its rules allow it; clear the hold mark; read the workspace;
rename the epic folder into `.archive`; write `archived_at` and the workspace to the epic's
frontmatter; re-key the sessions to an archive-only key. The cleanup runs before the rename because
the rescan that normally runs it no longer sees an archived Design. When the cleanup is not allowed
(the Design is not on trunk), Archive leaves the branches and worktrees alone. The Cleanup page
already scans archived epics (`withArchived`) and stays the way to remove them.

If `.archive/<name>` exists, the epic takes the first free suffix `-2`, `-3` and so on. The suffix
goes on the Design name, so a pair becomes `foo-2` and `foo-2-impl`, because gx finds a pair by
`<name>-impl`. A suffix is free when neither `<name>-N` nor `<name>-N-impl` exists in `.archive`. A
pair renames the Design first, then the `-impl` epic, and a failed second rename undoes the first
and refuses the row with `rename-failed`. Sessions the stop already ended stay stopped.

A rename never renames git branches. `foo`, `foo-impl` and `foo-design` keep their names. Every
epic's branch is its own name, so any rename needs `branch_name`: a plain loop epic as well as a
Design pair. After a rename, `branch_name` holds the epic's original branch name (the value already
there, else the name before the rename). It goes on the epic, or on the Design for a pair. An epic's
branch is `branch_name` when set, else its name. A pair's `-impl` epic has no field of its own. Its
branch is the Design's `branch_name` plus `-impl`, and the Design's worktree branch is that plus
`-design`. A rename to the original name clears it. Both Archive and Unarchive renames write it,
including the clash rename on Unarchive.

These lookups use the effective branch name: the Cleanup scan (`scanEpic`, which today compares
`b.Name == epic.Name`), the not-on-trunk check, and the designs code in `server/designs/designs.go`
(`designBranch`, `afterBranch`, `UnappliedPatches`, which build `<epic>-design` and `<epic>-impl`
from the epic name). So `CleanupDesign` finds the leftover worktree of a Design that was unarchived
as `foo-2`.

An active epic owns its name. The Cleanup scan builds its epic index (`epicIndex`) in two passes:
every epic by name, then each archived epic by `branch_name` only when no active epic has that
key. Without this, an archived `foo-2` with `branch_name: foo` and a new active `foo` would both
claim branch `foo`, and `withArchived` appends archived epics last, so the archived epic would win
and the active epic's iteration branches would be judged by the archived tickets. An archived epic
whose branch an active epic owns finds no branches, and the scan recommends nothing for them.

**Unarchive effects.** Rename the epic (or the pair) out of `.archive`, taking the first free
suffix when an active epic has the name. Read the workspace from the frontmatter and put the epic
there, or in Default when none is stored or the workspace no longer exists. Clear `archived_at` and
the stored workspace. Restore no hold mark. Re-key the sessions from the archive-only key to the
final address. Unarchive never refuses for a clash. It refuses only when the target is not in `.archive` (`unknown`).

**Sessions and rename.** A Session ties to an epic by the address its `ticket` carries,
`project:epic`. A Fix agent ties by `fix_of` instead, and `Store.place`
(`server/workspaces/membership.go`) reads `fix_of` first, then `ticket`. Archive re-keys the
sessions of the epic (and of a pair, both addresses) to an archive-only key built from the archive
address, suffix included. The key is `project:.archive/<name>`, so `gx:.archive/foo-2`. Archive
rewrites both `ticket` and `fix_of` where they hold the epic's address. No active epic can have
that key, because an epic name never contains `/`. A new active epic that reuses the name does not
take over the archived epic's agents. Unarchive rewrites both fields from the archive key to the
final address, so a `-2` rename keeps its agents.

Placement checks the `.archive/` prefix before it parses. `place` gives a session with an archive
key the Epic kind and that key, and never the Other fallback, which a key that does not parse would
reach. No sidebar row has that key, so the agents show nowhere, and the Other section stays empty
while the epic is archived. Nothing prunes these sessions.

**Schema.** The epic frontmatter gains `archived_at`, `archived_workspace` and `branch_name`. All
three live on the epic, or on the Design (the first epic of a pair), and the Design's values apply
to the pair. `branch_name` applies to any renamed epic, loop epics included, not only Designs. An
absent field means never archived, no workspace, or never renamed. A new migration step adds them,
and the schema snapshot is regenerated. The migration backfills nothing, so an epic archived or
renamed before this feature has no `branch_name`, and its branch is its name. The membership prune on the index rescan needs no
archive exception, because the workspace is stored in the frontmatter.

**Archive list endpoint.** `GET /v1/epics/archived`, read-only, no lock, mirrored by `gx server
epics archived [--json]`. It reads each project's `.archive` through `LoadArchived`; the loader's
dot-directory skip stays. A row carries `address` (the `.archive` address, with any suffix),
`project`, `name`, `title`, `archived_at`, `completed_at`, `kind`, `addresses` (a Design pair,
Design first), and `workspace` (stored name, empty means Default; a name that no longer exists
shows as Default). Newest `archived_at` first; rows without a stamp sort last. No ticket counts or
progress.

**Archived detail endpoint.** `GET /v1/epics/archived/detail?address=`, read-only, mirrored by `gx
server epics archived-detail <address> [--json]`. The address is the archive address from the list,
suffix included, with no `.archive` segment. Either address of a pair works. It answers with the
`/v1/epics/detail` payload, built from the epic loaded out of `.archive` by the same builder, plus
an `archived_at` field (omitted for an active epic). An unknown address answers 404, as the active
detail does. The payload carries no live decoration: not queueable, not queued, no pause actions,
no PR block, no sessions. An archived Design's `impl_address` is its archived `-impl` address.

**Archive pages.** The page at `/archive` lists rows. The detail view is the epic page rendered
read-only at `/archive/<address>`. Read-only means: the header shows the Archived stamp and one
action, Unarchive, and every other action is absent (Queue, Hold, Merge, Fix, Open design); ticket
rows are plain text, because the ticket page reads only the active index; an archived Design's
`-impl` link goes to its archive detail. **Deviation:** the archived Design bar also keeps an
"Open implementation epic" button (ticket 24), kept deliberately; the spec asks only for the `-impl`
link. The Archive page refetches the list on the event that
refetches the Epics page, once per burst, on mount and when the connection returns. No new event
type. Rows update from an action's response, so the person does not wait for the refetch.

**Events.** Archive, Unarchive and the re-key publish the epic-list event and the session-list
events the sidebar already follows, so every client updates without a reload.

**CLI details.** `gx server workspaces move` becomes `move <key>... <workspace>`, workspace last.
`gx server cleanup-archive` and the new `epics archive` share one implementation, so both rename,
stamp, write the workspace and re-key. Route and verb parity (`TestRouteTableHasCLIVerbs`) and the
`TestWeb_EverythingElseIs404` allowlist cover the new routes.

**No new pause kinds.** ADR 0042 applies. Nothing here pauses a ticket or an epic, so no new pause
and no new web recovery button is needed.

## Self-decided design questions

1. **Where does the archived detail view read from?**
   Choice: a new `GET /v1/epics/archived/detail?address=` that reuses the `EpicDetail` payload and
   the same builder, with one added optional `archived_at` field.
   Reason: the page already renders `EpicDetail`; a second payload shape would fork the page. The
   active detail route cannot serve it, because the index skips `.archive`.

2. **What address form does the detail route and page use?**
   Choice: the archive address (`project:name`, with any `-2` suffix) and no `.archive` segment. The
   page is `/archive/<address>`; the list is `/archive`.
   Reason: archived and active addresses never share a route, so the suffix alone tells them apart,
   and it is the address the list rows and the CLI already print.

3. **Does the detail view link to tickets?**
   Choice: no. Ticket rows are plain text.
   Reason: the ticket page reads the active index and would answer 404 for an archived ticket.

4. **How does a Design pair show in the detail view?**
   Choice: one row in the list, opening the Design's detail. Either address of the pair is accepted
   by the endpoint, and the Design's `impl_address` links to the archived `-impl` detail.
   Reason: matches the one-row rule of the list and Archive, and keeps `-impl` reachable.

5. **What can the detail view do?**
   Choice: Unarchive only. No other action is rendered.
   Reason: ticket 09 says so; every other action needs an active epic.

6. **Do the existing single-target routes break?**
   Choice: yes, once. Hold, Take off hold, Stop, Remove and Move take `targets`, every caller moves
   to it and the old single-target field is dropped.
   Reason: no TUI code calls these routes. Only `cmd/serverapi`, `cmd/agent` and the web do, and
   this epic rewrites all three. Keeping the field would need a second response shape for one
   caller that does not exist. ADR 0028 makes a route's reason codes the public contract, not its
   request field.

7. **How does the web build the Archive confirm?**
   Choice: a `dry_run` flag on the archive route (CLI `--dry-run`) that runs every rule and writes
   nothing, returning live session ids, trunk status and the clash-renamed address per row. Hold
   gets the same flag, and the web Hold confirm uses it.
   Reason: the confirm needs facts the client lacks (trunk status, the suffix, which Sessions are
   live), and a dry run reuses the real rules instead of copying them into the client. Without a
   dry run, a first Hold call would hold the rows that need no confirm before the person answers.
   A route flag needs a CLI twin, so both verbs get `--dry-run`.

8. **Does the Archive confirm show when nothing is live and every epic is on trunk?**
   Choice: yes, always.
   Reason: ticket 03 gave Archive one confirm that lists the rows, and Archive is the only way to
   put a row out of every list.

9. **What is the stale-state token?**
   Choice: the session id, `RunStartedAt` and whether the Session is live or Exited, as the row
   showed at selection. The server compares it with the current value per row and refuses a
   mismatch with the existing `stale` code.
   Reason: ticket 06 requires a token and "changed, reselect". Live or Exited is what Stop and
   Remove decide on. `SessionStateToken` also holds the Working/Idle state and its change time, so a
   bulk Stop on working agents would refuse rows as `stale` again and again.

10. **What are the new frontmatter field names?**
    Choice: `archived_at` and `archived_workspace`, and `branch_name` for any renamed epic, all on
    the Design for a pair.
    Reason: ticket 05 leaves the names open; ticket 08 reads them from the Design. The prefix keeps
    the workspace field from reading as the live membership.

11. **Is a workspace stored by name or by key?**
    Choice: by name.
    Reason: ticket 07 specifies the stored name, and a workspace that is renamed or deleted
    falls back to Default, which is the rule already chosen.

12. **What if the second rename of a pair fails?**
    Choice: undo the first rename and refuse the row with `rename-failed`. Sessions that were
    stopped stay stopped.
    Reason: half a pair in `.archive` breaks the pair rule. Stopping is already confirmed and
    cannot be undone.

13. **How does a target given as an `-impl` address behave?**
    Choice: it resolves to its Design's row, and the result is keyed by the address given.
    Reason: membership and Hold already treat `-impl` as its Design.

14. **Where do the Archive page's tooltips get their per-row reasons?**
    Choice: from the client fit rules for every action, and from the dry run for Archive's confirm.
    Reason: the tooltip must render without a request on hover, and the server stays the final
    judge of every row.

15. **Do the new routes get their own reason codes or reuse the Cleanup ones?**
    Choice: reuse the existing Cleanup, Hold and session codes (`queue-session`, `stale`,
    `session-not-live`), add the five listed under Results, and rename `hold-live-sessions` to
    `needs-yes`.
    Reason: they are a public contract, so reuse costs nothing, and a new code is added only where
    no existing one means the same thing. `needs-yes` covers three actions, so the Hold-only name
    no longer fits.

16. **Does the page select mode `s` key work when nothing has focus?**
    Choice: it enters select mode on the page if the page has focus, otherwise on the sidebar.
    Reason: ticket 08 says `s` enters select mode in the focused area; with no focus the sidebar is
    the area that is always present.

17. **What does Archive do to an epic's branches and worktrees?**
    Choice: it runs the Design cleanup first when that is allowed, and otherwise leaves branches and
    worktrees alone. The Cleanup page removes what is left.
    Reason: `CleanupDesign` runs from the index rescan, and the rescan does not see an archived
    Design, so a Design archived soon after `done` would keep its `-design` worktree for good.
    Deleting the branch of an epic gx cannot see on trunk would lose work.

18. **Where do an archived epic's sessions live?**
    Choice: under the archive-only key `project:.archive/<name>`. Archive moves them there, and
    Unarchive moves them to the final address.
    Reason: with the bare `project:epic` key, a new active epic that reuses the name would take
    over the old agents, and Unarchive could not tell which agents are its own.

19. **How does a clash rename keep a Design pair and its branches working?**
    Choice: the suffix goes on the Design name (`foo-2` and `foo-2-impl`), branches keep their
    names, and `branch_name` records the original name. The `-impl` branch is `branch_name` plus
    `-impl`.
    Reason: gx finds a pair by `<name>-impl`, so `foo-impl-2` would split it. Cleanup, the
    not-on-trunk check and the designs code map an epic to its branches by name, so they need the
    original name.

20. **What is the archive-only session key?**
    Choice: `project:.archive/<name>`, with the archive suffix. Archive rewrites `ticket` and
    `fix_of`. `place` tests the `.archive/` prefix before parsing and gives the session no row.
    Reason: an epic name has no `/`, so no active epic can own the key. `place` sends any Design or
    Fix session whose address does not parse to Other, so an unparsed key would list the archived
    epic's agents there. Fix agents are placed by `fix_of`.

21. **Do the web Stop and Remove routes accept both token formats?**
    Choice: no. They take only the new token, and the three web callers send it.
    Reason: two formats would need a branch in `checkAgentWrite` for callers that this epic rewrites
    anyway. `stateToken` stays for resume, interrupt and design open.

22. **Who owns a branch name when an archived epic's `branch_name` equals an active epic's name?**
    Choice: the active epic. The Cleanup epic index adds archived epics by `branch_name` only when
    no active epic has that key.
    Reason: otherwise the archived tickets' done state would judge the new epic's iteration
    branches and could earn them a delete recommendation. A missed recommendation for an archived
    epic is harmless, a wrong one loses work.

## Testing Decisions

A good test checks external behavior: what a route answers, what a verb prints and returns, what a
person sees and can do in the browser. It does not check which helper ran.

Seams, highest first:

- **Server rigs.** `servertest` rigs drive the HTTP routes: Archive and Unarchive of an epic, a
  Design with and without an `-impl` epic, a clash, a pair clash, a not-on-trunk epic, a live
  Session, a queued epic, a stale token, the Queue refusal on the web route, the dry run (also for
  Hold), the list and the detail, the hold and membership interplay, the session re-key, a name
  reused while its old epic is archived, the Design cleanup on Archive, and the branch lookup of a
  renamed epic. They also cover Hold, Stop, Remove and Move with many targets. Prior art: the Hold, workspace and epic detail rigs.
- **CLI tests.** `cmd/serverapi` tests run each verb through the fixture server: many targets,
  `--json` always `{"results":[...]}` (also for one target), the exit code, `--yes`, `--dry-run`,
  the alias. Prior art: the hold CLI tests.
- **Pure model, vitest.** The select model (enter, leave, toggle, Select all, sections, fit counts,
  rows leaving) and the key handler get unit tests. Components get vitest tests for the bar, the
  checkbox column, the Archive page and the read-only detail. Prior art: the sidebar model tests.
- **Migration.** The new step and the schema snapshot test.
- **One new Playwright flow** on a new scenario, `bulk-actions`, covering only what needs a server
  round trip and a real layout: select, Move, Hold, Stop, the Archive confirm, the Archive page and
  detail, Unarchive, live updates and the layout at 744 px and 390 px. Render and key logic that
  vitest covers stays out of it. `maxFlows` goes up by one. The flow follows `gx-playwright-e2es`:
  waits before state reads, no reload.

## Verification

A verify ticket runs these against a temporary gx server. Each entry starts with its seam: Vitest,
Rig (`servertest`), CLI (`cmd/serverapi` or a real verb run) or Flow. Shorthand: "bulk flow" is the
new flow on scenario `bulk-actions`; "workspaces flow" is the existing flow of `workspaces.e2e.ts`.

1. Flow: in the bulk flow, click the sidebar header "Select"; the checkbox column and the bar appear. Mock states: selecting.
2. Vitest: with the sidebar focused, `s` starts select mode; on the Epics page `s` starts the page's mode; with nothing focused it starts the sidebar's.
3. Vitest: every row with a menu, nested agent rows and Other rows included, has a checkbox; Queue agents have none. Mock states: selecting.
4. Vitest: Space on a Queue agent shows a toast and selects nothing.
5. Vitest: selected rows are tinted, the bar replaces the footer with "n selected", Select all or none and Done. Mock states: selecting, default.
6. Vitest: the On hold and Other section checkboxes select all their rows and show a mixed state for a partial pick. Mock states: selecting.
7. Vitest: Select all on the sidebar leaves a collapsed On hold section's rows unselected.
8. Vitest: Done, Escape and a run that leaves no selected rows each end the mode and clear the selection.
9. Vitest: the button reads "Archive 2 of 6" for a mixed selection and "Archive 2" when all fit. Mock states: selecting.
10. Vitest: hover a "n of m" button; the tooltip lists the rule and the skipped rows with reasons.
11. Flow: in the bulk flow, Move opens the workspace menu above the bar and moves the fitting rows. CLI: `gx server workspaces move <k1> <k2> <workspace> --json` prints two results.
12. Flow: in the bulk flow, Hold on a live Design asks and lists its Sessions, and Hold on an epic asks nothing. Vitest: the web Hold sends a dry run first, shows the confirm only when a result has `sessions`, then sends one real call, and matches `needs-yes` (the renamed `hold-live-sessions`) in `itemActions.tsx`. Rig: a Hold dry run holds nothing and returns `sessions`; a Hold of a live Design without `yes` gives `needs-yes`. CLI: `gx server hold <design>` exits 1 with `needs-yes`; with `--yes` it exits 0; with `--dry-run` it changes nothing.
13. Rig: Take off hold on a mix skips the row not on hold with "not on hold". CLI: `gx server unhold <held> <not-held> --json` gives one `done` and one `refused` with reason `not-on-hold`.
14. Flow: in the bulk flow, Stop confirms once and lists the rows; an Exited agent is skipped with "not live". CLI: `gx agents stop <live> <exited>` exits 1 with `session-not-live` for the second.
15. Rig: Remove on a live agent without `yes` gives `needs-yes`; with `yes` it stops and removes the agent. CLI: `gx agents remove <live> --yes` exits 0 and `gx agents list` no longer shows it; without `--yes` it exits 1 with `needs-yes`.
16. Vitest: a selected Queue agent cannot be toggled. Rig: the web stop and remove routes refuse a Queue session sent anyway with `queue-session` ("managed by the queue"); a row whose session went from live to Exited between select and click gets `stale` ("changed, reselect"); a Working/Idle flip alone does not, and a request with the old `SessionStateToken` format is refused as `stale`. Vitest: `useSessionManage`, `sessionWrites` and `design/SessionControls` send the new token. CLI: `gx agents stop <queue-session>` is not refused, as today.
17. Flow: in the bulk flow, the Archive confirm lists the rows, the live Sessions and one warning line per epic not on trunk. CLI: `gx server epics archive <epic> --dry-run --json` gives `fits` with the sessions and trunk status. Mock states: archive-confirm.
18. Rig: an epic whose branch gx cannot see on trunk archives after the confirm. CLI: `gx server epics archive <epic>` exits 0 and `gx server epics archived` lists it.
19. Rig: a Design at `done` with an `-impl` epic archives as one row; a Design before `done` is refused with `not-finished`; a Design with no `-impl` epic archives alone. CLI: the same three targets with `--json`. Rig: archive a Design at `done` whose cleanup is allowed; its `-design` worktree and branch are gone. Archive a Design not on trunk; its branches and worktrees stay, and the Cleanup page still lists them. Unarchive a Design as `foo-2` (a clash); `CleanupDesign` finds and removes its `foo-design` worktree and branch through `branch_name`, and `UnappliedPatches` checks `foo-impl`.
20. Rig: archive a name that exists in `.archive`; the result has `new_address` `...-2`, and for a pair the epics are `foo-2` and `foo-2-impl` and still form a pair. Branches keep their names, and the Cleanup scan and the not-on-trunk check still find the renamed epic's branches through `branch_name`. This also holds for a renamed plain loop epic, and the `-impl` epic's branch is `branch_name` plus `-impl`. Vitest: the toast says "Archived as ...-2". CLI: `gx server epics archive <epic> --json` has `new_address` set.
21. Vitest: a refused row stays selected and the toast lists its reason; an unfit row stays selected with a note. Mock states: refused-toast.
22. Vitest: a row that leaves the list leaves the selection; navigating away from the Epics page ends its select mode; navigating within the sidebar keeps its mode. Flow: in the bulk flow, archive a selected row through the CLI; it leaves the selection.
23. Vitest: on the Epics page the header "Select", the card checkbox and the sticky bar appear. Mock states: selecting.
24. Vitest: the Epics page bar offers Move, Hold, Take off hold and Archive and no Stop or Remove. Mock states: selecting.
25. Vitest: the Epics page header has an "Archive" link with no count. Flow: in the bulk flow, clicking it opens the Archive page. Mock states: default.
26. Vitest: the footer's Epics icon opens a menu with Epics and Archive. Flow: in the bulk flow, the footer keeps its width and Archive opens the Archive page. Mock states: footer-closed, footer-epics-menu-open.
27. Vitest: the Archive page header has a "← Epics" link. Flow: in the bulk flow, clicking it returns to the Epics page. Mock states: archive-selecting.
28. Flow: in the bulk flow, the Archive page lists archived epics newest first with title, project, kind, date and workspace. Rig: `GET /v1/epics/archived` has the same order. CLI: `gx server epics archived --json` matches the page order.
29. Rig: an epic archived with no `archived_at` (seeded) sorts last in the list. Vitest: its row shows no date.
30. Vitest: an empty Archive list shows the empty state; Flow: after unarchiving the last row in the bulk flow the Archive page shows it.
31. Vitest: Archive page select mode offers Unarchive only. Mock states: archive-selecting. Flow: in the bulk flow, Unarchive from an Archive row menu, outside select mode; the epic leaves the list and is active on the Epics page.
32. Flow: in the bulk flow, unarchive an epic that was in a named workspace; it is back there. Rig: an epic whose stored workspace was deleted lands in Default, and no hold mark comes back.
33. Rig: unarchive a name that clashes with an active epic; the result has `new_address` and the epic is renamed with the first free `-2`, not refused. CLI: `gx server epics unarchive <epic> --json` has `new_address`. Vitest: the toast names the new address.
34. Rig: after Unarchive the epic's agents show under its row again with the same session ids, also after a rename. CLI: `gx agents list` shows the same session ids under the new address. Flow: in the bulk flow, the agents show under the row after Unarchive. Rig: archive `foo` with a live Session, then create a new active `foo`; the new epic shows none of the old Sessions. Unarchive the old one; it takes `foo-2` and its Sessions show under `foo-2`. Also: with `foo` archived and its Fix and Design Sessions Exited by the Archive, the Other section stays empty and no row shows them, and Unarchive puts them back under their rows, so both `ticket` and `fix_of` were re-keyed. While `foo-2` (`branch_name: foo`) is archived and the new active `foo` exists, the Cleanup scan of the new `foo`'s branches is the same as without `foo-2`, with no delete recommendation from the archived tickets, and an active epic owns its name over an archived `branch_name`.
35. Flow: in the bulk flow, click a row, and press Enter on a focused row, outside select mode; the read-only detail opens at `/archive/<address>`.
36. Flow: in the bulk flow, the detail shows summary, tickets, cost and times, and only Unarchive as an action; ticket rows are not links. Rig: the archived detail payload has `archived_at` and no live decoration. CLI: `gx server epics archived-detail <address> --json` returns the payload with `archived_at`; an unknown address exits 1. Flow: in the bulk flow, Unarchive from the detail view; the epic is active again on the Epics page.
37. Vitest: an archived Design's detail links to its `-impl` detail. CLI: `archived-detail` accepts either address.
38. Flow: in the bulk flow, with the Archive page open, archive an epic through the CLI; the row appears without a reload, and after an Unarchive from the CLI it leaves.
39. Vitest: arrows and h/j/k/l move the sidebar item; h and l collapse and expand a row.
40. Vitest: arrows and h/j/k/l move across Epics page cards and Archive page rows by position.
41. Vitest: Space does nothing outside select mode and toggles inside it; Enter opens a row outside select mode and toggles it inside; Escape leaves.
42. Vitest: each area is one Tab stop, so Tab from the sidebar lands on the page area once and Shift+Tab returns; the focus ring is the blue inset ring.
43. Vitest: typing `shsl` into the New chat name field keeps focus in the field and starts no mode.
44. CLI: `gx server hold <a> <b>` and `gx server unhold <a> <b>` print per-target lines and a summary; with one target the output is as today.
45. CLI: `gx agents stop <id1> <id2>` and `gx agents remove <id1> <id2>` print per-target lines; `agents remove --exited` still takes no targets.
46. CLI: `gx server workspaces move <k1> <k2> <workspace>` moves both; with the workspace argument missing the verb exits non-zero and moves nothing.
47. CLI: `gx server epics archive <a> <b> --yes` and `unarchive <a> <b>`; `--dry-run` writes nothing (the folder list is unchanged).
48. CLI: one refused target gives exit 1 and every `--json` call prints `{"results":[...]}` with `reason` and `message`; all rows done gives exit 0.
49. CLI: `gx server epics archived [--json]` and `archived-detail <address> [--json]`; the route-to-verb parity test passes.
50. CLI: `gx server cleanup-archive <epic>` archives with a stamp and the stored workspace, and takes `--yes`. Flow: the Cleanup page's Archive button (`cleanup.e2e.ts`, flow 198) still works.
51. Rig: archive an epic with a live Fix Session; after the confirm the Session is Exited. CLI: `gx server epics archive <epic>` without `--yes` exits 1 with `needs-yes`.
52. CLI: after Archive the epic's `ticket.md` carries `archived_at` and `archived_workspace`, and a held epic's hold mark is gone from `gx server workspaces list`. Migration test: the new step runs once, adds `archived_at`, `archived_workspace` and `branch_name`, backfills nothing, and the schema snapshot is current.
53. Flow: in the bulk flow, at 744px and 390px the same bar and checkboxes work and nothing scrolls sideways.

**Mock states**

- `$(gx tickets root)/bulk-actions/mocks/03-select-mode-look.html` · default · 1360 px · `bulk-actions` ·
  page, page-header, sidebar, sidebar-footer, sidebar-header, sidebar-rows
- `$(gx tickets root)/bulk-actions/mocks/03-select-mode-look.html` · selecting · 1360 px ·
  `bulk-actions` · page, page-action-bar, page-header, sidebar, sidebar-action-bar, sidebar-header,
  sidebar-rows
- `$(gx tickets root)/bulk-actions/mocks/03-select-mode-look.html` · refused-toast · 1360 px ·
  `bulk-actions` · page, page-action-bar, page-header, sidebar, sidebar-action-bar, sidebar-header,
  sidebar-rows, toast
- `$(gx tickets root)/bulk-actions/mocks/03-select-mode-look.html` · archive-confirm · 1360 px ·
  `bulk-actions` · confirm-dialog, page, page-action-bar, page-header, sidebar, sidebar-action-bar,
  sidebar-header, sidebar-rows
- `$(gx tickets root)/bulk-actions/mocks/03-select-mode-look.html` · archive-selecting · 1360 px ·
  `bulk-actions` on the Archive page · page, page-action-bar, page-header, sidebar,
  sidebar-action-bar, sidebar-header, sidebar-rows
- `$(gx tickets root)/bulk-actions/mocks/04-sidebar-footer-layouts.html` · footer-closed · 280 px ·
  `bulk-actions` · epics-icon, new-button, sidebar-footer
- `$(gx tickets root)/bulk-actions/mocks/04-sidebar-footer-layouts.html` · footer-epics-menu-open ·
  280 px · `bulk-actions` · epics-icon, epics-menu, new-button, sidebar-footer

New flows or scenarios to write (these become implementation tickets):

- Scenario `bulk-actions`: a Default workspace and a named one; finished loop epics (one with no
  trunk evidence, one queued); a Design at `done` with an `-impl` epic, a Design at `done` with
  none, a Design before `done`; a held epic; a live Design Session and a live Fix Session; a
  Queue agent; an Exited agent; a clashing name in `.archive`; archived epics with and without
  `archived_at`, one with a workspace that no longer exists; seeded `.archive` rows for a pair.
  It needs fixtures at 1360 px for the mock states and 744 px and 390 px for story 53.
- The bulk flow on that scenario, covering the entries marked Flow, plus its `playwrightguard`
  budget entry (`maxFlows` plus one).
- Servertest rigs, CLI tests and vitest tests as named under Testing Decisions.

## Out of Scope

- TUI changes. Its sidebar and its read-only Archived section stay as they are.
- Archiving an unfinished epic or Design. Hold covers setting unfinished work aside.
- A general undo pattern. Unarchive is the only way back from Archive.
- An archived count in the footer, on `/v1/epics` or anywhere else.
- Backfilling `archived_at` on epics archived before this feature.
- Restoring a hold mark on Unarchive.
- Reading an archived ticket on its own page.
- Stdin or `--from -` target lists in the CLI.
- A look for widths other than 1360 px beyond reusing the same bar and checkboxes.

## Further Notes

The stored mocks already show the final labels: "Archive" with no count in the Epics page header,
the Epics menu and the Archive page, and no "5 archived" subtitle. Both mocks have one approved
width, 1360 px for mock 03 and 280 px for mock 04. Narrower widths reuse the same bar and
checkboxes.

The glossary already defines "Archived epic". Its line says an archived epic's agents are hidden
with it. That stays true: the sessions have no sidebar row until Unarchive re-keys and shows them.
"On hold" is a different state, and the glossary calls an agent a Session.

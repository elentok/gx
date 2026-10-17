# Context menu: one `m` menu for the Tickets, Queue and Agents tabs

Map: `gx:queue-tab-actions` (decisions 01, 02, 03; 04 re-filed as `gx:follow-ups/01`; review
findings decided 2026-10-17).

## Problem Statement

A person who wants to act on a ticket, an epic or a session has to know which tab offers which
menu, and which key opens it:

- The Tickets tab has two menus: `s` (status) and `m` ("Suggested Actions"). The Queue tab has only
  `m`, with a smaller item list. The Agents tab has its own "session menu". They look alike but
  list different things and behave differently.
- The menus only show actions that apply right now. A person can't see what else exists, or why an
  action they expected is missing.
- In server mode the Queue tab's `m` menu is broken. "Resume (I answered)" and "Unmute & Reopen"
  treat the row's address as a file path and fail. The Tickets tab lists "Unmute & Reopen" but never
  runs it. Status changes and the Queue tab's edit keys also read the row's file path, which a
  server-mode row does not reliably have.
- Some writes still fall back to editing the ticket file when no server runs (Queue delete, Answer
  then Resume). That breaks ADR 0030: orchestration writes belong to the server alone.
- Epics have almost no actions, and the Queue tab's epic headers can't be selected at all.
- "Watch" means different things per runner. A headless agent shows a snapshot that needs `r` to
  refresh. A PTY agent shows the holder screen. A herdr agent jumps to its pane. None of them
  follow the conversation live.

## Solution

One shared **Context menu**, opened with `m` on the Tickets, Queue and Agents tabs. It lists
everything a person can do to the focused **Target** (a ticket row, an epic row or a session):

- Suggested actions come first, then the rest in thin, named groups.
- Actions that can't run stay listed, dim, with the reason in place of their key hint.
- Each action that also has a direct key shows that key, so the menu teaches the keys.
- Typing filters the list (fuzzy). Esc closes it.
- Every action finds its Target by address. No action reads a row's path as a file. **Author
  writes** edit the store file the address resolves to. **Orchestration writes** call a server verb
  by address. With no server, orchestration items dim with "server not running".
- A new server verb, `gx server tickets unmute`, makes "Unmute & Reopen" an orchestration write.
- **Watch** opens one live modal that follows an agent's conversation as it happens. It works for
  headless and PTY agents, and later for herdr agents. It replaces the snapshot. The herdr pane jump
  moves to its own "Go to pane" item.
- A **right click** on a list row selects it and opens the same Context menu `m` opens.
- A **hover highlight** marks the list row under the mouse on the Tickets, Queue and Agents tabs. It
  never moves the selection.

The `s` status menu on the Tickets tab goes away; status changes move into the Context menu. The
Queue tab keeps `s` for "start server". The Agents tab's session menu moves onto the shared
component with its current items.

## User Stories

### Opening and using the menu

1. As a gx user, I want `m` to open the same Context menu on the Tickets, Queue and Agents tabs, so
   that I learn one key and one menu.
2. As a gx user, I want the menu to look and behave the same on every tab, so that I never have to
   think about which tab I'm in.
3. As a gx user, I want the menu title to name its Target (ticket title, epic name or session name),
   so that I know what I'm about to act on.
4. As a gx user, I want to type to filter the menu with fuzzy matching, so that I can reach an
   action in a long list in a few keys.
5. As a gx user, I want a filter that matches nothing to show "no matching actions", so that an
   empty menu is not confusing.
6. As a gx user, I want Esc to close the menu without doing anything, so that opening it is always
   safe.
7. As a gx user, I want up/down and ctrl+n/ctrl+p to move the cursor, so that the menu navigates
   like every other gx menu.
8. As a gx user, I want the cursor to skip dim items, so that Enter always lands on something that
   can run.
9. As a gx user, I want Enter on a dim item to do nothing, so that I can't trigger an action that
   would fail.
10. As a gx user, I want each item to show its direct key (for example `e e`, `y f`, `enter`), so
    that I learn the faster way to do it next time.
11. As a gx user, I want a dim item to show why it can't run (for example "server not running",
    "not muted", "no agent log") in place of its key hint, so that I know what to fix.
12. As a gx user, I want suggested actions at the top, so that the thing I most likely want is one
    Enter away.
13. As a gx user, I want the other items grouped under thin headers (status, server verbs, agent,
    queue, edit/copy, in that order), so that I can scan the list.
14. As a gx user, I want empty groups left out, so that the menu has no dead headers.
15. As a gx user, I want each status change and each server verb to be its own item ("Set open",
    "Set draft", "Park", "Cancel", "Relaunch", …), so that there are no submenus to dig through.
16. As a gx user, I want the row badge that marks "this row has suggested actions" to keep working,
    so that I can still spot rows that need me.
17. As a gx user, I want the menu to open only for the row I pressed `m` on, even if checking agent
    liveness takes a moment, so that a slow check never opens a menu for the wrong row.
18. As a gx user, I want mouse clicks on an enabled item to run it, as other gx menus allow, so that
    the mouse still works.
19. As a gx user, I want a right click on a list row in the Tickets, Queue or Agents tab to select
    that row and open the same Context menu `m` opens (centred, not at the cursor), so that the mouse
    reaches every action.
20. As a gx user, I want a right click on an epic row (a Tickets tab epic or a Queue tab epic header)
    to open the epic Context menu, so that right click and `m` always agree.
21. As a gx user, I want a right click on the preview panel, a blank area or a group header to do
    nothing, so that a stray click never opens a menu for the wrong thing.
22. As a gx user on the Agents tab, I want a right click inside an attached agent terminal to still
    go to the agent, so that the agent's own mouse handling keeps working.

### Ticket Target

23. As a gx user, I want the ticket Context menu to list every action that applies to a ticket row,
    so that I can see everything I could do, not only what applies right now.
24. As a gx user, I want checked-set and queue-wide keys (`r`, `a`, `D`, `C`, `c`, `p`, `x`) to stay
    keys only and stay out of the menu, so that the menu is only about the one Target.
25. As a gx user, I want "Answer…" on a needs-answer ticket to open my editor at its
    `## Needs Answer` section and then offer to resume, so that answering is one flow.
26. As a gx user, I want "Resume" on a parked ticket to unpark it through the server, so that the
    server stays the only writer of orchestration state.
27. As a gx user on the Queue tab in server mode, I want "Resume" on a needs-answer row to work, so
    that I don't have to switch to the Tickets tab or hand-edit a file.
28. As a gx user, I want "Unmute & Reopen" to work on both tabs, so that a muted ticket can be
    cleared from wherever I see it.
29. As a gx user, I want "Unmute & Reopen" to dim with "not muted" on a ticket with no mutes, so that
    I know it exists but doesn't apply.
30. As a gx user, I want "Investigate" on a needs-answer, needs-repair or error ticket to open a
    gx-investigate session in herdr, so that I can debug a stuck ticket in one step.
31. As a gx user with herdr not running, I want "Investigate" to dim with "herdr not running", so
    that I know why it's unavailable.
32. As a gx user, I want "Set open" and "Set draft" to work even with no server, so that I can
    still author my tickets while the server is down.
33. As a gx user, I want "Set open" on a ticket with an empty body to be refused with a clear
    message, so that nothing empty is ever scheduled.
34. As a gx user, I want "Set done" to work only on map-epic tickets, still refusing while
    `blocked_by` is unresolved, and to dim with "done is set by the server" on loop-epic tickets, so
    that I can't close a blocked or server-owned ticket by mistake.
35. As a gx user, I want the ticket's current status to show as a dim "Set <status>" item with
    "already <status>", so that the full set of statuses is always visible.
36. As a gx user, I want "Park", "Cancel" and "Relaunch" in the menu, so that I can run server
    verbs on a ticket without the CLI.
37. As a gx user, I want server verbs on a finished ticket to dim with "ticket is finished", so that
    I don't try to park or cancel a done ticket.
38. As a gx user, I want "Approve recovery proposal" in the Queue tab menu when the row has a
    pending proposal, so that `A` is discoverable.
39. As a gx user, I want "Edit file", "Edit file (split)", "Edit file (vsplit)" and "Edit file (tab)"
    in the menu with their `e` chords, so that I can find the edit keys.
40. As a gx user, I want "Yank epic - ticket" and "Yank file path" in the menu with their `y`
    chords, so that copying is discoverable.
41. As a gx user, I want "Transcript" to open the agent's past sessions, as today, so that I can
    read what an earlier agent did.
42. As a gx user on an archived row, I want the menu to open with only Yank, Edit (read-only open),
    Transcript and Watch enabled, and every write dimmed as "archived", so that I can still copy and
    read but can't change an archived ticket.
43. As a gx user, I want the Tickets tab's `s` key removed, so that status changes have one home.
44. As a gx user on the Queue tab, I want `s` to keep meaning "start server", so that my habit
    still works there.
45. As a gx user, I want `enter` on a parked row in server mode to keep unparking it, and the menu's
    "Resume" item to show `enter` as its key, so that the shortcut is visible.

### Epic Target

46. As a gx user, I want `m` on an epic row to open the epic Context menu, so that epics get the
    same menu as tickets.
47. As a gx user, I want "Start session" on a map epic, so that I can start a wayfinder session
    from the menu.
48. As a gx user, I want "Merge into <branch>" (for example "Merge into main") on an epic whose
    tickets are all done and not yet merged, so that I can land the epic on the branch it merges
    into, from the menu.
49. As a gx user, I want "Merge into <branch>" to dim with a reason (for example "tickets not done" or "already
    merged") when it can't run, so that I know what's left.
50. As a gx user, I want "Edit ticket.md" on an epic, so that I can edit the epic's own file.
51. As a gx user, I want "Yank" on an epic, so that I can copy its name.
52. As a gx user, I want "Check all" on an epic to check every ticket in it, so that I can then use
    the checked-set keys on the whole epic.
53. As a gx user, I want "Add epic to queue" to queue every `open`, not-yet-queued ticket of the
    epic through the server, in ticket order, skipping draft, claimed, parked and done ones, so that
    I can schedule a whole epic in one action.
54. As a gx user, I want "Add epic to queue" to report "queued N, refused M (reasons)", with the
    server refusing blocked tickets, and to dim with "nothing to queue" when no open unqueued ticket
    exists, so that a partial or empty result is visible.
55. As a gx user, I want "Set draft" and "Set open" on an epic, so that I can change the epic's
    status as an Author write.
56. As a gx user on the Queue tab, I want to select an epic header, so that I can open the epic
    Context menu there too.
57. As a gx user, I want the Queue tab's checked-set keys to ignore a selected epic header the same
    way they ignore other non-ticket rows, so that selecting a header never queues or deletes by
    accident.

### Agents tab

58. As a gx user, I want the Agents tab's session menu to become the Context menu with the same
    items (Stop, Resume, Remove, Mark read, Run Action), so that every tab's `m` menu works the same.
59. As a gx user on the Agents tab, I want items that don't apply to the session (for example
    "Resume" on a live session) to show dim with a reason, so that I see the full set.
60. As a gx user, I want the Agents tab menu items to show their direct keys (`x`, `r`, `X`, `a`), so
    that I learn them.

### No server

61. As a gx user with no server running, I want every orchestration item to dim with "server not
    running", so that I never get a half-done local write.
62. As a gx user with no server running, I want Author writes (Answer…, edit keys, Set draft, Set
    open) to keep working, so that I can still write tickets.
63. As a gx user with no server running, I want Queue tab `x` to no longer delete by editing files,
    so that only the server changes the queue.
64. As a gx user with no server running, I want "Answer…" to save my answer and then tell me the
    resume needs the server, so that I don't think the ticket was resumed when it wasn't.
65. As a gx user whose server goes down while a tab is open, I want the next menu I open to reflect
    that, so that dimming always matches the current link state.

### Unmute server verb

66. As a gx user or an agent, I want `gx server tickets unmute <project:epic/NN>`, so that I can
    clear a mute from the CLI the same way the TUI does.
67. As a gx user, I want unmute to clear the ticket's mutes, reopen it if it was parked as
    needs-repair, and leave a comment, so that the history explains what happened.
68. As a script author, I want unmute on a ticket with no mutes to be refused with reason
    `not-muted`, so that I can tell "nothing to do" from a failure.
69. As a script author, I want `--json` on unmute to return the standard result shape, so that it
    works like every other server verb.

### Watch

70. As a gx user, I want "Watch" in the Context menu of any ticket row with an agent (Tickets and
    Queue tabs), so that I can follow what the agent is doing.
71. As a gx user, I want Watch to show prompts, replies, tool calls and results, rendered the same
    as `gx server agents watch`, so that the TUI and CLI agree.
72. As a gx user, I want Watch to follow new lines as they arrive while I'm at the bottom, so that I
    see the agent work live.
73. As a gx user, I want Watch to stay put when I scroll up and show a "↓ new lines" mark, so that
    reading older output is not interrupted.
74. As a gx user, I want `G` to jump back to the bottom and resume following, so that I can catch
    up in one key.
75. As a gx user, I want j/k, g/G and page up/down to scroll Watch, so that it moves like other gx
    views.
76. As a gx user, I want esc or q to close Watch, so that leaving is one key.
77. As a gx user, I want Watch to open the latest run's log, and to show "agent ended" and stop
    updating when that agent exits without switching to a later resumed run, so that I know the
    output is final (I reopen Watch to see the new run).
78. As a gx user, I want Watch on a ticket whose agent has stopped (needs-answer, needs-repair, or
    done with its log still kept) to open and be marked "not live", so that I can read its last
    output.
79. As a gx user, I want Watch to dim with "no agent log" when the ticket has no agent log, so that
    I know there is nothing to watch.
80. As a gx user, I want Watch to work with no server for headless and PTY agents, so that reading
    a log never depends on the server.
81. As a gx user, I want Watch on a PTY agent to show its conversation, not its terminal screen, so
    that Watch means the same thing on every runner.
82. As a gx user, I want Watch on a herdr Claude agent to show its conversation in the same modal,
    so that herdr agents are no different to watch.
83. As a gx user with no server running, I want Watch on a herdr agent to dim with "server not
    running", so that I know why it's unavailable.
84. As a gx user, I want Watch on a herdr Codex agent to dim with "not supported for codex", so that
    I'm not left with an empty modal.
85. As a gx user, I want a "Go to pane" item in the Agent group for herdr agents, so that I can
    still jump to the agent's pane once Watch opens the modal instead.
86. As a gx user, I want the same Watch modal on the Tickets and Queue tabs, so that it behaves the
    same in both.
87. As a gx user on a laptop, I want Watch to poll only while it's open, so that an idle TUI costs
    nothing.
88. As a gx user watching a very long run, I want Watch to keep only the newest lines in memory, so
    that the TUI stays responsive.

### Hover highlight

89. As a gx user, I want the list row under the mouse in the Tickets, Queue and Agents tabs to get a
    faint background, weaker than the selection style, so that I see which row a click will hit.
90. As a gx user, I want the selected row to keep its selection style when I hover it, so that the
    selection is always clear.
91. As a gx user, I want the hover highlight to clear when the mouse leaves the list or a modal
    opens, so that no stale highlight is left behind.
92. As a gx user, I want hovering to never move the selection, so that moving the mouse never
    changes what my keys act on.
93. As a gx user on the Agents tab, I want an attached agent terminal to keep getting the mouse mode
    it asked for, so that hover tracking never breaks the agent.
94. As a gx user on a laptop, I want the TUI to re-render only when the hovered row changes, so that
    moving the mouse within a row, or leaving the TUI idle, costs nothing.

### Other key changes

95. As a gx user, I want `D` on the Tickets tab labelled "drain queue", so that the help text says
    what it does.
96. As a gx user, I want the `m r` and `m esc` chords removed from Worktrees, Status, PRs and Commit,
    so that `m` is free for a later Context menu there (`R` already refreshes).
97. As a gx maintainer, I want the impl epic's last ticket to file a follow-up with notes for
    git-tab Context menus and git-tab hover highlight, so that the next map starts from what we
    learned.

## Implementation Decisions

### The shared component

- New package `contextmenu` (under the UI tree) owns a generic `Menu[T]`, where `T` is the Target
  type (a ticket row, an epic row, a session). It replaces the tickets package's
  `actionsMenuModel`, the Tickets tab status menu, and the Agents tab's session menu state.
- An item carries: label, direct-key hint (display only), group, an enabled flag with a reason
  string, and a run function from the Target to a `tea.Cmd`. Sketch of the shape:

  ```go
  type Item[T any] struct {
      Label    string
      Key      string // direct-key hint, e.g. "e e"; display only
      Group    Group  // Suggested, Status, Server, Agent, Queue, EditCopy
      Disabled string // non-empty = dim, shown in place of Key
      Run      func(T) tea.Cmd
  }
  ```

- `Menu[T]` has the same Open/Update/View shape as today's embeddable menus (`IsOpen`, `Update`
  returning the next menu and a `tea.Cmd`). It reuses the existing fuzzy filter (`ui.FuzzyMatch`)
  and modal rendering. Every printable key is filter text, as in `components.UpdateMenu`, so the
  menu has no letter shortcuts of its own.
- The component, not each tab, enforces: cursor skips disabled items; Enter on a disabled item is a
  no-op; group order and header rendering; empty groups dropped; "no matching actions" on an empty
  filter result. While filtering, a group header shows only if one of its items matches.
- Group order: Suggested, Status, Server verbs, Agent, Queue, Edit/copy. An item shown under
  Suggested is not repeated in its own group. The Agent group holds Watch, Transcript, Investigate
  and, for herdr agents only, Go to pane.
- The modal title is "Actions"; the prompt line names the Target.

### One dispatch path keyed by address

- Each Context menu action is one function keyed by the Target's address, shared by the Tickets and
  Queue tabs. The tabs only build the Target and the availability inputs; they do not dispatch.
- No action reads a row's `Path` or `FilePath()` as a file. This retires the Queue tab's
  `cmdApplySuggestedAction(file, …)` path, the status menu's `ticket.FilePath()` writes, and the
  Queue edit chords' `row.ticket.FilePath()`.
- **Author writes** (Answer…, the edit chords, Set draft, Set open, epic Set draft/Set open, Edit
  ticket.md) and the map-epic **Set done** (ADR 0030's direct claimed/done exception) resolve the address to the store file through the existing `ticketFile` lookup, then
  write locally and ping the server (`TicketChanged`) when one is running.
- **Orchestration writes** always call a server verb by address through the tab's `ServerAPI`:
  Resume → `unpark` repair route; Park, Cancel, Relaunch → their ticket routes; Unmute & Reopen →
  the new unmute route; Merge → epic merge; Add epic to queue → `QueueAdd` per `open`, not-yet-queued
  ticket in ticket order; Approve → approve route.
- Answer… stays a local edit, then offers to resume; Resume is a server unpark. With no server it
  saves the answer and toasts that resuming needs the server.
- "Resume (I answered)" and the server menu's "unpark" merge into one "Resume" item, since both call
  the same server verb. "Answer in pane" is not an item.
- The ticket Target's item set: Answer…, Resume, Investigate, Unmute & Reopen (suggested when they
  apply); Set open, Set draft, Set done (status); Park, Cancel, Relaunch (server verbs); Watch,
  Transcript, Investigate when not suggested, Go to pane for herdr agents (agent); Approve recovery
  proposal (queue, Queue tab only); Edit file ×4, Yank epic - ticket, Yank file path (edit/copy).
- Set done runs only on map-epic tickets. On loop-epic tickets it dims with "done is set by the
  server". No server verb sets `done`.
- Archived rows open the menu. Only Yank, Edit (opens the file read-only), Transcript and Watch are
  enabled; every write dims "archived".
- The epic Target's item set (v1): Start session (map epic), "Merge into <branch>" (lands the epic on the
  branch it merges into), Check all, Add epic to queue, Set draft, Set open, Edit ticket.md, Yank. No archive-epic or
  drain-one-epic items.
- Add epic to queue skips draft, claimed, parked and done tickets and lets the server refuse blocked
  ones. It toasts "queued N, refused M (reasons)". It dims "nothing to queue" when no open, unqueued
  ticket exists.
- Suggested set = today's `statusActionItems` (Answer…, Resume, Investigate, Unmute & Reopen) plus
  Start session and Merge when they apply. The row badge keeps keying off this set.
- Disabled reasons are short, stable strings: "server not running", "herdr not running", "not
  muted", "not parked", "already <status>", "ticket is finished", "archived", "no agent log", "not
  supported for codex", "tickets not done", "already merged", "no recovery proposal", "done is set by
  the server", "nothing to queue".
- "No server" means the tab's `serverAPI` is nil, or the link is down and the tab reads disk. The
  menu reads this at open time.
- Local fallbacks are removed: Queue tab `x` without a server (`handleQueueDeleteKey`) and the
  Answer → Resume fallback that called `ralphloop.UnparkTicket` on a file. `x` with no server
  toasts "server not running".
- Liveness checks (ps, server iteration lookup, the Watch log lookup) run in a `tea.Cmd` before the
  menu opens; the menu opens on the result only if the same row is still selected and no other modal
  opened. Both tabs use this pattern (today only the Tickets tab does).

### New server verb: unmute

- Route `POST /v1/tickets/unmute` with CLI verb `gx server tickets unmute <project:epic/NN>` and
  `--json`, added in the same change (ADR 0028 route/CLI parity test).
- Behaviour matches `ralphloop.UnmuteTicket`: clear `mutes`; if status is needs-repair, set it to
  open and demote the `## Needs Repair` section; append a dated comment. The comment text drops the
  "via Suggested Actions" wording for a neutral one (for example "unmuted").
- Refuses with reason `not-muted` when the ticket's mutes are empty, and the usual unknown-ticket
  refusals. Refusal reasons are a public contract.
- It is an orchestration write in the same family as park/cancel, as the glossary lists it: the
  server writes the file, then updates its state. With no server the CLI refuses
  `server-not-running` and the TUI item dims "server not running". It is not a repair verb: no
  direct path and no `gx tickets unmute` alias (ADR 0030).
- `ServerAPI` gains a `TicketUnmute(ctx, address)` method.

### Watch

- Watch is a Context menu item only; no direct key.
- One modal, built on today's `watchModal`, used by the Tickets and Queue tabs. In Watch mode it has
  no `r` refresh and no `[`/`]` stepping; Transcript keeps using those.
- Keys: j/k, g/G, page up/down, esc/q. At the bottom it follows new lines. Scrolled up, it stays put
  and shows a "↓ new lines" mark until the user returns to the bottom. `G` resumes following.
- The header shows the Target address and a state: live, "not live", or "agent ended".
- A tick every 200ms reads new lines, only while the modal is open. The tick stops when the modal
  closes or the log ends.
- The follow loop in `runAgentsWatch` (`gx server agents watch -f`) moves into `agentlog` as one
  shared tail helper. It is a non-blocking stepper: each call returns the rendered lines added since
  the last call (partial lines held back) and whether the log has ended, doing one last drain after
  `Log.Ended` first reports true. The CLI wraps it in its existing sleep loop; the TUI calls it per
  tick. Both render with `agentlog.Render`, which handles Claude stream-json and transcripts.
- The modal keeps at most the newest 2000 rendered lines (today's `maxWatchLines`).
- The log is found by address through `agentlog.Locator`. For herdr, the tab passes a `Herdr`
  lookup backed by the server's iteration data. With several runs, Watch opens the latest run's
  log. When that agent ends, Watch shows "agent ended" and stops; it does not switch to a later
  resumed run. Reopening Watch picks up the new run.
- Availability comes from the Locator result, not from status (this amends map decision 03, which
  dimmed by status). The lookup runs in the background before the menu opens:
  - log found and agent alive → enabled, live;
  - log found, agent ended (including a done ticket whose log is kept) → enabled, "not live";
  - `ErrNoAgent` or pruned log → dim "no agent log";
  - `ErrUnsupported` (herdr Codex) → dim "not supported for codex";
  - server set but its iterations read fails → dim "server not running". With no server the
    Herdr lookup is nil: only the server knows herdr iterations, so a ticket without a native
    agent dims "no agent log".
- PTY agents: Watch reads the transcript the Locator returns. The holder-screen snapshot path
  (`holderScreen` seam) is removed from Watch.
- Herdr: until the herdr Watch ticket lands, Watch on a herdr agent keeps today's jump-to-pane
  behaviour. Once it lands, Watch opens the modal, and a new "Go to pane" item in the Agent group
  (herdr agents only) keeps the pane jump. Both ship in the herdr Watch ticket. A read-only PTY
  screen is the separate "Attach terminal" follow-up.
- No server stream is added. Watch is a local file read.

### Tab changes

- Tickets tab: remove the `s` binding and the status menu; `m` opens the Context menu on ticket and
  epic rows; `D` help title becomes "drain queue" (behaviour unchanged).
- Queue tab: epic headers become selectable rows and open the epic Context menu; checked-set keys
  ignore them. `s` stays "start server".
- Agents tab: the session menu becomes `contextmenu.Menu[server.SessionInfo]` with items Stop (`x`),
  Resume (`r`), Remove (`X`), Mark read, Run Action (`a`). Items that don't apply show dim instead of
  being dropped. The Agents tab gets no Watch item (its sessions already render live, ADR 0037).
- Worktrees, Status, PRs, Commit: remove the `m r` and `m esc` chords.

### Right click

- A right click on a list row in the Tickets, Queue or Agents tab selects that row, then opens the
  Context menu exactly as `m` would: same modal, centred, same liveness pre-check. It is not
  anchored at the mouse cursor.
- An epic row (Tickets tab epic, Queue tab epic header) opens the epic Context menu.
- A right click on the preview panel, a blank area or a non-epic group header does nothing.
- Agents tab: a right click inside an attached agent terminal is forwarded to the agent as today;
  only right clicks on the session list open the menu.
- This replaces today's rule that the Tickets and Queue tabs ignore non-left clicks. The tests that
  assert "non-left click must not move the selection" (`ui/tickets/queue_test.go` ~1260,
  `ui/tickets/model_test.go` ~665) change to assert the new behaviour. It reuses the existing
  left-click row hit-testing.

### Hover highlight

- Scope: list rows of the Tickets, Queue and Agents tabs only. Not the preview panel, menus or
  modals. The git tabs get it with their later map.
- The row under the cursor gets a faint background, weaker than the selection style. On the selected
  row the selection style wins. The highlight clears when the mouse leaves the list or a modal opens.
  Hover never moves the selection.
- Mouse mode: the TUI uses `tea.MouseModeCellMotion` today (`ui/view.go`), which reports motion only
  while a button is held. These three tabs switch to `tea.MouseModeAllMotion`; every other tab keeps
  `CellMotion`. On the Agents tab, an attached child terminal's own mouse mode still passes through
  (`ui/app/model.go`), so the agent gets what it asked for.
- Idle cost: a motion event re-renders only when the hovered row changes. Motion within the same row,
  or no motion at all, adds no work. No tick or poll is added.
- Independent of the menu work: it can land in any order with the Context menu tickets.

### Ticket order

1. Shared dispatch keyed by address, unmute verb, both write routes, no-server dimming, removal of
   local fallbacks (absorbs map ticket 02's server-mode bug). This rewrites the current dispatch
   code, so it goes first.
2. `contextmenu.Menu[T]` component, adopted by the Tickets and Queue tabs for ticket Targets; `s`
   removed from Tickets.
3. Epic Target on both tabs; Queue tab epic headers selectable.
4. Agents tab migration.
5. Right click opens the Context menu on the Tickets, Queue and Agents tabs.
6. `agentlog` tail helper; CLI `agents watch -f` uses it.
7. Live Watch modal for headless and PTY agents.
8. Herdr Watch, plus the "Go to pane" item.
9. Hover highlight on the Tickets, Queue and Agents tabs (independent of the menu tickets).
10. Key cleanup (`D` label, `m r`/`m esc` chords).
11. File a follow-up with notes for git-tab Context menus and git-tab hover highlight.

## Testing Decisions

- Good tests drive a tab model through key presses and assert what the user sees or what the server
  receives. They do not assert on internal menu state or private helpers.
- **Seam 1: the tab model with a fake `ServerAPI`.** This is the main seam. Prior art:
  `server_mode_test.go`, `server_queue_keys_test.go`, `epic_merge_test.go`, `start_session_test.go`,
  `queue_server_down_test.go`. Required tests:
  - Queue tab: `m` → Resume on a needs-answer row calls the server unpark with the address, never a
    file path.
  - Unmute & Reopen: same assertion on both the Tickets and Queue tabs.
  - With `serverAPI` nil, orchestration items render dim with "server not running" and Enter on
    them makes no call and writes no file.
  - Queue tab `x` with no server writes nothing.
  - Set open / Set draft resolve the address to the store file and write it (temp store fixture).
  - Epic Target: Add epic to queue calls `QueueAdd` once per open, unqueued ticket in ticket order,
    skips draft/claimed/parked/done, reports refusals, and dims "nothing to queue" when none qualify.
  - Set done writes on a map-epic ticket and dims "done is set by the server" on a loop-epic ticket.
  - Archived row: the menu opens with only Yank, Edit, Transcript and Watch enabled.
  - Queue tab epic header is selectable and `m` opens the epic menu.
  - A stale liveness result does not open a menu (prior art: `TestMenuReady_DroppedWhenStale`).
  - Right click on a ticket row selects it and opens the ticket menu; on an epic row (Tickets epic,
    Queue epic header) it opens the epic menu; on the preview panel, a blank area or a group header
    it does nothing. The existing "non-left click must not move the selection" tests
    (`queue_test.go`, `model_test.go`) are rewritten to these assertions.
  - Agents tab: right click on a session row opens the menu; right click inside an attached
    terminal is forwarded to the agent and opens no menu.
- **Hover highlight** is tested through the tab model with mouse motion messages: the hovered row
  renders the hover style; the selected row keeps the selection style; motion never changes the
  selection; leaving the list or opening a modal clears it; motion within the same row returns no
  re-render. The app view uses `MouseModeAllMotion` on the Tickets, Queue and Agents tabs,
  `CellMotion` elsewhere, and an attached Agents-tab terminal's own mode when one is attached.
- **Seam 2: the `contextmenu` component.** Unit tests through `Update`/`View`: filter, "no matching
  actions", cursor skips disabled, Enter on disabled is a no-op, Esc closes, group order and empty
  groups dropped, reason shown in place of key hint. Prior art: the menu tests in `ui/components`.
- **Seam 3: the `agentlog` tail helper.** Feed a growing fixture file: new lines appear per step,
  partial lines are held, the last drain after `Ended` catches final lines, then it reports ended.
  Prior art: `TestAgentsWatch_FollowStreamsUntilEnd` and `TestAgentsWatch_FollowExitsWhenAgentEnded`
  (these keep passing against the CLI wrapper).
- **Watch modal** is tested through the tab model with the `nativeAgentLog` seam faked: appended
  lines appear on the next tick when at the bottom; scrolled up shows "↓ new lines" and does not
  move; log end shows "agent ended" and stops the tick (no switch to a later run); closing stops
  the tick; a done ticket with a kept log opens "not live"; dim reasons for no agent log, Codex and
  herdr-without-server; "Go to pane" only on herdr agents. Prior art: `watch_agent_test.go`.
- **Server verb**: unmute route tests in the server package (clears mutes, reopens needs-repair,
  comments, refuses `not-muted`), plus the existing route/CLI parity test
  (`TestRouteTableHasCLIVerbs`) covers the new verb.
- **Agents tab**: the migrated menu runs the same session calls as before (prior art: the Agents tab
  action tests with a fake client).

## Out of Scope

- Context menus for the git tabs (Worktrees, Status, Log, Commit, PRs, Stash, …). The impl epic's
  last ticket files a follow-up with notes for them, including hover highlight for their lists.
- Hover highlight outside the Tickets, Queue and Agents list rows (preview panel, menus, modals, git
  tabs).
- A Context menu anchored at the mouse cursor. Right click opens the same centred modal as `m`.
- New server verbs for epic actions: archive epic and drain one epic.
- The gx-merge agent itself (`follow-ups/25`).
- Rethinking the ticket checkboxes (`gx:follow-ups/02`).
- The Queue tab showing a code-review agent's fork tickets late (`gx:follow-ups/01`).
- Watching Codex agents under herdr (owned by the multi-harness epic).
- "Attach terminal", a read-only live PTY screen. A separate follow-up from Watch.
- A server-side log stream for Watch.

## Further Notes

Decided with the user on 2026-10-17:

- **Every item appears once.** A suggested item is not repeated in its own group.
- **Modal title "Actions"**, prompt line names the Target.

The other review findings were decided with the user on 2026-10-17 and are now in the body above.
Right click and hover highlight were added in a later grilling session the same day.
"Target" keeps both meanings; GLOSSARY.md notes the difference on each entry. The epic Merge item
names the real branch ("Merge into main"); prose says "the branch the epic merges into" (the
glossary's Trunk entry avoids "target branch").

Code notes (at main `d0340843`):

- The seed placed the agents-watch follow loop in `cmd/server_logs.go`. That file holds the server
  log follower and `logFollowInterval`; the agent follow loop is `runAgentsWatch` in
  `cmd/server_agents.go`. The helper moves from there.
- Queue `handleQueueActionsMenuKey` sends Resume/Unmute to `cmdApplySuggestedAction(file)`; Tickets
  `handleActionsMenuKey` never dispatches `actionUnmuteReopen`; status items write through
  `ticket.FilePath()`; Queue edit uses `row.ticket.FilePath()`.

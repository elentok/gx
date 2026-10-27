# Domain Glossary

## Panels and Viewports

**Panel** — a rectangular region of the screen. A view is composed of one or more panels (e.g.
commit view = header panel + filetree panel + diff panel). Persistent layout panels render
frame-free (no border glyphs); a border is reserved for transient overlays (modals, menus,
confirms). See ADR 0013.

**List panel** — a panel that renders a navigable list of items (filetree, file list, commit list).
Items have a fixed height of one display row each.

**Diff panel** — a panel that renders a unified or side-by-side diff. Items are hunks or changed
lines depending on nav mode.

**Image diff** — the rendering of a changed image file as a side-by-side comparison of its old and
new versions, in place of the generic binary-file summary line. Available in any diff panel that
opts in: the status diff panel (working-tree vs index) and the commit detail panel used by the log
and stash tabs (`<ref>^` vs `<ref>`). Falls back to that summary line whenever the comparison can't
be shown faithfully (unsupported terminal, decode failure, oversized file, or user opt-out). See
ADR 0010.

**Detail panel** — an interactive, focusable panel that mirrors the currently selected list item and
supports its own structural keyboard navigation (e.g. hunk/line navigation in the commit detail
shown beside the log and stash lists). The user can move focus into it and back out. Contrast with a
preview panel, whose focus only ever means scroll-and-search — never structural navigation.

**Screen origin** — the absolute (column, row) of a panel's top-left cell on the terminal grid. A
page that owns the whole screen has origin (0, 0); a detail panel composed into a split view does
not — it only learns its width/height, so its origin is injected by the container that knows the
layout (`splitview.DetailOrigin`). Required only by features that paint outside bubbletea's render
loop at absolute coordinates — currently the image-diff kitty overlay (ADR 0010).

**Preview panel** — a panel, paired with a sidebar, that renders a read-only summary of the sidebar's
selected item (e.g. the worktrees preview panel, the tickets/queue preview panel). It never drives
selection or structural navigation — the sidebar owns that — but it can be focused for scroll and
in-panel search, distinguishing it from a plain read-only display. Clicking inside a preview panel's
bounds, or moving focus into it explicitly, must give it focus so its scroll/search keys work; every
sidebar+preview pairing owes its preview panel this same click-to-focus contract. Contrast with a
detail panel, whose focus additionally supports structural navigation.

**Sidebar** — a list panel shown alongside another panel (typically a detail panel), where navigating
the list drives what the other panel shows. Focusable and selection-driving, unlike a preview panel.
The five sidebars in this project: the commit list in the log view, the stash list in the stash
view, the filetree in the status view, the worktree list in the worktrees view (paired with its
preview panel), and the epic/ticket tree in the tickets view (paired with its preview panel).

**Sidebar mode** — the state a panel is in when it is rendered next to a detail or preview panel, as
opposed to standalone (the only content on screen). A panel in sidebar mode is visually distinguished
(e.g. a slightly darker background) so it reads as secondary to the panel beside it. See ADR 0013.

**Viewport** — the visible window into a panel's content. Defined by a scroll offset (first visible
row index) and a height (number of visible rows).

**Scroll offset** — the index of the first visible row in a panel's viewport. Independent of
selection.

## Find: Search and Filter

Two distinct ways to locate things in a view. They are different interaction concepts, owned by
different components, and must not be conflated.

**Search** — _highlight-and-jump_ over a content stream that stays fully visible. The user types a
query; every match is highlighted in place and `n`/`N` walk the viewport from one match to the next.
Nothing is hidden. Suited to long single-column streams (diffs, file trees) where staying oriented
matters. Owned by `ui/search`, which carries match positions (`ViewportRow`/`DataIndex`) and a match
cursor; the host computes what counts as a match.

**Filter** — _narrow-the-list_. The user types a query and non-matching items disappear; only matches
remain and the layout re-flows around them. There is no match cursor and no jump — the result _is_
the narrowing. Suited to short reference lists (keybindings help, and later the file tree / log)
where the goal is "show me only X." Owned by `ui/filter`, which carries only the query, mode, and
input box and emits `FilterChangedMsg`; the host owns the matching predicate (e.g. help matches a
binding's key _and_ title). Deliberately a separate component from **Search**, not an extension of
it.

**Chord dispatch priority** — the rule that a focused search or filter input must absorb every
keypress before the chord-key manager ever sees it. Chords are multi-key bindings (e.g. `tg`
toggle-graph, `to` toggle-orientation) recognized by a per-view chord manager; without this rule, a
character typed into a search box that also happens to start a chord (e.g. typing `t` to search)
gets swallowed by the chord manager instead of reaching the input. This is a priority ordering, not
a mode switch — the input doesn't disable chords globally, it just always goes first while focused.

## Selection and Active Item

**Selection** (list panels) — the index of the currently highlighted item in a list panel. Used for
navigation, opening, and future multi-select operations.

**Active item** (diff panel) — the currently focused hunk (NavModeHunk) or changed line
(NavModeLine). Governs keyboard navigation and yank/comment targets.

**Context menu** — the one menu, opened with `m`, that lists everything a person can do to the
focused item: suggested actions first, then the rest in groups. It looks and behaves the same in
every tab. An action that can't run on its Target stays listed, dim, with the reason it can't.
_Avoid_: actions menu, suggested-actions menu, session menu (retired names), Action (an Agents-tab
term).

**Target** — the focused item a Context menu acts on: a ticket row, an epic row, a session.
Not the branch a ticket merges into (see **Base** / **Target**); where both could be meant, name the
branch itself (e.g. "Merge into main"). _Avoid_: context (taken by the context window and `ViewContext`),
subject.

**Watch** — read an agent's conversation, live, without acting on it: its prompts, replies, tool
calls and results, the same on every agent runner. Distinct from attaching to its terminal, which
shows the screen. _Avoid_: tail, view log, attach.

**Snap** — when a scroll operation moves the viewport such that the selection or active item is no
longer visible, snap clamps it to the nearest visible item at the new viewport edge.

## Commit States (Log View)

**In-master** — the commit is reachable from `main`/`master` (it has been merged). Rendered in the
default color with no icon.

**Pushed** — the commit exists on both the local branch and its remote tracking branch (synced, not
yet in main). Rendered green with `✔`.

**Unpushed** — the commit exists locally only, and the branch has not diverged from its remote.
Rendered orange with `󰜷`.

**Diverged** — the commit exists locally only, and the branch has diverged from its remote tracking
branch. Rendered red with `󰃻`.

**Remote-only** — the commit exists on the remote tracking branch but not on the local branch (fetch
without pull). Rendered purple with `󰜮`.

## Ticket States (Tickets View)

**RenderedStatus** — the tickets tab's collapse of a ticket's raw `Status:` value into a small,
fixed set of user-facing states, each with its own icon:

- **Draft** — raw `Status: draft`; work its author set aside deliberately. Never shown as Paused. Neither open (it never
  enters the frontier, so no agent claims it) nor done (it still counts as outstanding). This is the
  status `gx tickets add` stamps on a freshly allocated stub, until `set --status open` promotes it.
- **Open** — raw `Status: open`: unclaimed, nothing blocks picking it up. `status` is required,
  so a ticket without one renders as **Error** rather than defaulting to open.
- **Claimed** — raw `Status: claimed`.
- **Blocked** — an overlay, not a raw status: applied whenever an open or claimed ticket has a
  blocker that is still **blocking** (see Ticket Dependencies). Paused and done tickets keep their
  own state regardless of `blocked_by`.
- **Paused** — raw `Status: paused`; the ticket stopped and waits for a person or for a fix. It
  always carries a **Pause reason**, and the reason's **Pause class** says whether a person is asked
  for an answer or something must be repaired. Written by gx when an agent asks through
  `gx tickets ask` (the agent keeps waiting live), when gx hits a fault it can't resolve on its own,
  or when a person pauses a ticket by hand. Agents never write it. Says nothing about whether an
  iteration survives; that's **Reclaim**'s question. Replaces the statuses `needs-answer` and
  `needs-repair`. _Avoid_: Needs-answer, Needs-repair, Needs-info, Needs-attention (retired names,
  see ADR 0018), treating it as "a pane is alive" — see Reclaim.
- **Done** — the ticket's own work is complete. Says nothing about its fork subtree.
- **Waiting-for-children** — an overlay on Done, not a raw status: the ticket's own `Status:` is
  done, but its fork subtree isn't. Derived from the graph on every render, never written to a file.
  An epic containing one is not complete.
- **Error** — either the ticket file couldn't be read, or its raw `Status:` value doesn't match any
  of the above; still selectable, and its raw markdown body still renders in the preview panel if
  the file itself is readable.

Within an epic, tickets are listed in **plan order** — ticket number ascending, with lettered fork
siblings tie-breaking on display number so an original sorts before its replacements. Status does
**not** group or reorder them: a ticket never jumps out of its place once it finishes, so the list
reads as the epic's intended order of execution.

**Paused** — one state for the queue, an epic and a ticket that stop and wait. Every Paused thing
carries a **Pause reason** that says why. Three levels:
- A **ticket** is Paused when its status is `paused`. Tickets of a map epic are never Paused. A
  `draft` ticket shows as Draft and a broken ticket (`error`) shows as Error, never as Paused.
- An **epic** is Paused when it has no runnable work left but at least one Paused ticket. It keeps
  scheduling everything else, then waits and notifies, releasing its slot in the epic-level
  concurrency cap, rather than exiting. It shows the reason of its first Paused ticket (by ID) and
  "+N more". A merge-blocked epic shows "Paused: merge blocked" and a **Deadlocked** epic shows
  "Paused: dependency deadlock". A person resumes the ticket, never the epic, and there is no manual
  epic pause.
- The **queue** is Paused when no new ticket starts. See **Finish and pause**.

Per ADR 0020, **stall** names the opposite thing, the invisible failure this design exists to
eliminate, and must not be used for the deliberate, visible state described here. _Avoid_: Parked
(retired name, replaced by Paused), Stalled, Human-clearable (retired names, see ADR 0020), Settled
(removed from the vocabulary entirely, see ADR 0017).

**Pause reason** (`pause_reason` frontmatter field) — the enum that says why a ticket is Paused, for
example `self-reported`, `zero-commit`, `ambiguous-base`, `manual` or `iteration-error`. gx writes it
with `status: paused`. Cleared on claim; the event keeps the history. The UI shows a plain label for
each reason ("Waiting for your answer", "Merge conflict", "Paused by you") and an action named after
what the person does. Replaces `park_kind`. A ticket stored before this field existed carries no
reason and is read as `zero-commit`, or `iteration-error` for a repair, until `gx tickets migrate`
rewrites it. _Avoid_: Park kind, "gate park", "announce-and-stop park" (retired names).

**Pause class** — `answer` or `repair`, derived from the **Pause reason**, split by who acts next.
`answer`: a person acts next, by replying to the waiting agent or by picking a choice that gx carries
out. `repair`: gx or an investigation must fix something first. It decides `--wait` exit codes
3 (answer) and 4 (repair), notification wording and counts, and only a repair pause gets an
investigation.
Replaces the split between the statuses `needs-answer` and `needs-repair`. It is not stored.

**Resume** — lift a pause, and nothing else. A person resumes a Paused ticket with `gx server tickets
resume`: it goes back to the frontier, or, for a ticket waiting on an answer, the answer itself
resumes the waiting agent (see **Answered**). `resume` dispatches by reason. A `zero-commit` ticket
never resumes on liveness alone, since a person has to look at the ticket first. The UI shows the
word "Resume" only for a pause a person chose (`manual`, and a queue pause a person lifts). Other
reasons show an action named after what the person does ("Answer", "Retry", "Choose base"). Pauses
that end by themselves, such as the rate-limit and smart-zone queue pauses, show no Resume. Replaces
**Unpark**. _Avoid_: Unpark, `clear` (the Queue tab's clear-checked/-complete keymaps already own
that verb, and mean deleting tickets from the queue), using "resume" for a session restore (see
**Restore**) or for Claude's `--resume` flag.

**Finish and pause** — the queue control that starts no new ticket, lets running tickets finish,
then leaves the queue Paused until Resume. The Paused queue is saved, so it survives a server
restart. The header says what is happening: "Paused. N tickets still running." or "Paused. No
tickets running." Replaces the old pause and drain, which differed only in whether they survived a
restart. `D` and `gx server queue drain` remain as hidden aliases for one release. The budget limits
and a person's pause are queue pauses with their own reasons, lifted by Resume. The rate-limit and
smart-zone pauses lift themselves and show no Resume. _Avoid_: Drain, Draining.

**Start over** — throw away a ticket's work and run it again from its base, keeping a backup branch.
`gx server tickets start-over`; the UI says "Start over". Replaces the ticket verb `reset`, not
`git reset`.

**Retry** — launch a fresh agent for a Paused ticket now, on the same branch, keeping its commits.
It closes an idle leftover agent; a working one is stopped only on request (`--stop`, "Stop agent
and retry"). `gx server tickets retry`; the UI says "Retry". The verb most repair-class pauses
offer. Not **Resume**, which only lifts a pause, and not **Start over**, which throws the work
away. Replaces the ticket verb `relaunch`. _Avoid_: relaunch.

**Merge (ticket level)** — put a ticket's commits onto its **Target** branch. `gx server tickets
merge` does what `land` did, and the UI shows the target ("Merge into the feature branch"). A
**Merge ticket** is a ticket whose job is to merge. Epic-level merge states keep their names.
_Avoid_: land, landing.

**Deadlocked** — an epic state (the server reports it per epic), shown as "Paused: dependency
deadlock": an epic with no runnable work and no Paused ticket either (an `error` ticket counts as
Paused here; map epics are never Deadlocked). It is a genuine dependency error, reported as a
failure and not waited on. Its record names the blocked tickets and what they wait on.

**Retired terms** — old words and their replacements, for reading old specs, ADRs, logs and retros.
They survive only in one read shim and a few hidden aliases, until `gx:follow-ups/70` removes them.
- Parked, Park, Unpark → **Paused**, pause, **Resume**.
- Park kind (`park_kind`), Park reason → **Pause reason**.
- Needs-answer, Needs-repair → **Paused** with a reason of class `answer` or `repair`. The
  `iteration_status` value `needs-answer` is now `asking`.
- Park round → **Pause round**.
- Pause and drain (queue) → **Finish and pause**.
- Land, landing (ticket) → **Merge (ticket level)**.
- Reset (ticket) → **Start over**.
- Relaunch, `resume-on-start` (session) → **Restore**.
- Relaunch (ticket) → **Retry**.

**Recently finished** — the last few completed loop epics (by `completed_at`), listed apart from and
dimmer than the live epics so a finished epic does not just vanish from the Queue. Map epics never
appear. Not the Designs picker's "Recently finished" section, which lists Designs whose `<epic>-impl`
epic was queued in the last 7 days.

**Background task** — a shell command, Monitor or async subagent a Claude agent moved off its own
foreground turn (recorded as a `backgroundTaskId`, Monitor `taskId` or async `agentId` in the session
transcript), resolved later by a `task-notification` matched on task id — except an interim one for
a subagent still waiting on its own background work. "Outstanding" while unresolved, "resolved" once a matching notification is seen (its
reported status — success, failure, killed — doesn't matter, only that one exists), "aged out" once
it's stayed outstanding past a generous cap without resolving. Turn-level, inside one still-live
iteration — distinct from the pane-level **Reclaim** (see The Server), which describes the iteration as
a whole. A subagent's (sidechain) background task is never a signal about its parent iteration.
Claude-only; Codex iterations have no equivalent signal.

**`status` / `iteration_status`** — a ticket's status splits across two fields with different
owners. **`status`** is gx-owned and is the sole scheduling authority (the six `RenderedStatus`
values above). **`iteration_status`** is agent-owned and reports on the current claim alone
(`working` / `asking` / `finished`, or absent — gx clears it on every claim and reclaim, so
a report is never readable outside the claim that produced it). Not to be confused with **herdr's
`agent_status`** (`idle`/`done`/`blocked`, on the pane payload, `herdr/agent.go`) — the two names
collided during design specifically because both describe "what is the agent up to," which is why
`iteration_status` was picked instead of the more obvious `agent_status`: it names what the field
describes (the current iteration's state) rather than who wrote it, and can't be confused with
herdr's field in the one file that holds both. See ADR 0019.

**Adopt** — the verb for gx accepting an `iteration_status` report and acting on it: seeing
`asking` and writing `status: paused` with reason `self-reported`, or seeing `finished` and
entering the merge path (only writing `status: done` if the merge succeeds). Not "promote", which
already names two unrelated things in this repo (`draft` → `open`, and an epic entering
`MaxConcurrentEpics`'s auto-promotion queue). An agent's report can start a merge; it can never
conclude one — the commit count and the cherry-pick are what conclude it. See ADR 0019. A **late
finish** is a `finished` report that arrives after gx already paused the ticket because its agent
stopped without reporting (`zero-commit`, or a merge ticket's `merge-unreported`); gx adopts it
while the ticket is still Paused and either has commits (it merges), reported commitless (it closes
`done` with nothing to merge), or is a merge ticket whose merge checks pass (it closes `done`), the
same as an on-time one.

**Pause reason text** — the explanation shown beside a **Pause reason**. For a ticket waiting on an
answer, the summary line of its open question. For any other Paused ticket, the first non-empty line
after its `## Paused` heading. Markdown markers are stripped and the line is ellipsised for display.
It is what the Queue row shows as subtext for a Paused ticket, read fresh from disk on every render
rather than cached, so the row can't go stale between a pause and a restart. Never written to
frontmatter.

**Question block** — the optional fenced `gx-question` YAML block in a question's text. It gives the
question's type (`yes-no`, `options` or `text`), its options, and which option is recommended. A
question without one is a free-text question. A question in a **Grilling round** carries one too.
Not the **Pause reason text**, which is the question's summary line.

**Grilling round** — the numbered `❓ **Qn**` questions an agent asks a person in one turn. The person
answers them together, in one prompt. Each question may carry a **Question block**. Not a **Pause
round**, which is one question a Paused ticket asks.

**Answered** — a ticket that is `open` again after a person answered its pause (reason `self-reported`) and that
gx has not yet claimed. It is derived, never written: the ticket's latest question has an answer. It
ends when gx claims the ticket. Not a status value.

**Ticket log** — a ticket's append-only conversation record, kept beside its markdown file. It holds
the pause rounds and the person's permission decisions. Not the run log, which records what happened
to a ticket.

**Pause round** — one pause of a ticket, from the moment it becomes Paused to what ends it. An
answer pause opens with a question; a repair pause has none. Every round records how it ended
(answer, resume, start over, manual merge, relaunch, cancel, or gx on its own) and where (web, TUI,
CLI, or gx). The ticket's open question is the latest question whose round has not ended. Not an
iteration, and not a `pause` event in the run log.

**Friction summary** — the counts gx stamps on a ticket when it becomes done or cancelled: pauses by
reason, run events by type, pause endings by where they ended, and total paused time. A merged loop
epic carries the sum of its tickets' summaries. The Stats page reads these stamps, never the logs
(ADR 0041).

**Action card** — the part of a ticket's page that asks a person to act: the open question and its
answer form, a pending permission prompt, the text that says why the ticket stopped, the action that
sends it back to work, and the result of an action just taken. It can show on a running ticket.

## Agent Runners

**Agent runner** — the thing that hosts one agent session for an iteration: it starts the agent,
takes its prompts, and reports whether it is working, idle or blocked. herdr is one agent runner.
An iteration keeps the agent runner that started it; changing the setting affects only new launches.
_Avoid_: "runner" alone, "backend", "run".

**Harness** — the agent program a session runs: claude today; codex or opencode later. Separate
from the agent runner that hosts it: the runner decides how a session is hosted, the harness decides
what runs inside it. Say "agent harness" in full where "test harness" could be meant. _Avoid_:
"kind", "agent kind", "provider".

**Native agent runner** — any agent runner that `gx server` hosts itself, with no external
multiplexer. There are two: the headless runner and the PTY runner. A person can watch a native
agent read-only and act on it with one-shot prompt, interrupt and answer verbs. Only an interactive session can be viewed live and typed into. _Avoid_: "herdr-free runner" (describes what it lacks, not what it is).

**Session** — one agent hosted by an agent runner, identified by its label. Replaces herdr's
tab, pane and workspace triple as what ralph-loop holds on to. _Avoid_: "pane", "tab" outside herdr.

**Stale holder** — a leftover agent that holds a ticket's label when gx launches for that ticket:
no live run tracks it, and it is idle or done. gx closes it and launches fresh; it never adopts it.
A leftover agent that is still working is not a stale holder: the ticket is Paused and a person or the
investigate agent decides. _Avoid_: zombie, orphan pane.

**Interactive session** — a session a person starts and types into from the Agents tab, rather than
one the loop launches for an iteration. `gx server` keeps it alive while no TUI is open. _Avoid_:
"chat", "agent" alone (an agent is the program in the session, not the session).

**Restore (interactive session)** — relaunch an Exited interactive session on its latest
conversation, in the same dir, keeping the same session (name, Unread, Actionable). A person does it
by hand, or gx does it once on server start for sessions that were live before
(`agents.restore-sessions-on-start`; the old `resume-on-start` key is a hidden alias). Not
**Resume**, which only lifts a pause, and not Claude's `--resume` flag, which a claim uses to reuse
an agent's last session. _Avoid_: relaunch, resume, restart (sounds like a fresh conversation).

**Session ticket** — the map epic or map ticket an interactive session was started from. Set once,
when the session is created, and shown with the session. It never changes the ticket's status: the
person in the session works the ticket. _Avoid_: assignee, claim (a claim is a ticket status).

**Design** — the web UI's name for a map epic. A Design is **Live** while one of its design sessions
has not Exited. A Design is **Not started** while its Destination is unconfirmed and none of its
tickets is resolved. _Avoid_: "design session" for the map epic itself (a design session is the agent
session working on it); draft for a Not started Design (a draft is work set aside deliberately).

**Epic card** — one card of the Epics page, rendered from one `/v1/epics` row: a progress ring, the
title (the only link), a "project · status" line, a state pill and reason when the card needs
attention, and a footer of at most two buttons. It has one of four **card kinds**: **Design** (a map
epic, or a joined Design and its `-impl` epic until Branch), **Implementation** (a loop epic, or a
joined card from Branch on), **One-off** (one ticket of the `one-offs` epic) and **Follow-ups** (one
per project with open follow-ups). _Avoid_: tile, row (a row is the data behind a card).

**Design session** — an interactive session with a **Session ticket**: where gx-design runs on a
map. It runs in the project's trunk checkout. A map epic or map ticket can have more than one at a
time. _Avoid_: planning session.

**Progress** — the list of gx-design steps for a Design (Chart, Resolve tickets, Spec, Review, Grill
findings, Create tickets, Branch, Done), derived by the server from the tracker, the trunk checkout
and the repo's refs. It shows current state only and has no round counter. A "step" is one of these;
a run is one gx-design invocation, and many runs can stay in one step. _Avoid_: "next step" for the
run-end Continue action.

**Session failure** — the `failure` the server derives on an interactive session: `crashed`, `ended`,
`auth`, `rate-limit`, `stalled` or `blocked`, with a plain label and a button named after what the
person does (Restore, New session, Retry, Screen, or none). It clears when its condition clears. An
unreadable Question block is not a session failure. _Avoid_: error, broken.

**Session status** — what an interactive session is doing now: Starting, Working, Idle, Blocked,
Rate-limited or Exited. Finer than an agent runner's working/idle/blocked, which the loop keeps
using. _Avoid_: "state" for this (state is the runner's coarse view).

**Unread** — an interactive session did something worth seeing (finished a turn, became blocked or
rate-limited, or exited) while no person was looking at it. Viewing the session marks it read.
Says nothing about whether the person must act. _Avoid_: needs attention (retired name, see ADR
0018), unseen.

**Actionable** — the agent needs something specific from the person: an answer to a permission
prompt, question or dialog (while Blocked), a next step the agent itself raised, or prompts gx could
not send (the input held a draft, or the first prompt failed). Viewing does not clear it; resolving
the block or typing into the session does, except for one gx raised itself, which only running it or
a newer Actionable or session exit replaces. Always carries a reason.
_Avoid_: unread (a read session can still be actionable).

**Auto-advance** — the server sends a Design session's Continue by itself after a countdown.

**Chain** — a design run moving on to the next step or map ticket in the same session.

**Action** — the prompts an agent attaches to an Actionable it raised, which gx can send into the
session on the person's request (e.g. clear, then run the next design step).

**Headless runner** — the native agent runner that drives claude through its non-interactive,
structured-event mode. The default native choice.

**PTY runner** — the native agent runner that drives interactive claude on a terminal gx owns. The
backup for when the headless runner is unavailable; never picked automatically.

**GX system prompt** — the fixed text gx appends to the system prompt of every Claude agent it
launches or resumes. It tells the agent how to ask a person (`gx tickets ask`) instead of using an
interactive tool. Not a skill, and not the agent's first prompt.

## Tickets, the Store, and Addresses

**Ticket** — the one unit of scheduled work. An epic, a fork child and an investigate ticket are all
tickets. Only top-level tickets are directories (entry file `ticket.md`); everything below one is a
flat file in its `issues/`, linked by `parent`. _Avoid_: node (Go identifiers only), job.

**Epic** — a top-level ticket with children: a hand-authored set of dependent tickets. Its
`ticket.md` carries `status`, `blocked_by` and timing in frontmatter; the body is optional
(a plan, or a link to a spec). A forked ticket that has children is still just a ticket.

**Map epic** — an epic whose body is a wayfinder map and whose tickets are decisions a person
resolves by hand. Nothing schedules it; the person driving it claims and closes its tickets. Its
implementation work goes into a separate epic. _Avoid_: "map" alone for the epic (the map is its
body).

**Ticket type** (`type` frontmatter) — what kind of work a ticket is. `implement` is the default
code-writing type. _Avoid_: `task` (retired name; validation rejects it, `gx tickets migrate` rewrites it to
`implement`).

**Ticket store** — the one global git-backed directory holding every project's tickets and event
logs, laid out as `<store>/<project>/<epic>/…`. Markdown is the truth and any server state is an
index rebuilt from it (ADR 0026). Agents, the CLI and humans never run git in it. _Avoid_: bare
"store" — it collides with the Go `*Store` types.

**Address** — a ticket's canonical name, `project:epic/06` (ADR 0027). Short forms (`06`,
`epic/06`) are input only and are resolved before anything is stored. _Avoid_: path (agents name
tickets by address, never by file path).

**Original address** — the address a ticket was created at, stamped once at creation. A move or a
close never changes it. A ticket is a **Follow-up** when its original address is in a `follow-ups`
epic.

**Project** / **project name** / **project file** — a registered repo (or `scratch`, which has no
repo), named by the first address segment and defined by `<ticket store>/<project>/project.json`.

**Event log** — the typed record of a ticket's events, committed to the ticket store beside its
top-level ticket (one per top-level ticket; `run-log.jsonl` until the file is renamed), plus the
server event log for events that belong to no ticket. Distinct from `server.log` (process
diagnostics). _Avoid_: run log.

## The Server

**Server** — the long-lived gx process that is the sole scheduler (`gx server …`). The TUI and CLI
are its clients; they reach it over its API (ADR 0028). _Avoid_: daemon, orchestrator.

**Author write** — a change a person may make to a ticket: its body, or its status to `draft` or
`open`. It names the ticket by address and edits the ticket's file in the store.

**Orchestration write** — a change only the server may make: pause, resume, unmute, cancel,
restore, merge, start over and the other server verbs. A client asks for one by address and never writes the
file itself. With no server running, it can't happen.

**Server lock** — the one flock in the state dir that makes the server a singleton. A second
`gx server start` fails on it instead of racing the first. Replaces the per-repo attach lock that
let one TUI process own the Queue. _Avoid_: Attach / Attached / Detach / SelfAttached, Attach lock,
Foreign attachment.

**Snapshot** — the full state a server event stream sends first (`GET /v1/snapshot`), tagged with
the `seq` it was taken at. A client subscribes to the stream from that `seq`; a gap or reconnect
means taking a new snapshot.

**Explain** — the scheduler's own verdict on why a ticket is or isn't running
(`gx server tickets explain`). _Avoid_: diagnose.

**Server queue** — the one server-wide collection of queued top-level tickets, across every
project, in FIFO order (`gx server queue`). Replaces the per-repo **Queue** and the per-epic
ralph-loop run: a second gx process can no longer be "foreign" to it, because only the server
schedules. Queue membership and order live in the server's state dir, not in the tickets.
_Avoid_: Epic run.

**Direct write** — a write the CLI makes straight to a ticket's markdown under the per-ticket lock,
then pings the server. Used for content (`gx tickets show|add|section|set`). The JSON result
carries `"via": "direct"`. Works with the server down.

**Server write** — a write only the server performs, because it changes scheduling (enqueue,
dequeue, finish and pause, resume, override). The server writes the file first, so markdown stays the
truth. The JSON result carries `"via": "server"` and an `actor` (who asked). Refused with
`server-not-running` when the server is down (ADR 0030). _Avoid_: daemon write.

**View model** — what the TUI keeps of server state: a snapshot plus the events applied on top.
It holds no scheduler state of its own and never starts agents. _Avoid_: loop registry (the
in-process ralph-loop it replaced).

**Server indicator** — the colored icon on the right of the title row that shows the server's
state on every view: green when up, red with `server down`, yellow with `read-only — restart` or
`herdr unavailable`; nothing before the first probe. On narrow rows the words shorten, then drop;
the icon never drops. When down, the Queue tab is cleared and offers to start the server; the
Tickets tab still reads markdown.

**Launch** — the server starting an iteration: claim the ticket, create the worktree, open the
herdr tab and start the agent. _Avoid_: spawn (retired), start (ambiguous with `gx server start`).

**Reclaim** — the server, after a restart, taking back an iteration whose pane is still live and
owned (found by its recorded handle). A handle that no longer matches pauses the ticket
with reason `handle-mismatch`. Replaces **Reattach** and Re-adopt; there is no prompt. A surviving herdr pane alone
is not a reclaim — the handle must match. _Avoid_: Reattach / Reattach signal, Live (queue sense),
re-adopt, "has a pane".

**Base** / **Target** — a ticket's **base** is the branch its work starts from; its **target** is
the branch it merges into. Only `base:` is declared (a branch or a ticket address); the target is
derived (a child's is its parent's branch, a top-level ticket's is trunk) and is none for a
commitless ticket. An epic has one base and one target, taken from its **Root ticket**: it merges
into trunk, or on its blocker's branch until that branch reaches trunk. The resolved base is
recorded at claim as read-only `resolved_base`. `gx merge` takes `target`, not `base`.
Not the Context menu's **Target** (the focused item); where both could be meant, name the
branch itself (e.g. "merges into main").

**Root ticket** — an epic's lowest-numbered ticket, whatever its status. The epic's **Base** and
**Target** come from it and from the epic's own `base:` and `blocked_by`; another ticket's `base:`
moves only where that ticket starts. _Avoid_: first ticket.

**Trunk** — the branch a project's top-level tickets merge into: `main`, `develop`, a release branch.
Set per project; detected from the repo when unset. A top-level ticket's **Target** is trunk.
_Avoid_: target branch, main branch, master.

**Dirty trunk** — a **Trunk** checked out in a worktree whose local changes make git refuse a merge.
Changes the merge doesn't touch don't count. gx never discards them: the epic is **Merge blocked**
until a person commits or discards them and merges again.
_Avoid_: unclean trunk, trunk conflict (a conflict is between branches, not with a checkout).

**Missing trunk** — a project whose **Trunk** names no local branch, whether set or detected. gx
never falls back to another branch: the server claims nothing for the project until the branch
exists, then carries on by itself. A ticket whose merge finds trunk missing is **Paused** with reason `trunk-missing`.
_Avoid_: held, paused or blocked project (a project is never Paused; blocked is a ticket's
`blocked_by` edges).

**Fix ticket** — a ticket a code-review ticket opens to repair what the review found. Its `parent`
is the code-review ticket, so it sits in that ticket's fork subtree.

**Follow-up** — a ticket filed for later in a project's `follow-ups` epic, outside any planned epic.
Unlike a fix ticket it has no parent, and it is not scheduled until it is triaged into a real epic.
Each follow-up has a **category**: a **bug** (something that works wrongly) or an **idea** (something
missing or painful). The category is not the ticket's type: the type still picks which skill works
it. A follow-up leaves the epic in one of two ways: it is **resolved** into a real epic, or it is
**closed** with a reason (fixed already, obsolete, duplicate, not worth it) and kept in the project's
**follow-ups archive** so the same finding is not filed again. A ticket stays a follow-up after it
moves. What makes it one is the epic it was filed in. _Avoid_: process, triage (for the whole run).

**Retro** — a look back at one finished loop epic's run, judging the tooling and process (not the
project's code) and filing follow-ups for what went wrong. An epic gets at most one retro; once it
has run, or was skipped, the epic counts as retro'd. _Avoid_: evolve run, scan.

**Budget day** — one local calendar day, from midnight to midnight. The run budget is global to the
server and resets at the start of each budget day.

**Budget ledger** — the persisted per-day record of live cost deltas that the soft and hard budget
limits are checked against. It replaces the Attach baselines. One-offs and investigate tickets
count. A hard kill pauses the ticket with reason `budget-killed`.

## Ticket Forking

**Fork** — dividing a ticket into new sibling tickets mid-flight, when it turns out to be larger
than its budget or mixes concerns that should merge separately. _Avoid_: Split.

**Parent** — the ticket a forked ticket came from, or the code-review ticket that opened a fix
ticket. Frontmatter field `parent`, written on the descendant at creation. This is the only
structural edge between tickets: nothing is recorded on the ticket being pointed at. _Avoid_: Split
from, the `children` frontmatter field (removed). `parent` means containment, so **children** is the
accurate word for the tickets that point at it.

**Fork subtree** — a ticket plus every ticket reached by following `parent` reverse-edges down from
it, at any depth. Derived from `parent` alone; there is no stored child list. What `blocked_by`
resolution actually asks about.

**Fork suffix** — the letter appended to the parent's number to name each forked child (`04` forks
into `04a`, `04b`; one level deeper, `04b1`). _Avoid_: Split suffix.

## Ticket Dependencies

**Blocker** — a ticket, or a whole epic, named in another ticket's `blocked_by` list. A ticket in
another epic that needs commits is *widened*: it blocks as its whole epic, because its code can't be
used before that epic merges. Only a commitless ticket in another epic blocks on its own.

**Blocking** — a ticket blocker is *blocking* until its own work is `done` **and** every ticket in
its fork subtree is likewise no longer blocking. This is the only question `blocked_by` resolution
asks, and it is the reason a ticket's own `done` is not enough to release its dependents. A
whole-epic blocker is blocking until every ticket in it is done and the epic is a **Merged epic**
(`on-landed` policy) or a complete epic (`on-done` policy). With auto-merge on, an epic is complete
only once it has merged, so both policies wait for the merge. _Avoid_: Resolved, satisfied,
fully-done.

**Frontier** — the tickets an epic could hand to an agent right now: status `open`, with no blocker
still blocking.

**Fork inheritance** — a forked ticket inherits its parent's *position in the dependency graph* by
carrying `parent`, and inherits none of its parent's `blocked_by` entries. A fork child's own
`blocked_by` is empty unless it declares a genuine new dependency.

## Decorations and Badges (Log View)

**Decoration** — a git ref (local branch, remote branch, tag, or `HEAD`) that points directly at a
commit. A commit may carry zero, one, or several. Decorations are shown wherever a commit is
presented in detail: each log row, and the commit detail header.

**Badge** — the rendering of a single decoration as its own colored pill.

**Badge group** — multiple decorations on one commit rendered as a single merged pill: one shared
background with each decoration's name keeping its own text color, instead of one pill per
decoration. Used only by condensed rows; normal-width rows render each decoration as its own
separate badge.

**Condensed row** — the narrow-width rendering of a log row: relative dates drop their "ago" suffix,
decorations render as a badge group instead of separate badges, and the gap between subject and
decorations narrows from two spaces to one. Triggered below the same width threshold used elsewhere
for narrow layouts (see Split view). Normal-width rows are unaffected by any of this.

## Navigation Modes

**NavModeHunk** — diff navigation moves between hunks. Active item is a hunk index.

**NavModeLine** — diff navigation moves between individual changed lines. Active item is a changed-
line index.

## App Navigation

**TabID** — canonical identifier of a top-level app destination (`worktrees`, `log`, `status`,
`stash`). This is the only term used for tab identity. `commit` no longer exists as a standalone
tab — commit detail is rendered as the right/bottom panel of the log split view.

**ViewState** — the full navigation payload for a screen. Composed of a `ViewContext` and
`ViewOptions`. This is the canonical navigation term.

**ViewContext** — the durable subset of `ViewState` that determines tab page identity: `Tab`,
`WorktreeRoot`, `Ref`, `InitialPath`. Tab reuse/reset decisions are keyed to `ViewContext`
equality only.

**ViewOptions** — the transient subset of `ViewState` that tunes behavior inside an active view:
`FocusSubject`, `FilterPath`, `FilterStartLine`, `FilterEndLine`. Changes to `ViewOptions` do not
trigger page reconstruction.

**Tab memory** — the app-shell record of the most recent `ViewState` seen for each `TabID`. Used
when switching tabs so users return to their last context in that tab.

**Selected worktree** — the currently highlighted worktree row in the worktrees tab. This is a
focus identity and is distinct from `worktreeRoot` (repository/worktree context used by other
tabs).

**Split view** — the layout used by the log and stash tabs. A list panel (left or top) paired with
a commit detail panel (right or bottom). Orientation is auto-detected from terminal width (same
threshold as status `useStackedLayout`) and toggled manually via the `to` chord
(`toggle-layout-orientation`).

**Panel visibility state** — the three states a split view can be in:

- _Collapsed_ — only the list panel is visible. Default for the log tab.
- _Split_ — both panels are visible. Default for the stash tab. Detail auto-updates as list
  selection changes (j/k navigation).
- _Fullscreen_ — one panel fills the entire screen, the other is hidden. Toggled with `f` on
  the currently focused panel.

Focus and collapse rules: Enter on a list item in collapsed state → expands to split, focuses
detail. Esc from detail → returns focus to list (stays in split). Esc from list while split →
collapses back to collapsed state.

**Pseudo-log-line** — a always-present synthetic row at the top of the log list representing the
working tree. Background-loaded; shows three states: loading, clean ("no local changes"), or dirty
(staged · unstaged · untracked counts). Pressing Enter on it switches to the status tab carrying
the current worktree context.

**Shared worktree context** — log and status tabs share the same `WorktreeRoot`. Switching between
them (via number keys or `g+l`/`g+s`) carries the active worktree to the target tab. The worktrees
tab remains the explicit way to change which worktree the other tabs point at.

**Navigation messages** — the four app-shell message types that child models emit to drive
navigation. All are defined in `ui/nav`:

- `Open(ViewState)` — deep navigation: pushes a new entry onto the global history stack. Reversible
  with `Back`. Used for drill-down flows (e.g., log → commit, status → filtered log).
- `Switch(ViewState)` — tab switching: changes the active tab without adding history depth. Restores
  tab memory for the target tab when no explicit context is supplied. Does not pollute `Back` depth.
- `Back()` — reverse deep navigation: pops the top of the global history stack. When the stack is
  empty (at root), `Back` quits the app.
- `ViewStateChanged(ViewState)` — live view state update: emitted when the active page's internal
  state changes (selection moves, filter changes, ref advances). Updates tab memory but does not
  alter the history stack or trigger page reconstruction.

The app-shell `Update` wrapper calls `AppendViewStateChanged` after every child `Update`, comparing
pre/post `ViewState` and emitting `ViewStateChanged` automatically when navigation is enabled.
Explicit `ViewStateChanged` emissions remain supported for specialized timing needs.

**Leader key** — the one key that, while an embedded terminal has focus, says the next key is a gx
action, not input for the session. Pressing it twice sends it to the session.
_Avoid_: prefix key, escape key.

**Copy mode** — a mode of an embedded terminal where keys move a cursor through the session's
scrollback and select text to copy, instead of going to the session. Scrolling back with the mouse
wheel alone is not copy mode.
_Avoid_: scroll mode, select mode.

## Pull and Push Lifecycle

A pull or push runs as a sequence of **phases**, some of which are interactive prompts and some of
which execute a git command (fetch, pull, push, rebase, stash). The user can leave the flow early in
two distinct ways, which must not be conflated:

**Decline** — the user says No / Esc at a _prompt_ phase (confirm push, stash-before-pull, diverged
menu, force-push confirm). Nothing was executing, so the repository is untouched. Surfaced as
`Result{Aborted: true}`; suppresses the success notification.

**Interrupt** — the user kills a git command that is _mid-execution_. Implemented on top of
`CommandRunner.Cancel()`, which kills the running process. Reuses `Result{Aborted: true}` (so the
success notification is suppressed, same as a decline) but additionally emits a
`notify.Warning("push aborted")`, so the two cases are distinguished only by that warning, not by a
separate `Result` field. Unlike a decline, the working tree _can_ be left in a partial state, so
interrupt is only offered on phases where killing is clean — currently the push flow's network
phases (fetch, push, force-push, tag-push). The local `rebase` phase is deliberately
non-interruptible because killing it mid-rebase leaves the repo in a `rebase-in-progress` state.
Triggered by Esc, gated behind an "Abort push?" confirm modal (default No) to guard against an
accidental keypress; if the command completes while that confirm is showing, completion wins and the
abort becomes a no-op.

## Merge onto Trunk

**`gx merge <branch>`** — the deterministic core of merging a branch/worktree onto the project's
**Trunk**; it never merges onto another branch. Resolves `<branch>` (a literal branch name, or a
worktree-dir name looked up via `git.Worktree`) and trunk, then attempts `git merge --ff-only`. Never
rebases and never leaves the repo mid-operation — on non-fast-forward it reports `needs_rebase` plus
the resolved `branch`/`target`/`worktree_path` and stops. See ADR 0015.

**`gx-merge`** (the skill) — owns the judgment half: on `needs_rebase`, runs `git rebase <base>`
against the target's worktree (or the bare branch if `worktree_path` is empty), invoking
[gx-resolving-merge-conflicts](skills/gx-resolving-merge-conflicts/SKILL.md) on conflicts, running
this repo's checks, then re-calling `gx merge` for the final ff-only merge. It runs with no prompts
and stops on a command error, a missing worktree or failed checks. `gx-cleanup`'s ff-only-merge step
calls into this same skill rather than duplicating the flow.

**Merge ticket** (`type: merge`) — a system-generated ticket in a finished epic whose only job is to
get the epic onto its target with `gx-merge`, when the server's fast-forward merge isn't possible.
_Avoid_: land ticket, rebase ticket.

**Complete epic** — an epic whose tickets are all done or cancelled. Its branch may not have merged
yet; once it has, the epic is also a **Merged epic**. A map epic can be complete but never merged,
since it has no branch.
_Avoid_: closed epic, finished epic.

**Merge blocked** — an epic state: a **Complete epic** whose merge failed and waits for a person. It
is shown as "Paused: merge blocked" with its existing merge action. It carries the reason (a **Dirty trunk**, a git error, a branch that still needs a rebase after the
merge-ticket cap) and the fix. A person fixes the cause and merges again.
_Avoid_: merge failed, root paused (the server's internal name for the same event).

**Merging** — an epic state: every ticket is done or cancelled except an open or claimed **Merge
ticket**, so an agent is rebasing the branch before the server merges it.
_Avoid_: landing (the merge itself is instant), rebasing.

**Merged epic** — an epic whose branch has merged into its target. Distinct from a complete epic,
whose tickets are all done but whose branch may not have merged yet.
_Avoid_: landed epic, closed epic.

## Launching External Programs

gx runs external programs (`$EDITOR`, the comment editor, `lazygit`, `git commit`) in one of two
modes, which have opposite feedback needs and must not be conflated:

**Takeover launch** — gx suspends and hands the _entire_ terminal to the program; the TUI is not
visible while it runs and resumes when the program exits. Because the takeover itself is the
feedback, no "opening…"/"closed" toast is shown. On return the screen simply refreshes (the diff /
filetree updates under the user). The exception is a **mutation** run this way — `git commit` —
which reports its outcome like every other mutation: `notify.Success("committed")` plus the
repo-mutated signal. Errors always surface loudly.

**Split launch** — the program opens in a split (tmux, kitty with remote control, herdr, or Ghostty on macOS 1.3.0+) and the TUI keeps running beside it. Here
a toast (`"opened <app> split: …"`) _is_ shown, because it is the only signal that the program
launched and where it went.

## Tab Caching and Reload

**Live page cache** — the app shell keeps one live `tea.Model` per `TabID` (`livePageByTab`).
Switching tabs reuses the same instance, so in-tab view state (selection, scroll offset, split
state, filetree expansion) is preserved across switches. A page is only reconstructed when its
`ViewContext` changes (different worktree/ref). Switching tabs does **not** reconstruct or, by
itself, reload a cached page.

**Repo epoch** — a single monotonic counter on the app shell, bumped once per completed mutating
git operation. It is the canonical "the repository changed" signal. Global (not keyed per worktree)
for now: a mutation in any worktree advances the one epoch. The shell records, per cached page, the
epoch the page's data was last loaded at (`loadedEpoch`, stored shell-side on `livePage`, not inside
the page model).

**`RepoMutated`** — a fifth navigation message (`ui/nav`) emitted as a `tea.Cmd` by any operation
that mutates the repository (commit, amend, reword, bump, rebase, push, pull, stage/unstage, stash
apply/pop/drop/create, worktree create/delete). The emitter only declares "the repo changed"; it
does not name which tabs are affected. The shell intercepts it, bumps the **repo epoch**, and stamps
the currently active page as fresh at the new epoch (the active page is the mutator and self-reloads
to show its own result).

**Auto-reload** — a system-initiated, state-preserving reload the shell triggers on tab activation
_only when the page is stale_ (`loadedEpoch < repo epoch`). Exposed by each cacheable page as
`AutoReload() tea.Cmd` (satisfying the `pageAutoReloadable` interface). Because the user did not ask
for it, it preserves maximum view state (e.g. status uses `refreshPreserveScroll`). This replaces
the previous unconditional reload-on-every-activation.

**Manual reload** — a user-initiated reload via the `R` key. Louder than
auto-reload only in that it flashes a "refreshed" notification; like auto-reload it preserves scroll
position and expand/collapse state. It is also the escape hatch for changes made _outside_ gx
(external terminal git commands), which do not bump the repo epoch.

**Scratch watch** — the Tickets and Queue tabs' event-driven notice that `.scratch` changed. It
exists because `.scratch` is written by processes the repo epoch cannot see: another gx, a
ralph-loop running in a sibling worktree, or a hand edit. It is explicitly **best-effort**: it may
miss changes, and it may fail to start at all. Active only while its own tab is active.

**Scratch poll** — the slow periodic reload of `.scratch` that runs alongside the scratch watch.
Unlike the watch it is a guarantee, not an optimization: it is what makes "the tab eventually shows
what is on disk" true even when no event ever arrives (see ADR 0025). Also active only while its own
tab is active; both watch and poll are started when the tab is activated and are allowed to stop
when it is deactivated.

## Queue Lifecycle (Queue Tab)

**Queue** — the Queue tab's view of the **Server queue**: one server-wide collection of queued
tickets across every project and epic. With no server the Queue tab is empty.

**Hand-driven epic** / **Loop-driven epic** — the two kinds of epic `.scratch/` holds, distinguished
by who writes `status`, not by file format or by anything on disk. In a hand-driven epic (a
wayfinder map, or any epic a person works directly) the person is the sole writer and
`gx tickets set` is their channel. In a loop-driven epic gx owns `status` in-process. The two share
one format, one validator, and one CLI; nothing marks which is which, because ownership is a
property of the writer, not of the epic.

**Replace queue** (`r`) / **Add to queue** (`a`) — the two queueing actions from the Tickets tab,
both server writes behind a confirmation. Replace swaps one project's queue for the checked tickets
(they must all belong to that project); Add enqueues the checked tickets. Neither needs a live run
first, and neither opens a run-start modal. _Avoid_: drain-and-replace, run-start modal (both
removed with the in-process loop, no replacement).

## Slots and Caps

**Slot** — one live agent: a node that is claimed and has a running pane. Paused and waiting-for-children
nodes hold no slot, and merging is daemon work, so it takes none. A slot is a plain count, never weighted by model or context window.

**Cap** — a limit on slots in use. A node starts only when every applicable cap has room; there is
no "which binds first" order. Three caps exist: the **global cap** (`execution-queue.max-agents`,
`config.json` only), the **per-root cap** (`execution-queue.max-agents-per-root`, counted per root
node — an epic or a one-off — and project-overridable) and the optional **project cap**
(`max-agents` in `project.json`; unset means none). Lowering a cap stops new starts only: live
agents are never stopped, and the daemon applies the new value at its next scheduling decision.
`explain` names every full cap with its count (`waiting: global cap 4/4`).

**No bypass** — one-offs, `scratch` jobs and `type: investigate` nodes all count against the same
caps. `--front` sets queue position only. See ADR 0032.

## One-offs

**One-off** — a commitless ticket in the project's `one-offs` epic, usually submitted from outside gx
with `gx server one-off "<prompt>"` (or `--file <path>`). It is `type: prompt` unless `--type` says
`evolve` or `diagnose`, and it merges no commits. The project is `--project`, else the one owning the
current directory, else `scratch`. One-offs share one **one-off cap** (`max-one-off-agents`) instead
of the per-root cap, and still count against the global and project caps. A `done` or `cancelled`
one-off moves to `one-offs-archived`; one that failed or is Paused stays in `one-offs` until a
person closes it. _Avoid_: standalone ticket, job. There is one verb and no spool: when the server
is down the submit is refused, never queued for later. See ADR 0033.

**Dedupe key** — the key (`--unique <key>`; with `--file`, by default the file's absolute,
symlink-resolved path; `--no-unique` opts out; a plain prompt has none) that refuses a live
duplicate. Another ticket of the same project with the same key that is not `done` or `cancelled`
counts as live, Paused copies included. The refusal is `duplicate-live`: it returns the existing
ticket's address and writes a `submit-refused` event.

**Result** — the `## Result` section a one-off's agent writes its answer into (with `gx tickets
section`, under the `gx-one-off` skill). `--wait` prints it and `--notify` sends it to chat.

**Wait** — `--wait` blocks until the ticket ends and survives a server restart. `--timeout` stops
only the waiter: the ticket keeps running. On a duplicate-live refusal `--wait` follows the
existing ticket.

## Recovery

When a ticket is Paused, the server tries to get it moving again without a person. Recovery repairs the
orchestration, never the work: it may nudge a pane or call the server's own verbs, but it never
writes code or commits. See ADR 0034.

**Recovery catalog** — the known failure signatures, as Go data in the `recovery` package and
printed by `gx recovery catalog`. Each entry names a `(type, kind)` signature, an executor (`rule`,
`agent` or `person`), an authority (`low`, `medium`, `high`), the remedy verbs it may use, and
whether it is enabled. `recovery.enabled` is the global kill switch.

**Recovery rule** — a catalog entry the server applies itself, in Go (`executor: rule`). It calls the
same verbs as a person, with the same refusals and merge lock, stamped `actor: recovery`. _Avoid_:
bare "rule".

**Investigate ticket** — a `type: investigate` child the server opens when a failure needs judgment
(`executor: agent`) or matches no entry. It runs `gx-investigate` in unattended mode, is queued
`--front`, and is commitless by type, so nothing it commits can merge. Its report is its `## Result`.
It is never itself recovered; any other ticket opts out with `recover: false`.

**Authority** — how far recovery may act on a matched entry. `low` and `medium` apply on their own.
`high` never does: recovery writes the remedy it would run as a `## Proposed Remedy` section and a
`recovery-proposed` event, then escalates. A person runs it with `gx server tickets approve <addr>` (key
`A` on the Queue tab), which refuses `proposal-stale` if the ticket changed since.

**Escalation** — recovery giving the ticket back to a person: no match, a `person` entry, a high
proposal, a cap reached, or a **failed recovery** (the recovered ticket fails again in its next
iteration). Recovery runs at most once per `(ticket, kind)` and three times per ticket.

## Notification Surfaces

The four places a run or session event can surface. They are distinct surfaces, not levels of the
same thing: an event may reach any combination of them. (A **destination** is something else — see
below.)

**TUI** — Queue tab state, driven by `reduceLiveEvent`. Everything gx knows shows up here.

**Toast** — transient in-TUI feedback (`ui/notify`). Only meaningful while someone is looking at the
screen.

**Chat** — the outbound Slack/Telegram surface (the wrapper sinks). Deliberately a small, fixed
subset of run events, for a person away from the terminal. The word **push** is retired for this
sense: it collided with `git push` and with mobile push notifications, and `notify` was already
taken by the toast package.

**Banner** — an OS-level notification, sent once by the server (never by a TUI), with an optional
sound. For a person at the machine who isn't looking at the session it concerns. Distinct from a
**toast**, which lives inside the TUI.

**AFK mode** — a person-set, machine-wide state in which interactive session events also go to
**Chat**, for a person away from the machine. It adds a surface; it never silences banners.

**Destination** — a chat transport plus its target (a Telegram chat id or a Slack webhook URL). The
unit of chat batching, gating and muting: two projects with the same target share one destination.
A `project.json` `notifications` block replaces the global one whole; with no override a project
uses the **global destination**, which also receives every server-level event. `gx notify
--enable/--disable <transport>` acts on every destination of that transport, `--project <name>`
narrows it to that project's destination.

**Counts line** — the line of state tallies a chat message may carry, in place of a fraction:
`8 done · 2 in progress · 1 paused: 07 · 10 total`. A **ticket counts line** tallies the ticket
states within one epic (`done · in progress · paused · blocked · ready · total`); a **queue counts
line** tallies the epic states across the Queue (`done · in progress · paused · failed · total`).
Zero clauses are suppressed except `done` and `total`, which always render. Counts are always
**epic-truth** — recomputed from disk over the whole epic — never scoped to one run's own progress.

## Skills

**Bundled skill** — a skill that ships inside gx and is installed with it. Every bundled skill is
named `gx-*`. A bundled skill calls only other bundled skills, so gx works without any skill the
person installed separately.

**Vendored skill** — a bundled skill copied from a third-party repo, its **upstream**, and kept in
step with it. gx may change its copy; a sync pulls the upstream's later changes and keeps gx's.
Contrast with a bundled skill gx wrote itself (e.g. `gx-verify`), which has no upstream.
_Avoid_: external skill (for a vendored one).

**Sync strategy** — how a sync brings a vendored skill's upstream changes into gx's copy. **Merge**:
a 3-way merge of the upstream change into gx's copy, then a review of the result. **Port**: no
merge; each upstream change is read and carried over by hand only if it matters to gx's copy. Port
suits a copy gx has largely rewritten.

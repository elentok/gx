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

- **Draft** — raw `Status: draft`; work its author parked deliberately. Neither open (it never
  enters the frontier, so no agent claims it) nor done (it still counts as outstanding). This is the
  status `gx tickets add` stamps on a freshly allocated stub, until `set --status open` promotes it.
- **Open** — raw `Status: open`: unclaimed, nothing blocks picking it up. `status` is required,
  so a ticket without one renders as **Error** rather than defaulting to open.
- **Claimed** — raw `Status: claimed`.
- **Blocked** — an overlay, not a raw status: applied whenever an open or claimed ticket has a
  blocker that is still **blocking** (see Ticket Dependencies). Needs-answer, needs-repair, and
  done tickets keep their own state regardless of `blocked_by`.
- **Needs-answer** — raw `Status: needs-answer`; a person is being asked for something — an answer
  or a decision — and nothing is broken. Written either by an agent that stops to ask, or by gx's
  own interactive-prompt gate when a pane blocks on an involuntary prompt. Says nothing about
  whether an iteration survives; that's **Reattach**'s question. _Avoid_: Needs-info (retired
  name, see ADR 0018).
- **Needs-repair** — raw `Status: needs-repair`; gx hit a fault it can't resolve on its own and a
  person must investigate. Never agent-authored. Also says nothing about whether an iteration
  survives: of its several producers, only the operator-attention gate leaves one. _Avoid_:
  Needs-attention (retired name, see ADR 0018), treating it as "a pane is alive" — see Reattach.
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

**Parked** — a ticket's state (`needs-answer`, `needs-repair`, or `draft`; `isParked`) and, derived
from it, a run-level state: an epic with no runnable work left but at least one parked ticket keeps
scheduling everything else, then parks indefinitely and notifies, releasing its slot in the
epic-level concurrency cap, rather than exiting. Per ADR 0020, **stall** names the opposite thing —
the invisible failure this design exists to eliminate — and must not be used for the deliberate,
visible state described here. _Avoid_: Stalled, Human-clearable (retired names, see ADR 0020),
Paused (that's the operator-driven whole-queue toggle), Settled (removed from the vocabulary
entirely — see ADR 0017).

**Unpark** — clearing a parked ticket back to the frontier: a person (or, for a `blocked-pane` or
`self-reported` park, gx itself once the pane is live and unblocked) writes `status: open`. A
`zero-commit` park never auto-unparks on liveness alone — it requires a person to look at the
ticket and flip its status by hand, or new commits to appear on its branch while the pane is live.
Not `resume` — the pause `Gate` already owns that verb — and not `clear` — the Queue tab's
clear-checked/-complete keymaps already own that verb, and mean deleting tickets from the queue.

**Park kind** (`park_kind` frontmatter field, `needs-answer` parks only) — which of three things
caused a `needs-answer` park: `blocked-pane` (a genuinely blocked interactive prompt — pane stays
live), `self-reported` (the agent itself asked a real question via `iteration_status:
needs-answer` — its pane/worktree/tab is already released by this point, unlike the other two),
or `zero-commit` (gx's own uncertain guess: no commits, no self-report, pane left alive for
inspection). A ticket parked before this field existed carries no `park_kind` and is treated as
`zero-commit` — the conservative default. Not written for `needs-repair`/`draft` parks, which keep
their own liveness-only unpark rule untouched. _Avoid_: "gate park", "announce-and-stop park"
(retired informal names — collided with the already-established `Gate` type and its `.gates()`
verb).

**Deadlocked** — a run-level state: an epic with no runnable work and nothing parked either — a
genuine dependency error, reported as a failure rather than parked on.

**Reattach** — reconnecting a run to an iteration that is still live and owned. Run at startup for
tickets found `claimed`, and again when a parked run resumes. Its result, not the ticket's status,
decides how a park resumes: reattached → `claimed`, the same iteration continues; not reattached →
`open`, a fresh iteration is launched. A surviving herdr pane is *not* the same thing — a pane
outlives its goroutine. _Avoid_: "has a pane".

**Background task** — a shell command a Claude agent moved off its own foreground turn (recorded as
a `backgroundTaskId` in the session transcript), resolved later by a `task-notification` matched on
task id. "Outstanding" while unresolved, "resolved" once a matching notification is seen (its
reported status — success, failure, killed — doesn't matter, only that one exists), "aged out" once
it's stayed outstanding past a generous cap without resolving. Turn-level, inside one still-live
iteration — distinct from the pane-level Reattach/Live terms above, which describe the iteration as
a whole. A subagent's (sidechain) background task is never a signal about its parent iteration.
Claude-only; Codex iterations have no equivalent signal.

**`status` / `iteration_status`** — a ticket's status splits across two fields with different
owners. **`status`** is gx-owned and is the sole scheduling authority (the six `RenderedStatus`
values above). **`iteration_status`** is agent-owned and reports on the current claim alone
(`working` / `needs-answer` / `finished`, or absent — gx clears it on every claim and reattach, so
a report is never readable outside the claim that produced it). Not to be confused with **herdr's
`agent_status`** (`idle`/`done`/`blocked`, on the pane payload, `herdr/agent.go`) — the two names
collided during design specifically because both describe "what is the agent up to," which is why
`iteration_status` was picked instead of the more obvious `agent_status`: it names what the field
describes (the current iteration's state) rather than who wrote it, and can't be confused with
herdr's field in the one file that holds both. See ADR 0019.

**Adopt** — the verb for gx accepting an `iteration_status` report and acting on it: seeing
`needs-answer` and writing `status: needs-answer`, or seeing `finished` and entering the landing
path (only writing `status: done` if landing succeeds). Not "promote", which already names two
unrelated things in this repo (`draft` → `open`, and an epic entering `MaxConcurrentEpics`'s
auto-promotion queue). An agent's report can start a landing; it can never conclude one — the
commit count and the cherry-pick are what conclude it. See ADR 0019.

**Park reason** — the first non-empty line after a parked ticket's `## Needs Answer` or `## Needs
Repair` heading, with markdown markers stripped and ellipsised for display. What the Queue row
shows as subtext for a parked ticket, read fresh from disk on every render rather than cached, so
the row can't go stale between a park and a restart.

## Ticket Forking

**Fork** — dividing a ticket into new sibling tickets mid-flight, when it turns out to be larger
than its budget or mixes concerns that should land separately. _Avoid_: Split.

**Parent** — the ticket a forked ticket came from, or the code-review ticket that opened a fix
ticket. Frontmatter field `parent`, written on the descendant at creation. This is the only
structural edge between tickets: nothing is recorded on the ticket being pointed at. _Avoid_: Split
from, Children, the `children` frontmatter field (removed).

**Fork subtree** — a ticket plus every ticket reached by following `parent` reverse-edges down from
it, at any depth. Derived from `parent` alone; there is no stored child list. What `blocked_by`
resolution actually asks about. _Avoid_: Children.

**Fork suffix** — the letter appended to the parent's number to name each forked child (`04` forks
into `04a`, `04b`; one level deeper, `04b1`). _Avoid_: Split suffix.

## Ticket Dependencies

**Blocker** — a ticket named in another ticket's `blocked_by` list.

**Blocking** — a blocker is *blocking* until its own work is `done` **and** every ticket in its fork
subtree is likewise no longer blocking. This is the only question `blocked_by` resolution asks, and
it is the reason a ticket's own `done` is not enough to release its dependents. _Avoid_: Resolved,
satisfied, fully-done.

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

## Merge onto Main

**`gx merge <branch>`** — the deterministic core of merging a branch/worktree onto the repo's
detected main branch. Resolves `<branch>` (a literal branch name, or a worktree-dir name looked up
via `git.Worktree`) and the base branch, then attempts `git merge --ff-only`. Never rebases and
never leaves the repo mid-operation — on non-fast-forward it reports `needs_rebase` plus the
resolved `branch`/`base`/`worktree_path` and stops. See ADR 0015.

**`gx-merge`** (the skill) — owns the judgment half: on `needs_rebase`, runs `git rebase <base>`
against the target's worktree (or the bare branch if `worktree_path` is empty), invoking
[gx-resolving-merge-conflicts](skills/gx-resolving-merge-conflicts/SKILL.md) on conflicts, pausing
to show the rebased diff, running this repo's checks, then re-calling `gx merge` for the final
ff-only merge. `gx-cleanup`'s ff-only-merge step calls into this same skill rather than duplicating
the flow.

## Launching External Programs

gx runs external programs (`$EDITOR`, the comment editor, `lazygit`, `git commit`) in one of two
modes, which have opposite feedback needs and must not be conflated:

**Takeover launch** — gx suspends and hands the _entire_ terminal to the program; the TUI is not
visible while it runs and resumes when the program exits. Because the takeover itself is the
feedback, no "opening…"/"closed" toast is shown. On return the screen simply refreshes (the diff /
filetree updates under the user). The exception is a **mutation** run this way — `git commit` —
which reports its outcome like every other mutation: `notify.Success("committed")` plus the
repo-mutated signal. Errors always surface loudly.

**Split launch** — the program opens in a tmux/kitty split and the TUI keeps running beside it. Here
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

**Manual reload** — a user-initiated reload via the `R` key (and status `m r`). Louder than
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

## Queue and Attach Lifecycle (Queue Tab)

**Queue** — the single, per-repo collection of checked/queued tickets across all epics
(`QueueStore`, keyed off the repo's `.scratch` dir). One Queue per repo, not per epic.

**Attach / Attached / Detach** — at most one gx process, repo-wide, may be Attached to the Queue at
a time. A process attaches when its first epic run starts (`loopRegistry.tryStart` acquiring the
attach lock while its internal `attachCount` is 0) and detaches when its last running epic run ends
(`attachCount` returning to 0 in `loopRegistry.finish`). Attachment doesn't limit how many epics that
one process runs concurrently — only the separate `maxConcurrent` slot cap does that. `SelfAttached`
reports this process's own attachment for the Queue tab label's "(attached)" suffix.

**Attach lock** — the on-disk record of the attached process, one per repo:
`.scratch/queue-attach.json`, holding the holder's pid and process start time (so a reused pid after
reboot isn't mistaken for the same process — see `attachLockIsStale`).

**Foreign attachment** — the Queue is attached to a different gx process (`ForeignAttachPID` returns
a nonzero pid). Hard-blocks starting a new epic run in this process with `"a ralph-loop is already
running (attached by process %d)"`.

**Epic run** — the per-epic ralph-loop execution (`loopRegistry.runs[epicName]`). Several can run
concurrently inside whichever process holds the attachment, up to the concurrency slot cap.

**Hand-driven epic** / **Loop-driven epic** — the two kinds of epic `.scratch/` holds, distinguished
by who writes `status`, not by file format or by anything on disk. In a hand-driven epic (a
wayfinder map, or any epic a person works directly) the person is the sole writer and
`gx tickets set` is their channel. In a loop-driven epic gx owns `status` in-process. The two share
one format, one validator, and one CLI; nothing marks which is which, because ownership is a
property of the writer, not of the epic.

**Reattach / Reattach signal** — per-ticket detection that a specific ticket's session is still
alive, checked via `ralphloop.ScanForReattachable`. A special case of Attach: it only fires when the
Queue is Detached (`attachLockHeld` false) and at least one ticket is left `claimed`/
`needs-repair`, and only proceeds after the user confirms the "Found a detached live queue…
Reattach?" prompt (`handleDetachedLiveDetected`) — there is no silent auto-reattach.

**Live** — the Queue (or a specific ticket) has at least one `claimed`/`needs-repair` ticket with
a still-alive session, as found by a Reattach signal scan (`cmdCheckDetachedLive`'s `alive` count).

**Replace queue** (`r`) / **Add to queue** (`a`) — the two queueing actions from the Tickets tab.
Replace clears both the not-yet-started (pending) and already-finished (done) queue selection,
replacing it with the checked tickets, then jumps to the Queue tab — a running or errored entry is
left untouched (queue safety: a live run's own state isn't something Replace should silently
discard). It is blocked process-wide ("Can't replace a live queue") while any epic run is live,
regardless of which epic the checked tickets belong to. Add widens an already-running epic's frozen
scope (`ralphloop.RunScope.Add`) with the checked tickets under that epic, after a confirmation
naming the count — it requires the epic under the cursor to already have a live run.

## Notification Surfaces

The three places a run event can surface. They are distinct destinations, not levels of the same
thing: an event may reach any combination of them.

**TUI** — Queue tab state, driven by `reduceLiveEvent`. Everything gx knows shows up here.

**Toast** — transient in-TUI feedback (`ui/notify`). Only meaningful while someone is looking at the
screen.

**Chat** — the outbound Slack/Telegram surface (the wrapper sinks). Deliberately a small, fixed
subset of run events, for a person away from the terminal. The word **push** is retired for this
sense: it collided with `git push` and with mobile push notifications, and `notify` was already
taken by the toast package.

**Counts line** — the line of state tallies a chat message may carry, in place of a fraction:
`8 done · 2 in progress · 1 parked: 07 · 10 total`. A **ticket counts line** tallies the ticket
states within one epic (`done · in progress · parked · blocked · ready · total`); a **queue counts
line** tallies the epic states across the Queue (`done · in progress · parked · failed · total`).
Zero clauses are suppressed except `done` and `total`, which always render. Counts are always
**epic-truth** — recomputed from disk over the whole epic — never scoped to one run's own progress.

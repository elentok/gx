# A run stalled on a human parks and releases its concurrency slot

> Superseded in part by ADR [0018](0018-needs-answer-vs-needs-repair.md), which re-cuts
> `needs-info`/`needs-attention` into `needs-answer`/`needs-repair` on a new axis; this ADR's
> parking, reattach-decides-resume, and concurrency-permit decisions are untouched.

> Superseded in part by ADR [0020](0020-parked-not-stalled.md), which renames this ADR's
> "stalled"/"stall" vocabulary to "parked", reserving "stall" for the invisible-progress failure
> mode alone; the decision here is unchanged.

An iteration ending `needs-info` or `needs-attention` used to end the run: both counted as settled,
so `AllSettled` went true and the loop exited — and if nothing else was in flight, `needs-attention`
returned a hard error (`"epic %q paused with no running iterations left"`). That made the obvious
workflow impossible: the agent asks a question in its pane, you answer it, and the run continues.
The machinery to wait already existed (`Gate`, the resume signal, the notification sinks); the loop
simply declined to use it.

We decided a run with no runnable work left but a human-clearable ticket in it is **stalled**: it
keeps scheduling every other unblocked ticket, then parks indefinitely and notifies, rather than
exiting. Human-clearable means `needs-info`, `needs-attention`, or `draft`. It resumes automatically
when the stalled ticket's status clears; Enter on the epic's Queue-tab row is the manual override.
Only a run with no runnable work *and* nothing a human could clear is **deadlocked**, and that is
still an error. This removes "settled" from the vocabulary entirely: the loop exits when everything
is done, parks when stalled, errors when deadlocked.

How it resumes is decided by iteration ownership, not by the stalled status. The run attempts to
reattach the ticket's iteration, using the path it already runs at startup for tickets found
`claimed`: reattach succeeds and the ticket goes to `claimed` with that iteration continuing in
place; reattach fails and it goes to `open` with a fresh iteration launched. Keying this off the
status instead was considered and is wrong — `needs-attention` is written by three different
producers and only the operator-attention gate leaves an owned iteration, while a generic iteration
error is recorded after the goroutine has exited and reconciliation writes it for a ticket that
never had one. Testing for a surviving pane is not a sufficient proxy either, since a pane outlives
its goroutine and writing `claimed` restores neither the run's active count nor any attachment.

## Consequences

- `MaxConcurrentEpics` is redefined to count epics with at least one running iteration, so a parked
  epic releases its slot and doesn't starve queued epics. The cap now bounds concurrent *agents* —
  which is what it was always for — rather than live `Run()` calls. The accepted cost is that more
  than `MaxConcurrentEpics` epics may hold worktrees and herdr panes at once: disk and tab clutter,
  not load.
- Enforcing that cap requires an admission seam the registry didn't have. It admitted once, when a
  run was first started, after which each live `Run` polled and claimed on its own — so two parked
  runs resuming at the same instant would both claim before the registry saw either. The registry
  now issues an acquire/release permit that `Run` holds across each zero-active → first-claim
  transition, acquired synchronously before the first claim and released on park, so the cap is
  checked at every admission rather than only the first.
- An epic whose only remaining tickets are `draft` parks rather than erroring, which makes the
  deadlock error a genuine corruption signal instead of the normal outcome of authoring an epic
  before running it.
- The parked wait has no timeout. A notification already fires, and an epic that exits takes its
  panes' recoverability with it — the state most worth preserving when you come back hours later.
- `needs-info` and `needs-attention` stay distinct statuses even though they now produce identical
  loop behavior and identical resume behavior. They differ in what they tell the operator:
  `needs-info` is the agent asking a question about the work, `needs-attention` is the machinery
  reporting that it broke. Whether there is a pane to jump to is reported separately, from the
  reattach result, because it does not follow from the status.
- The orphaned `resume-signal` file plumbing (`ralphloop.Resume`, `resumeSignalPath`,
  `ResumeSignaled` and its three poll sites) is deleted rather than wired up. `ralphloop.Run` is
  only ever called from the TUI process, so resume is in-process; a headless loop is not a goal, and
  dead code that looks like working pause/resume infrastructure is actively misleading.

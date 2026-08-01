# 03 — Needs-attention lifecycle

**What to build:** When Codex asks for intervention, ralph-loop makes that
state visible and durable rather than silently waiting: the ticket becomes
`needs-attention`, scheduling pauses, and the operator can handle the request
in its herdr pane. The loop automatically continues once Codex no longer
needs attention, while allowing an optional manual recheck/resume.

**Blocked by:** 01 — Codex agent launch and skill handoff.

**Status:** done

- [x] A Codex pane that enters the agent `blocked` state marks its ticket
      `needs-attention`, records the pane and reason in the run log, and keeps
      its worktree/tab available for intervention.
- [x] New iterations do not start while any ticket needs attention; already
      running iterations may finish normally.
- [x] When the intervened pane leaves `blocked`, the loop restores the ticket
      to claimed, re-observes it, and resumes scheduling without a separate
      command.
- [x] A manual recheck/resume is safe when a pane has already recovered or is
      still blocked.
- [x] Restart reconciliation and tracker rendering recognize `needs-attention`
      as a durable, actionable state rather than an error or schedulable work.
- [x] Tests cover detection, automatic recovery, manual recheck, and restart.

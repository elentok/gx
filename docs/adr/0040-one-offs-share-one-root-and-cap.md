# One-offs share one root and one cap

All one-offs of a project are tickets in a single `one-offs` epic, so they share one queue root,
`project:one-offs`. The per-root cap does not apply to that root. A separate cap,
`execution-queue.max-one-off-agents` (default 3, project-overridable), limits live one-off agents,
and the global and project caps still apply (ADR 0032). We rejected one queue root per one-off
because it gave each one its own cap, so the number of one-off agents had no bound. The shared root
is hard to undo: addresses, the archive move and the Queue card all assume it. The `one-offs` epic
stays `draft`, so loop-epic landing never picks it up.

# Server launches never adopt an existing agent

A server launch (human, recovery or reclaim relaunch) never attaches to an agent already holding
the ticket's label, even when its cwd matches. A merge ticket's cwd is always the feature worktree,
so the old cwd match always hit: the relaunch attached to the old idle agent, never prompted it,
read its idle pane as a finish and parked again. Telling whether an old agent is safe to reuse is
harder than starting fresh, so gx closes a **Stale holder** and retries once, and parks a working
holder for a person or the investigate agent. Restoring live runs from runs.json on server start is
not a launch and is unchanged. This deliberately differs from the in-process loop's `adoptedLaunch`,
which stays as it is.

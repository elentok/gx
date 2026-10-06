---
name: gx-resolving-merge-conflicts
description: "Use when you need to resolve an in-progress git merge/rebase conflict."
---

1. **See the current state** of the merge/rebase. Check git history, and the conflicting files.

2. **Find the primary sources** for each conflict. Understand deeply why each change was made, and
   what the original intent was. Read the commit messages, check the PRs, check original tickets
   (gx's local tracker — see [gx-local-tracker.md](../gx-local-tracker.md) — where applicable).

3. **Resolve each hunk.** Preserve both intents where possible. Where incompatible, pick the one
   matching the merge's stated goal and note the trade-off. Do **not** invent new behaviour. Always
   resolve; never `--abort`. Resolve each hunk with `Edit` on the marker block, never a scripted
   splice (python/sed heredocs, `cat >`).

4. Discover the project's **automated checks** and run them — typically typecheck, then tests, then
   format. Fix anything the merge broke.

5. **Finish the merge/rebase.** Stage everything and commit. If rebasing, continue the rebase
   process until all commits are rebased.

6. **Finish with zero background shells.** If a call was moved to the background, read its output,
   then stop it (TaskStop). ralph-loop will not land a ticket while one is open.

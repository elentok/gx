---
name: gx-resolving-merge-conflicts
description: "Use when you need to resolve an in-progress git merge/rebase conflict."
---

1. **See the current state** of the merge/rebase. Check git history, and the conflicting files.
   Run `gx brief conflicts`: one call lists the operation, every unmerged file and its conflict
   marker line numbers. Don't hand-build a status command.

2. **Find the primary sources** for each conflict. Understand deeply why each change was made, and
   what the original intent was. Read the commit messages, check the PRs, check original tickets
   (gx's local tracker — see [gx-local-tracker.md](../gx-local-tracker.md) — where applicable).

3. **Resolve each hunk.** Preserve both intents where possible. Where incompatible, pick the one
   matching the merge's stated goal and note the trade-off. Do **not** invent new behaviour. Always
   resolve; never `--abort`. Resolve each hunk with a file edit (Edit) on the marker block, never a scripted
   splice (python/sed heredocs, `cat >`).

   Inside a cherry-pick, `--ours` is HEAD (the branch being built on), and `--theirs` is the
   cherry-picked commit. Never `git checkout --ours <file>` / `--theirs <file>` a whole file: it
   drops the other side's changes. Before continuing a cherry-pick, run
   `git show CHERRY_PICK_HEAD --stat` and confirm every file it changes still carries that
   commit's change in the resolved tree, including files that did not conflict.

4. Run only the checks that touch the conflicted files, with the `gx-verify` skill on every
   conflicted file, e2e flows (`*.e2e.ts`) included. Rerun them before
   `git cherry-pick --continue`, even when vitest, tsc and vet pass: a conflicted e2e flow still
   needs its Playwright run. Build and vet every package the
   cherry-picked or merged commit touches (not only the conflicted files' packages), then
   targeted tests (`-run <pattern>`) for the code the conflict changed. Never run a whole package
   or the full suite for a small conflict. Skip checks the original ticket already passed. Fix
   anything the merge broke. Run format checks only on the files the branch changed
   (`git diff --name-only <target>...HEAD`). A repo-wide `gofmt -l`
   lists files the branch never touched; those are out of scope. Finally, reread the ticket's
   stated goal and confirm it still holds in the resolved files; a build that passes does not
   show the kept side is the right one.

5. **Finish the merge/rebase.** Stage the resolved files by name (`git add <file>...`), never
   `git add -A` or a directory, then commit. If rebasing, continue the rebase
   process until all commits are rebased.

6. **Never edit a ticket file or its status.** You may read tickets for context, but gx closes the
   conflict-resolution ticket itself when you exit; a hand-written status can pause the parent.

7. **Never leave the worktree root with a bare `cd`.** The shell keeps the new directory, so the
   next command runs in the wrong place. Use a subshell, `(cd webui && pnpm exec vitest ...)`, or
   absolute paths. Use the same worktree root for Bash and for Read/Edit paths.

8. **Finish with zero background shells.** If a call was moved to the background, read its output,
   then stop that background shell (TaskStop), as the Tool names table in
   [gx-local-tracker.md](../gx-local-tracker.md) says. ralph-loop will not merge a ticket while one is
   open.

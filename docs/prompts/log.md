# gx log and commit

## Overview

I want to add two TUI pages: log and commit, similar to the worktrees and status pages:

- The log page should include a list of commits starting from HEAD (or other commit, depends on the
  use case)
- The commit page should be identical to the status view, but without the stage/unstage separation
  (I will want to add actions on the files and hunks like restore a version, or restore a hunk, or
  delete a hunk from a commit)

## Log view

Should show a table of commits, showing:

- hash
- relative date (4d ago)
- author (initials)
- subject
- tags (as badges)
- basic graph representation (we can use the default that is generated from git log, I always use
  rebase/merge-and-squash so I rarely have graphs, but I still want to support them)

Actions:

- j/k/arrows to navigate between commits (one active commit at the time)
- / search modes (same as in worktrees and status)
- Pressing enter on a commit will send the user to the commit view

## Commit view

This should be identical to the status view, but:

1. without the stage/unstage functionality.
2. with a commit info frame showing:
   - first line:
     - hash
     - subject
     - full date + relative date: `2026.05.01 (5 days ago)`
   - body

It should include other keymaps:

- b - toggle message body (collapse/expand) - when collapsed show the first line with "..."

- r - restore the version of the file/hunk/line/selection, should open a menu popup (like the "N"
  for new session in worktrees) with two options:
  - "a" - restore the local file to the version _after_ this commit
  - "b" - restore the local file to the version _before_ this commit

- d - delete change (file/hunk/line/selection) from the commit (show confirmation)

- y - show the new chained keymaps modal described below, and in addition to the usual yank keymaps
  also add:
  - h - hash
  - s - subject
  - m - message (subject + body)

## Interaction between worktrees, status, log and commit

I want to have a unified experience between worktrees, status and log by
adding tabs to the bottom bar where the active one will be highlighted with
an orange background and the inactive ones will be dim.

```
worktrees log status
```

Or with nerdtree

```
worktrees log status
```

I want to be able to move between them in multiple ways:

- global hotkeys:
  - gw - goto worktrees
  - gl - goto log
  - gs - goto status
- from worktrees:
  - pressing enter on a worktree should switch to the log of that worktree

The commit view should share the log tab (pressing "gl" in the log tab will switch to the log tab).

We'll need to change the mapping for lazygit to something else, let's use "L" for now.

## Navigation

Currently pressing `q` exits, I want it (and also <esc>) to "go back", e.g.

- If you reached the log from worktrees, q/esc will send you back to the worktrees view
- If you reached the status from the log/worktrees, q/esc will send you back to the worktree
- If you reached the commit view from the log, q/esc will send you back to the log

So we need a navigation system, perhaps something like react router, is there something like that
for bubbletea?

It would be nice to have a sort of `navigateBack` and `navigateTo` apis, e.g.

- `navigateTo('/commit/{hash}')`
- `navigateTo('/status')`
- `navigateTo('/worktree/{name}')`

I'm not sure it's necessary though, depends on how we decide to implement.

## Chained keymaps UX update

In worktrees, we've recently added the "N" keymap that opens a popup to let the user pick how to
open the new worktree (in a new session, tab, or split window).

I want this to be used for all chained keymaps, it should be a shared component.

---

## Follow-up prompt

- Regarding "Add a new root model under `ui/` or `ui/app/` that hosts child pages." - what would
  you suggest and why?
- Regarding "delete change from commit" - I do want history rewrite (let's keep this for the last
  milestone)
- Regarding docs/log-commit-plan.md L95-97 (load commit history), I want to be able to run:
  - `gx show ${hash or ref}` - will open on commit view of the specific hash/ref
  - `gx log ${hash or ref}` - will open on log view of the specific hash/ref
- Something else I thought about - in the log view, if showing HEAD and there are uncommited changes
  (unstaged, staged, untracked) then add a "pseudo-commit" row above all rows, if you press enter on
  it you navigate to the status view.
- Do we have enough existing test coverage so we can be sure we don't break anything?

Please update the plan file.

Do you have any questions for me before we begin?

---

## Follow-up prompt #2

1. `gx log <ref>` should start exactly at ref and walk ancestores from there.

Answering your questions:

1. For `gx log <ref>`:
   should the table start at exactly <ref> and walk ancestors from there, or should <ref> just become the selected row inside the normal HEAD
   history when reachable?

A: start at exactly `<ref>` and walk ancestors from there.

When running with an existing ref we need a way to reset the custom ref back to head,
maybe a mapping like "gh" (goto head)? wdyt?

2. For `gx show <ref>`:
   if `<ref>` resolves to a branch name, should commit view open on that branch’s current tip commit, or do you want branch-specific context
   preserved in the header/footer too?

we should show the branch's current tip commit. what branch-specific context can we show?

3. For history rewrite:
   should d operate only on commits reachable from current HEAD, or do you also want it allowed when viewing arbitrary detached refs where rewrite
   would need extra guardrails?

only on commits reachable from current HEAD (when trying on unreachable commits show an error
message explaining why)

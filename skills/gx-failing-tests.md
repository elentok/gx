# Sorting a failing test

Use this for every failing test you did not expect. Sort it before you fix anything.

1. Rerun **only that test** 3 times through `gx brief` (Go: `gx brief go-test -count=1 -run
   '^TestName$' ./pkg`). Any pass makes it a **flake**.
2. If it failed all 3 times, run it on the base: `git merge-base <target> HEAD`, checked out in a
   temporary detached worktree that you remove after.
   - It fails the same way there: a **pre-existing failure**.
   - It passes there: a **branch fault**.

## What to do

- **Flake or pre-existing failure:** file it with [gx-follow-up](gx-follow-up/SKILL.md), with the
  test name in the title. Then carry on with your ticket.
- **Small fix:** a fix of 30 changed lines or fewer in at most 2 files may go inside your ticket.
  Record it under `## Spec deviations` in the ticket body, and file the follow-up as fixed.
- **Branch fault:** fix it, or report it as the branch's problem. Do not file it as a follow-up.

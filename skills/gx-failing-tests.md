# Sorting a failing test

Use this for every failing test you did not expect. Sort it before you fix anything.

**More than 10 failing tests in one run?** Skip the steps below and use
[Many failures at once](#many-failures-at-once). With 10 or fewer, use these steps.

1. Rerun **only that test** 3 times through `gx brief` (Go: `gx brief go-test -count=1 -run
   '^TestName$' ./pkg`). Any pass makes it a **flake**.
2. If it failed all 3 times, run it on the base: `git merge-base <target> HEAD`, checked out in a
   temporary detached worktree that you remove after.
   - It fails the same way there: a **pre-existing failure**.
   - It passes there: a **branch fault**.

## What to do

- **Flake or pre-existing failure:** file it with [gx-follow-up](gx-follow-up/SKILL.md), with the
  exact test or check name in the title. Expect a `duplicate-follow-up` refusal when an open one
  already has that title: it prints the existing address, so add "seen again" there instead. Then carry on with your ticket, and finish with
  `gx tickets set --check-failures` (one ticket per failing test).
- **Small fix:** a fix of 30 changed lines or fewer in at most 2 files may go inside your ticket.
  Record it under `## Spec deviations` in the ticket body, and file the follow-up as fixed.
- **Branch fault:** fix it, or report it as the branch's problem. Do not file it as a follow-up.

## Many failures at once

For a run with more than 10 failing tests. No per-test reruns.

1. Run the whole failing set once on the base (`git merge-base <target> HEAD`, in a temporary
   detached worktree that you remove after).
2. Tests that also fail on the base are **pre-existing failures**. File one grouped follow-up with
   [gx-follow-up](gx-follow-up/SKILL.md) that names the set.
3. Tests that pass on the base: rerun that set once at low workers (for example `--workers=2`).
   - Now pass: **load-only** failures. Note them in one grouped follow-up.
   - Still fail: **branch faults**. Fix them or report them as the branch's problem.
4. Finish with `gx tickets set --check-failures` naming the one grouped follow-up.

---
name: gx-verify
description:
  Verify changed files after completing a task or before commit using cheap targeted checks. Use for
  staged, commit-all, or working-changes verification when handing work back or when another skill
  needs changed-file checks.
---

Run cheap, targeted verification for changed files. `gx check` does the work.

## Workflow

1. Select the mode. The caller names it:
   - `staged`: verify only what is staged for the commit. Run `gx check --staged`.
   - `commit-all`: verify every changed and new file, because the caller will commit all of them.
     Run `gx check`.
   - `working-changes`: verify every changed and new file after completing a task, without staging.
     Run `gx check`.
2. Run it in the foreground, unpiped, with Bash `timeout: 600000`: the default 120s moves it to the
   background, and `./server` tests alone take about 4 minutes. Do not run the package separately
   first. Report the exit code:
   - `0`: pass
   - `1`: fail
   - `2`: could not run
   - `3`: no applicable checks

   For a failing test, follow [gx-failing-tests](../gx-failing-tests.md).

## Boundaries

- Do not run whole-repo build, lint, format, or test commands unless the user explicitly asks.
- Do not perform secret scanning or semantic review; those belong to the caller.

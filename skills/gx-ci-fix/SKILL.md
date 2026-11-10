---
name: gx-ci-fix
description: >-
  Fix one failed CI run on an epic's pull request branch. Reads the failure, reproduces it locally,
  fixes it, has a read-only reviewer check the diff, pushes, and writes a Resolution. Launched for
  tickets with `type: ci-fix`. Does not poll or loop.
disable-model-invocation: true
---

# gx CI Fix

Fix one failed CI run for an epic's pull request. One run, one fix, one Resolution. You do not poll
for the next result and you do not loop; gx launches a new `ci-fix` ticket if CI fails again.

Run `gx tickets validate <addr>` on the ticket first. If it fails, stop and fix its frontmatter.

See [gx-local-tracker.md](../gx-local-tracker.md) for the ticket and "Skill conventions".

## Steps

1. **Read earlier attempts.** Read every earlier `ci-fix` Resolution the ticket lists, before
   anything else. Never repeat a change that already failed.
2. **Fetch the log.** Use the run id and job id the ticket gives:
   `gh run view <run> --log-failed` (or `gh run view --job <job> --log`). For a check with no
   workflow run, read the check name and its `targetUrl`/`detailsUrl` from the ticket and fetch what
   they point to. When nothing can be read, or a code change cannot address the failure (runner
   outage, expired credentials, missing secrets), the outcome is `unfixable`.
3. **Run the failing check locally** with the repo's own commands, and see it fail before you change
   anything.
4. **Load the person's rules.** After reading the failure, read each skill in the ticket's
   `skills.ci-fix` list (by path to its `SKILL.md`) and follow it as the person's own rules.
5. **Fix it**, then run the same check locally and see it pass.
6. **Review before the push.** Run `gx agent model strongest` and start one read-only reviewer
   subagent with that output as the `model` field (the effort rules are in
   [Skill conventions](../gx-local-tracker.md#skill-conventions)). Give it the failure, the diff and your rationale;
   it inspects only and does not edit. Apply what it flags.
7. **Commit and push.** Fetch the remote epic branch, rebase onto it, then push with
   `--force-with-lease`. Rules:
   - never overwrite foreign commits;
   - never skip hooks (no `--no-verify`);
   - never edit CI config to turn a check off.

   If the rebase conflicts with foreign commits, abort the rebase, push nothing, make no commit, and
   report `Outcome: foreign-conflict` with the reason.

## Resolution

Write it with `gx tickets section <addr> Resolution -`, with these lines:

- `Check`: the failing check's name.
- `Cause`: what broke.
- `Change`: what you changed.
- `Commit`: the pushed commit's SHA.
- `Outcome`: `fixed`, `unfixable` or `foreign-conflict`.

`unfixable` and `foreign-conflict` finish with no commit and the reason in the Resolution. Finish
with `gx tickets set <addr> --iteration-status finished` (add `--commitless true` when there is no
commit).

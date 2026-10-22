---
name: sync-vendored-skills
description: Sync gx's vendored skills with their upstream repos. Maintainers run it by hand. Use when asked to sync, update or check vendored skills.
disable-model-invocation: true
---

Sync the skills listed in `scripts/vendored-skills.manifest` with upstream. The script fetches, merges and reports. You judge. Spec: `docs/specs/vendored-skills.md`, "The sync agent's rules".

This skill is repo-local. It is not in the bundle, so never copy it under `skills/`.

## Steps

1. Run `scripts/sync-vendored-skills.sh --check`. If every skill is up to date, stop.
2. Run `scripts/sync-vendored-skills.sh --merge`. It prints, per skill, the upstream commits, the upstream diff, and for `merge` skills the clean and conflicted files. It edits gx's copy under `skills/` and does not move the pin.
3. Review every upstream hunk, clean ones too. Resolve every conflict marker in `skills/`.
4. Handle the cases below.
5. Run the bundle tests: `rtk go test ./skills -timeout 60s`. They fail on a leftover conflict marker and on a call to a skill outside the bundle.
6. After the tests pass, set each synced skill's pin in `scripts/vendored-skills.manifest` to the newest commit in that skill's report (the first line of its commit list). Do not move the pin of a skill blocked by a license refusal.
7. Commit the sync. The message lists every skipped upstream change (a change not ported, or a merge hunk you dropped) with a one-line reason. A skipped change still moves the pin.

## Decide alone

- Renames of new upstream skill references to `gx-*` names.
- Tracker links, following gx's local tracker.
- Conflicts where both sides keep their intent. Combine them.

Renames are ordinary gx edits. The merge keeps them. There is no rename table.

## Ask the maintainer

Follow `skills/gx-grilling/SKILL.md` for the question format: numbered questions, each with your recommended answer, worded so "yes" accepts it. Ask about:

- An upstream behaviour change.
- A conflict with a gx decision.
- A changed upstream `LICENSE`. The script writes nothing for that skill and prints the license diff. Ask whether the new license still allows vendoring. If yes, re-run with `--accept-license <skill>`, which copies the new `LICENSE`. If no, the skill stays at its pin.
- A pinned path that is gone (the script reports it and writes nothing for that skill). First search the upstream repo for a skill with the same `name:`. Then ask: follow the move, keep the skill as gx-owned (drop its manifest entry), or remove it.
- A vendored skill that now calls a skill outside the bundle. Ask: vendor that skill, drop the call, or point it at a `gx-*` skill.

## Strategies

- `merge`: the script has already merged. If most of a skill's hunks conflict, suggest switching that skill to `port` in the manifest. The maintainer decides.
- `port`: the script prints the upstream diff and touches no file. Carry each upstream change that matters over to gx's copy by hand. List the ones you skip in the commit message.

## Script flags

- `--check`: lists skills with new upstream commits. Writes nothing.
- `--merge`: runs the 3-way merge and prints the report.
- `--accept-license SKILL`: accepts a changed upstream `LICENSE` for that skill. Repeatable.
- `--manifest FILE`, `--skills-dir DIR`: override the manifest and the skills directory.

---
name: gx-pr-text
description: >-
  Write the title and body of an epic's pull request. Launched for tickets with `type: pr-text`.
  Follows the person's named skill when the ticket gives one, otherwise the default rules.
disable-model-invocation: true
---

# gx PR Text

Write the pull request text for one epic. See [gx-local-tracker.md](../gx-local-tracker.md).

Run `gx tickets validate <addr>` on the ticket first. If it fails, stop and fix its frontmatter.

## Rules

When the ticket names a skill (the value of `skills.pr-text`), read that skill's `SKILL.md` by path
and follow it instead of the default rules below. It is read, not run as a command.

With none named, the defaults apply:

- The title is the epic title.
- The body is a short summary, then a link to the spec when one exists (otherwise the epic
  description), then a checklist of the closed tickets.

Read the spec, the ticket titles and the diff yourself before writing.

## Resolution

Write it with `gx tickets section <addr> Resolution -`:

- `Title:` one non-empty line.
- `Body:` a non-empty section; it runs to the end of the Resolution.

Then finish with `gx tickets set <addr> --iteration-status finished --commitless true`.

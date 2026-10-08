---
name: gx-one-off
description: >-
  Do a one-off `type: prompt` ticket: read its body, do the work, write `## Result`, and report
  finished. Launched by the gx server for prompt tickets instead of gx-implement.
disable-model-invocation: true
---

# gx One-Off

Do the work a `type: prompt` ticket asks for. It is not an implementation ticket: no TDD, no
forking, no commit required. Read [gx-local-tracker.md](../gx-local-tracker.md) for the ticket
commands.

1. Read the ticket with `gx tickets show <addr>` — never open its file by path.
2. Do the work the body asks for. Read only the files you need.
3. Write the outcome with `gx tickets section <addr> Result <content|->`. Put the answer there, not
   only in the chat: the person waiting reads `## Result`.
4. Report with `gx tickets set <addr> --iteration-status finished --commitless true`. A one-off never
   lands a commit; without `--commitless true` the run looks stalled.

If only a person can answer, follow the tracker's announce-and-stop rule: write `## Needs Answer`
and `## Handoff`, report `--iteration-status needs-answer`, and exit. Never call an interactive
prompt.

Finish with zero background shells.

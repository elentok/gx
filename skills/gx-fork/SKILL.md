---
name: gx-fork
description: >-
  Fork topics out of this chat or Design session into new chats or Designs. Writes one brief per topic,
  asks for approval on one card per fork, saves each approved fork as a draft in this workspace,
  prints one link per draft and keeps working on the kept topics. Run
  `/gx-fork for A and B to new chats and continue with C`. Manual.
disable-model-invocation: true
---

# gx Fork

`/gx-fork <what to fork>` moves some topics of this session into new drafts and keeps working on the
rest. It touches no code and stops nothing: the source session goes on with the kept topics. Start runs
later, on the draft page.

Read [gx-unslop](../gx-unslop/SKILL.md) before writing a brief, and
[gx-grilling](../gx-grilling/SKILL.md) for the question-card format.

## Who may fork

Read this session's own row: `GX_AGENT_SESSION` picks the row in `gx agents list --json` (a JSON array;
match on `id`). Refuse in one line and create nothing when:

- `GX_AGENT_SESSION` is unset, or no row matches it;
- `queue` is true (a Queue agent);
- `fix_of` is set (a Fix agent);
- `flow` is set and `design` is false (a Triage or another stepped session);
- `ticket` is set and `design` is false (a one-off ticket session).

A Design source is a row with `design: true`; its map address is `flow_key` (`<project>:<epic>`). Read
project, `dir`, harness, name, model and effort from the same row.

## Steps

1. **Settle the topics.** Take them from the command. If there are more than 5, ask the person to
   group them or pick 5 before any card. The command may name the kind (chat or Design) and the folder.
2. **Propose** a kind per topic, a short name per topic and a folder per chat fork. The folder is the
   source's folder by default, the trunk checkout when the source is a Design session, and a new
   worktree from trunk only when the command asks.
3. **Write every brief** by [gx-handoff](../gx-handoff.md), before any card. Sibling briefs name each
   other by topic and chat name, never a link.
4. **Run `gx agent actionable`** so the session shows Actionable while the cards wait.
5. **One yes-no approval card per fork** in the `gx-grilling` format, with a note. Show the topic, the
   kind, the folder and the first lines of the brief. A note instead of Yes means redraft that fork,
   then ask again with a new card. No draft exists before its card is approved, and a refused fork gets
   none. The cards are the last thing in the turn: no tool call, no text after them.
6. **Save each approved fork**, in order.
   - First fix the **Handled elsewhere** line in the sibling briefs of refused forks so it names the
     source, not a fork that never exists, and tell the person.
   - Write each brief to a scratch file and pipe it on stdin, never as a shell argument.
   - A chat: `gx agents drafts create --project <project> --brief - --name <name> --folder source|trunk|worktree < <brief file>`,
     plus `--harness`, `--model` and `--effort` when they differ from the source's.
   - A Design: `gx designs drafts create --project <project> --slug <slug> --idea - < <brief file>`.
   - Both print the new draft id.
   - A failed save is reported and the rest go on; its topic stays with the source. After a failed
     save, name each saved draft whose **Handled elsewhere** line still names the failed fork, and
     give the line to change on its draft page.
7. **Design source only**: after each draft saves, add one line to the map's `## Forked out`:
   `gx tickets section <project:epic> "Forked out" --append -`, with topic, kind and the new session's
   name, no link. A failed save adds none. Then follow the `## Forked out` rule of
   [gx-wayfinder](../gx-wayfinder/SKILL.md) for each saved fork: add the Out of scope line, close the
   topic's open ticket, remove it from Not yet specified and trim the Destination. A chat source gets
   no section and no trace.
8. **Print one line per saved draft with its link.** Get the web base URL from
   `gx server web-url --json` (its `base_url` field, never `url`), then add `/#/chat-drafts/<id>` for a
   chat draft or `/#/drafts/<id>` for a Design draft. With no web UI, print the draft id.
9. **Keep working only on the kept topics.** The next turn touches no forked topic.

## Failure handling

- **Failed save**: report it and go on with the rest. Nothing is left behind for that fork.
- **Brief over 32 KiB**: shorten it before the call. Never let the call reject it.
- **Start failures** happen later, on the draft page, so this skill never sees them. The draft page
  shows the error and names any leftover.
- A failed chat Start is retried with Start on the same draft.
- A Design Start refused before the map exists keeps the draft; fix the slug there.
- A Design Start that fails after the map exists is retried from the new Design page.
- There is no retry command, and this skill deletes nothing.

## The Design fork brief

The first line of a Design fork brief is exactly this sentence:

`Forked from a chat or Design session: the decisions below are settled.`

[gx-handoff](../gx-handoff.md) holds the section order for chat and Design fork briefs; a skills test
fails when the copies of this line differ.

Use the `GLOSSARY.md` words: Fork, Design, Design session, Workspace.

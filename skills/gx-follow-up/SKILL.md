---
name: gx-follow-up
description: >-
  File a follow-up (a bug or an idea for a person to plan) into a project's `follow-ups` epic:
  choose the project, dedupe against existing follow-ups, fill the sections, file it. Use when the
  user says "create a follow-up", "file a follow-up" or "add a follow-up", or when another skill
  hands over a finding to file.
disable-model-invocation: false
---

# gx Follow-up

The one route for filing a follow-up, for a person and for every skill. The format (sections,
`category`, `draft` + `research`, routing) lives in [gx-local-tracker.md](../gx-local-tracker.md)
"Follow-ups"; read it there, don't restate it here. This skill owns the procedure.

Change files with a file edit (Edit) or file write (Write) only, never `sed -i`, `cat >`, `cat >>` or a heredoc. Write the file first, then use it (for section text, write a file and pipe it with `gx tickets section ... -`). Run one command per call, because a blocked call runs nothing.

## Inputs

A person runs `/gx-follow-up` or asks in passing; the finding comes from the conversation. A calling
skill passes, in prose:

- **the finding** — what is wrong or missing, the evidence, a suggested fix — and its **Source**
  details (epic, ticket addresses, session ids, the calling skill);
- optionally a **target project** — skip project choice;
- optionally **`unattended`** — nobody can answer a question (ralph-loop, a server-launched run);
- optionally a **match** — `match: <addr>` or `match: none`, from a caller that already deduped;
- optionally a **category** — `bug` or `idea`.

**Unattended, it never asks.** Each rule below that would ask names its unattended default instead.

## 1. Choose the project

First match wins:

1. The user or caller named the project.
2. The finding is about gx tooling (skills, ralph-loop, the server, tickets, the TUI) → `gx`. Decide
   with the tracker's routing test: would this still exist if the epic you were working in had never
   been authored? If yes, it's tooling.
3. The cwd is the gx repo → `gx`.
4. Otherwise → the cwd's project.

Ask only when the routing test is unsure, or when the cwd is not a registered project (`gx tickets
root` fails) and no project was named. Ask with AskUserQuestion on Claude, in plain text on Codex.
Unattended: the routing test decides; an unregistered cwd with no target → `gx`.

## 2. Dedupe

A passed match skips the search, not the outcomes below: run `gx tickets show <addr>` on it to learn
its state (open, archived `fixed`, or other archived) before picking an outcome. `match: none` goes
straight to step 3.

Otherwise the candidates are the tickets of `<project>:follow-ups`, `<project>:follow-ups-archived`
and, when the project has an epic named `test-health`, the tickets of `<project>:test-health`. Find
the follow-ups ones with
`gx tickets follow-up search --project <project> <words...>`: it matches any of the words in a
title or `## Problem` (a quoted phrase counts as separate words) and prints address, status and title per hit, plus the close reason for an archived
hit. Open hits all print; archived hits print newest first, at most 10, and a last line counts the rest (`--all` prints them). No hit prints `no matches`. Never print the whole listings. When the finding names a failing test, search the candidates
for that test name first. Judge each hit yourself, by title and `## Problem`: the same problem, not the same area.

`follow-up search` does not read `test-health`. Search it with two reads and no new verb:

1. `gx tickets show <project>:test-health` prints its tickets with titles; most titles hold the
   failing test or flow name.
2. When the test name is not in a title, `rg -l "<TestName or flow number>"` over the `test-health/issues`
   folder under the path `gx tickets root` prints finds ticket bodies (several tickets name two
   tests). Example: `rg -l "TestDeadMany" "$(gx tickets root)/test-health/issues"`.

Then `gx tickets show <addr>` on each hit and judge it like a follow-ups hit. Check the exact test
name first, then the file, then the error text. A `test-health` match needs no question, attended or
not.

- **No match** → file it (step 3).
- **An open or draft ticket** (in `follow-ups` or `test-health`) → file nothing. Append a dated "seen again" entry naming the new source to its
  `## Evidence` with `gx tickets section --append <addr> Evidence -` (entry on stdin; flags come
  first). It keeps the existing text. Empty content is refused.
- **An archived `fixed` ticket, or a `done` ticket in `test-health`** → file a regression: a new
  follow-up whose `## Source` and `## Evidence` cite that address.
- **Any other archived ticket** (`obsolete`, `duplicate`, `not-worth-it`, `promoted`), **or a closed
  or cancelled one in `test-health`** → drop it. File nothing; report the address and its reason.

## 3. Fill and file

- **Category**: required by the verb. Use the caller's, else infer `bug` (something behaves wrongly)
  or `idea` (something is missing or painful). Ask only when truly unclear, and never when a target
  project was passed or unattended: then pick `bug` when there is concrete evidence of wrong
  behaviour, else `idea`. Put it in the body's frontmatter as `category: bug|idea`, or pass
  `--category bug|idea`.
- **Sections**: fill Source, Problem, Evidence, Suggested fix and (only when there are any) Open
  questions from the conversation or the caller's input. No preview.
- **A flake, hang or timeout** → also follow [gx-flake-tickets](../gx-flake-tickets.md) for the
  Evidence and the Suggested fix. A new flake goes to `<project>:follow-ups` first, after the dedupe
  above. `gx-triage-follow-ups` then moves it on, and the survivors end up in `test-health`. Never
  file a new flake straight into `test-health` and never invent a different epic.
- **Several problems** → one follow-up each, each through step 2. A caller's group of findings that
  name the same fix counts as one problem.
- File each with `gx tickets follow-up add <project>:follow-ups --body -`. Write the body to a
  scratchpad file with Write and pipe it in, never a heredoc (see "Write a body once" in
  [gx-local-tracker.md](../gx-local-tracker.md)). The body is shaped as:

  ```
  ---
  category: bug
  ---
  # <title>

  ## Source
  ...
  ```

  Put the exact failing test or check name in the title. The verb refuses
  (`duplicate-follow-up`) a title an open follow-up already has and prints that address: treat it as
  the "open ticket" outcome above and append "seen again" there. `--force` files it anyway; use it
  only when the problem is genuinely different. If the verb refuses otherwise, fix
  the gap its code names and retry. Never fall back to a plain `gx tickets add`.

## 4. Report

Print every address: each new follow-up the verb printed, each ticket that got a "seen again" entry,
and each archived address a finding was dropped against.

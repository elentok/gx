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

Otherwise the candidates are the tickets of `<project>:follow-ups` and
`<project>:follow-ups-archived` only. List an epic's tickets with `gx tickets show <project:epic>`.
When the finding names a failing test, search the candidates for that test name first. Otherwise
judge a match yourself, by title and `## Problem`: the same problem, not the same area.

- **No match** → file it (step 3).
- **An open ticket** → file nothing. Append a dated "seen again" entry naming the new source to its
  `## Evidence` (`gx tickets section <addr> Evidence -`, content = existing text plus the entry).
- **An archived `fixed` ticket** → file a regression: a new follow-up whose `## Source` and
  `## Evidence` cite the archived address.
- **Any other archived ticket** (`obsolete`, `duplicate`, `not-worth-it`) → drop it. File nothing;
  report the archived address and its reason.

## 3. Fill and file

- **Category**: required by the verb. Use the caller's, else infer `bug` (something behaves wrongly)
  or `idea` (something is missing or painful). Ask only when truly unclear, and never when a target
  project was passed or unattended: then pick `bug` when there is concrete evidence of wrong
  behaviour, else `idea`. Put it in the body's frontmatter as `category: bug|idea`, or pass
  `--category bug|idea`.
- **Sections**: fill Source, Problem, Evidence, Suggested fix and (only when there are any) Open
  questions from the conversation or the caller's input. No preview.
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

  If the verb refuses, fix
  the gap its code names and retry. Never fall back to a plain `gx tickets add`.

## 4. Report

Print every address: each new follow-up the verb printed, each ticket that got a "seen again" entry,
and each archived address a finding was dropped against.

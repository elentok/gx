---
name: gx-triage-follow-ups
description: >-
  Empty a project's `follow-ups` epic: verify each item against the code, dedupe, route every item
  to a ready epic or an area map epic, and close what is fixed or obsolete. Manual; one approval of
  one table. Never grills.
disable-model-invocation: true
---

# gx Resolve Follow-ups

Empty `<project>:follow-ups` until it holds no tickets. The project is the current one, or the one
named by `--project <name>`. Read [gx-local-tracker.md](../gx-local-tracker.md) for the ticket
commands, the Follow-ups format and the Map epics section; don't restate them here.

You are the main session. Subagents verify; you plan; the user approves once; then you write. This
skill never grills: a design question is routed to a map epic, not asked.

## 0. List the items

`gx tickets show <project>:follow-ups` lists the epic's tickets. A follow-up without a `category`
predates the format: infer `bug` or `idea` from its body, and read its sections as they are.

## 1. Verify

Send one message with one `Agent` call per item (`general-purpose`, `model: "sonnet"`, foreground),
about 8 in parallel; start the next batch as each finishes. Each gets the ticket address and this
brief:

"Read the ticket with `gx tickets show`. Read the code and logs it cites and `git log` since it was
filed. Do not reproduce it and do not run tests. Report: (1) verdict `relevant`, `fixed` or
`obsolete`, with evidence (a commit, a file and line, a log line); (2) the files a fix would touch;
(3) 0–N blocking questions, each tagged `quick` (one-line answer from the user) or `design` (a
decision with trade-offs); (4) a suggested destination: ready epic as `implement`, or map epic as
`grilling` or `research`. If you cannot decide, say `relevant`. Under 300 words."

An undecided verdict counts as **relevant**.

## 2. Dedupe

Compare verdicts only; don't re-read tickets. For items describing the same problem, keep the
**oldest**. Merge each newer one's Source and Evidence into the kept ticket's `## Evidence` as a
"seen again" entry with the date (`gx tickets section <addr> Evidence -`, content = the existing
text plus the entry). Close the newer one with `gx tickets close <addr> --reason duplicate --note
<kept-address>`.

Hold these writes for step 5; step 3 shows them in the table.

## 3. Plan

Print **one table** and write nothing. One row per item: address, title, verdict, action, and the
suggested destination. Actions:

- **close (reason)** — `fixed` or `obsolete`, with the evidence.
- **merge into X** — a duplicate of X.
- **ready epic** — `follow-ups-<date>`, as `implement`.
- **map epic Y** — as `grilling` or `research`, with the area slug Y.

Routing, first match wins:

1. Any `design` question, or more than 2 questions → map epic, as `grilling`.
2. Needs investigation but no decision → map epic, as `research`.
3. Otherwise → ready epic, as `implement`.

Group map-epic items by **area of code or tool**, not bug vs idea: 1–8 items per epic, a singleton is
fine, plain descriptive slug. For the ready epic, list which items touch overlapping files; they get
chained by `blocked_by` in step 5.

## 4. Approve once

Show the table and let the user edit it or approve it. Ask every `quick` question of the items going
to the ready epic in this same step, in one message. Write each answer into that ticket's body
(`gx tickets section <addr> "Open questions" -`, or a `## Decisions` section). Never ask a `design`
question: it travels with its ticket into the map epic.

Make each question answerable on its own; the user should not have to match numbers back to the
table. Group the questions by ticket. For each ticket, give its address and title, one or two lines
on what the ticket is about and what the fix changes, then each question with the options and a
default.

Do not write anything before approval.

## 5. Execute

Run the approved table with the tracker's verbs: `gx tickets epic add`, `close`, `move`, `set`. Use
`gx tickets move --dry-run` first if a move might refuse.

**Closes** — `gx tickets close <addr> --reason fixed|obsolete|duplicate|not-worth-it --note ...`.

**Ready epic** — one per run, `follow-ups-<date>`:

- `gx tickets epic add <project>:follow-ups-<date> --kind loop --status open`. Do not queue it.
- `gx tickets move <addr> follow-ups-<date>`, then `gx tickets set <new-addr> --type implement
  --status open`. Only `implement` tickets go here: the queue does not run `research` tickets.
- Chain tickets whose expected files overlap: `gx tickets set <new-addr> --blocked-by
  <earlier-new-id>`.

**A move renumbers the ticket.** `move` prints the new address; the old one no longer exists. Capture
it and use it for every later `set`, `section` and `--blocked-by`, never the pre-move address.

**Area map epic** — one per area:

- `gx tickets epic add <project>:<slug> --kind map --status open --body -`, the body opening with a
  frontmatter block carrying `destination_unconfirmed: true`, then: a **Destination** (provisional:
  "decide these items, then a spec `docs/specs/<slug>.md`"), **Notes** ("Triaged from follow-ups on
  `<date>`" and the source addresses), and empty **Decisions so far** and **Not yet specified**.
- A design item: move it in, then `gx tickets set <new-addr> --type grilling --status open`, and add
  a `## Question` built from its Open questions (`gx tickets section <new-addr> Question -`). It
  keeps its follow-up sections.
- A research-only item: move it in, then `gx tickets set <new-addr> --type research --status open`.

`gx tickets validate` one ticket of each new epic.

## 6. Done

**Done** = `<project>:follow-ups` holds no tickets. Run `gx tickets show <project>:follow-ups` and
report what is left, if anything; a ticket the user held back stays and the run isn't done.

For follow-ups already `done` or `cancelled` before this run, `gx tickets close --sweep [project]`
moves them into `follow-ups-archived`; run it first when the epic has such tickets.

Finish with a short report: counts per action, the ready epic's address, and each map epic's
address. Finish with zero background shells.

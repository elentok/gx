---
name: gx-evolve
description: >-
  Retro of one finished epic: read the cheap flag table, deep-read only flagged tickets, triage the
  findings with a higher-tier consultant, and file what survives as follow-ups. Launched by the gx
  server for `type: evolve` one-offs. Never edits code.
disable-model-invocation: true
---

# gx Evolve

A retro of **one epic**. Look at how the tooling and process behaved (gx, ralph-loop, skills,
prompts, agent docs), and file the findings worth fixing as follow-ups. It is not a code review:
project code quality stays with the epic's `code-review` ticket.

This skill never edits code and never commits. Its outputs are follow-up tickets and its own
ticket's body. Read [gx-local-tracker.md](../gx-local-tracker.md) for the ticket commands and the
Follow-ups format (the one place it lives); don't restate it here.

## 1. Validate and resolve the target

- `gx tickets validate <addr>` on your own ticket. If it fails, stop and fix the frontmatter first.
- `gx tickets show <addr>` — never open the file by path. The body reads `Retro of epic
  <project>:<epic>`; that is the **target epic**. One retro looks at one epic only.

## 2. Run the inputs verb yourself

Run `gx evolve inputs <epic>` directly in this session. It prints the per-ticket flag table and the
directory holding one text digest per session. Never read transcripts or raw JSONL in the main
session; only subagents read digests, and only for tickets you pick in step 3.

A session reported `missing` is a gap, not a clean run — say so in the finding or the table.

## 3. Deep-read the flagged tickets

Pick tickets from the flag table. A ticket with no flags is **never** read.

- **Always**: the epic's `code-review` ticket and its fix tickets, and, for a conflict-hit ticket,
  its conflict-resolution session.
- **Then by severity** (needs-repair/park > conflict > overrun > the rest) until you reach the cap:
  `gx config show` prints the effective config as JSON; read `.["evolve.deep-read-cap"]`.
- Tickets flagged past the cap are not read; they appear as rows in step 4.

Send **one message** with one `Agent` call per picked ticket (`general-purpose`, `model: "sonnet"`,
foreground). Each gets the ticket id, its flag row, and the digest paths for its sessions. Brief:
"Read these digests. Report what in the tooling or process cost time, tokens or correctness —
prompts, skills, agent docs, gx/ralph-loop behaviour — each with a quoted line or ticket id as
evidence. Ignore project-code quality. Under 500 words."

## 4. Join by symptom

Group the subagent reports by symptom across tickets: one cause seen in five tickets is one finding
listing all five. Keep the evidence verbatim. Add a plain table row (ticket, flags, "not read: past
cap") for every flagged ticket you did not deep-read, so nothing flagged disappears.

## 5. Consultant triage

Start **one** fresh-context, read-only subagent with `model: "opus"` (as in gx-code-review). Brief it
with:

- The joined findings and the table rows from step 4.
- Titles and `## Problem` lines (read with `gx tickets show`) of every ticket in each **target
  follow-ups epic** (`gx:follow-ups`, and `<project>:follow-ups` when the project is not gx), of
  each one's archive (`follow-ups-archived`), and of every open ticket carrying a `category`,
  including follow-ups already moved out of the follow-ups epic.

Ask it to dedupe, apply the bar below, rate impact, and approve or reject each finding with a
one-line reason. It only advises; it must not edit anything.

**The bar** — a finding is approved only when it has all of:

1. A tooling or process cause (gx, ralph-loop, skills, prompts, agent docs), not project code.
2. Evidence: log lines, ticket ids, commits.
3. A suggested fix.
4. A target (`gx:follow-ups` for tooling; `<project>:follow-ups` for that project's own agent docs)
   and a `category` (`bug` or `idea`).

Zero approved findings is a valid result. Don't pad.

## 6. File the approved findings

For each approved finding, check the consultant's match against the follow-ups it was shown:

- **No match** → file it: `gx tickets follow-up add <project:epic> --body -`, in the format from the
  tracker's Follow-ups section. `## Source` names this epic, the ticket addresses, the session ids
  and `gx-evolve`.
- **Matches an open ticket carrying a `category`** → don't file a duplicate. Append a "seen again in
  `<epic>`" entry, with the date and the new evidence, to its `## Evidence`
  (`gx tickets section <addr> Evidence -`, content = existing text plus the entry).
- **Matches an archived `fixed` ticket** → file it as a regression: a new follow-up whose `##
  Source` and `## Evidence` cite the archived address.
- **Matches any other archived ticket** (`obsolete`, `duplicate`, `not-worth-it`) → reject it, and
  record the archived address as the reason.

## 7. Record in your own ticket

Append to the ticket body with `gx tickets section <addr> <heading> -`; don't replace its existing
sections. No repo file.

- **Result** — never empty. State how many findings were filed, how many were seen-again, and
  which were rejected; for a clean epic, say so and why (e.g. "no ticket flagged").
- **Flag table** — the step 2 table.
- **Raw findings** — the subagent reports, verbatim, per ticket.
- **Consultant triage** — verbatim from step 5.
- **Final disposition** — per finding: which follow-up it became, which it was appended to, or why
  it was rejected.

## 8. Close out

`gx tickets set <addr> --iteration-status finished --commitless true`. This skill never commits, so
`--commitless true` is always correct, including when nothing was filed. Never write `--status
done`; landing status is gx's alone to set.

If only a person can answer, follow the tracker's announce-and-stop rule: write `## Needs Answer`
and `## Handoff`, report `--iteration-status needs-answer`, and exit. Never call an interactive
prompt. Finish with zero background shells.

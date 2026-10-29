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
- **Then by severity** (paused-repair/paused-answer > conflict > overrun > the rest) until you reach the cap:
  `gx config show` prints the effective config as JSON; read `.["evolve.deep-read-cap"]`.
- Tickets flagged past the cap are not read; they appear as rows in step 4.

Send **one message** with one `Agent` call per picked ticket (`general-purpose`, model from `gx agent model worker`,
foreground). Each gets the ticket id, its flag row, and the digest paths for its sessions. Brief:
"Read these digests. Report what in the tooling or process cost time, tokens or correctness —
prompts, skills, agent docs, gx/ralph-loop behaviour — each with a quoted line or ticket id as
evidence. Also list loud or hand-built shell commands: one that printed far more output than the
agent used, or a long pipeline built by hand again and again (`| rg`, `| tail`, `2>&1 | ...`).
Quote the command and say what the agent was looking for. Ignore project-code quality. Under 500
words."

A loud or hand-built command seen in more than one session is a finding: its suggested fix is a
`gx brief` helper that prints only what the agent was looking for (category `idea`).

### Verify parts

A verify part is a ticket with `expected_context_window`, so the table's `context-overrun` flag
already covers it: actual tokens minus the 50K session baseline pass twice the budget. That flag is
the only automatic check; don't add another. Compare every verify part yourself, flagged or not,
using only the table and the ticket body (`gx tickets show`), never its session:

- Its actual tokens (minus the baseline) against its budget.
- Its budget against the sum of the per-story estimates in its body, plus the 15K base. They should
  match; if not, the part was written wrong.

A cap stop is a `verify-cap-stopped` event in the run log. A part that has one had a budget that was
too low, whatever the numbers say: treat it as off even if the actual tokens look fine.

When a part is off, make one finding per symptom, not per part (step 4): the estimate rule in
[gx-to-tickets](../gx-to-tickets/SKILL.md) needs adjusting. Evidence: the part's ticket id, budget,
actual, story estimates and any cap stop. Target `gx:follow-ups`, category `idea`. It goes through
the usual triage (step 5) and [gx-follow-up](../gx-follow-up/SKILL.md) (step 6). Parts within budget
produce no finding.

### Check rows

After the ticket table, `gx evolve inputs` can print `slower:` and `flake:` rows (`slower` and
`flakes` in the JSON). Each row is a finding; don't deep-read a session for it.

- **Slower**: a check kind whose latest full verify run is at least 20% and 30 s above the median of
  the last 5 full runs before the epic's first claim. Name the check kind in the finding. Category
  `idea`.
- **Flake**: a test that failed and then passed on the same tree. Name the test in the finding, with
  the failing run id, the passing run and the tree id. Category `bug`.

Both go through triage (step 5) and are filed through [gx-follow-up](../gx-follow-up/SKILL.md) (step
6). Its test-name dedupe finds a follow-up that the failing-tests rule already filed for the flake.

## 4. Join by symptom

Group the subagent reports by symptom across tickets: one cause seen in five tickets is one finding
listing all five. Keep the evidence verbatim. Add a plain table row (ticket, flags, "not read: past
cap") for every flagged ticket you did not deep-read, so nothing flagged disappears.

## 5. Consultant triage

Start **one** fresh-context, read-only subagent on the model from `gx agent model strongest` (as in gx-code-review). Brief it
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

Hand each approved finding to the [gx-follow-up](../gx-follow-up/SKILL.md) skill, **unattended**,
with:

- the finding, its evidence and suggested fix; Source names this epic, the ticket addresses, the
  session ids and `gx-evolve`;
- the consultant's target as the target project, and its `category`;
- the consultant's match: `match: <addr>`, or `match: none`.

The skill decides the outcome (file, seen again, regression or drop) and prints the addresses step
7 records.

## 7. Record in your own ticket

Append to the ticket body with `gx tickets section <addr> <heading> -`; don't replace its existing
sections. No repo file. Write the text to a scratchpad file with Write and pipe it in; never a
heredoc, never edit the ticket file by path.

- **Result** — never empty. It is the one message a person reads, so it has a fixed, short shape
  (detail belongs in the other sections):
  1. A headline with counts: `3 new · 5 seen again · 6 not filed · 10 flagged unread`.
  2. Headed groups, **New follow-ups**, **Seen again**, **Not filed**, **Coverage**, one item per
     line: the full address, then a short title. Never a bare `/02`; omit empty groups.
  3. **Not filed** is the findings the triage did not file (never "Rejected"), grouped by reason
     (out of scope / weak evidence / contradicts design), one short reason per item. No triage
     ids (J4…); those belong in **Final disposition**.
  4. **Coverage** is last and holds the run stats (tickets, deep-read cap, tickets read). Make
     flagged-but-unread tickets stand out: it means the review was partial.
  5. End with one **Next** line, always present, e.g. "triage gx:follow-ups/23–25", "raise the
     deep-read cap and re-run", or "nothing to do" when nothing was filed.

  For a clean epic, say so and why (e.g. "no ticket flagged").
- **Flag table** — the step 2 table.
- **Raw findings** — the subagent reports, verbatim, per ticket.
- **Consultant triage** — verbatim from step 5.
- **Final disposition** — per finding: which follow-up it became, which it was appended to, which
  archived ticket it was dropped against, or why it was not filed (with its triage id).

## 8. Close out

`gx tickets set <addr> --iteration-status finished --commitless true`. This skill never commits, so
`--commitless true` is always correct, including when nothing was filed. Never write `--status
done`; merge status is gx's alone to set.

If only a person can answer, follow the tracker's announce-and-stop rule
([gx-local-tracker.md](../gx-local-tracker.md)): run `gx tickets ask` and end your turn. Never call
an interactive prompt. Finish with zero background shells.

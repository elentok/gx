---
name: gx-implement
description:
  Implement a piece of work against gx's local ticket tracker, unattended under ralph-loop. Use to
  claim and implement the next ticket from a directory of tickets.
disable-model-invocation: true
---

# gx Implement

Implement a ticket from gx's local markdown tracker (see [gx-local-tracker.md](../gx-local-tracker.md)).

Use DRY, KISS & YAGNI principles.

If given a directory of tickets rather than a single ticket, work the **frontier**: the
lowest-numbered ticket that is unblocked (every ticket in its `blocked_by` is `done`) and unclaimed.
Implement exactly that one ticket, then stop — do not continue on to the next ticket in the same
run, even if it's now unblocked.

Under ralph-loop, a frontier ticket with `type: code-review` never reaches this skill: the
scheduler launches the iteration with `/gx-code-review` directly instead of `/gx-implement`
(see `ralphloop.codeReviewSkill`), since [gx-code-review](../gx-code-review/SKILL.md) sets
`disable-model-invocation: true` and can only be entered via an explicit `/command`, not a
Skill-tool call from inside a running session. If you're reading this document against a
`type: code-review` ticket anyway (e.g. invoked by hand), stop — don't implement it and don't
try to invoke gx-code-review yourself via the Skill tool; ask the user to run
`/gx-code-review <addr>` instead.

Before starting work on the ticket you're about to claim, run `gx tickets validate <addr>` on it. If
it fails, stop and fix the ticket's frontmatter (or hand it back) before doing anything else — do
not begin implementation against a ticket that fails validation.

## Plan the reading before you read

A ticket should fit in the smart-zone section of the context window (~130K tokens). What overflows
it is reading, not writing.

Before the first edit:

1. Read the ticket with `gx tickets show <addr>` — never open its file by path. **Only** the ticket. Go to the spec/map for a named section, never in full - one
   naive read of a spec is most of a window.
2. Write the seams down ([gx-tdd](../gx-tdd/SKILL.md)) and, from them, **list the files you will
   touch**. Skipping this turns a plan into a sequence of rediscoveries.
3. Read exactly those files, once each.

If the ticket has an `expected_context_window` field, check your file list against it before
editing - budgets come from the ticket's prose and routinely undercount (one 40K/two-file estimate
became 19 files, ~500K). If your list is materially larger, **say so in one line and carry on**; the
user may prefer a fork.

Then, while working:

- **Read a file once.** After an `Edit` succeeds the file in context is current - never re-read to
  verify. Repeat reads are the largest leak. The one exception: re-read the exact range (`offset`/
  `limit`) after an `Edit` fails with `String to replace not found`.
- **Never edit code you have not read, and copy indentation exactly from the Read.** Don't guess
  tabs or spaces.
- **Never read source through a shell command** (`cat`, `sed -n`, `head`, `tail`). Same tokens as a
  direct file read, no benefit.
- **Prefer the project's LSP tooling to grepping**. A references lookup answers in 3 lines what a
  grep returns kilobytes for.
- **Delegate any survey wider than ~3 files** to a subagent - you keep the conclusion, not the file
  dumps.
- **Edit code with `Edit`/`Write`, never a shell script** (python/sed heredocs, `cat >`). A
  heredoc that hangs is invisible; an `Edit` that fails says why.
- **Never run a command that can read stdin**: give `rg`/`grep` an explicit path (`rg foo .`),
  never a bare `cat`. Stdin never closes here, so it hangs forever.
- **Finish with zero background shells.** If a call was moved to the background, read its output,
  then stop it (TaskStop) before reporting finished. `TaskStop` is a deferred tool: if it isn't
  callable, load it first with `ToolSearch` query `select:TaskStop`. ralph-loop will not land a
  ticket while one is open.
- **One check per Bash call.** A chained call (`rg ...; go test; gofmt -l`) can return the output
  of one segment only. Run each check in its own call.
- **Never report a result you did not see.** If a check's output is missing or cut off, re-run it.
  Do not claim "no remaining imports" or "gofmt is clean" from silence.
- **On a test failure, re-run it with `rtk proxy go test ...`.** `rtk` truncates multi-line
  assertion text and server logs. If a shell read of a file ends in "(exporter truncated)"
  or has lost its indentation, use the `Read` tool instead.

## Forking when the ticket outgrows budget

Check the live budget at every natural checkpoint — after a subagent survey returns, after each file
read above, and before starting a new seam. To check: run `gx agent context-window`, which prints
the current session's context token count without either agent needing to know the other's
transcript format. Trigger a fork at **90K** — that leaves headroom under the 130K smart-zone
budget for growth the check can't see (mid-turn, between checkpoints). If the check itself fails
(the command errors or can't identify the session), treat that as over-budget — fail toward
forking, not past it.

Also fork, independent of token count, on a **kind-mismatch**: if mid-implementation you discover
the ticket actually needs new plumbing/infra the original ticket didn't scope for (not just an
extension of existing logic), that's gx-to-tickets' plumbing/feature split showing up late, not a
budget problem — fork it the same way regardless of how much budget is left.

When either trigger fires:

1. **Finish the current thread to green.** Get to the nearest point where tests pass and the code
   compiles/typechecks — don't fork off a broken half-edit. If nothing's been coded yet (the
   trigger fired during exploration/design), there's nothing to make green; skip to step 2 and carry
   the design reasoning forward as notes instead of a diff.
2. **Commit.**
3. **Create the follow-up ticket(s)**, following [gx-local-tracker.md](../gx-local-tracker.md)'s
   mid-flight-fork numbering and `parent` conventions, and gx-to-tickets' estimation method.
   Allocate each new ticket's ID with `gx tickets add <epic> --parent <original-id> --slug <descriptive-slug>`
   — it atomically picks the next free ID (safe against another
   parallel iteration doing the same fork at the same time), writes the stub straight to
   `<id>-<slug>.md` (`--slug` is required, so there's no separate rename step to remember), and
   writes the child's `parent`, which is the only edge a fork produces: give the new ticket no
   `blocked_by` naming the original, and record nothing about it on the original. The stub lands
   `draft` and is printed by address; fill in its body with `gx tickets section <new-addr> <heading>
   <content|->` per section, then `gx tickets set <new-addr> --status open` to hand it over — this
   is the one `--status` write an iteration agent ever makes, since a fresh fork must be handed
   over and nothing else can do it. (Or pass `--body -` to `gx tickets add` to write the body and
   open the ticket in one call.) This chain is uncapped — each fork narrows what's left, so it's
   self-limiting. Move any not-yet-finished acceptance criteria off the original ticket onto the new
   one(s). Do this **autonomously** — no pause for user approval; this exists to keep the outer loop
   unattended.
4. **Close the original**, with `gx tickets set <addr> --iteration-status finished` and a body note
   of the token count from the last budget check (e.g. `Tokens used: ~85K`) — so it can be matched
   against the ticket's `expected_context_window` later. (`actual_context_window` itself is
   gx-written at cherry-pick time, not by the agent; so is landing `status: done` — the report can
   only start a landing, never conclude one, gx's own commit count decides that.) Nothing about the
   new tickets is recorded on the original; their own `parent` is the whole edge. If step 1 had
   nothing to commit (design/exploration only, no diff), also pass `--commitless true`:
   `gx tickets set <addr> --iteration-status finished --commitless true`. Without it, ralph-loop's
   stalled-agent detection flags the forked original `needs-repair` instead of adopting it as done.

## When only a person can answer

Follow [gx-local-tracker.md](../gx-local-tracker.md)'s announce-and-stop rule: never call an
interactive prompt. Commit whatever is green first, then run `gx tickets ask` (wording and syntax
are in the tracker) and end your turn. This is not a fork — the same ticket resumes once a person
answers.

## Record deliberate spec deviations

If you deliberately narrow, skip or change something the spec or ticket asked for, record it in a
`## Spec deviations` section of the ticket body (`gx tickets section <addr> "Spec deviations"
<content|->`). One bullet per deviation: what the spec said, what you did instead, and why.
[gx-code-review](../gx-code-review/SKILL.md) collects these so its reviewers don't report a known
narrowing as a bug. Only deliberate decisions belong here — not bugs or unfinished work.

## Comments: fewer, and no numbers in them

A comment quoting a measured value duplicates it, and duplicates drift - stale figures ship.

- **A number goes in a test, not a comment.**
- **Comment the non-obvious decision, not the code.** Why this and not the obvious alternative, what
  broke last time. Anything the identifiers already say is noise.
- **One explanation, one home**. If two declarations need the same rationale, hoist what they share
  and explain it there.

## Other notes

Use [gx-tdd](../gx-tdd/SKILL.md) where possible, at pre-agreed seams. Every declared seam needs a
real assertion; if one cannot be observed, park `needs-answer` naming the seam instead of finishing
(see "When only a person can answer").

Run typechecking regularly and single test files regularly. At the end, test only the packages you
touched plus their direct importers (see `.ai/index.md` "Testing") — not the full suite.

While iterating, always pass `-timeout 60s` and `-run <pattern>` to `go test`, and run one package
per call. Run the whole package once at the end. `server/` is exempt from the 60s limit.

Tests in `server/` take over 120s, the Bash default timeout. Start them with `run_in_background`
up front, then wait for the completion notice. Don't poll with Monitor or Workflow, don't write
`sleep` or `until ...; do sleep` loops, and don't start a second run of the same suite while one is
going.

Before every commit, and before `--iteration-status finished`: run `gofmt -l` on the changed `.go`
files only (e.g. `gofmt -l $(git diff --name-only HEAD -- '*.go')`; add `--cached`/new files as
needed). The output must be empty — fix with `gofmt -w` on those files. Then run `/verify`. Never
run `gofmt -l .`: files on main that are already unformatted are out of scope. No goimports.

Claiming already writes `status: claimed` for you — never write it yourself.

Commit first. Report `finished` only after the commit has succeeded, in a later tool call - never in
the same parallel block as the commit. A rejected commit (a hook, a failed check) next to a
`finished` report leaves the ticket finished with nothing committed.

Once done, run `gx tickets set <addr> --iteration-status finished`. This report can only _start_ a
landing, never conclude one: gx adopts it and decides `status: done` itself from its own commit
count and cherry-pick outcome — never write `--status done` yourself, even paired with
`--iteration-status finished`; landing status is gx's alone to set (the one exception is the
draft→open fork handover in step 3 above). Code review runs separately, batched across the epic,
not per-ticket — do not invoke it from here.

If you conclude this ticket needs **no commit** — e.g. exploration shows the behavior already
exists, or the ticket only needed a fork with no code changes of its own — report finished plus
`commitless: true` in one call: `gx tickets set <addr> --iteration-status finished --commitless
true`, explaining why in the ticket body. Without `commitless: true`, ralph-loop treats a
zero-commit finish as a stalled agent and flags the ticket `needs-repair` for a human to check.

Commit your work to the current branch. Start every commit subject with
`{epic}/{ticket id}: `, substituting the `.scratch/<epic>/` directory name (not the branch name,
which is always namespaced under a literal `ralph-loop/` prefix regardless of epic — see
[gx-local-tracker.md](../gx-local-tracker.md)) and the ticket's frontmatter `id` (for example, if
the epic directory is `.scratch/widget-queue-fixes/`, a commit for ticket `03` starts with
`widget-queue-fixes/03: Add smart-zone observability`).

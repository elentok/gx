# gx-follow-up: one route for filing a follow-up

## Problem Statement

Today three skills file follow-ups, and each one carries its own copy of the procedure:
`gx-investigate`, `gx-to-tickets` and `gx-evolve`. Each copy restates part of the routing rule, the
call to `gx tickets follow-up add`, and the "seen again" handling, with small differences. A person
in a live session who wants to note a bug or an idea for later has no skill at all: they must
remember the format in `gx-local-tracker.md`, pick the right project, check for a duplicate by hand,
and type the `follow-up add` call.

Some real findings are also lost. When the `gx-code-review` consultant marks a finding "deferred",
nothing files it. The finding lives only in the code-review ticket's "Final disposition", where
`gx-resolve-follow-ups` and `gx evolve export` never look.

Dedupe is also blocked today: `gx tickets show <project>:follow-ups` refuses an epic address
("malformed-address"), so no skill can list a follow-ups epic through the tracker's own verbs.
`gx-resolve-follow-ups` and `gx-design` already rely on that call and hit the same error
(`gx:follow-ups/30`).

## Solution

A new bundled skill, `gx-follow-up`, is the single route every person and every skill uses to file
a follow-up. A person runs `/gx-follow-up` (or says "create / file / add a follow-up"). A skill
calls it with what it found. The skill:

1. Chooses the target project (or takes the one the caller passed).
2. Dedupes against that project's follow-ups: the `follow-ups` epic, its archive, and every open
   ticket carrying a `category`. A caller that already judged the match passes it in, and the
   search is skipped. The match decides the outcome: file, "seen again", regression, or drop.
3. When it files, infers the `category` (bug or idea) and fills the body sections from the
   conversation, with no preview, and files with `gx tickets follow-up add`.
4. Prints the address of every new or matched follow-up.

It asks a question only when the project or the category is unclear, and never in an unattended
call. On Codex, which has no questions UI, it asks in plain text.

The format stays in `gx-local-tracker.md` "Follow-ups". The new skill owns the procedure, including
the dedupe rules. The four callers (`gx-investigate`, `gx-to-tickets`, `gx-evolve`,
`gx-code-review`) drop their own copies and call the skill. `gx-code-review` also starts filing its
consultant's deferred findings as follow-ups.

`gx tickets show` learns to accept an epic address, so the skill can list the candidates.

## User Stories

### A person filing a follow-up

1. As a gx user in a live session, I want to type `/gx-follow-up`, so that I can note a bug or an
   idea for later without leaving my current task.
2. As a gx user, I want the model to reach for the skill when I say "file a follow-up", "create a
   follow-up" or "add a follow-up", so that I don't have to remember the skill name.
3. As a gx user, I want to name the target project myself ("file a follow-up in gx"), so that my
   choice always wins over any guess.
4. As a gx user working in some other project, I want a finding about gx tooling (skills,
   ralph-loop, the server, tickets, the TUI) to go to `gx:follow-ups`, so that tooling problems
   don't clutter my project's follow-ups.
5. As a gx user working in the gx repo itself, I want follow-ups to go to `gx:follow-ups` by
   default, so that I don't have to say so each time.
6. As a gx user in another registered project, I want a project-specific finding to go to that
   project's `follow-ups` epic, so that it is triaged with the rest of that project's work.
7. As a gx user, I want the skill to apply the same "would it still exist if the epic had never
   been authored?" test the tracker doc already defines, so that routing is the same whether I or a
   skill files the finding.
8. As a gx user, I want the skill to ask me which project when the routing test is unsure (e.g. a
   bug in my project's code that a gx skill caused), so that a finding never lands in the wrong
   tracker silently.
9. As a gx user in a directory that is not a registered project, I want the skill to ask me which
   project to use when I didn't name one, so that it never guesses a target that `follow-up add`
   would refuse.
10. As a gx user, I want the skill to file without showing me a preview, so that filing costs me
    one sentence, not a review round.
11. As a gx user, I want the skill to infer `bug` or `idea` from what we discussed, so that I am
    asked only when it is truly unclear.
12. As a gx user, I want the skill to fill Source, Problem, Evidence, Suggested fix and (when there
    are any) Open questions from the conversation, so that the follow-up is complete enough for
    `gx-resolve-follow-ups` to verify.
13. As a gx user who describes several independent problems at once, I want one follow-up per
    problem with no question asked, so that each can be resolved on its own.
14. As a gx user, I want the skill to print every new or matched address, so that I can open it,
    cite it, or block a ticket on it.
15. As a Codex user, I want the skill to ask its rare questions in plain text, so that it works
    without a questions UI.

### Dedupe

16. As a gx user, I want the skill to search the target project's `follow-ups` epic, its
    `follow-ups-archived` epic, and every open ticket carrying a `category`, so that a problem
    already filed (even one moved out of the follow-ups epic) is not filed twice.
17. As a gx user, I want a match on an open ticket to get a dated "seen again" entry in its
    `## Evidence`, so that the existing ticket records how often the problem comes back.
18. As a gx user, I want a match on an archived `fixed` ticket filed as a regression (a new
    follow-up whose Source and Evidence cite the archived address), so that a returning bug is not
    lost.
19. As a gx user, I want a match on any other archived ticket (`obsolete`, `duplicate`,
    `not-worth-it`) dropped, with the archived address reported as the reason, so that a decision
    already made is not re-opened.
20. As a gx user, I want the skill to judge a match itself by reading each candidate's title and
    `## Problem`, so that filing one finding needs no subagent.
21. As a calling skill that has already judged the match, I want to pass it in (`match: <addr>` or
    `match: none`), so that the skill skips the search and only applies the outcome.

### Calling skills

22. As a skill author, I want one skill that files follow-ups, so that a change to the procedure is
    made in one place.
23. As a calling skill, I want to pass the target project and an "unattended" flag, so that the
    skill skips project detection and never asks a question.
24. As a calling skill in an unattended call, I want an unclear category to default to `bug` when
    there is concrete evidence of wrong behaviour and `idea` otherwise, so that the call never
    stalls.
25. As `gx-investigate`, I want to hand a tooling diagnosis to the skill with target `gx` and the
    unattended flag, so that I no longer restate the follow-up call, format and dedupe.
26. As `gx-investigate` in unattended mode, I want filing via `gx-follow-up` listed as always
    allowed, so that my repair-only verb grant doesn't block a diagnosis.
27. As `gx-to-tickets`, I want to route "a diagnosis or idea for a person to plan" to the skill, so
    that I never file a follow-up with my own copy of the procedure.
28. As `gx-evolve`, I want to keep my consultant's match, and hand each approved finding plus its
    match to the skill, so that the batch judgment is kept and the outcome rules live in one place.
29. As `gx-code-review`, I want the three verdicts defined as approved = fix ticket in this epic,
    deferred = follow-up ("real, but not worth fixing in this epic"), rejected = dropped, so that
    the consultant uses them the same way every run.
30. As `gx-code-review`, I want the consultant to give each deferred finding a target (`gx` or
    `<project>`) and a `category`, so that the skill never guesses either.
31. As `gx-code-review`, I want deferred findings that name the same fix grouped into one
    follow-up, so that one fix is one follow-up, as with approved fix tickets.
32. As `gx-code-review`, I want each deferred follow-up's Source to name the epic and the
    code-review ticket, so that a person can trace it back to the review.
33. As `gx-code-review`, I want deferred findings to go through the skill's normal dedupe, so that a
    finding already filed gets a "seen again" entry instead of a duplicate.
34. As `gx-code-review`, I want approved findings to stay fix tickets in the epic and rejected ones
    to be dropped, so that the existing review flow does not change.
35. As `gx-code-review`, I want my "Final disposition" to name the follow-up each deferred finding
    became or was appended to, so that the review ticket records where every finding went.

### Listing an epic

36. As a skill (or person), I want `gx tickets show <project:epic>` to print the epic's entry and
    list its tickets (id, status, title), so that I can list a follow-ups epic or read a map
    without reading files by path.
37. As `gx-resolve-follow-ups` and `gx-design`, I want that call to work, so that the steps that
    already use it stop failing with "malformed-address".

### Docs and bundle

38. As a person reading `gx-local-tracker.md`, I want the "Filing" and "Seen again" bullets to point
    at the skill, so that I know the procedure lives there and the format lives here.
39. As `gx-resolve-follow-ups`, I want every follow-up, from any filer, to have the same shape, so
    that I verify and route them all the same way.
40. As a gx maintainer, I want the new skill embedded in the bundle and installed to both Claude and
    Codex roots, so that every install method ships it.
41. As a gx maintainer, I want the bundle tests to pin the skill's presence, its invocation policy,
    and that no other skill restates `follow-up add`, so that drift fails in CI.

## Implementation Decisions

### The new skill

- A new bundle skill directory, `gx-follow-up`, with one `SKILL.md`. It is added to the bundle's
  embed list, to the bundle tests' required-files list, and to the bundle doc's Layout section.
- Frontmatter: `name: gx-follow-up`, a `description` that names the trigger phrases ("create / file
  / add a follow-up"), and `disable-model-invocation: false` (model-invocable). The bundle doc's
  "Invocation policy" section gains a sentence saying why: filing a follow-up is a cheap, safe
  write that a person often asks for in passing, and other skills must be able to call it.
- Codex: the same file ships to the Codex root with the same relative layout, as every bundle skill
  does. Codex never auto-invokes, so there it runs only by `/gx-follow-up` or when a calling skill
  invokes it. Its questions are asked in plain text there.
- The skill links to `gx-local-tracker.md` "Follow-ups" for the format and does not restate it.

### Inputs from a caller

A calling skill passes, in prose (the way `gx-code-review` already calls `gx-to-tickets`):

- the finding (what is wrong or missing, the evidence, a suggested fix) and its Source details;
- optionally a **target project** — when given, the skill skips project detection;
- optionally **`unattended`** — the skill never asks; an unclear project falls to the routing test
  and an unclear category to the default below;
- optionally a **match** — `match: <addr>` or `match: none` — when the caller already judged
  dedupe (used by `gx-evolve`); the skill skips the search and applies the outcome;
- optionally a **category** — when given (e.g. by `gx-code-review`'s consultant), the skill does
  not infer one.

gx has no env var or prompt marker for ralph-loop, so "unattended" is only ever what a caller says.
A run without the flag (a person's `/gx-follow-up` or a model auto-trigger) is interactive.

### Choosing the project

First match wins:

1. The user (or caller) names the project.
2. The finding is about gx tooling (skills, ralph-loop, the server, tickets, the TUI) → `gx`.
3. The cwd is the gx repo → `gx`.
4. Otherwise → the cwd's project.

Step 2 uses the tracker's existing routing test ("would this still exist if the epic had never been
authored? If yes, it's tooling"). The skill asks (AskUserQuestion on Claude; plain text on Codex)
only when:

- the routing test is unsure (e.g. a project-code bug that a gx skill caused), or
- the cwd is not a registered project (`gx tickets root` fails) and no project was named.

In an unattended call it never asks: the routing test decides, and a non-registered cwd with no
target falls to `gx`.

### Dedupe

The skill owns the dedupe rules; no caller restates them.

- **Candidates**: every ticket in `<project>:follow-ups`, in `<project>:follow-ups-archived`, and
  every open ticket in the project carrying a `category` (follow-ups already moved out). The skill
  lists them with `gx tickets show <project:epic>` and reads each one's title and `## Problem`.
- **Judge**: the skill itself, one finding at a time, by title and `## Problem` — the same problem,
  not the same area. No subagent.
- **Outcomes**:
  - No match → file it.
  - Matches an open ticket carrying a `category` → append a dated "seen again" entry naming the new
    source to its `## Evidence` (existing text plus the entry, via `gx tickets section`), file
    nothing new, print the matched address.
  - Matches an archived `fixed` ticket → file a regression: a new follow-up whose `## Source` and
    `## Evidence` cite the archived address.
  - Matches any other archived ticket (`obsolete`, `duplicate`, `not-worth-it`) → drop it and
    report the archived address as the reason.
- A caller-passed match skips the candidate search; the outcome rules still apply.

### Filling and filing

- The skill infers `category` (`bug` / `idea`) and the body sections (Source, Problem, Evidence,
  Suggested fix, optional Open questions) from the conversation or the caller's input. No preview.
- Unattended default for an unclear category: `bug` when there is concrete evidence of wrong
  behaviour, else `idea`.
- Several independent problems in one call → one follow-up each, every address printed, no
  question.
- It files with `gx tickets follow-up add <project>:follow-ups --body -`, and prints the address the
  verb prints. A refusal from the verb (missing category or section) is fixed and retried, never
  worked around with a plain `add`. The verb creates a missing `follow-ups` epic itself, so no
  `epic add` is needed.

### `gx tickets show` on an epic

- `gx tickets show` accepts an epic address (`project:epic`, or a bare epic slug when the project is
  implied). It prints the epic's entry (frontmatter and body) followed by a list of its tickets: id,
  status, title. A ticket address keeps today's output.
- This closes `gx:follow-ups/30`: once the implementing ticket exists, close it with
  `--reason duplicate` and the implementing ticket's address as the note.
- The tracker doc's "Address a ticket" bullet says `show` takes an epic address too.

### Tracker doc

- `gx-local-tracker.md` "Follow-ups" keeps the format. Its "Filing" bullet points at the
  `gx-follow-up` skill as the procedure. Its "Seen again" bullet is replaced by a pointer to the
  skill's dedupe. Its opening list of skills that link to the section is updated to match.

### Callers

- **`gx-investigate`** (Output, tooling bullet): keeps the routing explanation but replaces the
  inline `follow-up add` call, format and "seen again" instructions with "file it with the
  `gx-follow-up` skill, target `gx`". Its unattended-verbs grant adds filing via `gx-follow-up`
  (target `gx`, unattended) as always allowed.
- **`gx-to-tickets`**: the sentence that files "a diagnosis or idea for a person to plan" now says to
  use the `gx-follow-up` skill instead of naming the verb, unattended when it runs unattended.
- **`gx-evolve`** (step 6): keeps its consultant match (step 5 unchanged). Step 6 becomes: hand each
  approved finding, with its target, category and the consultant's match (`match: <addr>` or
  `match: none`), to `gx-follow-up`, unattended. The four outcome branches move to the skill.
  Source still names the epic, ticket addresses, session ids and `gx-evolve`.
- **`gx-code-review`**:
  - Step 5 defines the three verdicts: approved = worth a **fix ticket** in this epic (the current
    "worth a follow-up ticket" wording changes), deferred = follow-up ("real, but not worth fixing
    in this epic"), rejected = dropped. For each deferred finding the consultant also gives a target
    (`gx` or `<project>`) and a `category`, and groups findings that name the same fix.
  - A new step after step 6 files each deferred group through `gx-follow-up`, unattended, with the
    target and category passed. Source names the epic and the code-review ticket. Normal dedupe
    applies.
  - Step 7's "Final disposition" names the follow-up each deferred finding became, was appended to,
    or was dropped against.
  - Approved findings still become fix tickets via `gx-to-tickets`; rejected ones are dropped.

## Testing Decisions

A good test checks external behaviour: what the shipped bundle says and contains, and what
`gx tickets show` prints, not how either is built internally.

- **Seam 1: the bundle test suite** (the existing tests in the `skills` package that read the
  on-disk bundle and the embedded copy). Extend it so that:
  - `gx-follow-up/SKILL.md` is in `requiredFiles` (the "every skill dir is embedded" test already
    catches a missing embed entry);
  - `wantInvocationPolicy` lists `"gx-follow-up": false`, and its frontmatter name and description
    pass the metadata test;
  - relative links in the new skill resolve, and it uses no retired tracker term (existing tests).
- **Content pins**, in the style of `TestResolveFollowUpsUsesMovedAddress` and
  `TestInvestigateUnattendedVerbs`:
  - no skill file names `follow-up add` except `gx-local-tracker.md` and `gx-follow-up` itself
    (today this catches `gx-investigate`, `gx-to-tickets` and `gx-evolve`);
  - `gx-follow-up`'s description names the trigger phrases;
  - `gx-follow-up` states the "never ask when unattended" rule;
  - `gx-code-review` defines "deferred";
  - `TestInvestigateUnattendedVerbs` stays green with the new grant.
- **Seam 2: the `gx tickets show` command tests** (`cmd/tickets_show_test.go`): an epic address
  prints the entry and its ticket list; a ticket address is unchanged; an unknown epic refuses.
- The rest of the skill's behaviour (routing, dedupe judgment, inference) is prose and is checked by
  hand. This is accepted.
- Prior art: the `skills` package bundle tests and `cmd/tickets_show_test.go`.

## Out of Scope

- `gx-code-review` fix tickets and spec-review fix tickets filed into the active epic. They are
  not follow-ups and keep their current flow.
- The follow-up format itself (sections, category, `draft` + `research`). It stays as defined in
  `gx-local-tracker.md`.
- Any change to `gx tickets follow-up add`. The only verb change is `gx tickets show` on an epic.
- `gx-resolve-follow-ups` and `gx evolve export`, which read follow-ups but do not file them (they
  only benefit from the `show` change).
- Server recovery follow-ups that the server files itself from investigate reports (Go code, not a
  skill).
- An env var or marker for ralph-loop. "Unattended" is a caller-passed flag only.

## Further Notes

- All open questions from the first draft were resolved in the design review (see the map's
  `## Review findings`).

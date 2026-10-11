# gx-follow-up: one route for filing a follow-up

## Problem Statement

Today four skills file follow-ups, and each one carries its own copy of the procedure:
`gx-investigate`, `gx-to-tickets`, `gx-evolve` and (implicitly, by dropping them) `gx-code-review`.
Each copy restates part of the routing rule, the call to `gx tickets follow-up add`, and the "seen
again" handling, with small differences. A person in a live session who wants to note a bug or an
idea for later has no skill at all: they must remember the format in `gx-local-tracker.md`, pick the
right project, check for a duplicate by hand, and type the `follow-up add` call.

Some real findings are also lost. When the `gx-code-review` consultant marks a finding "deferred",
nothing files it. The finding lives only in the code-review ticket's "Final disposition", where
`gx-resolve-follow-ups` and `gx evolve export` never look.

## Solution

A new bundled skill, `gx-follow-up`, is the single route every person and every skill uses to file
a follow-up. A person runs `/gx-follow-up` (or says "create / file / add a follow-up"). A skill
calls it with what it found. The skill:

1. Chooses the target project (or takes the one the caller passed).
2. Searches that project's `follow-ups` epic for the same problem. On a match it appends a "seen
   again" entry to the existing ticket's `## Evidence` and files nothing new.
3. Otherwise infers the `category` (bug or idea) and fills the body sections from the conversation,
   with no preview, and files it with `gx tickets follow-up add`.
4. Prints the address of the new (or matched) follow-up.

It asks a question only when the project or the category is unclear, and never when a caller
passed a target or when it runs under ralph-loop. On Codex, which has no questions UI, it asks in
plain text.

The format itself stays in `gx-local-tracker.md` "Follow-ups". The new skill owns the procedure.
The four callers drop their own copies and call the skill. `gx-code-review` also starts filing its
consultant's deferred findings as follow-ups.

## User Stories

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
8. As a gx user, I want the skill to ask me which project when the signals conflict, so that a
   finding never lands in the wrong tracker silently.
9. As a gx user in a directory that is not a registered project, I want the skill to ask me which
   project to use, so that it never guesses a target that `follow-up add` would refuse.
10. As a gx user, I want the skill to check the target `follow-ups` epic for the same problem
    first, so that I don't create a duplicate.
11. As a gx user, I want a match to get a dated "seen again" entry in its `## Evidence`, so that the
    existing ticket records how often the problem comes back.
12. As a gx user, I want the skill to file without showing me a preview, so that filing costs me
    one sentence, not a review round.
13. As a gx user, I want the skill to infer `bug` or `idea` from what we discussed, so that I am
    asked only when it is truly unclear.
14. As a gx user, I want the skill to fill Source, Problem, Evidence, Suggested fix and (when there
    are any) Open questions from the conversation, so that the follow-up is complete enough for
    `gx-resolve-follow-ups` to verify.
15. As a gx user, I want the skill to print the new follow-up's address, so that I can open it,
    cite it, or block a ticket on it.
16. As a gx user, I want the skill to print the matched ticket's address when it only appended a
    "seen again" entry, so that I know where my finding went.
17. As a Codex user, I want the skill to ask its rare questions in plain text, so that it works
    without a questions UI.
18. As a skill author, I want one skill that files follow-ups, so that a change to the procedure is
    made in one place.
19. As a calling skill, I want to pass the target project, so that the skill skips detection and
    never asks a question.
20. As a calling skill running unattended under ralph-loop, I want the skill never to ask, even if I
    pass no target, so that an unattended session never stalls on a question nobody can answer.
21. As a calling skill under ralph-loop with no target, I want the routing test alone to decide the
    project, so that the finding still lands somewhere sensible.
22. As `gx-investigate`, I want to hand a tooling diagnosis to the skill, so that I no longer
    restate the follow-up call, format and dedupe in my own body.
23. As `gx-to-tickets`, I want to route "a diagnosis or idea for a person to plan" to the skill, so
    that I never file a follow-up with my own copy of the procedure.
24. As `gx-evolve`, I want to keep my consultant's match against existing follow-ups and my own
    "seen again", regression and rejection handling, so that the retro's richer dedupe (archive
    reasons, regressions) is not lost.
25. As `gx-evolve`, I want to call the skill only for "no match → file it", with its dedupe skipped,
    so that a finding my consultant already deduped is not searched a second time.
26. As `gx-code-review`, I want the consultant's "deferred" verdict defined as "real, but not worth
    fixing in this epic", so that the consultant uses the three verdicts the same way every run.
27. As `gx-code-review`, I want each deferred finding filed as a follow-up through the skill, so
    that a real problem left out of the epic is not lost.
28. As `gx-code-review`, I want a deferred finding about project code to go to
    `<project>:follow-ups` and one about gx tooling to go to `gx:follow-ups`, so that each lands
    where it will be triaged.
29. As `gx-code-review`, I want a deferred defect filed as `category: bug` and anything else as
    `idea`, so that the category matches the finding.
30. As `gx-code-review`, I want each deferred follow-up's Source to name the epic and the
    code-review ticket, so that a person can trace it back to the review.
31. As `gx-code-review`, I want deferred findings to go through the skill's normal dedupe, so that a
    finding already filed gets a "seen again" entry instead of a duplicate.
32. As `gx-code-review`, I want approved findings to stay fix tickets in the epic and rejected ones
    to be dropped, so that the existing review flow does not change.
33. As `gx-code-review`, I want my "Final disposition" to name the follow-up each deferred finding
    became (or was appended to), so that the review ticket records where every finding went.
34. As a person reading `gx-local-tracker.md`, I want the "Filing" bullet to point at the skill, so
    that I know the procedure lives there and the format lives here.
35. As `gx-resolve-follow-ups`, I want every follow-up, from any filer, to have the same shape, so
    that I verify and route them all the same way.
36. As a gx maintainer, I want the new skill embedded in the bundle and installed to both Claude and
    Codex roots, so that every install method ships it.
37. As a gx maintainer, I want the bundle tests to pin the skill's presence and invocation policy, so
    that a change that breaks either fails in CI.

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

A calling skill may pass, in prose (the way `gx-code-review` already calls `gx-to-tickets`):

- the finding (what is wrong or missing, the evidence, a suggested fix) and its Source details;
- optionally a **target project** — when given, the skill skips project detection and never asks;
- optionally an instruction to **skip dedupe** — used only by `gx-evolve`.

### Choosing the project

First match wins:

1. The user names the project.
2. The finding is about gx tooling (skills, ralph-loop, the server, tickets, the TUI) → `gx`.
3. The cwd is the gx repo → `gx`.
4. Otherwise → the cwd's project.

The skill reuses the tracker's existing routing test ("would this still exist if the epic had never
been authored? If yes, it's tooling") to decide step 2. When signals conflict, or the cwd is not a
registered project, it asks (AskUserQuestion on Claude; plain text on Codex). When a caller passed a
target, none of this runs. Under ralph-loop it never asks; the routing test decides.

### Dedupe

- Before filing, the skill reads the target `<project>:follow-ups` epic (`gx tickets show
  <project>:follow-ups` lists its tickets) and looks for the same problem.
- On a match it appends a dated "seen again" entry, naming the new source, to that ticket's
  `## Evidence` (existing text plus the entry, via `gx tickets section`), files nothing new, and
  prints the matched address.
- When the caller says to skip dedupe, this step is skipped.

### Filling and filing

- The skill infers `category` (`bug` / `idea`) and the body sections (Source, Problem, Evidence,
  Suggested fix, optional Open questions) from the conversation or the caller's input. No preview.
- It asks only when the category or the project is unclear, and never when a target was passed or
  under ralph-loop.
- It files with `gx tickets follow-up add <project>:follow-ups --body -`, and prints the address the
  verb prints. A refusal from the verb (missing category or section) is fixed and retried, never
  worked around with a plain `add`.

### Tracker doc

- `gx-local-tracker.md` "Follow-ups" keeps the format. Its "Filing" bullet now points at the
  `gx-follow-up` skill as the procedure. Its opening list of skills that link to the section is
  updated to match.

### Callers

- **`gx-investigate`** (Output, tooling bullet): the bullet keeps the routing explanation but
  replaces the inline `follow-up add` call, format and "seen again" instructions with "file it with
  the `gx-follow-up` skill". Its unattended mode passes the target, or relies on the never-ask rule.
- **`gx-to-tickets`**: the sentence that files "a diagnosis or idea for a person to plan" now says to
  use the `gx-follow-up` skill instead of naming the verb.
- **`gx-evolve`** (step 6): keeps its consultant match and its "seen again", regression and
  rejection branches. Only the "No match → file it" branch changes: it calls the `gx-follow-up`
  skill with the target from the consultant's finding and dedupe skipped. Source still names the
  epic, ticket addresses, session ids and `gx-evolve`.
- **`gx-code-review`**:
  - Step 5 defines the three verdicts: approved (fix in this epic), rejected (drop), deferred
    ("real, but not worth fixing in this epic").
  - A new step after step 6 files each deferred finding through `gx-follow-up`, unattended, with
    the target passed: `<project>:follow-ups`, or `gx:follow-ups` for a gx tooling finding.
    `category: bug` for a defect, else `idea`. Source names the epic and the code-review ticket.
    Normal dedupe applies.
  - Step 7's "Final disposition" names the follow-up each deferred finding became or was appended
    to.
  - Approved findings still become fix tickets via `gx-to-tickets`; rejected ones are dropped.

## Testing Decisions

A good test here checks what the shipped bundle says and contains, the way an installing user or an
agent would see it, not how the Go code embeds it. The skills are prose, so the seams are the
bundle's own content tests.

- **Seam 1: the bundle test suite** (the existing tests in the `skills` package that read the
  on-disk bundle and the embedded copy). Extend it so that:
  - `gx-follow-up/SKILL.md` is a required, embedded file (the "every skill dir is embedded" test
    already catches a missing embed entry);
  - the invocation-policy table lists `gx-follow-up` as model-invocable, and its frontmatter name
    and description pass the metadata test;
  - relative links in the new skill resolve (the existing relative-reference test);
  - the new skill uses no retired tracker term (the existing test).
- **Optional content pins**, in the style of the existing `TestResolveFollowUpsUsesMovedAddress`
  and `TestInvestigateUnattendedVerbs` tests: the callers no longer restate the filing verb
  (`gx-investigate`, `gx-to-tickets` and `gx-code-review` name `gx-follow-up`, not
  `follow-up add`), and `gx-code-review` defines "deferred". These are cheap guards against a
  caller drifting back to its own copy.
- No new Go behaviour is added, so no new Go seams. `gx tickets follow-up add` keeps its existing
  tests.
- Prior art: the `skills` package bundle tests listed above.

## Out of Scope

- `gx-code-review` fix tickets and spec-review fix tickets filed into the active epic. They are
  not follow-ups and keep their current flow.
- The follow-up format itself (sections, category, `draft` + `research`). It stays as defined in
  `gx-local-tracker.md`.
- Any change to `gx tickets follow-up add` or another `gx` verb.
- `gx-resolve-follow-ups` and `gx evolve export`, which read follow-ups but do not file them.
- Server recovery follow-ups that the server files itself from investigate reports (Go code, not a
  skill).

## Further Notes

Open questions the map did not decide:

1. **How the skill knows it runs under ralph-loop.** Other skills (`gx-tdd`, `gx-to-tickets`) say
   "under ralph-loop" without naming a signal. Is there an env var or prompt marker the skill should
   check, or does it rely on the caller passing a target?
2. **Detecting "cwd is not a registered project".** Which command does the skill use: `gx tickets
   root` failing, `gx server project list` (needs the server), or something else?
3. **Dedupe scope.** The map says "search the target `follow-ups` epic". Should the skill also look
   in `follow-ups-archived` and in follow-ups already moved out (as `gx-evolve`'s consultant does)?
   If an archived `fixed` ticket matches, is that a regression to file, as in `gx-evolve`?
4. **How the match is judged.** Does the skill compare titles and `## Problem` lines itself, or use
   a subagent? A large follow-ups epic may need a cheap first pass.
5. **Missing `follow-ups` epic.** If the target project has no `follow-ups` epic yet, does
   `follow-up add` create it, or must the skill create it first with `gx tickets epic add`?
6. **Tooling-vs-project test for `gx-code-review`.** A code-review finding is usually about the
   project's code. Which signal marks a deferred finding as gx tooling, so it goes to
   `gx:follow-ups`?
7. **Defect vs not for `gx-code-review`.** Is "defect" the consultant's call (an extra field in its
   triage output), or the skill's inference from the finding?
8. **Grouping deferred findings.** Should several deferred findings that name the same fix become
   one follow-up (as approved findings are grouped into one fix ticket), or one each?
9. **Under ralph-loop with an unclear category.** The never-ask rule wins, but which default is
   used: `idea`, or `bug` when there is evidence?
10. **Multiple findings in one person-run call.** When a person describes two problems at once,
    does the skill file two follow-ups or ask?

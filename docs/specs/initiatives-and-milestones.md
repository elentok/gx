## Problem Statement

A feature that is too big for one Design takes too long to design, burns the person out and ends in a huge PR that needs many fixes before it ships. Plans written up front for the later parts go stale as soon as the first part teaches something. gx has no place to hold a feature that spans several Designs, no way to record what a Design pushed to "later", and no check between one slice and the next. Also, "epic" and "ticket" in gx clash with Jira epics and tickets.

This spec covers Milestone 1 of the feature, the "Initiative tracer": the thinnest end-to-end path through its riskiest assumptions, sized for one reviewable impl epic. Later Milestones are sketches in the map and are not specified here.

## Solution

Add the **Initiative**: one Destination and an ordered list of **Milestones**. A person starts one with `gx initiatives new` and shapes it with the `/gx-initiative` skill. Each Milestone gets its own Design (a map epic) and one `-impl` epic, so one Milestone is one PR. Only Milestone 1 is designed in full. Later Milestones stay a name, a one-line goal and the **Assumptions** they test.

While a Design runs, gx-design reads its Initiative, asks scope questions with three answers (in this Milestone, later in a named Milestone with a reason, out with a reason) and records "later" answers as **Deferred items** on the Initiative. When the Milestone's impl epic is merged, the person re-runs `/gx-initiative` to run the **Gate**: the person confirms each Assumption and each deferred-item placement, and a gate note records one outcome: continue, reshape or stop. A read-only web page shows the Initiative, and a nav entry lists them. Design and impl epic cards and headers show "Milestone N of <Initiative>".

The Initiative's line formats are owned by `gx initiatives` write verbs. The web page parser and the verbs share one definition of each format. Skills call the verbs and never format lines.

## User Stories

1. As a person with a feature too big for one Design, I want to start an Initiative from an idea, so that I can split the feature into Milestones instead of one huge Design.
2. As a person, I want `gx initiatives new <project:slug> "<idea>"` to create the Initiative holding only my idea, so that the first session shapes it with me.
3. As a person, I want `gx initiatives new` to refuse a slug that clashes with an existing epic or with `<slug>-design`, so that Initiative, Design and impl slugs never collide.
4. As a person, I want the Initiative to be created without a worktree, a draft, a Progress row or a design step, so that starting one is cheap.
5. As a person, I want New Design to work exactly as today, so that a small feature still starts as a plain Design.
6. As a person, I want `/gx-initiative` to grill me on the Destination, the Milestone list (name and goal each), the Assumptions and which Milestone is first, so that the Initiative is shaped by my answers.
7. As a person, I want the skill to check only that Milestone 1 fits one reviewable PR as a tracer bullet through the riskiest Assumptions, so that the first PR stays small and later Milestones stay sketches.
8. As a person, I want the skill to create Milestone 1's Design only after I confirm the first Milestone, so that nothing is created from a guess.
9. As a person, I want that Design to be a map epic carrying `initiative:` and `milestone:` and an Initial prompt seeded from the Initiative, so that it starts with the right context and runs like any Design.
10. As a person, I want the Design's slug to be `<initiative>-<milestone-slug>`, so that I can tell which Milestone it belongs to.
11. As a person, I want `/gx-initiative` on an Initiative that already has a Design for its latest Milestone to print a pointer to that Design and stop, so that I do not shape twice.
12. As a person, I want `/gx-initiative` to run in a plain agent session in the trunk checkout and edit the Initiative only through gx commands, so that it needs no worktree.
13. As a person, I want the skill to work in both Claude and Codex like the other bundled skills, so that harness choice does not matter.
14. As a person, I want gx-design to read the Initiative's Destination, its own Milestone entry, the Assumptions and the deferred items at the start of every run, so that scope answers fit the whole Initiative.
15. As a person, I want gx-design to never edit the Initiative's Destination or Milestone list, so that only the Gate reshapes the plan.
16. As a person, I want gx-design's scope questions to have three answers, so that nothing is silently dropped.
17. As a person, I want a "later" answer to name a Milestone and give a reason, so that the item has a destination.
18. As a person, I want an "out" answer to carry a reason and stay on the map's `## Out of scope`, so that out items are not copied to the Initiative.
19. As a person, I want a "later" answer written as a deferred item on the Initiative, so that the decision lives in one place.
20. As a person, I want the Design's `## Out of scope` to keep only a pointer line ("deferred to <milestone>, see the Initiative"), so that the decision is not duplicated.
21. As a person, I want a "later" answer that matches no Milestone stored as unplaced, so that the Design never edits the Milestone list.
22. As a person, I want `follow-ups` left for bugs and ideas found after something ships, so that planned scope does not mix with it.
23. As a person, I want the next Milestone's Design seeded with the Milestone goal, the Assumptions it tests and the open deferred items for it, shown as candidates and not decisions, so that deferred items get picked up.
24. As a person, I want the design review to check that every open deferred item for the Milestone appears in the spec as in, later or out, so that none is lost.
25. As a person, I want gx-design to set each such item to taken, or re-defer it with a new reason, as the spec is written and whenever a review finding changes its answer, so that item states match the spec's last version.
26. As a person, I want gx-design's Chart step, in a plain Design, to say so and tell me to run `/gx-initiative`, then stop, when it judges the idea too big for one reviewable PR, so that I find the Initiative flow without remembering it. In a Design that has `initiative:`, I want the hint to tell me to narrow this Milestone by answering "later" for the excess, and never to point to `/gx-initiative`, so that I do not loop.
27. As a person, I want that hint to have no threshold and no automatic conversion, so that I stay in charge of the split.
28. As a person, I want Assumptions recorded on the Initiative with a tested-by Milestone, a state (unknown, confirmed or broken) and evidence, so that I can see what a Milestone must prove.
29. As a person, I want only `/gx-initiative` to add Assumptions and only the Gate to change their state and evidence, so that a Design cannot rewrite them.
30. As a person, I want the Gate to refuse until the Milestone's impl epic is merged, so that evidence exists before the check.
31. As a person, I want the Gate to start only when I re-run `/gx-initiative <project:initiative>` and never by itself, so that gx adds no pause and no automation.
32. As a person, I want the agent to draft the gate note from the spec, the impl epic's tickets, the merged PR and the verify results, so that I confirm facts instead of writing them.
33. As a person, I want to confirm each Assumption's state and each deferred item's placement in grilling rounds with a recommended answer, so that nothing changes without my answer.
34. As a person, I want the gate note to record one outcome: continue, reshape, stop, or done for the last Milestone, so that the page and the next run know what happened.
35. As a person, I want `continue` to shape the next Milestone and create its Design once I confirm it fits one reviewable PR, so that the next slice starts from evidence.
36. As a person, I want `reshape` to let me edit the Milestone list and the Destination with my confirmation, and the note to say what changed and why, so that the plan can change in one place only.
37. As a person, I want `stop` to record why, set open deferred items to dropped with that reason and leave the Initiative open, so that the history stays.
38. As a person, I want a re-run after a `continue` or `reshape` gate whose next Milestone has no Design to resume at shaping that Milestone without repeating the Gate, so that an interrupted session loses nothing.
39. As a person, I want a re-run in any other state (Design exists, impl epic not merged, stopped, done) to print a pointer, so that I always know the next step.
40. As a person, I want the Gate to read the Retro's follow-ups as evidence and never copy them, so that tooling findings and product findings stay apart.
41. As a person, I want one gate per Milestone, never re-run, and a wrong note fixed by a later gate entry or a manual edit, so that history is append-only.
42. As a person, I want `gx initiatives` write verbs for deferred add and set (state, placement), assumption add and set (state, evidence), gate write (outcome, per-assumption lines, taught and placements lists) and milestone set (Design link and the list edits a reshape needs), so that line formats are defined once and validated.
43. As a person, I want those verbs to refuse input that breaks a format or names an unknown Milestone, Assumption or deferred item, so that the Initiative stays parseable.
44. As a person, I want `gx tickets section` to still work for manual edits, so that I can fix anything by hand.
45. As a person, I want the web page to show a line that does not parse verbatim as plain text in its own section, and never fail, so that hand edits never hide data.
46. As a person, I want a Design or impl epic whose `initiative:` or `milestone:` does not resolve to get no "Milestone N of" line, so that a broken link does not mislead.
47. As a person, I want gx-design and `/gx-initiative` to stop at their next run and name a broken link (missing Initiative, unknown Milestone slug, unparseable Milestones section, a Milestone whose linked Design does not exist, a Design whose Milestone links a different Design). gx-design checks only its own link and stops when its Milestone's Design link is not its own slug. `/gx-initiative` checks the full list, so that nothing runs on a guess.
48. As a person, I want an Initiatives nav entry next to Backlog with a plain per-project list showing Initiative, state, "N of M done" and the current Milestone, so that I can find my Initiatives.
49. As a person, I want no Start button on that list, so that Milestone 1 stays read-only.
50. As a person, I want an Initiative page with a header and a state pill (Active, Stopped or Finished), then Destination, Milestones, Assumptions, Deferred and Gate notes in one column, so that I can read the whole picture.
51. As a person, I want each Milestone to show its derived state (sketch, designing, implementing, merged, gated), its goal and links to its Design and impl epic, so that I can see where it is.
52. As a person, I want a sketch Milestone to show no links, so that I know no Design exists yet.
53. As a person, I want an empty Gate notes section to say to re-run `/gx-initiative <project:initiative>`, and other empty sections to show an empty-state line, so that a new Initiative reads clearly.
54. As a person, I want "Stopped" and "Finished" derived from the last gate note's outcome, so that the page needs no stored state.
55. As a person, I want Milestone state derived from the Design, the impl epic and the gate notes and never stored, so that it cannot drift.
56. As a person, I want the Design card, the impl epic card and both page headers to show "Milestone N of <Initiative>" linking to the Initiative page, and the Initiative page to link back to both, so that I can move between them.
57. As a person, I want the Epics page to skip Initiatives and show no new card kind, so that the page keeps listing only scheduled work.
58. As a person, I want `gx tickets epics` to list Initiatives and filter them, so that scripts can find them.
59. As a person, I want gx never to schedule or queue an Initiative, so that it is never mistaken for work.
60. As a person, I want existing ticket stores to migrate with one named step and a schema snapshot update, so that the new kind and the two map fields are covered by the migration gate.
61. As a person, I want this feature to add no pause kind, so that [ADR 0042](../adr/0042-few-pauses-all-recoverable-from-the-web.md) stays true.
62. As a person, I want a Milestone's slug to stay fixed once a Design or a gate note names it, and to be changeable before that with its `tested by:` and `to:` lines rewritten in the same write, so that a reshape never breaks a link.
63. As a person, I want the Gate to grill the Assumptions tested by this Milestone, and any other Assumption the evidence touches, and to re-place the open deferred items whose `to:` is the gated Milestone or the next Milestone, plus all `unplaced` ones, so that I know what a Gate covers. A reshape runs its list edits before the placement round, so the round sees the items they unplaced. On outcome `done`, open items are set to `dropped` with the reason "Initiative done", like `stop`.
64. As a person, I want gx-wayfinder's out-of-scope step, in a map with `initiative:`, to ask the same three-value scope question, to record "later" through the deferred-add verb, to write the pointer line under `## Out of scope` and to use `unplaced` when no Milestone fits (stories 17 to 21), so that no path skips the third answer.
65. As a person, I want a Milestone whose linked Design no longer exists to be named as a broken link and to let the milestone-set verb clear the link, so that the next run shapes the Milestone again.
66. As a person, I want adding a Milestone, and creating its Design, to refuse before writing anything when `<initiative>-<milestone-slug>`, its `-design` name or its `-impl` name is taken, so that a clash is caught when the slug is first written. When the clashing epic is a map with matching `initiative:` and `milestone:` that no Milestone links (an orphan from a failed link write), I want the skill to set the link instead of refusing.
67. As a person, I want every write verb to keep a line it cannot parse verbatim and in place, and touch only the line it changes, so that a hand edit is never lost.
68. As a person, I want a spike to be an ordinary first Milestone I may name during shaping, with the same size check, so that a large unknown has a place without a new kind.

## Implementation Decisions

Vocabulary follows GLOSSARY.md (Initiative, Milestone, Assumption, Deferred item, Gate, Design, Map epic, Epic, Backlog, Follow-up, Flow, Epic card) and ADRs 0049 and 0050 (written this run). Initiatives are optional.

**Kind and schema** (ADR 0049)
- An Initiative is an epic of a new kind, `initiative`, in the normal ticket store, next to `map`, `loop` and `backlog`. Its kind never changes, like a backlog's. It has `status: open` until the person closes it; no Milestone 1 verb or page reads status. It has no design steps and no Progress.
- Epic creation accepts and validates the new kind and its error text names it. The epic model gains an Initiative marker, and every place that already skips backlogs for scheduling or listing treats an Initiative the same way where scheduling is concerned: it holds no loop work, queueing refuses it with its own refusal reason, the Epics page and the TUI sidebar skip it, stats and archive views do not count it, and Branch and merge logic never see it.
- `gx tickets epics` lists Initiatives with the plain listing. A filter flag shows only Initiatives, like the backlog filter. The `--maps` filter excludes them. The flag name is the implementer's choice.
- A map epic gains two optional frontmatter fields, `initiative:` (the Initiative's slug in the same project) and `milestone:` (the Milestone slug). They are set when the Design is created and are settable through epic creation like other epic fields.
- One new named migration step covers the new kind value and the two map fields. Nothing in existing stores needs rewriting, so the step rewrites nothing (like the earlier optional-field steps). The schema snapshot is updated in the same change. The step name is the implementer's choice and is never renamed once released.

**Initiative body contract.** Sections in order: `## Initial prompt` (written by `gx initiatives new`), `## Destination`, `## Milestones`, `## Assumptions`, `## Deferred`, `## Gate notes`. The formats below are the contract between the verbs, the page parser and the skills.
- Milestones: an ordered list, one line per Milestone, with a slug, a name, a one-line goal and the slug of its Design once started. List order is the Milestone order. The exact punctuation is the implementer's choice, built from the same separator and `key:` style as the other lines, and defined once.
- Assumption line: `- A<n> | <assumption> | tested by: <milestone-slug> | state: <unknown|confirmed|broken> | evidence: <text or ->`. New lines start `unknown` with evidence `-`.
- Deferred line: `- <item> | to: <milestone-slug or unplaced> | why: <reason> | from: <design> | state: <open|taken|dropped>`. New lines start `open`.
- Gate note: one `### <milestone-slug>` entry per Milestone in `## Gate notes`, holding an outcome line (`continue`, `reshape`, `stop` or `done`), one `A<n> | <state> | <evidence>` line per Assumption, a "taught" list and a "placements" list. Reshape changes and stop reasons go in those two lists. The marker words are the implementer's choice.
- A line that does not match its format is shown verbatim, never dropped, by the page.

**`gx initiatives` verbs** (this replaces ticket 05's default of "no new CLI verb"; the person superseded it so that each format is defined once and validated)
- `gx initiatives new <project:slug> "<idea>"` creates the epic with only `## Initial prompt`. It refuses a slug that clashes with an existing epic or an `<slug>-design` epic.
- Write verbs, which own the formats and validate input: deferred add; deferred set (state, placement); assumption add; assumption set (state, evidence); gate write (outcome, per-assumption lines, taught list, placements list); milestone set (the Design link, and the list edits a reshape needs, including adding, removing, renaming and reordering Milestones and editing the Destination text through the same verbs or a destination verb).
- Reshape rules. A Milestone's slug cannot change once a Design or a gate note names it. Once a Design or a gate note names the slug, a rename changes only the name and goal. The verb refuses to remove a Milestone that has a Design or a gate note. Removing a sketch Milestone moves its open deferred items to `unplaced`, and the verb refuses while an Assumption is still `tested by:` it. The milestone-set verb can clear a Design link, and refuses to clear the link of a Milestone that has a gate note. While no Design or gate note names a slug, the milestone-set verb may change it and rewrite the `tested by:` and `to:` lines in the same write. The milestone-add verb refuses a slug whose `<initiative>-<slug>`, `-design` or `-impl` epic exists. The verb refuses to move or insert a Milestone before any Milestone that has a Design or a gate note, so only sketches after the current Milestone can be reordered. At a Gate, the assumption-set verb can also move `tested by:` to another Milestone, so a reshape can remove a sketch. An Archived epic counts as existing for the Design link and the impl epic: only an epic missing from the store is a broken link.
- Every write verb keeps a line it cannot parse verbatim and in place, and touches only the line it changes.
- The Initiative has no name field in Milestone 1. The page, the list and the back-link line show the slug.
- Each verb refuses a change that breaks a format or names an unknown Milestone, Assumption or deferred item, and leaves the store untouched on refusal.
- Verb and flag names are the implementer's choice.
- The verbs and the page parser share one format definition, in one module. Skills and gx-design call the verbs and never format lines. `gx tickets section` still works for manual edits.
- Gate write refuses until the Milestone's impl epic is merged, refuses a second gate for the same Milestone, and refuses outcome `done` unless the Milestone is the last one.
- Verbs write the store directly, the same way other `gx tickets` writes do. They need no worktree and add no pause.

**Derived Milestone state** (a shared read module used by the page, the list and the skills' detection)
- A Milestone whose Design link points to an epic missing from the store is shown with a "broken link" marker, the Design slug as plain text and no links. The read verb reports it.
- sketch: no Design linked. designing: a Design exists and no `<design>-impl` epic. implementing: the impl epic exists and is not a Merged epic. merged: the impl epic is a Merged epic and there is no gate note for the Milestone. gated: a gate note exists.
- Initiative state: Stopped when the last gate note's outcome is `stop`, Finished when it is `done`, otherwise Active. "N of M done" counts gated Milestones. The current Milestone is the first not gated.
- The read module also resolves a Design's or impl epic's back-link (Milestone number and Initiative) and reports a broken one.

**`/gx-initiative <project:initiative>` skill.** A one-shot skill, bundled for Claude and Codex like the others. No new Flow, no Progress, no Continue or auto-advance. It reads `gx-grilling`, `gx-domain-modeling` and `gx-unslop` before grilling, prints rounds in the grilling format, and calls the verbs for every write. Detection, in order:
1. A broken link (including a Milestone whose Design does not exist: the skill names the link, prints the exact verb command to clear it and does not write) or unparseable Milestones section: stop and name it.
2. No Milestone has a Design: shape (Destination, Milestones, Assumptions, which is first, Milestone 1's size), then on confirmation create Milestone 1's Design and set its link.
3. The latest Milestone with a Design is merged with no gate note: run the Gate. It covers the Assumptions tested by this Milestone, and any other Assumption the evidence touches, and re-places the open deferred items whose `to:` is the next Milestone plus all `unplaced` items (draft from evidence, grill each Assumption state and deferred placement with the agent's proposal as the recommendation, then write the gate note, assumption states and placements through the verbs, nothing before the answers).
4. The latest Milestone is gated with `continue` or `reshape` and the next Milestone has no Design: resume at shaping the next Milestone (confirm the one-PR size, then create its Design). Reshape is continue plus the confirmed list and Destination edits.
5. Otherwise (Design exists, impl epic not merged, stopped, done): print a pointer and stop.
- A new Design is a map epic with `initiative:` and `milestone:` and an Initial prompt holding the Milestone goal, the Assumptions it tests and the open deferred items whose `to:` is that Milestone, as candidates. Its Destination is the Milestone goal, with the Initiative's Destination as context. This seeding is one shared procedure used for Milestone 1 and for later Milestones. The skill never creates a Design for a Milestone other than the one just confirmed. Creating the Design refuses before writing anything when `<initiative>-<milestone-slug>`, its `-design` or its `-impl` name is an existing epic, with the same rule as `gx initiatives new`, as a guard behind the milestone-add check. The exception is an orphan map with matching `initiative:` and `milestone:` that no Milestone links: the skill sets the link instead.
- A spike is an ordinary first Milestone the person may name during shaping. It has no special kind, and the size check is the same.
- On `stop`, open deferred items are set to `dropped` with the reason. On `done` nothing is shaped next. The Initiative stays open either way.

**gx-design changes.**
- Detection gains a first check: for a map with `initiative:`, read the Initiative (Destination, its Milestone entry, Assumptions, deferred items) at the start of every run, and stop naming the broken link if it does not resolve.
- Chart: treat the seeded Initial prompt content as candidates. When the idea is too big for one reviewable PR (no threshold), say so and stop. In a plain Design, point to `/gx-initiative`. In a Design with `initiative:`, tell the person to narrow this Milestone by answering "later" for the excess (deferred items), and do not point to `/gx-initiative`: reshaping is allowed only at a Gate.
- Grilling rounds on scope use the three answers. A "later" answer calls the deferred-add verb and writes the pointer line under `## Out of scope`. An "out" answer stays on the map.
- After the spec is written, and after any review finding that changes an item's answer, set each open deferred item for the Milestone to `taken` or re-defer it with a new reason through the deferred-set verb.
- The design-review rubric gains one check: every open deferred item for the Milestone appears in the spec as in, later or out. A missing one is a finding.
- gx-wayfinder, in a map with `initiative:`, asks the same three-value scope question in its out-of-scope operation (closing a mis-scoped ticket, adding an Out of scope line) and records "later" through the deferred-add verb.
- gx-design never edits Assumptions, the Destination or the Milestone list, and never formats Initiative lines.

**Web.**
- A read-only Initiative page and an Initiatives list page, each served by a new read route. Under the existing route-parity guard each route gets a CLI verb; verb names are the implementer's choice. Pages load and refresh the way the Backlog page does.
- Nav: an "Initiatives" entry next to Backlog. The list is plain, grouped by project, with Initiative, state pill, "N of M done" and the current Milestone ("Stopped at Milestone N gate" when stopped). Projects with no Initiative are omitted.
- The Initiative page has one column in this order: header with state pill, Destination, Milestones (state pill, goal, links to Design and impl epic; none for a sketch), Assumptions table, Deferred table, Gate notes. Empty sections show an empty-state line, and an empty Gate notes section tells the person to re-run `/gx-initiative <project:initiative>`.
- The Design card and impl epic card, and both page headers, show one line "Milestone N of <Initiative>" linking to the page; the page links back to both. No new card kind and no buttons.
- Wire types follow the existing generation flow for web types.

## Testing Decisions

A good test checks external behaviour: what a verb prints and changes, what the fixture server serves, what a skill's text requires. It does not check internal markers.

Seams, highest first:
1. **Fixture-server flows** (the `gxfixture` scenarios, driven by the Playwright catalog and by CLI reads against the fixture): the Initiative page, the Initiatives list, the back-link line on cards and headers, the Epics page skipping Initiatives. One new scenario family seeds an Initiative in each Mock state.
2. **CLI verb tests** (the `cmd` verb-test style, like the `tickets` write and triage tests): `gx initiatives new`, every write verb with accepted and refused input, `gx tickets epics` listing and filtering, queue refusal, and the read verbs behind each new route. These also cover the shared format module through its callers: round-trip tests prove a line written by a verb parses on the page side, and malformed hand edits show verbatim.
3. **Skill text checks** (the `skills` rule tests, like the gx-design and triage rule tests): `/gx-initiative` detection order and refusals, gx-design reading the Initiative, three-value scope, deferred-item writing and states, the Chart hint, the new rubric check, and "skills call verbs, never format lines".
4. **Migration schema snapshot test**: the new step and snapshot update.

Prior art: the backlog kind and its migration test, the triage and backlog fixture scenarios, the route-parity table test, the existing skill rule tests.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. Flow numbers are the implementer's to take from the next free numbers in the e2e catalog.

Starting an Initiative (stories 1 to 5, 12, 58 to 61)
- Catalog flows: none exist. CLI: `gx initiatives new gx:demo-init "idea"` exits 0 and `gx tickets epics` lists `demo-init`; the filter flag lists only Initiatives and `--maps` omits it. The same command with a slug equal to an existing epic, or whose `<slug>-design` exists, exits non-zero and creates nothing. `gx tickets show gx:demo-init` shows kind `initiative`, status `open` and only `## Initial prompt`. Queueing an Initiative is refused. An old store opens with the new migration step applied once.
- New flows or scenarios to write: scenario `initiative-fresh` (an Initiative with only its prompt); a CLI check of the migration step on a fixture store.

Shaping and the Gate (stories 6 to 11, 13, 30 to 41, 47, 63, 65, 66, 68)
- Catalog flows: none (needs a real agent). Manual run of `/gx-initiative` in a fixture plain chat and in a Codex chat: grilling rounds for Destination, Milestones, Assumptions, first Milestone; Milestone 1's Design is created only after confirmation with `initiative:` and `milestone:` set. Re-run in each detected state and expect the matching action or pointer, including the resume after `continue` with no next Design. Skill text checks cover the order and refusals.
- CLI: gate write on a Milestone whose impl epic is not merged exits non-zero; on a merged one it writes the note, a second gate for the same Milestone exits non-zero, `done` on a non-last Milestone exits non-zero.
- New flows or scenarios to write: scenarios `initiative-gate-ready` (Milestone 1 impl epic merged, no gate note) and `initiative-resume` (gated `continue`, next Milestone without Design), seeded through the fixture with merged impl epics.

Design reads the Initiative (stories 14 to 29, 42 to 44, 46, 47, 62, 64, 67)
- Catalog flows: none (needs a real agent). Manual run of `/gx-design` on a Milestone Design: the three-value scope round, a "later" answer lands in `## Deferred` and leaves a pointer under `## Out of scope`, the seeded Initial prompt lists candidates, the Chart hint stops the run, the review flags a missing open item, item states are `taken` or re-deferred after the spec lands.
- CLI: deferred add, deferred set, assumption add and assumption set each accept a valid change and exit non-zero on a bad state, an unknown item or Milestone, or a format break; milestone set links a Design and applies list edits for a reshape; `gx tickets section` on `## Deferred` still reads and writes.
- Reshape CLI: milestone set refuses a slug change, refuses removing a Milestone that has a Design or a gate note, refuses removing a sketch Milestone while an Assumption is `tested by:` it, moves that Milestone's open deferred items to `unplaced` when allowed, accepts add, rename (name and goal only), reorder and Destination edits, and clears a Design link (refused for a Milestone with a gate note), changes a slug that nothing names and rewrites its `tested by:` and `to:` lines, refuses moving or inserting a Milestone before one that has a Design or gate note, and moves `tested by:` at a Gate through assumption-set. milestone-add refuses a slug whose epic, `-design` or `-impl` exists. A file with an unparseable line in `## Assumptions`, `## Deferred` or `## Milestones` keeps that line verbatim and in place after each verb, and likewise for `## Gate notes` after gate write and `## Destination` after a Destination edit.
- Skill text checks: gx-wayfinder's out-of-scope step asks the three values for a map with `initiative:` and writes the pointer line and `unplaced`; the Chart hint differs between a plain Design and a Milestone Design; the Design-creating path refuses a clashing slug and sets the link for an orphan Design; gx-design stops only on its own link mismatch.
- Gate CLI: gate write on `done` sets open items to `dropped` with "Initiative done".
- New flows or scenarios to write: none beyond `initiative-fresh`.

Web pages (stories 45 to 57)
- Catalog flows: none cover Initiatives. The Mock states list below is the look check for these stories; the stories refer to a state by name.
- CLI: each new read verb returns the page's data as JSON for the seeded Initiatives, including a line that does not parse (shown verbatim), a Design whose `initiative:` does not resolve (no back-link line), a Milestone linking a missing Design (broken-link marker, no links, in one fixture scenario) and an Archived Design (counts as existing).
- New flows or scenarios to write: one flow per Mock state below and a nav flow that opens Initiatives from the nav and follows a row to its page; fixture scenarios named per state.

Mock states (stored mock `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`; width 1280 for each; the page is one column, the Epics state is a card grid):
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Initiative page: M1 implementing", width 1280, fixture scenario `initiative-m1-implementing` (new), parts: header with Active pill, Destination, three Milestones (one implementing with Design and impl links, two sketches without links), Assumptions table (two unknown), Deferred table (one placed open, one unplaced open), Gate notes empty state naming `/gx-initiative`.
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Initiative page: M1 gated, continue", width 1280, fixture scenario `initiative-m1-gated` (new), parts: header with Active pill, Milestone 1 gated and Milestone 2 designing with Design link only, Assumptions table (one confirmed with evidence, one broken), Deferred table (one taken, one open), one gate note with outcome continue, per-Assumption lines, taught and placements.
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Initiative page: Stopped", width 1280, fixture scenario `initiative-stopped` (new), parts: header with Stopped pill, gated Milestone 1 and two sketches, one broken Assumption, one dropped deferred item, gate note with outcome stop and a dropped-items placement line.
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Initiative page: Finished", width 1280, fixture scenario `initiative-finished` (new), parts: header with Finished pill, two gated Milestones, two confirmed Assumptions, empty Deferred state, two gate notes (continue, done).
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Epics page: cards with the back-link line", width 1280, fixture scenario `initiative-epics-cards` (new), parts: Implementation card and Design card each with "Milestone N of <Initiative>" linking to the Initiative page, a plain Design card without the line, no Initiative card.
- `$(gx tickets root)/initiatives-and-milestones/mocks/07-initiative-page.html`, state "Initiatives list: active and stopped rows", width 1280, fixture scenario `initiative-list` (new), parts: project heading, an Active row ("1 of 3 done" with current Milestone) and a Stopped row ("Stopped at Milestone N gate"), no Start button.

## Out of Scope

- Jira sync, import and link fields. The Jira mapping lives only in GLOSSARY.md.
- Renaming "epic".
- Splitting a running Design into an Initiative, and the sizing nudge with a measure in New Design (Milestone 2 "Start paths"). In a plain Design, the Chart hint only points to `/gx-initiative`.
- Web buttons to create, edit or gate, and cuttable marks on nice-to-haves (Milestone 3 "Web actions").
- The agent drafting the next Milestone from gate evidence and deferred items beyond what the Gate in this spec does (Milestone 4 "Guided next Milestone").
- More than one impl epic per Milestone, and a separate Initiative entity or store.
- Any new pause kind, any automatic gate, any server-side enforcement beyond the gate-write refusals above.
- Handing the map's `## Later Milestones` sketch over to a real Initiative: manual in Milestone 1, by running `gx initiatives new` with those Milestones.

## Further Notes

- Dogfood: this Design follows its own rule and specs Milestone 1 only. The seeding of a new Design's Initial prompt is done when the Design is created, by `/gx-initiative`, and gx-design reads it at each run.
- The verbs supersede ticket 05's "no new CLI verb" default, at the person's request, so that line formats are defined once and shared with the page parser.
- Implementer's choices left open: the Milestones line punctuation, the gate-note marker words, the Initiative kind constant and Initiative marker names, the filter flag name, all new verb and flag names, route and wire type names, the migration step name, and empty-state, label and finished-row wording.
- The Retro stays separate: it judges tooling, the Gate judges the product. The Gate may read Retro follow-ups as evidence and never copies them.

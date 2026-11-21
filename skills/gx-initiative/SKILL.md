---
name: gx-initiative
description: >-
  Run an Initiative one step: shape its Milestones and first Design, run the Gate of a merged Milestone, or
  shape the next one. Run `/gx-initiative <project:initiative>` in a plain session in the trunk checkout. Manual.
disable-model-invocation: true
---

# gx Initiative

`/gx-initiative <project:initiative>` takes an Initiative one step forward and stops. It runs in a plain
agent session in the trunk checkout: no worktree, no `gx agent link`, no Flow, no Progress, no Continue
or auto-advance.

Read [gx-grilling](../gx-grilling/SKILL.md), [gx-domain-modeling](../gx-domain-modeling/SKILL.md) and
[gx-unslop](../gx-unslop/SKILL.md) before the first round. Print every round in the `gx-grilling` format,
with a recommended answer for each question. Ask the person nothing in prose outside a round.

## Rules for writing

- Edit the Initiative only through the `gx initiatives` verbs. Read their names and flags with
  `gx initiatives --help` and the help of each verb.
- Never format a Milestone, Assumption or deferred line yourself. Never edit the Initiative with
  `gx tickets section`. The person edits the prose sections by hand.
- Read state with `gx tickets show <project:initiative>` and `gx tickets show` on each Design and impl
  epic. Nothing is written before the person's answers.
- Use the `GLOSSARY.md` words: Initiative, Milestone, Destination, Assumption, Gate, Design.

## Steps

Run the steps in this order. The first one that matches runs and the rest are skipped.

1. **Broken link or unparseable Milestones section.** Check the full Milestone list, not one link. If a
   link is broken or the Milestones section does not parse, stop and name it. A Milestone whose Design
   epic is not in the store is one such link. Name the Milestone and print the command that clears it,
   then stop and write nothing:

   ```
   gx initiatives milestone set <project:initiative> <slug> --clear-design
   ```

2. **No Milestone has a Design: shape.** Grill, in rounds, the Destination, the Milestone list (a name and
   a one-line goal each), the Assumptions, and which Milestone is first. Check only that Milestone 1 fits
   one reviewable PR as a tracer bullet through the riskiest Assumptions. Later Milestones stay sketches.
   A spike is an ordinary first Milestone the person names, with the same size check and no special kind.
   After the answers, write each Milestone with `gx initiatives milestone add` and each Assumption with
   `gx initiatives assumption add`. When the person confirms the first Milestone, run
   [Create the Design](#create-the-design). Nothing is created from a guess, and no Design exists before that
   confirmation.

3. **Gate: the latest Milestone with a Design is merged and has no gate note.** Run the Gate once for that
   Milestone. The verbs refuse until its impl epic is merged. Do not try to get around a refusal.
   1. Draft the gate note from the spec, the impl epic's tickets, the merged PR and the verify results. Read
      the Retro's follow-ups as evidence. Never copy them into the note.
   2. Grill in rounds, with your draft as the recommendation: each Assumption this Milestone tests and any
      other Assumption the evidence touches. Then the placement of every open deferred item whose `to:` is
      the gated Milestone or the next Milestone, and every `unplaced` one.
   3. If the outcome is `reshape`, run the list and Destination edits the person confirms
      (`gx initiatives milestone add|move|remove|set`, `gx initiatives destination`) before the placement
      round, so the round sees the items they unplaced.
   4. Write nothing before the answers. Then run `gx initiatives gate <project:initiative> <milestone>` with
      one `--outcome`: `continue`, `reshape`, `stop`, or `done` for the last Milestone. Pass the Assumption
      states and evidence with `--assumption`, the placements with `--placed`, and what the Milestone taught
      with `--taught`. A `reshape` note says what changed and why.
   5. `stop` records why with `--reason` and sets every open deferred item to `dropped` with that reason.
      `done` does the same with the reason "Initiative done". The Initiative stays open either way.
   6. One gate per Milestone, never re-run. A wrong note is fixed by a later gate entry or by hand.

4. **Resume: the latest gated Milestone has outcome `continue` or `reshape`, and the next Milestone has no
   Design.** Do not repeat the Gate. Confirm the next Milestone fits one reviewable PR, then run
   [Create the Design](#create-the-design).

5. **Otherwise** (a Design exists and its impl epic is not merged, the Initiative is stopped, or it is
   done): print a pointer to the next step and stop. On a re-run where the latest Milestone already has a
   Design, the pointer names that Design, and the skill shapes nothing twice.

## Create the Design

Used by steps 2 and 4. It runs only for the one Milestone the person just confirmed. Never create a Design
for any other Milestone.

1. **Check the names before any write.** `<initiative>-<milestone-slug>`, its `-design` name and its `-impl`
   name must all be free, as `gx initiatives new` checks. If one is taken, stop and write nothing. The
   exception is an orphan: a map epic with matching `initiative:` and `milestone:` that no Milestone links,
   left by a failed link write. Then skip to the link in step 3 and do not refuse.
2. **Create the map epic** `<initiative>-<milestone-slug>` with `initiative:` and `milestone:` set, using
   `gx tickets epic add <project:epic> --kind map --initiative <initiative> --milestone <slug>`. Its
   Destination is the Milestone goal, with the Initiative's Destination as context. Seed its
   `## Initial prompt` with the Milestone goal, the Assumptions it tests, and the open deferred items whose
   `to:` is that Milestone, shown as candidates and not decisions.
3. **Link it** with `gx initiatives milestone set <project:initiative> <slug> --design <project:epic>`.

Then print the pointer to the new Design and stop.

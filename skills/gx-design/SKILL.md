---
name: gx-design
description: >-
  Walk one epic through map, review and implement: chart and resolve a wayfinder map, draft and
  review a spec, then publish an `<epic>-impl` epic. Detects the next step from tracker state; run
  `/gx-design <epic>`, or `/gx-design <epic/NN>` to resolve one map ticket. Manual; depends on
  external skills.
disable-model-invocation: true
---

# gx Design

`/gx-design <epic>` drives one epic from idea to a handed-off implementation epic. The tracker is
always gx's local tracker, and the destination is always a spec: this skill never implements or
touches code. `<epic>` is `<project:epic>`, or a bare epic slug in the current project. A ticket
address (`<project:epic/NN>`, or `<epic/NN>`) names the epic plus one map ticket to work: see step 3.
Read
[gx-local-tracker.md](../gx-local-tracker.md) for the ticket commands and the Map epics section;
don't restate them here.

Three phases: **map** (wayfinder charts and resolves the map) → **review** (spec drafted,
design-reviewed, grilled) → **implement** (tickets published). Each run inspects tracker and disk
state and runs the next step. There is no phase field: the detection list below is the state machine.

## 0. Dependency check

This skill does not vendor its helpers. `wayfinder`, `to-spec`, `grilling` and `domain-modeling`
must exist in the agent's skill root (`~/.claude/skills/` for Claude, `~/.codex/skills/` for Codex).
If any is missing, stop. Name the missing ones and say they come from
`https://github.com/mattpocock/skills`. Do not continue without them.

`wayfinder` and `to-spec` set `disable-model-invocation: true`, so the Skill tool can't reach
them. Wherever a step says "follow wayfinder's…" or "follow to-spec's…", `Read` that skill's
`SKILL.md` directly and execute it inline, or hand a subagent the same instruction. Never call the
Skill tool for them. Tell wayfinder to use the tracker's **Map epics** section for every ticket
operation (chart, claim, close, frontier). `gx-to-tickets` is model-invocable and can be called
normally.

## Detecting the next step

First run `gx agent link <project:epic>` (the ticket address, when given one). The Agents tab then
marks the session as a design run. Outside a gx session it prints a note and does nothing.

Read the map with `gx tickets show <project:epic>`. Evaluate in order; run the first match.

1. **No map epic** (`gx tickets epics --maps` doesn't list it) → follow wayfinder's "Chart the map"
   for the epic, including its Destination grilling.
2. **`provisional_destination: true`** → follow `grilling` on the Destination alone, then clear the
   flag with `gx tickets set <project:epic> --provisional-destination false`. Continue to step 3 in
   the same session, unless the grilling changed the Destination enough that seeded tickets must
   change. Then rework them and stop.
3. **Open tickets, or a non-empty `## Not yet specified`** → follow wayfinder's "Work through the
   map", resolving exactly **one** ticket. Never more per run: HITL tickets must stay interactive.
   Given a ticket address, resolve that ticket instead of the first frontier ticket. If it is not
   open, or its `blocked_by` are not all closed, say so and stop. A ticket address only ever runs
   this step: when step 1 or 2 matches first, run that step instead and name the ticket in the
   "Next:" line.
4. **No `docs/specs/<epic>.md`** → dispatch a subagent to read `to-spec` and follow it. Seed it with
   the map body (Destination, Decisions so far) and each closed ticket's title and resolution gist,
   not full ticket bodies.
5. **No `## Review findings` section on the map** → dispatch a design-review subagent against the
   spec (rubric below). It writes the findings as a markdown checklist (`- [ ] …`) with
   `gx tickets section <project:epic> "Review findings" -`. Write `- [x] No findings` if there are none.
6. **Unchecked findings** → run a `grilling` session on them, resident in this same run. Amend
   `docs/specs/<epic>.md` as each finding resolves, and check it off with
   `gx tickets section <project:epic> "Review findings" -`. Do not `/clear` between rounds. When
   every finding is checked, continue to step 7 in the same run.
7. **No `<epic>-impl` epic** → follow [gx-to-tickets](../gx-to-tickets/SKILL.md) on the amended
   spec, publishing into `<epic>-impl`. Never publish into the map epic: its tickets are
   hand-resolved decisions, and implementation tickets belong to ralph-loop. Continue to step 8 in
   the same run.
8. **No `<epic>-impl` branch** (`git rev-parse --verify refs/heads/<epic>-impl` fails) → commit the
   design docs into the feature branch, so that implementation agents see them in their worktrees.
   The server adopts a branch that already exists, so creating it here is safe.
   - Start the branch from trunk (the repo's default branch). If `<epic>-impl` sets `base:` or is
     blocked by another epic, stop and ask which branch to start from.
   - Create it in a temporary worktree: `git worktree add -b <epic>-impl <tmp-dir> <trunk>`.
   - Copy in `docs/specs/<epic>.md`, the ADRs this epic added under `docs/adr/`, and only this
     epic's `GLOSSARY.md` hunks. If the checkout has other uncommitted edits in those files, leave
     them out; ask when a hunk's owner is unclear.
   - Commit as `<epic>: Add spec, ADRs and glossary terms`, then `git worktree remove <tmp-dir>`.
   - Remove the committed changes from the original checkout: delete the new files, and reverse
     only the committed `GLOSSARY.md` hunks. Leave every other edit in place.
9. **Otherwise** → report the `<epic>-impl` slug and stop. Launch it from the TUI Queue tab.

## Design-review rubric (step 5)

Review the spec against the map it came from and its own template, not in the abstract:

- Does every user story trace back to the map's Destination and Decisions so far?
- Is each user story testable?
- Does anything creep past the map's `## Out of scope` or the Destination's boundary?
- What did the spec silently assume that the map never decided?
- What holes remain, or what question does the spec raise but not answer?

## After every run

Start a fresh session between runs (`/clear` in Claude, `/new` in Codex). The run that just ended
is not the place for the next one, except where a step says to continue.

Steps 4–8 can also chain. After step 4, 5 or 6 finishes, run `gx agent context-window`. If it
reports under **50K** tokens used, go back to "Detecting the next step" in the same session instead
of stopping. If it reports 50K or more, or the command errors, stop as usual. Never chain out of
steps 1–3: map runs stay one per session.

End with one line, naming the phase, not a step number:

```
✅ <phase>: <what happened this run>. Next: <phase> — <how to trigger it>
```

Examples:

```
✅ Map: resolved ticket 04-foo. Next: map — start a fresh session (/clear in Claude, /new in Codex), run /gx-design <epic>
✅ Map: the way is clear. Next: review — run /gx-design <epic> in a fresh session
✅ Review: findings resolved, spec amended, epic <epic>-impl created. Next: implement — launch it from the TUI Queue tab
```

Right after printing that line, raise the session as Actionable with the same "Next:" text. When
the next step is a prompt, add it as the Action, after the fresh-session command
(`/clear` in Claude, `/new` in Codex):

```
gx agent actionable "<phase>: <what happened>. Next: <phase>" --send "/clear" --send "/gx-design <epic>"
gx agent actionable "Review: <epic>-impl created. Next: implement — launch it from the TUI Queue tab"
```

Outside a gx session it prints a note and does nothing, so always call it.

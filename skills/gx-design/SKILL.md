---
name: gx-design
description: >-
  Walk one epic from idea to handoff: chart and resolve a wayfinder map, draft and
  review a spec, then publish an `<epic>-impl` epic. Detects the next step from tracker state; run
  `/gx-design <epic>`, or `/gx-design <epic/NN>` to resolve one map ticket. Manual.
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

Each run inspects tracker and disk state and runs the next step: the detection list below is the
state machine. The map's `design_step` field is what Progress shows. This skill moves it with
`gx tickets set <project:epic> --design-step <step>`:

- `chart` when it charts a map (step 1).
- `resolve-tickets`, `spec`, `review`, `grill-findings` and `create-tickets` in steps 3 to 7.
- `branch` when it creates the `<epic>-impl` branch (step 8).
- `done` only when this run itself closes the map. Reporting the `<epic>-impl` slug (step 9) does not.
- A move to an earlier step needs `--allow-backward`. Pass `--allow-backward` only for a review finding that becomes a map ticket
  (steps 5 and 6) and for the move back to `review` for the second review (step 6a), never for any other move.

## Prefer the correct fix

ralph-loop builds the final design without a person watching, so a slower fix costs agent time,
not the person's time. When a choice is between a quick fix (a workaround, a special case, a patch
on the symptom) and a slower fix that is clean and correct (it fixes the cause, or puts the logic
where it belongs), pick the correct fix. This applies to every recommended answer in a grilling
round, every map ticket Resolution, every implementer's choice and self-decided question in the
spec, and every ticket in the breakdown. Pick the quick fix only when the person asks for it, and
then record the reason in the spec.

## Calling other skills

Every call between bundled skills is a relative link to the callee's `SKILL.md`: `Read` it and
follow it inline, or hand a subagent the same link. Wherever a step says "follow gx-wayfinder's…"
or "follow gx-to-spec's…", that means [gx-wayfinder](../gx-wayfinder/SKILL.md) and
[gx-to-spec](../gx-to-spec/SKILL.md). Never call the Skill tool for them.

## Asking questions

Whenever you grill the person (gx-wayfinder's Destination and frontier grilling, a HITL ticket, the
provisional Destination, the review, the create-tickets quiz of step 7), read [gx-grilling](../gx-grilling/SKILL.md),
[gx-domain-modeling](../gx-domain-modeling/SKILL.md) and [gx-unslop](../gx-unslop/SKILL.md) first,
and print each round in the format `gx-grilling` defines. gx marks the session
Actionable itself when it sees a round, so call nothing to raise it.

The questions must be the last thing in the turn: no tool call, and no text, after them. The UI can
hide earlier text of a turn once later tool calls or text follow it, so questions printed before a
trailing call go unseen. Never pad the end with a tool call or a closing line.

## Detecting the next step

First run `gx agent link <project:epic>` (the ticket address, when given one). The Agents tab then
marks the session as a design run. Outside a gx session it prints a note and does nothing; report
that note to the person.

Then settle where the run works, in this order. Edit nothing until it is settled.

1. **After Branch.** The map is after Branch when its `design_step` is `branch` or `done`, or the
   `<epic>-impl` branch exists (the server's rule). On `<epic>-design`, read `design_step` and
   `git rev-parse --verify refs/heads/<epic>-impl`. Elsewhere, read `after_branch` from the
   `gx designs worktree <project:epic> --json` output (step 3 below). If after Branch: go to step 9 when
   the `<epic>-impl` epic exists, else stop with a message that the design is past Branch and
   `<epic>-impl` is missing. Edit nothing either way.
2. **No map yet.** For `/gx-design <epic> <idea>`, first check that no `<epic>-design` epic exists
   (`gx tickets epics`). If one does, stop with a message that it clashes with the design branch
   name, and create nothing. Otherwise create the map (step 1's "No map epic" writes it to the
   tracker, not the repo), then continue here.
3. **Branch check.** Check the run is on `<epic>-design` (`git branch --show-current`). If not (for
   example a plain Claude session in the trunk checkout), run `gx designs worktree <project:epic> --json`.
   It makes the design worktree if needed and rebases it onto trunk. Use its `path` as the
   worktree. Report its `note` (why a rebase was skipped), if any.

From here on, **never `cd` and never edit the trunk checkout**:

- Every git command is `git -C <worktree> …`. Every file path is absolute under `<worktree>`.
- Detection checks `docs/specs/<epic>.md` and the ADRs under `<worktree>`, not the trunk checkout.
- Every subagent (gx-to-spec, the design reviews, gx-to-tickets) gets `<worktree>`'s absolute path,
  this rule, and the "Prefer the correct fix" section.
- When the worktree is outside the start folder (bare repos), Claude may ask for permission on the
  first edits. Tell the person to allow them.
- The server does not see a plain session, so it can remove the worktree mid-run. Before the run-end
  commit, run `gx designs worktree <project:epic> --json` again if `<worktree>` is gone. There is no lease.

Read the map with `gx tickets show <project:epic>`. Evaluate in order; run the first match.

At each detection, including after a chain, report the step it starts with
`gx agent step <step> [--title "<ticket title>"]`. Use the step name from the list above; pass
the ticket title during `resolve-tickets`.

1. **The map has no `## Destination`** → report the step as `chart`. Read `## Initial prompt` as the
   loose idea and follow gx-wayfinder's "Chart the map" into the existing map, including its
   Destination grilling. After charting, end the run with the run-end line: never chain into a later
   step in this run.
   - **Tiers.** The charting session runs on the strongest tier. Its subagents follow the tier list,
     not the session: `gx agent model strongest` for the design reviews, `gx agent model worker`
     for gx-to-spec, gx-to-tickets and gx-research. Research tickets spawned while charting also use
     `gx agent model worker`. Pass the output's first word as `model` and its second word, if any, as
     `reasoning_effort` (see [Skill conventions](../gx-local-tracker.md#skill-conventions)).
   - **No map epic** (`gx tickets epics --maps` doesn't list it): a direct invocation. Text after the
     epic address in `/gx-design <epic> <idea>` is the idea; without it, ask for the idea first.
     Write the map as the tracker's "Chart" operation says (`kind: map`, `design_step: chart`, body
     holding only `## Initial prompt` with the idea text). That map has no Destination, so go on
     with the chart step above.
2. **`destination_unconfirmed: true`** → follow `gx-grilling` on the Destination alone, then clear the
   flag with `gx tickets set <project:epic> --destination-unconfirmed false`. Then end the run with
   the run-end line: never chain into step 3 in this run. If the grilling changed the Destination
   enough that seeded tickets must change, rework them first.
3. **Open tickets, or a non-empty `## Not yet specified`** → follow gx-wayfinder's "Work through the
   map", resolving one frontier ticket at a time. Skip claimed tickets.
   For a `grilling` ticket, load the skills named under "Asking questions" before the first
   question. Each ticket keeps its own grilling round, Resolution and Decisions so far line.
   Given a ticket address, resolve that ticket instead of the first frontier ticket. If it is not
   open, or its `blocked_by` are not all closed, say so and stop. A ticket address picks only the first ticket;
   after it closes, the run chains to the frontier. When step 1 or 2 matches first, run that step instead and
   name the ticket in the "Next:" line. Set `--design-step resolve-tickets` before resolving the ticket.
   - After a map ticket closes, chain to the next frontier ticket while the `--fits` check passes
     with the map-ticket growth (see "After every run").
   - Grilling, research and AFK task tickets chain. A chain never enters a prototype or HITL task ticket:
     end with the run-end line just before it. A run that starts on one ends after it.
   - The last map ticket closing chains into the spec step (step 4) when the budget allows.
4. **No `docs/specs/<epic>.md`** → dispatch a subagent (model from `gx agent model worker`) to read [gx-to-spec](../gx-to-spec/SKILL.md) and follow it. Seed it with
   the map body (Destination, Decisions so far) and each closed ticket's title and resolution gist,
   not full ticket bodies. Also seed it with the list of stored mocks under `<root>/<map>/mocks/`.
   The spec is where the map's gaps first show, so tell the subagent to sort
   every fact the map never decided:
   - **Implementer's choice** (a name, a config key, an internal marker): the spec says the
     implementer picks it, and moves on.
   - **Design question** (behaviour, scope, parity, which runners or harnesses it covers): the
     subagent writes no spec and returns the questions instead.

   If it returns design questions with suggested answers, grill the person in this same session
   (`gx-grilling`), then dispatch the subagent again with the answers. Continuing the same subagent
   is fine where the harness allows it.
   - **One round = one closed ticket.** Record each grilling round as one closed `grilling` map
     ticket with the slug prefix `spec-round-` (Question is the questions, Resolution is the
     answers) plus a Decisions so far line. The next dispatch gets them through the closed-ticket gists.
   - **2-round cap.** Count the `spec-round-` tickets, so the count survives `/clear`,
     auto-advance and detours. A detour never resets it. A question too big for one round becomes
     the next round.
   - **Before each redraft**, run `gx agent context-window --fits spec`. On a stop, end the run
     with the run-end line.
   - **From the third draft on**, the subagent decides new design questions itself and lists each
     in a `## Self-decided design questions` section of the spec: the question, the choice and
     the reason.
   - **Research or prototype.** Only a question that needs research or a prototype becomes an open
     `grilling` map ticket. File it after the rest of its batch is grilled and recorded, then stop
     without a redraft: the next run matches step 3 and resumes the map.
   - A spec never carries open design questions to the review.
   - Set `--design-step spec` only after the draft is written.
5. **No `## Review findings` section on the map** → dispatch a design-review subagent (model from `gx agent model strongest`) against the
   spec (rubric below). It writes the findings as a markdown checklist (`- [ ] …`) with
   `gx tickets section <project:epic> "Review findings" -`. Write `- [x] No findings` if there are none. Then set `--design-step review`.
6. **Unchecked findings** → set `--design-step grill-findings`, then run a `gx-grilling` session on them, resident in this same run. Amend
   `docs/specs/<epic>.md` as each finding resolves. A finding with no objection to its "(default)"
   resolution is checked off after the person's next reply. Check each finding off with
   `gx tickets section <project:epic> "Review findings" -`. Write a one-line resolution after each
   checked finding. Add the marker `(stories/scope changed)` when the fix added or removed user
   stories or changed scope. Do not `/clear` between rounds. When every finding is checked, go back
   to "Detecting the next step".
6a. **Every review finding is checked, at least one has the `(stories/scope changed)` marker, and the
   map has no `## Re-review findings` section** → run `gx agent context-window --fits review`; on
   stop, end the run. Otherwise move back with `gx tickets set <project:epic> --design-step review
   --allow-backward` and dispatch the second design-review subagent (model from `gx agent model strongest`). Give it the first checklist
   with its resolutions and the full spec. It checks only the changed stories and scope and what
   they affect, and writes the findings as a checklist with `gx tickets section <project:epic>
   "Re-review findings" -`. It may write `- [x] No findings`.
6b. **Unchecked findings in `## Re-review findings`** → set `--design-step grill-findings`, then run
   one `gx-grilling` round on them, resident in this same run, checking each off as in step 6. Then
   continue to step 7, even if this round changes stories again. Never run a third review.
7. **No `<epic>-impl` epic, and neither findings section has an unchecked finding** → follow [gx-to-tickets](../gx-to-tickets/SKILL.md) on the amended
   spec (its subagents use the model from `gx agent model worker`), publishing into `<epic>-impl`. Never publish into the map epic: its tickets are
   hand-resolved decisions, and implementation tickets belong to ralph-loop. Set
   `--design-step create-tickets` before publishing, then continue to step 8 in the same run.
8. **No `<epic>-impl` branch** (`git -C <worktree> rev-parse --verify refs/heads/<epic>-impl` fails) →
   put the design branch into a new `<epic>-impl`, so that implementation agents see the design
   docs in their worktrees. The server adopts a branch that already exists, so creating it here is safe.
   - Commit pending edits in `<worktree>` on `<epic>-design` first.
   - Start from trunk (the repo's default branch), or from the base the person names. If
     `<epic>-impl` sets `base:` or is blocked by another epic, stop and ask which branch to start from.
   - Create it in a temporary worktree: `git -C <worktree> worktree add -b <epic>-impl <tmp-dir> <base>`.
   - With trunk as base, merge `<epic>-design` there: `git -C <tmp-dir> merge <epic>-design`.
   - With another base, rebase a detached copy of the design tip and merge that copy. Never rewrite
     `<epic>-design`, and never detach the design worktree. Run the rebase in the temporary worktree:
     `git -C <tmp-dir> rebase --onto <base> <trunk> <design-tip-sha>` (this detaches `<tmp-dir>` only),
     then `git -C <tmp-dir> merge <the rebased sha>`.
   - On a conflict, follow [gx-resolving-merge-conflicts](../gx-resolving-merge-conflicts/SKILL.md).
   - Then `git -C <worktree> worktree remove <tmp-dir>` and set `--design-step branch`.
   - Step 8 removes neither the design worktree nor the `<epic>-design` branch: the server does,
     once it sees the Design is past Branch.
9. **Otherwise** → report the `<epic>-impl` slug and stop. Launch it from the TUI Queue tab.

## Researching existing UI behavior

When a map ticket or the spec depends on how the web UI behaves today, read
[gx-ui-check](../gx-ui-check/SKILL.md) and follow it inline to see it in a running fixture. Don't
guess from the source, and don't call the Skill tool for it.

## Design-review rubric (step 5)

Review the spec against the map it came from and its own template, not in the abstract:

- Does every user story trace back to the map's Destination and Decisions so far?
- Is each user story testable?
- Does every user story have an entry in the spec's `## Verification` section? A story with no
  entry is a finding. An epic with no user-visible behavior may say so in one line instead.
- Does each state of each stored mock have a Mock states line in `## Verification`? A missing line
  is a finding, like a story with no entry.
- Does anything creep past the map's `## Out of scope` or the Destination's boundary?
- What did the spec silently assume that the map never decided?
- What holes remain, or what question does the spec raise but not answer?
- Does Further Notes hold an open design question? Each one is a finding: step 4 should have sent
  it back to the map.
- Does `## Self-decided design questions` hold a choice you disagree with, or one that changes user
  stories or scope? Each one is a finding. File none otherwise.
- Does the spec pick a quick fix where a clean, correct fix exists, without a reason from the
  person? Each one is a finding (see "Prefer the correct fix").
- Does the spec add a pause kind, or a new way into an existing one? Each one is a finding unless
  the spec says why gx cannot recover on its own and which web button ends the pause (see
  [ADR 0042](../../docs/adr/0042-few-pauses-all-recoverable-from-the-web.md)).

## After every run

Start a fresh session between runs (`/clear` in Claude, `/new` in Codex). The run that just ended
is not the place for the next one, except where a step says to continue.

Every step except charting and Destination confirmation (steps 1 and 2 end the run) chains by the same check. After a step finishes, run detection to find the next step,
then run `gx agent context-window --fits <next step>`. On exit 0, chain: go back to "Detecting the
next step" in the same session. On exit 1 or an error, end the run with the run-end line below;
the server's auto-advance starts the next step in a fresh session. For a map ticket, use
`resolve-tickets` as the next step.

Set `--design-step` to the step the next run starts, with
`gx tickets set <project:epic> --design-step <step>`, before printing the run-end line.

Before the run-end line, commit every edit in `<worktree>` on `<epic>-design`
(`git -C <worktree> add -A`, then commit). The message starts with `<epic>:`. Skip it when the
worktree has no changes, or when the run stopped before the branch check.

End with one line:

```
✅ <what ran>: <what happened this run>. Next: <what to do next>
```

Examples:

```
✅ Resolve tickets: resolved ticket 04-foo. Next: start a fresh session (/clear in Claude, /new in Codex), run /gx-design <epic>
✅ Create tickets: epic <epic>-impl created. Next: launch it from the TUI Queue tab
```

`Next:` only marks that the run ended: gx reads no phase from the text after it. This line must be
the last output of the turn. gx reads it and raises the session as Actionable itself, so call
nothing to raise it.

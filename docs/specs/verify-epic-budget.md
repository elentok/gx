# Verify epic budget

## Problem Statement

A person who runs a large epic through ralph-loop cannot get its verify to finish on its own. The
verify ticket of gx:united-epics-page-impl listed 41 user stories. Each session ran out of tokens
long before the end: round 3 passed 15 of 41 stories, round 4 passed 2 of 26. Every round started
from zero. It re-read the spec, re-read the ticket, started the fixture again and stopped at the
token limit with "the unchecked stories" skipped.

Four things cause this:

- One verify ticket holds the whole story list, and no session can check 40 stories inside one
  context window.
- The token limit is a manual `gx agent context-window` call in the skill. Agents ran past it
  (60181, 63453, then 65988 tokens against a 60K check).
- Verify tickets carry no expected-token budget (`expected_context_window` is 0), so gx-evolve's
  overrun check has nothing to compare against.
- A round that stops at the token limit counts toward the 3-round cap, so a big epic reaches the cap
  without ever checking every story.

## Solution

gx-to-tickets splits the verify up front. It reads the spec's User Stories and Verification
sections, estimates a token cost per story, and packs the stories into verify parts of at most 70K
expected tokens. Each part is its own `type: verify` ticket. It copies its stories and their
Verification entries word for word, so the session reads only its own ticket. All parts run in
parallel once code review is done.

Parts only record. Each part writes one line per story (pass, fail or unchecked) under `## Results`
in its own ticket. One collect ticket per round waits for every part of that round. It merges their
results into one table, decides what comes next, and opens the work: an inline fix, fix tickets,
the next round's parts, or a question for a person. The latest collect ticket is the one place to
read the state of the verify.

ralphloop enforces a token cap for verify tickets, 20K below the smart zone. At the cap it
interrupts the pane and sends one finish-up prompt, with no compaction. The session then follows the
cap-stop step for its role in gx-verify-epic. A part marks the rest of its stories unchecked. The
collect step opens extra parts for those stories in the same round. Only a round that rechecks
fixes moves the round number up, so a cap stop never uses up one of the 3 rounds.

A small epic whose stories fit in one part keeps one verify ticket, as today. That lone ticket does
both jobs: it checks the stories and it makes the collect decisions.

## User Stories

### Splitting the verify

1. As a person who runs an epic with about 40 user stories, I want its verify to finish without my
   help, so that I do not restart rounds by hand.
2. As gx-to-tickets, I want to estimate a token cost for each user story from the flows and checks
   in its Verification entry, so that I can pack stories by cost instead of by count.
3. As gx-to-tickets, I want to pack stories into verify parts of at most 70K expected tokens, so
   that each part fits in one session. A single story estimated above 70K gets a part of its own,
   with a note that it is over budget.
4. As gx-to-tickets, I want stories that use the same fixture scenario to stay in the same part, so
   that two parts do not set up the same scenario. The 70K budget wins: a scenario group over 70K is
   split across parts.
5. As gx-to-tickets, I want to publish one top-level `type: verify` ticket per part at publish time,
   so that the split exists before any verify runs.
6. As gx-to-tickets, I want to publish one collect ticket for round 1 next to the parts, so that the
   round has an owner from the start.
7. As gx-to-tickets, I want an epic whose stories fit in one part to get one verify ticket, as today,
   so that small epics do not pay for a collect step they do not need.
8. As the scheduler, I want every round-1 part to become eligible once the epic's code-review
   ticket is done, so that the parts run in parallel.
9. As a verify part, I want to start my own fixture server, so that parallel parts never share
   state.
10. As a person who published an epic without verify parts, I want `gx tickets ensure-verify` to stay
    a no-op when the epic already has a verify ticket, and to stamp out one lone verify ticket
    otherwise, so that every epic with user-visible behaviour still gets checked.

### What a part reads

11. As a verify part, I want my ticket to hold each story's text and its Verification entry, copied
    word for word from the spec, so that I do not read the spec.
12. As a verify part, I want my ticket to name the resolved spec path and the fixture scenario, so
    that no round guesses a path that does not exist.
13. As a verify part, I want to read only my own ticket, the `## Spec deviations` sections of the
    epic's tickets and, in a next round, my fix tickets' diffs, so that my setup stays small.
14. As a verify part, I want to open the spec only for a story whose entry is unclear, and only that
    story's lines, so that one unclear story does not cost a full spec read. The one other case is a
    story I add from a fix diff (story 53).
15. As a maintainer of gx-to-spec, I want a part that had to open the spec to file a follow-up with
    gx-follow-up, so that I learn which Verification entries are too vague.
16. As the `ensure-verify` lone ticket, I want my body to name the spec path when a spec exists, so
    that I read the right file. I have no story list and read the spec as today.

### Recording results

17. As a verify part, I want to write `- Story <n>: pass|fail|unchecked, <check or reason>` under
    `## Results` in my own ticket only, so that parallel parts never write to the same file.
18. As a verify part, I want to write each story's line right after I check that story, so that a
    cap stop loses no finished story.
19. As a verify part, I want to record each finding with enough detail to fix it, and never fix it
    myself, so that parallel parts never make conflicting commits.
20. As a person or agent, I want the latest collect ticket to hold one table of every story (story,
    verdict, covering check, round last checked, finding with fix ticket or inline-fix commit), so
    that I read one place to know where the verify stands.
21. As a collect ticket, I want to carry the table forward from the previous collect ticket, so that
    stories checked in earlier rounds keep their verdicts.
22. As a reader of results, I want every part and table to use the spec's story numbers, so that
    rows from different parts and rounds merge without guessing.
23. As a person, I want to read the table on the web ticket page and in `gx tickets show`, so that I
    need no new view.

### Deciding what comes next

24. As the scheduler, I want a collect ticket to become eligible only once every part of its round is
    done, so that it decides on complete results.
25. As a collect ticket, I want to fix exactly one small finding (30 lines or fewer, at most 2 files)
    inline and rerun the failed check, so that a trivial bug does not cost a whole round. If the
    rerun passes, there are no other findings except those sent to gx-follow-up, and the fix touched
    no code of other passed stories, I close the round as clean and open no next round.
26. As a collect ticket, I want to open one child fix ticket per finding when there is more than one
    finding or the fix is larger, so that each fix is a normal implement ticket.
27. As a collect ticket, I want to send a finding outside the epic's scope to gx-follow-up instead of
    fixing it, so that the verify does not grow the epic.
28. As a collect ticket, I want to open the next round's parts and collect ticket, rechecking the
    failed and unchecked stories plus every passed story I expect a fix to touch, judged from the
    finding, so that a fix cannot break a story silently. Story 53 catches what this guess misses.
29. As a collect ticket, I want to write next-round parts in the same format as round-1 parts, adding
    each failed or fix-touched story's finding and the fix ticket it rechecks, so that the next part
    knows what changed.
30. As the scheduler, I want a next-round verify ticket to wait for the tickets in its `blocked_by`,
    so that it does not recheck a fix that has not landed.
31. As a collect ticket of a clean round (every story passes, no findings), I want to close and end
    the verify, commitless unless I made the inline fix, so that the epic can finish. Findings sent to gx-follow-up do not stop a
    round from being clean; the table names each follow-up.
32. As a collect ticket of `Round: 3` that still has failing stories, I want to run `gx tickets ask`
    naming those stories, so that a person decides after two fix rounds.
33. As a lone verify ticket, I want to check the stories and also make the collect decisions, so that
    a one-part epic works as today.

### Budget, cap and rounds

34. As a ticket writer, I want to set `expected_context_window` on every verify ticket from its
    per-story estimates plus a fixed base for fixture start and the ticket read, so that the budget
    is never 0.
35. As gx-to-tickets, I want to set the budget on round-1 parts and the round-1 collect ticket, so
    that the first round has budgets.
36. As a collect ticket, I want to set the budget on the next-round parts and collect ticket I open,
    so that later rounds have budgets too.
37. As `gx tickets ensure-verify`, I want to set 70K as the budget on the lone ticket I stamp out, so
    that the safety net has a budget.
38. As ralphloop, I want to stop a verify session at 20K below the smart zone (110K with today's
    fixed 130K `SmartZoneCeiling`), so that no session runs past a fixed cap.
39. As ralphloop, I want the verify cap to follow the `SmartZone` run option, so that a change to the
    smart zone moves the cap with it. When the smart zone is 20K or less, there is no verify cap and
    only the smart zone applies.
40. As ralphloop, I want the cap to cover Claude and Codex panes, so that every harness gets the same
    stop.
41. As ralphloop, I want to interrupt the pane at the cap, the way smart-zone recovery does, then
    send one finish-up prompt per session with no compaction, so that the session stops mid-check
    and writes what it has.
42. As ralphloop, I want the finish-up prompt to point to the cap-stop step in gx-verify-epic for each
    role, so that one prompt works for parts, collect tickets and lone tickets.
43. As a verify part at the cap, I want to mark the remaining stories unchecked, write my results,
    stop my fixture server and report finished, so that the collect step can pick them up and no
    server keeps running.
44. As a lone verify ticket at the cap, I want to write my results, then open extra parts for the
    unchecked stories plus a collect ticket, all with the same `Round: N`, and write my results into
    that collect ticket as its starting table, so that the rest gets checked without a fix round and
    my results are part of the merge.
45. As a collect ticket at the cap, I want to write the current table with the decisions made so far,
    then open a second collect ticket for the same round that starts from that table, so that no
    decision is lost.
46. As a collect ticket after a part's cap stop, I want to open extra parts for the unchecked stories
    plus a second collect ticket with the same `Round: N`, so that a cap stop does not count as a
    round.
47. As a reader of the table, I want a story left unchecked at a cap stop to show `unchecked` until
    the extra parts of the same round run, so that the table never claims a check that did not
    happen.
48. As ralphloop, I want the smart zone to stay active for verify panes as a backstop, so that a
    session that ignores the finish-up prompt still gets compacted.
49. As a verify session, I want the skill to drop the manual `gx agent context-window` check, so that
    I spend no tokens on a check ralphloop now does.
50. As gx-evolve, I want to compare each verify part's actual tokens with its expected budget and
    per-story estimates, and open follow-ups when the estimates are off, so that the estimates
    improve over time.
51. As a collect ticket, I want to give a story that cap stops left `unchecked` for the second time
    in one round an extra part of its own, and to mark it `fail` with the finding "too large for one
    session" only when that solo part also stops at the cap, so that cap stops inside one round
    cannot loop forever and a small story is never called too large. For such a story I record every
    other decision and open all other work first, then run `gx tickets ask`; the opened work waits
    for the answer.
52. As a collect ticket, I want my ticket to hold the spec's `## Out of Scope` section, so that I can
    tell an out-of-scope finding without reading the spec. gx-to-tickets copies it into the round-1
    collect ticket. Every step that opens a collect ticket (the collect step, and the lone and
    collect cap-stops) copies it forward. A lone ticket without the section copies it from the spec
    it already reads.
53. As a next-round verify part, I want to read the diff of the fix tickets in my `blocked_by`
    before I start, and add any other passed story whose code that diff changed, so that a fix that
    touched more than the collect step expected still gets rechecked.

## Implementation Decisions

### Scope

- Only `type: verify` tickets change: the gx-verify-epic skill, the verify part of gx-to-tickets,
  `gx tickets ensure-verify`, the scheduler's verify blockers, and how ralphloop runs a verify
  session. Implement tickets keep their current estimates and smart zone.

### Roles

- A verify ticket has one of three roles: part, collect or lone. All three keep `type: verify`. The
  implementer picks how a ticket names its role (a body marker, a frontmatter field or a section).
  The skill reads the role at step 1 and follows the steps for that role.
- A part checks stories and records every finding. It never fixes, never judges scope and never
  opens tickets, except a gx-follow-up for an unclear spec entry.
- A collect ticket checks nothing by itself except the one inline fix's rerun. It merges, decides and
  opens work.
- A lone ticket checks like a part and decides like a collect ticket. One-part epics and the
  `ensure-verify` stub are lone tickets.

### gx-to-tickets

- After the implement tickets and the code-review ticket, gx-to-tickets estimates each story's token
  cost from the flows and checks in its Verification entry. The skill gives the agent a rule for
  this estimate. The implementer picks the rule and the fixed base estimate for fixture start and
  the ticket read.
- It packs stories greedily into parts under 70K expected tokens. Stories that share a fixture
  scenario stay in one part. The 70K budget wins over that rule: a scenario group over 70K is split
  across parts, and a single story over 70K gets a part of its own with a note that it is over
  budget. If the whole list fits in one part, it publishes one lone verify ticket instead of parts
  and a collect ticket.
- It copies the spec's `## Out of Scope` section into the round-1 collect ticket (or the lone
  ticket).
- Each part body holds: `Round: 1`, the resolved spec path, the fixture scenario, and per story its
  spec number, its text and its Verification entry word for word, plus its estimate. The layout and
  section names are the implementer's choice.
- It sets `expected_context_window` on every part (base plus its stories' estimates) and on the
  collect ticket. The collect ticket's fixed estimate is the implementer's choice.
- The collect ticket names its round's parts in its `blocked_by`, or another way the implementer
  picks, so the scheduler can tell when the round is done. Parts must not be children (`parent`) of
  their collect ticket: a child waits for its parent, which would deadlock.
- `gx tickets ensure-verify` still runs at the end as a no-op safety net.

### `gx tickets ensure-verify`

- Unchanged when the epic already has a verify ticket.
- Otherwise it stamps out a lone verify ticket with `expected_context_window` set to 70K. When
  `docs/specs/<epic-slug>.md` or the epic's `spec.md` exists, the body names that path. The rest of
  the body stays as it is.

### Scheduler

- `verifyBlockers` today synthesizes only the epic's code-review tickets and ignores a verify
  ticket's own `blocked_by`. It changes so a verify ticket also waits for the tickets in its literal
  `blocked_by` (fix tickets, and a collect ticket's parts). The implementer picks how.
- Verify tickets with no `blocked_by` still never wait on each other, so round-1 parts run in
  parallel once code review is done.
- Code-review tickets keep ignoring every ticket in a verify subtree.
- Round-1 parts and the round-1 collect ticket have no `parent`: they are top-level tickets of the
  epic.
- Every ticket a collect ticket or a lone ticket opens (fix tickets, next-round parts and collect
  ticket, extra parts, a second collect ticket) is its child (`parent`). A child waits for its
  parent to finish, as code-review fix tickets do today. This is safe because the parent is done
  before the child can start.

### ralphloop verify cap

- For a `type: verify` ticket, ralphloop's context check uses a second limit: the smart zone minus
  20K (110K with the default `SmartZoneCeiling` of 130K in `ralphloop/loop.go`). It follows the
  `SmartZone` run option; no user setting changes it. It reads the same context occupancy the smart
  zone check reads, so it covers Claude and Codex panes.
- At the cap, ralphloop interrupts the pane the way smart-zone recovery does, then sends one
  finish-up prompt. It does not compact. It sends the prompt once per session. A later poll above
  the cap does nothing.
- The prompt says the session hit the verify token cap and must follow the cap-stop step in
  gx-verify-epic for its role. The wording is the implementer's choice.
- The smart zone still applies to verify panes. If a session passes the smart zone after the
  finish-up prompt, normal smart-zone recovery runs.
- The event sink and the agent log record the cap stop as their own event. The name is the
  implementer's choice.
- If the smart zone is 20K or less, there is no verify cap and only the smart zone applies.

### gx-verify-epic skill

- Step 1 reads the role and the `Round: N` line. A part or a collect ticket reads only its own
  ticket, plus the `## Spec deviations` sections of the epic's tickets. The implementer picks how a
  session reads only those sections: one command, or a list the collect step writes into the parts.
  A lone ticket reads the spec as today.
- A part opens the spec, at the path in its ticket, only for an unclear story's lines. Each time it
  does, it files a follow-up for gx-to-spec with gx-follow-up.
- A part writes each story's line under `## Results` right after it checks that story, in the
  existing format. Findings go below the lines. It never fixes.
- The manual `gx agent context-window` check and the 90K stop leave the skill.
- A next-round part, before it checks anything, reads the diff of the fix tickets in its
  `blocked_by` and adds any other passed story whose code that diff changed. It copies each added
  story's text and Verification entry from the spec, the one other case where a part opens the spec.
  This read files no gx-to-spec follow-up.
- The cap-stop step, per role. Every role that started a fixture server stops it before it reports
  finished.
  - Part: mark the remaining stories `unchecked`, write the results, report finished.
  - Lone: write the results, then open extra parts for the unchecked stories plus a collect ticket,
    all with the same `Round: N`. The lone ticket writes its results into that collect ticket as
    its starting table and does not name itself in `blocked_by`: the collect ticket is its child,
    and `Blocking` waits on the named ticket's whole fork subtree, so it would wait for itself.
    Then report finished.
  - Collect: write the current table with the decisions made so far, then open a second collect
    ticket for the same round, same `Round: N`, that starts from that table, then report finished.
  - Lone and collect both copy `## Out of Scope` into the collect ticket they open. A lone ticket
    without the section copies it from the spec.
- The collect step:
  - Reads `## Results` from every verify ticket in its `blocked_by`, and its starting table (the
    previous collect ticket's table, or the results a lone cap-stop wrote into it). Writes one table for all stories with the columns story, verdict, covering
    check, round last checked, and finding with fix ticket, follow-up or inline-fix commit. The
    section name is the implementer's choice.
  - Any unchecked story from a cap stop: opens extra parts and a second collect ticket with the same
    `Round: N`. The table records each cap stop that left a story unchecked; the implementer picks
    the form. Counts cover one round only. A story left unchecked for the second time in a round
    gets an extra part of its own. If that solo part also stops at the cap before it finishes the
    story, the story is marked `fail` with the finding "too large for one session". The collect
    step then records every other decision and opens all other work first, and runs
    `gx tickets ask` for the story last. The work it opened is its children, so it waits for the
    answer.
  - Findings: exactly one small finding (30 lines or fewer, at most 2 files) is fixed inline, the
    failed check rerun, and the commit named in the row. Otherwise one child fix ticket per finding.
    Out-of-scope findings, judged against the `## Out of Scope` section in its own ticket, go to
    gx-follow-up.
  - Opens the next round (`Round: N+1`) when there are fix tickets, or when an inline fix's rerun
    failed or touched code of other passed stories. Next-round parts cover the failed and unchecked
    stories plus every passed story it expects a fix to touch, judged from the finding. They use
    the round-1 format plus each story's finding and fix ticket. It also opens a new collect ticket.
    Each next-round part names its fix tickets in `blocked_by`. The collect step sets every new
    ticket's `expected_context_window`, and copies its `## Out of Scope` section into every collect
    ticket it opens.
  - At `Round: 3` with stories still failing, it opens no next round and runs `gx tickets ask`
    naming those stories, following the announce-and-stop rule.
  - A clean round closes and ends the verify, commitless unless it made the inline fix. A round is clean when every story passes
    and every finding is either fixed inline with a passing rerun that touched no other passed
    story's code, or sent to gx-follow-up.
- The 3-round cap counts the `Round: N` line, as today. Only a fix round raises N, so cap stops do
  not use up rounds.

### gx-evolve

- Its overrun check reads the verify budgets like any other ticket's. For verify parts it also
  compares actual tokens with the sum of the per-story estimates, and opens follow-ups to adjust the
  estimate rule in gx-to-tickets when they are off. No new check type.

## Testing Decisions

- A good test drives the public seam and checks what a person or the scheduler sees: which tickets
  are eligible, what ralphloop sends to the pane, what a command writes. It does not check internal
  helpers.
- Seam 1, the scheduler: `tickets` epic status and the ralphloop frontier. Tests build an epic in
  memory with parts, a collect ticket, fix tickets and next-round parts, and assert eligibility.
  They cover the parent layout: top-level round-1 parts and collect ticket, and children of a
  collect or lone ticket that wait for it to finish.
  Prior art: `TestEpic_RenderedStatus_VerifyBlockedWhileCodeReviewNotDone`,
  `TestEpic_RenderedStatus_VerifyTicketsDontBlockEachOtherOrWaitOnChildren` and
  `TestFrontier_VerifyEntersOnlyAfterCodeReviewDone`.
- Seam 2, ralphloop's wait-for-finish loop with a fake runner that reports context occupancy. Tests
  assert the interrupt, the single finish-up prompt, no `/compact`, no cap for non-verify tickets,
  the cap moving with the smart zone, both agents, and the smart-zone backstop after the cap. Prior
  art: `TestRun_SmartZoneBreach_AutoRecoversWithoutBlockingScheduler` and the
  `TestRecoverSmartZoneBreach_*` tests.
- Seam 3, the `ensure-verify` command against a temp store. Prior art:
  `TestRunTicketsEnsureVerify_CreatesOneValidStubThenNoops`.
- The skill changes (gx-to-tickets, gx-verify-epic, gx-evolve) have no unit tests. The verify ticket
  checks their text, and the first large epic run after this one checks them in practice.

## Verification

This epic changes ralphloop and scheduler behaviour, one CLI verb and skill text. It changes no web
page, so no flow-catalog flow covers it. Checks use Go tests, a temp ticket store and the skill
files. "Skill text" means reading the named skill file and quoting the line that covers the story.

1. Covered by stories 2 to 53 together. Skill text: gx-to-tickets, gx-verify-epic and gx-evolve
   carry the split, cap-stop and collect steps. No end-to-end check, since a 40-story epic run does
   not fit in a verify.
2. Skill text: gx-to-tickets gives a per-story estimate rule based on the Verification entry's flows
   and checks.
3. Skill text: gx-to-tickets packs parts under 70K expected tokens, and puts a single story over 70K
   in a part of its own with an over-budget note.
4. Skill text: gx-to-tickets keeps stories that share a fixture scenario in one part, and splits a
   scenario group over 70K across parts.
5. Skill text: gx-to-tickets publishes one top-level `type: verify` ticket per part at publish time.
   Go test (seam 1): a round-1 part with no `parent` is eligible once code review is done.
6. Skill text: gx-to-tickets publishes one round-1 collect ticket that names its parts.
7. Skill text: gx-to-tickets publishes one lone verify ticket when the stories fit in one part.
8. Go test (seam 1): with code review done, every round-1 part with no `blocked_by` is in the
   frontier at once; with code review open, none is.
9. Skill text: gx-verify-epic step 2 starts a fixture server per part session.
10. CLI check: in a temp store, `gx tickets ensure-verify <epic>` on an epic with a verify ticket
    prints the no-op message and adds no file; on an epic without one it adds one lone verify
    ticket, and `gx tickets validate` passes on it. Go test (seam 3) covers both.
11. Skill text: gx-to-tickets copies each story's text and Verification entry word for word into the
    part.
12. Skill text: gx-to-tickets writes the resolved spec path and the fixture scenario into each part.
13. Skill text: gx-verify-epic step 1 tells a part to read only its own ticket, the epic's
    `## Spec deviations` sections and, in a next round, its fix tickets' diffs. If the implementer
    added a command for those sections, run it on a temp epic and check it prints only those
    sections.
14. Skill text: gx-verify-epic opens the spec only for an unclear story's lines, or for a story a
    next-round part adds from a fix diff.
15. Skill text: gx-verify-epic files a gx-follow-up for gx-to-spec each time a part opens the spec.
16. CLI check: in a temp store with `docs/specs/<epic-slug>.md`, the stub from
    `gx tickets ensure-verify` names that path; without a spec it names none. Go test (seam 3).
17. Skill text: a part writes the story line format under `## Results` in its own ticket only.
18. Skill text: a part writes each line right after it checks that story.
19. Skill text: a part records findings and never fixes or commits.
20. Skill text: the collect step writes one table with the five named columns. CLI check:
    `gx tickets show` on a temp ticket with such a table prints it.
21. Skill text: the collect step carries the table forward from the previous collect ticket.
22. Skill text: parts and tables use the spec's story numbers.
23. Existing web ticket page renders markdown tables; no new check beyond story 20's `gx tickets show`.
24. Go test (seam 1): a collect ticket whose `blocked_by` names its parts is not eligible while any
    part is open, and is eligible once all are done.
25. Skill text: the collect step fixes exactly one small finding inline and reruns the failed check,
    and closes the round as clean when the rerun passes, no other finding exists except those sent
    to gx-follow-up, and the fix touched no other passed story's code.
26. Skill text: the collect step opens one child fix ticket per finding otherwise.
27. Skill text: the collect step sends out-of-scope findings to gx-follow-up.
28. Skill text: the collect step opens next-round parts for failed and unchecked stories plus the
    passed stories it expects a fix to touch, judged from the finding, plus a collect ticket. Go
    test (seam 1): a next-round part whose `parent` is an open collect ticket is not eligible; once
    the collect ticket is done and its `blocked_by` fix tickets are done, it is.
29. Skill text: next-round parts use the round-1 format plus the finding and fix ticket per story.
30. Go test (seam 1): a next-round verify part whose `blocked_by` names an open fix ticket is not
    eligible; once the fix ticket is done, it is.
31. Skill text: a clean round closes the collect ticket, commitless unless it made the inline fix,
    and findings sent to
    gx-follow-up do not stop a round from being clean; the table names each follow-up.
32. Skill text: at `Round: 3` with failing stories the collect step runs `gx tickets ask` naming them
    and opens no next round.
33. Skill text: a lone ticket follows both the part steps and the collect steps.
34. Skill text: every ticket writer sets `expected_context_window` from per-story estimates plus a
    base.
35. Skill text: gx-to-tickets sets the budget on round-1 parts and the collect ticket.
36. Skill text: the collect step sets the budget on every ticket it opens.
37. CLI check: the stub from `gx tickets ensure-verify` has `expected_context_window: 70000` (or the
    store's 70K form) in its frontmatter. Go test (seam 3).
38. Go test (seam 2): a verify ticket at occupancy above smart zone minus 20K gets an interrupt and
    the finish-up prompt; an implement ticket at the same occupancy gets neither.
39. Go test (seam 2): with a non-default `SmartZone` run option, the cap is that smart zone minus
    20K. With a smart zone of 20K or less, a verify ticket gets no cap stop and only smart-zone
    recovery.
40. Go test (seam 2): the cap fires for both the Claude and the Codex agent.
41. Go test (seam 2): at the cap the fake runner sees an interrupt, then one prompt, and no
    `/compact`; a second poll above the cap sends nothing more.
42. Go test (seam 2): the finish-up prompt names gx-verify-epic's cap-stop step. Skill text: the
    cap-stop step exists for part, collect and lone, and each role stops its fixture server before
    it reports finished.
43. Skill text: the part cap-stop marks remaining stories unchecked, writes results, stops its
    fixture server and reports finished.
44. Skill text: the lone cap-stop opens extra parts plus a collect ticket with the same `Round: N`,
    and writes the lone ticket's results into that collect ticket as its starting table, without
    naming the lone ticket in `blocked_by`. Go test (seam 1): a collect ticket whose `parent` is a
    done lone ticket and whose `blocked_by` names only its extra parts becomes eligible once those
    parts are done.
45. Skill text: the collect cap-stop writes the table and opens a second collect ticket for the same
    round.
46. Skill text: the collect step opens extra parts plus a second collect ticket with the same
    `Round: N` for stories unchecked at a cap stop. Go test (seam 1): a second collect ticket
    waits for the extra parts in its `blocked_by`.
47. Skill text: an unchecked story stays `unchecked` in the table until its extra part runs.
48. Go test (seam 2): a verify session above the smart zone after the finish-up prompt gets normal
    smart-zone recovery.
49. Skill text: gx-verify-epic no longer mentions `gx agent context-window` or a 90K stop. Run
    `rg -n 'context-window|90K' skills/gx-verify-epic/SKILL.md` and expect no match.
50. Skill text: gx-evolve compares verify parts' actual tokens with their budgets and per-story
    estimates and opens follow-ups when they are off.
51. Skill text: the collect step gives a story left unchecked for the second time in one round a solo
    extra part; when that solo part stops at the cap, it marks the story `fail` with "too large for
    one session", opens all other work first and runs `gx tickets ask` last. Go test (seam 1):
    children of a paused collect ticket are not eligible until it is unpaused and done.
52. Skill text: gx-to-tickets copies the spec's `## Out of Scope` into the round-1 collect or lone
    ticket; the collect step and the lone and collect cap-stops copy it into every collect ticket
    they open; a lone ticket without it copies it from the spec; the collect step judges
    out-of-scope findings against it. Skill text: a part never judges scope.
53. Skill text: a next-round part reads the diff of the fix tickets in its `blocked_by` before it
    starts and adds any other passed story whose code that diff changed.

## Out of Scope

- Context estimates for implement tickets. That is gx:agent-skills/01.
- A dedicated web or TUI view of verify results. The ticket page and `gx tickets show` already
  render the table. A view can be a follow-up.
- Sub-agents inside a verify session.
- Changes to the smart zone itself or to implement-ticket smart-zone recovery.

## Further Notes

- Source: gx:follow-ups/15, from the retro of gx:united-epics-page-impl (gx:one-offs/43). Ticket
  gx:verify-epic-budget/01 holds the evidence. Tickets 02 to 07 hold the decisions; 07 settled the
  cap edge cases (round 3 ask, lone and collect cap stops, interrupt before the prompt, both
  harnesses).
- The cap (110K today, from the fixed 130K `SmartZoneCeiling`) sits above the 70K packing budget on
  purpose. The 40K gap absorbs estimate error, and the cap only catches a part whose estimate was
  badly low. Ticket 06 first listed 110K as the smart zone; the rule "smart zone minus 20K" is what
  the person chose, so the cap follows it.

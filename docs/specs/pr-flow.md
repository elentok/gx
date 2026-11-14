# PR flow

Handoff spec from the `gx:pr-flow` map (decision tickets 01 to 10 in the ticket store). It is handed
off as the `pr-flow-impl` epic. Terms follow `GLOSSARY.md`: **Delivery**, **Auto-PR**, **Awaiting
PR**, **ci-fix ticket**, **pr-text ticket**, **Fixing CI**, **PR activity list**, **"CI failing"
pause**, **"PR text failed" pause**, **Merge ticket**, **Merge blocked**, **Complete epic**,
**Merged epic**, **Trunk**. ADRs respected: 0015 (gx merge core), 0019 (status and iteration status),
0026 (markdown is truth), 0028 (server API mirrors the CLI), 0042 (few pauses, all recoverable from
the web). The Settings page work is `settings-page.md`; this spec adds registry entries under its
rules.

## Problem Statement

In some projects the person merges an epic's branch straight into trunk. In others they open a pull
request, wait for CI, fix whatever CI finds, and merge on GitHub. gx only knows the first way. For the
second, the person pushes by hand, writes the PR text, watches CI, starts an agent for each failure,
and gx still shows the epic as complete or merge-ready while the PR is open. An epic whose PR has not
merged is not done, but gx cannot tell.

## Solution

A project setting, **Delivery**, is `merge` (today's behavior, the default) or `pr`. In a `pr`
project a complete epic shows **Awaiting PR** and offers Create PR. Create PR (or **Auto-PR**, which
opens it by itself) has an agent write the title and body, pushes the epic branch, and opens a draft
PR. The server then watches the PR through `gh`. Each failed CI run gets one short **ci-fix ticket**
that fixes it and pushes. When CI is green the PR is marked ready for review. The person merges on
GitHub; gx sees the merge, updates local trunk and the epic becomes a **Merged epic**, which releases
the epics waiting on it. When gx cannot fix CI it pauses the epic ("CI failing") with a Try again
button. The epic page shows a **PR activity list** of what gx did.

## User Stories

1. As a person with personal projects, I want Delivery to stay `merge` by default, so that nothing changes for projects where I merge straight into trunk.
2. As a person with team projects, I want to set a project's Delivery to `pr`, so that its epics go through a pull request.
3. As a person, I want `auto-pr` next to `auto-ff-merge` in a `pr` project, so that a finished epic opens its PR without me pressing a button.
4. As a person, I want Delivery, `auto-pr`, `skills.ci-fix` and `skills.pr-text` on the web Settings page, so that I change them without editing JSON.
5. As a person, I want a finished epic in a `pr` project to show "Awaiting PR" in Needs you, so that I know it waits for me.
6. As a person, I want a Create PR button on the epic card and the epic page header, so that I open the PR from where I see the epic.
7. As a person with a `merge` project, I want Create PR as the second button next to Merge, so that I can send one epic through a PR.
8. As a person, I want Create PR to check that `gh` is installed, logged in, has access to the repo and that `origin` exists before anything is pushed, so that a failure leaves the epic untouched.
9. As a person, I want the reason (for example "gh is not logged in, run `gh auth login`") shown on the epic card and page when that check fails, so that I know the fix, without a pause.
10. As a person, I want the PR title and body written by an agent from the spec, ticket titles and diff, so that I do not write them.
11. As a person, I want `skills.pr-text` to name a skill the text agent follows, so that PR text matches my team's style, with a bundled default when it is empty.
12. As a person, I want the PR never to open without valid text, so that I never get an empty or placeholder PR.
13. As a person, I want a "PR text failed" pause with Try again after the text ticket's retries run out, so that a failing text agent does not silently block the epic.
14. As a person, I want the PR opened as a draft, so that reviewers are not pinged while CI is red.
15. As a person, I want the PR marked ready for review once CI is green, so that review starts on a green PR.
16. As a person, I want the PR to open only after every ticket, including code-review and verify, is done, so that review sees finished work.
17. As a person, I want the server to poll the PR's CI through `gh` with no agent waiting, so that watching costs no agent time.
18. As a person, I want each failed CI run to start one short `ci-fix` ticket, so that fixes are small and each leaves a record.
19. As a person, I want the fix agent to read every earlier attempt on the PR first, so that it does not repeat a change that failed.
20. As a person, I want the fix agent to run the failing check locally and never skip hooks, force-push over foreign commits or edit CI config, so that fixes are honest.
21. As a person, I want the server to rerun a failed job once before it starts a fix, and a flaky run recorded with a follow-up filed, so that no agent waits on CI and flakes get fixed later.
22. As a person, I want a read-only strongest-tier reviewer to check the fix before it is pushed, so that bad fixes are caught.
23. As a person, I want my own CI rules in `skills.ci-fix`, overridable per project, so that the fix agent knows my repo's quirks.
24. As a person, I want at most 3 fix attempts per failing check and 10 per PR, so that gx does not loop forever.
25. As a person, I want an `Outcome: unfixable` fix to pause the epic at once, so that gx does not burn attempts on a failure no code change fixes.
26. As a person, I want a "CI failing" pause in Needs you with the reason, the PR link and Try again, so that I can step in or give gx more attempts.
27. As a person, I want Try again to reset the counts, so that gx gets a fresh budget.
28. As a person, I want gx to clear the "CI failing" pause itself when I fix CI by hand and the run turns green, so that I do not press anything.
29. As a person, I want one notification when gx gives up, and none per attempt, so that I am not spammed.
30. As a person, I want the epic to show "Fixing CI (attempt N of 3)" while a fix ticket runs, outside Needs you, so that Needs you lists only what needs me.
31. As a person, I want a PR with no checks to count as green after a short grace window, so that a repo without CI is not stuck as a draft.
32. As a person, I want a check that appears after the window to start the normal fix loop, so that late checks are handled.
33. As a person, I want CANCELLED, STALE and ACTION_REQUIRED checks left for me to rerun on GitHub, so that gx does not "fix" a check that is not a code failure.
34. As a person, I want checks on older commits ignored, so that a superseded run does not start a fix.
35. As a person, I want a conflicting PR branch rebased by a Merge ticket, up to 3 Merge tickets and then Merge blocked, so that conflicts resolve themselves within the existing limit.
36. As a person, I want every ticket that pushes to fetch and rebase onto the remote branch first and never overwrite foreign commits, so that a teammate's push is safe.
37. As a person, I want a rebase conflict with foreign commits to show Merge blocked with the reason and a Merge again button for after I resolve it, so that I resolve it.
38. As a person, I want the epic to stay unfinished until its PR merges on GitHub, so that Merged epic means merged.
39. As a person, I want gx to read the merge from the PR's state, so that squash merges are detected.
40. As a person, I want gx to fetch and fast-forward local trunk after the merge, so that dependents start from code that holds the merge.
41. As a person, I want a failed trunk update after the PR merged to show Merge blocked with the reason and buttons that retry the fetch and fast-forward, so that I can fix local trunk and go on.
42. As a person, I want epics that depend on this one to start only once trunk holds the merged code, even when the project uses `landing: on-done`, so that they build on the right base.
43. As a person, I want a PR closed without merging (or its remote branch deleted) to return the epic to Awaiting PR with "PR closed without merging" in the list, and no automatic reopen, so that I decide what comes next.
44. As a person, I want an open PR for the epic's branch that I made by hand to be adopted, and a merged one to make the epic merged, so that gx does not make duplicates.
45. As a person, I want a crash between GitHub creating the PR and gx writing its number to cause no duplicate PR, so that a restart is safe.
46. As a person, I want a PR that merges while a fix or Merge ticket runs to win: the ticket is cancelled and the post-merge steps run, so that no stale work lands.
47. As a person, I want "Mark as merged" on the card and page, and `gx server epics mark-merged <project:epic>`, so that a merge done outside gx is recorded.
48. As a person, I want the mark-merged verb to refuse an incomplete epic or one with a running or queued ticket, so that it cannot hide unfinished work.
49. As a person, I want gx-cleanup to call mark-merged after it verifies a merge, so that it never writes `merged_at` itself.
50. As a person, I want a PR chip (`#412 · draft`, `· merged`) and an "Open on GitHub" link on the card and the page header once the PR exists, so that I reach it in one click.
51. As a person, I want a PR line on each card with a PR: number chip, check summary and link, so that I see CI state without opening GitHub.
52. As a person, I want the epic page to show a PR activity list under the header, newest first, with fix entries linking to their ticket, so that I see what gx did and why.
53. As a person, I want a "GitHub unreachable" line and retry with backoff when `gh` fails while watching, and no pause, so that a network blip does not stop the epic.
54. As a person, I want my PR to be kept across a server restart, so that a restart does not lose it.
55. As a person, I want Delivery and `auto-pr` to be read live, so that a change applies on the next tick.
56. As a person, I want an epic with an open PR to keep PR tracking after I switch to `merge` until the PR merges or closes, so that a switch never orphans a PR.
57. As a person, I want turning `auto-pr` off while the text ticket runs to let the flow finish, so that a half-started flow does not strand.
58. As a person, I want Awaiting merge and Awaiting PR epics to follow a changed Delivery on the next tick, and a running text ticket cancelled when a no-PR epic switches to `merge`, so that the setting is honored.
59. As a person, I want a running Merge ticket to finish without interruption when I switch `merge` to `pr`, and then the epic waits for a PR, so that nothing reaches main behind my back.
60. As a person, I want `auto-pr` ignored in a `merge` project while the manual Create PR button still works, so that the key has one meaning.
61. As a person, I want one-offs to stay out of the PR flow, so that commitless work never opens a PR.
62. As a contributor, I want every new pause kind to have a row in the pause-to-verb table and a web button, so that ADR 0042 holds.
63. As a person, I want the epic to show "Writing PR text" in Running while the text ticket runs, so that the epic does not look idle or incomplete.
64. As a person, I want every way into Merge blocked in a `pr` project to name the button that ends it, so that I am never stuck.

## Implementation Decisions

### Delivery and auto-pr

- `project.json` gains `delivery` (`merge` | `pr`, default `merge`) and `auto-pr` (bool, default
  false), beside `auto-ff-merge`. Both are project values and are read live on every server tick, with
  no per-epic snapshot, no epic field and no migration. Both keys are added to `projectKeys`
  (`config/project_overrides.go`) and `ProjectFile`; `delivery` is validated like `landing`, and an
  unknown value loads as `merge`. A Go test covers the keys and the fallback.
- `auto-ff-merge` is ignored in a `pr` project, and so is `landing: on-done`: dependents are released
  on the trunk update only (a squash merge rewrites the blocker's branch). A Go test covers it.
- `auto-pr` is ignored in a `merge` project; the manual
  Create PR still works there.
- The PR opens on `origin`, with the project's Trunk as base. One-offs never use the flow.
- Mid-flow changes: an epic with `pr_number` set keeps PR tracking (polling, fix tickets, chip,
  timeline; the plain Merge action is not offered) until the PR merges or closes, then follows the
  current Delivery. `pr` to `merge` on an Awaiting PR epic makes it Awaiting merge on the next tick
  and cancels a running `pr-text` ticket (no PR opens). `merge` to `pr` makes an Awaiting merge epic
  Awaiting PR on the next tick; a running Merge ticket finishes, nothing is merged, and the epic is Awaiting PR. `auto-pr` off while a
  `pr-text` ticket runs or retries lets it finish and the PR open; `auto-pr` on opens the PR for an
  Awaiting PR epic on the next tick through the same `gh` check as the button.

### Opening the PR

- Create PR and Auto-PR do the same thing, in order: check `gh` (installed, logged in, access to the
  repo, `origin` present; on failure change nothing, show the reason on the card and page, and check
  again on the next attempt); look for an open PR on the head branch and adopt it if found (records
  its number, adds "adopted PR" to the list; a merged PR on the branch makes the epic a Merged epic;
  a closed one is ignored); otherwise run the `pr-text` ticket, push the branch, and open a draft PR
  with `--head`, `--base`, `--title` and `--body-file` and `-R owner/repo` on every call. The lookup
  before creating closes the crash gap.
- The PR opens only once every ticket in the epic except `pr-text` tickets is done. The epic must be a
  Complete epic, ignoring `pr-text` tickets.
- The server pushes non-interactively (`GIT_TERMINAL_PROMPT=0`, ssh BatchMode). A failed push is
  handled like a failed `gh` check: the reason shows on the card and page, no pause, and the next
  attempt reuses the existing `pr-text` Resolution.
- Create PR is a new server route with a CLI verb, per ADR 0028 and the "Adding a server route" rule
  in `.ai/index.md`. Names are the implementer's choice.

### PR text

- A `pr-text` ticket (new system-generated ticket type) runs on the light tier, commitless, in the
  epic's feature worktree, which it only reads. A `finishCheck` validates `Title:` and `Body:`; a
  failed `finishCheck` closes the attempt as cancelled with the reason (it never pauses the ticket),
  the server forks the next `pr-text` ticket, and after the third it sets `pr-text-failed`.
  `TypePRText` joins `IsCommitless` and `commitlessByDesignTypes`, and `IterationModeFor` gives it
  Commitless with `ScratchDir` set to the feature worktree. While it runs the epic shows "Writing PR text" in Running
  (derived like Merging, not in Needs you). Awaiting PR and the open gate ignore `pr-text` tickets. It receives the epic
  address and the worktree path, reads the spec, ticket titles and diff itself, and writes `Title:`
  (one non-empty line) and a `Body:` section (non-empty) in its Resolution. gx checks nothing else.
  A long body is trimmed to GitHub's size limit.
- `skills.pr-text` names one skill the agent reads and follows (it is not run as a command). When
  empty, a bundled default skill runs: title is the epic title, body is a short summary, a link to the
  spec when one exists (otherwise the epic description) and a checklist of closed tickets.
- An invalid Resolution is a failed attempt. After the first try and two retries the epic shows the
  "PR text failed" pause with the last failure reason, and no PR opens. Auto-PR follows the same path.
  Why a pause (ADR 0042): gx cannot write a reviewable description on its own, and the web Try again
  button ends it. The text is written once at opening and never updated.

### Watching the PR

- The server polls through `gh` (the server so far never ran `gh`; the existing `git/pr` helpers for
  running `gh`, GraphQL requests and classifying errors are reused and extended with exit 4, 403, 404
  and 429 kinds). One GraphQL query per poll covers all watched PRs for cheap fields (state, draft,
  head SHA, merge commit, mergeable); the per-check detail (check name, status, conclusion, job id,
  run id, workflow id) is fetched when the rollup is failing or changed, or the head SHA changed. The interval is
  30 to 60 seconds while CI is pending and 2 to 5 minutes otherwise (the implementer picks the exact
  values). Rate limits and `retry-after` back
  off.
- Merge is detected by PR state MERGED, never by ancestry (a squash merge leaves no ancestor).
  Mergeable state is read only while the PR is OPEN; CONFLICTING (or merge state DIRTY) means a
  conflict.
- A check is keyed `(workflow id, check name)`; legacy statuses by their context. A failed
  StatusContext, or a check with no workflow run, is keyed `(null workflow, name)` and starts a ci-fix
  ticket that gets the check name and its `targetUrl`/`detailsUrl` in place of run and job ids; the
  agent reads what it can and otherwise reports `unfixable`. Only checks on the
  PR's current head count. FAILURE, TIMED_OUT and STARTUP_FAILURE are failed. CANCELLED, STALE and
  ACTION_REQUIRED are neither failed nor green: the epic stays "PR draft · CI running", no fix starts,
  and the person reruns or fixes the check on GitHub. STALE is GitHub's own conclusion for a run it
  gave up on, not a rebase signal; rebases come from CONFLICTING. The existing TUI check logic (`failedCheckConclusions`, `git/pr/pr.go`) is corrected to match: it
  counts STARTUP_FAILURE as failed, drops CANCELLED and ACTION_REQUIRED from the failed set, and maps
  CANCELLED, STALE and ACTION_REQUIRED to pending.
- Green means every check on the head is COMPLETED with SUCCESS, NEUTRAL or SKIPPED. When the head reports no checks,
  gx waits a grace window after the PR opens or the branch is pushed; checks that appear are handled
  as usual; if none appear by the end of the window the PR counts as green. A check that appears later
  starts the normal fix loop. The window is a global `config.json` key with a 2 minute default; the
  implementer picks its name.
- On green, gx marks the PR ready for review (only when it is still a draft). On a `gh` failure while
  watching, gx keeps the last known state, adds a "GitHub unreachable" entry (once per outage, plus "GitHub reachable again"
  when it ends) and retries with backoff; it never pauses.
- Delivered state in the PR is live data (head SHA, CI state, draft, mergeable) and is never stored.

### Fixing CI

- `ci-fix` is a new system-generated ticket type, created by the server in a complete epic, never
  authored by a person. It runs like a Merge ticket (the same iteration mode) in the epic's worktree
  on the epic branch; its commits stay on the epic branch and are not cherry-picked. `TypeCIFix`
  joins `IsCommitless` like Merge. "No change" and `unfixable` are read from the Resolution's
  Outcome and never trip the zero-commit pause. At most one is
  open per epic, so failed runs are fixed one at a time. One ticket is created per failed run.
- It counts as an ordinary ticket for Complete epic. Code-review and verify do not rerun after a fix.
  Only the state derivation treats it specially.
- The bundled skill `gx-ci-fix` (from the dotfiles `babysit-ci`) has no polling and no loop. For one
  failed run it: reads every earlier `ci-fix` Resolution on the PR first; fetches the log with the
  run and job ids gx passes in; runs the failing check locally with the repo's own
  commands; loads each skill in `skills.ci-fix` after reading the failure and follows it as the
  person's own rules; has a read-only reviewer subagent on the strongest tier check the diff against
  the failure before the push; fetches, rebases and pushes the epic branch without overwriting
  (`--force-with-lease` stays); never skips hooks, never force-pushes over foreign commits and never
  edits CI config to turn a check off.
- Before it creates a ci-fix ticket the server reruns a failed job once (`gh run rerun --job`), only
  after its workflow run has completed, and never for a check with no workflow run. It writes a "reran
  <check>" entry (check key and head SHA) to `pr-activity.jsonl` and reruns at most once per (check
  key, head SHA), so a restart does not rerun again. If the rerun passes, the check counts as green
  for the PR, the server adds "flaky: <check>" to the activity list and writes the follow-up itself
  through the ticket-write code into the project's `follow-ups` epic (fixed sections: check, run
  URL, head SHA), skipping it when an open follow-up already names the check key; if it fails, the normal ci-fix ticket starts. The agent never
  reruns, so no agent waits on CI.
- Its Resolution has the lines Check, Cause, Change, Commit and Outcome (`fixed`, `unfixable` or
  `foreign-conflict`). `unfixable` finishes with no commit and the reason, and pauses the epic at
  once. `foreign-conflict` (also written by the PR-mode Merge ticket) means the rebase onto the
  remote branch conflicted with foreign commits: it finishes with no commit, and the server sets
  `pr-foreign-conflict` from that Outcome line.
- Attempt caps: 3 per failing check and 10 per PR, both global `config.json` keys (names are the
  implementer's choice) with these defaults. When the server creates a ci-fix ticket it writes a "CI failed" entry to
  `pr-activity.jsonl` with the check keys and the ticket id. Counts per check and per PR come from
  those entries after the newest Try again entry, never from parsing Resolution text. No counter
  field exists, so a restart keeps the counts. `pr-text` retries are counted from `pr-text` tickets
  created after the newest Try again entry.
- A conflict (CONFLICTING) forks a Merge ticket that rebases onto `origin/<trunk>` and pushes with
  `--force-with-lease`. The existing `maxMergeTickets` (3) applies: up to 3 Merge tickets, then Merge
  blocked ("needs rebase"). In a `pr` project the Merge ticket has a PR variant of its body (fetch,
  rebase onto `origin/<trunk>`, push with lease), and the merger skips `mergeRoot` for an epic with
  `pr_number` set.
- Every ticket that pushes (ci-fix, Merge) first fetches the remote branch and rebases onto it. A
  rebase conflict with foreign commits makes the epic Merge blocked with the reason.

### Epic states, pauses and pills

- **Fixing CI**: derived like Merging; every ticket done or cancelled except an open or claimed
  `ci-fix`. Card text "Fixing CI (attempt N of 3)". Not in Needs you. A paused fix ticket falls
  through to the "CI failing" pause.
- **"CI failing"** and **"PR text failed"** are stored like Merge blocked: two new `land_block_kind`
  values (`ci-failing`, `pr-text-failed`) beside `land_block_reason`, so each is a new PauseReason
  value with a row in the pause-to-verb table (`tickets/schema/pause_verbs.go`, ADR 0042) and a new
  `VerbTryAgain` (the existing `VerbRetry` means "start a fresh agent"). The `pause_reason` and
  `pause_kind` enum change needs a migration step (see Storage and schema). The web verb for both is
  Try again (a new server route with a CLI-mirrored verb, names the implementer's choice); it appends
  a reset entry to `pr-activity.jsonl` and clears the block. When a green run clears "CI failing" by
  itself, the server writes the same "Try again reset" entry, so the attempt counts start fresh. "CI failing" is entered when a check
  hits its cap, the PR's total hits its cap, or a fix reports `unfixable`; gx notifies once when it
  gives up. Why gx cannot recover on its own (ADR 0042): gx has used its attempt budget, or the agent
  judged that no code change fixes it, so only a person can add budget or fix it by hand. Try again
  on the web, or a green run (which clears it itself), ends it. "PR text failed" is entered when the
  text ticket's tries run out and ends on Try again.
- **Writing PR text**: derived like Merging; the epic has an open or claimed `pr-text` ticket. Shown
  in Running, not in Needs you.
- **Merge blocked in a `pr` project** (ADR 0042: each way in names its kind, the web button that ends
  it, and why gx cannot recover):

  | Way in | `land_block_kind` | Button | Why gx cannot recover |
  | --- | --- | --- | --- |
  | PR conflicts with trunk and 3 Merge tickets are used | `needs-rebase` | Merge again: forks a PR-mode Merge ticket | The attempt cap is used up |
  | Rebase conflict with foreign commits | new `pr-foreign-conflict` | Merge again, after the person resolves it | A person must judge the foreign commits |
  | Local trunk dirty after the PR merged | new `pr-dirty-trunk` | Stash, merge and restore, and Merge again, both retargeted to fetch and fast-forward local trunk | Local trunk is outside gx |
  | Other local trunk update failure after the PR merged (failed fetch, diverged trunk) | new `pr-trunk-update` | Merge again, retargeted to fetch and fast-forward | Local trunk or the network is outside gx |

  The PR merge and mark-merged both write the last two kinds, and the verbs dispatch on the kind. All
  three new kinds go in the `pause_kind` migration step and the pause-to-verb table. Merge blocked
  after a merged PR is stored, not derived from the "PR merged" entry. `EpicMergeBlocked` excludes
  `ci-failing` and `pr-text-failed`; `summarizeEpic` gives those two their own reasons ("CI failing",
  "PR text failed"). A Go test covers it.
- **Awaiting PR** is a new pill: a Complete epic in a `pr` project with no PR (including after a
  closed PR). In Needs you. Awaiting PR, Merge blocked, "CI failing" and "PR text failed" are the
  only PR-related items in Needs you; draft with CI running, fixing and ready for review are in Running
  with no pill.
- Card status words: "PR draft · CI running", "Fixing CI (attempt N of 3)", "Ready for review", "CI
  failing", "Writing PR text", "PR merged" (with Merge blocked), "Implemented" (no PR yet). A
  Merge-blocked epic whose PR merged shows the buttons from the table above.
- In a `pr` project an epic releases its dependents only once the PR has merged and local trunk holds
  the merge, whatever `auto-ff-merge` or `landing` say. The seam: `ProjectFile.LandingPolicy()` returns
  `on-landed` when `delivery` is `pr` (it feeds `EpicReleaseOf` and `MergingOnMerged`, so base
  derivation is covered), and `EpicRelease.stamped` also requires `merged_at` for any epic with
  `pr_number` set (story 56). A Go test covers release and base derivation.

### After the PR merges

- gx records "PR merged" in `pr-activity.jsonl`, cancels every open, claimed or paused `ci-fix` and Merge ticket (and
  their agents), clears `ci-failing`, and ignores its unpushed work, then runs `git fetch origin` and fast-forwards local trunk to
  `origin/<trunk>`. Only after local trunk holds the merge does it stamp `merged_at` (the stamp
  hand-merge detection uses) and add "trunk updated". A failure leaves `merged_at` empty and the epic
  Merge blocked with a `pr-dirty-trunk` or `pr-trunk-update` kind, and dependents wait. Once the "PR merged" entry exists, gx stops polling the PR; each tick retries
  the fetch and fast-forward until `merged_at` is stamped or a block is set, writes no second entry,
  and shows "PR merged" meanwhile. (`EpicMergeBlocked` needs `merged_at` empty, and dependents are
  released on `merged_at`.)
- A PR closed without merging, or a deleted remote branch (which closes it), clears `pr_number` and
  `pr_url` and returns the epic to Awaiting PR with "PR closed without merging" in the list. Create PR opens a new PR; Auto-PR does
  not reopen one for that epic by itself: it skips an epic whose list has a close entry newer than
  its last open. gx does not recreate the branch. No new pause.
- New server route and CLI verb `gx server epics mark-merged <project:epic>` (with `--json` and a
  `{"refused":true,"reason":...}` refusal) plus a "Mark as merged" button on the card and page. It sets
  `merged_at` in any Delivery, refuses an incomplete epic or one with a running or queued ticket, and
  on an epic with an open PR stops the watching and adds "marked merged by hand". In a `pr` project
  it first does the same fetch and fast-forward and stamps `merged_at` only when local trunk holds the
  merge; on failure the epic is Merge blocked and the page shows the reason. gx-cleanup calls this verb after it verifies a merge; it never writes `merged_at` itself.

### Storage and schema

- `pr_number` and `pr_url` are optional epic frontmatter fields written when the PR opens or is
  adopted. A restart reads them from disk with no `gh` call.
- The ticketless PR activity entries (PR opened, adopted PR, marked ready for review, reran <check>, CI failed
  (with check keys and ci-fix ticket id), flaky: <check>, Paused "CI failing", Try again reset, PR
  merged, trunk updated, PR closed without merging, GitHub unreachable, GitHub reachable again,
  marked merged by hand) are an append-only
  `pr-activity.jsonl` next to the epic's `ticket.md`, written by gx, following the per-ticket `.jsonl`
  precedent in the ADR 0026 amendment. Entries from tickets (fix tickets, the `pr-text` ticket) are
  read from their Resolutions, not copied. The entry schema is the implementer's choice.
- Migration steps (`migrations/steps.go`, then update `migrations/testdata/schema.snapshot`, per
  `.ai/index.md`): one named step for the `ci-fix` and `pr-text` ticket type values, one for the
  `pr_number` and `pr_url` epic fields, and one for the `pause_reason`/`pause_kind` values
  (`ci-failing`, `pr-text-failed`, `pr-foreign-conflict`, `pr-dirty-trunk`, `pr-trunk-update`). They rewrite no data. The first implementation ticket carries
  them. Step names are the implementer's choice, and a released name is never reused.

### Web and CLI

- The epic page (layout A of the approved mock) shows the PR section as a timeline under the header
  and above the tickets, newest first; each entry has a time, a dot, one line and an optional detail
  line; a fix entry links to its ticket. A "CI failing" notice with Try again sits above the timeline
  while paused. The header shows the PR chip and "Open on GitHub" after the PR exists, replacing
  Create PR.
- Create PR is the main button in a `pr` project (card footer and page header) and the second
  button next to Merge in a `merge` project.
- Settings page: registry entries in `config/registry/entries_project.go` (beside `auto-ff-merge`) and
  the Agents section for the skills (per `settings-page.md`, with its guard test and generated schemas)
  for `delivery`, `auto-pr`, the two attempt caps, the grace window, `skills.ci-fix` and
  `skills.pr-text`. `delivery` and `auto-pr` are project values (like `auto-ff-merge`); the caps and
  grace window are global only; `skills.ci-fix` and `skills.pr-text` are overridable per project, so
  the project override whitelist, the skills merge and the registry's overridable list are extended
  from `implement` and `code-review`. Section placement follows that spec's rules (skills in Agents;
  the implementer places the rest).
- New terms are already in `GLOSSARY.md`.

## Testing Decisions

- Good tests assert external behavior: an epic's state and pill, the tickets the server creates, the
  `gh` calls made, files in the store, and web output. They do not assert internal polling structure.
- The first implementation ticket builds the test seams: a fake `gh` (a PATH binary scripted by a
  state file) and a local bare `origin` repo, where the fixture writes the squash commit to simulate
  GitHub's merge and the fake records fetches and pushes. The fixture sets the grace-window key to a
  few seconds.
- The one seam is a fake `gh` behind the existing server test rig (`servertest`, with its leaf rigs),
  scripted per scenario with PR state, checks and conflict, driven through the server tick and the
  web/CLI verbs. A web test that needs a repo also needs `SetProjectRepoNoAutoMerge`. Fake agents
  idle by default, so ci-fix and pr-text fake agents need an explicit script, and any new
  idle-path behavior gets an off switch off by default in `servertest`.
- Go tests cover: Delivery and mid-flow switches (story 55 to 60); opening, adoption, closed PR, gh
  failures; check classification and the grace window; attempt counting with Try again resets;
  merge detected during a running ticket; mark-merged refusals; the `project.json` keys, the `landing: on-done` rule, the Merge ticket PR variant and the skipped
  `mergeRoot`; the migration steps and schema snapshot guard; the pause-to-verb table rows.
- Web: vitest for card and page render by state; the existing flow "16. Needs you" (`united.e2e.ts`) is extended with a `pr-flow-*` scenario step, and no
  new flow is added, per `skills/gx-playwright-e2es`.
- Prior art: the auto-merge and epic-land-visibility server tests, the hand-merge detection tests,
  the Merge ticket tests, and the registry guard tests from `settings-page.md`.
- Run Go tests with `-exec "$PWD/scripts/proc-cap"`.

## Verification

Flows are run by a verify ticket against a temporary gx server with the fake `gh` fixture. The
nearest catalog flows (`webui/e2e/CATALOG.md`) are "2. Epic looks right", "7. Epic completion",
"16. Needs you" and "58. Dirty trunk on the Epic page (and 60. Merge again)"; the new fixture
scenarios below are implementation tickets (`pr-flow-awaiting`, `pr-flow-draft-ci-running`,
`pr-flow-fixing`, `pr-flow-ready`, `pr-flow-ci-failing`, `pr-flow-merge-blocked`,
`pr-flow-merge-project`), each scripting the fake `gh`.

1. Delivery default: a project with no `delivery` key behaves as `merge`; `gx server epics merge` still merges; the Merge button is unchanged.
2. Set `delivery: pr`: a complete epic shows Awaiting PR. Mock states: `card-awaiting-pr`.
3. `auto-pr: true` in a `pr` project: a fixture complete epic opens its PR on the next tick with no click; `gx server epics` output shows the PR number.
4. Settings page: the four keys plus caps and grace window appear and save; `gx server settings --json` lists them; the registry guard test passes.
5. Awaiting PR is in Needs you. Mock states: `card-awaiting-pr`.
6. Create PR on card footer and page header. Mock states: `card-awaiting-pr`, `page-a-ci-failing`.
7. `merge` project: Create PR is the second button. Mock states: `card-merge-project-create-pr`.
8. Fake `gh` not logged in: Create PR leaves the epic unchanged and no push is recorded.
9. Same scenario: the reason text shows on the card and the page; no pause.
10. Create PR runs a `pr-text` ticket first; the opened PR carries its title and body.
11. With `skills.pr-text` set the ticket prompt names that skill; with it empty the bundled default is used.
12. Invalid text (empty body) never opens a PR.
13. Three invalid tries: "PR text failed" pause in Needs you; Try again re-runs the text ticket.
14. The fake records the PR created as draft. Mock states: `card-draft-ci-running`.
15. All checks green: the fake records `gh pr ready`. Mock states: `card-ready-for-review`.
16. A complete epic with an open verify ticket does not open a PR.
17. During a CI pending scenario no agent session is running for the epic.
18. A failed run creates one `ci-fix` ticket with its run and job ids. Mock states: `card-fixing`.
19. The second fix ticket's prompt lists the earlier Resolution.
20. The ci-fix prompt carries the run and job ids; a rules test on the bundled `gx-ci-fix` skill text (like `skills/mock_rules_test.go`) checks it forbids skipping hooks, force-pushing over foreign commits and editing CI config, and names the local check run.
21. Flake path: the server reruns the failed job once; when the rerun passes it adds "flaky: <check>", files the follow-up in the `follow-ups` epic and creates no ci-fix ticket; when it fails the ci-fix ticket starts. After a server restart the same (check key, head SHA) is not rerun again, and a repeat flake adds no second follow-up.
22. The bundled skill text tells the agent to use a read-only strongest-tier reviewer before the push (rules test).
23. `skills.ci-fix` listed skills appear in the fix ticket prompt; a project override replaces the list.
24. A fourth failure of one check, and an eleventh failure on a PR, create no further ticket and show "CI failing".
25. `Outcome: unfixable` pauses at once with attempts left. Mock states: `card-ci-failing`, `page-a-ci-failing`.
26. "CI failing" in Needs you with reason, PR link and Try again. Mock states: `card-ci-failing`, `page-a-ci-failing`.
27. Try again: the counts reset and a new fix ticket starts; `pr-activity.jsonl` has the reset entry.
28. A green run after a hand push clears the pause without Try again and writes a "Try again reset" entry.
29. One notification on give-up and none per attempt (notification log).
30. Fixing CI is outside Needs you. Mock states: `card-fixing`.
31. A PR with no checks is marked ready after the grace window (the fixture sets the key to a few seconds), not before.
32. A check appearing after the window starts a fix ticket.
33. A CANCELLED, STALE or ACTION_REQUIRED check starts no fix and the epic stays "PR draft · CI running". Mock states: `card-draft-ci-running`.
34. A failed check on an older head SHA starts no fix.
35. CONFLICTING creates a Merge ticket in PR mode (fetch, rebase onto `origin/<trunk>`, push with lease) and the merger runs no local merge; conflicts create up to 3 Merge tickets, then Merge blocked with Merge again.
36. A fix ticket pushes after fetching a foreign commit and the foreign commit survives.
37. A rebase conflict with a foreign commit shows Merge blocked with the reason and Merge again.
38. The epic is not a Merged epic and releases no dependents while the PR is open.
39. The fake reports MERGED with a squash commit not on the branch: the epic becomes merged.
40. After the merge the fake records fetch and a fast-forward; local trunk holds the commit.
41. Dirty local trunk after the merge shows "PR merged" with Merge blocked, `merged_at` still empty, and the retry buttons from the table; after the person cleans trunk and presses a button, `merged_at` is stamped. Mock states: `card-merge-blocked`.
42. A dependent epic starts only after the trunk update, including in a project with `landing: on-done`.
43. CLOSED state returns the epic to Awaiting PR with "PR closed without merging"; auto-pr does not reopen it. Mock states: `card-awaiting-pr`.
44. An existing open PR on the branch is adopted ("adopted PR"); a merged one marks the epic merged.
45. Kill and restart the server between PR creation and number write: restart adopts and no second PR is created.
46. The PR merges during a running fix ticket: the ticket is cancelled and the trunk update runs.
47. `gx server epics mark-merged <project:epic>` stamps `merged_at`; the card button does the same. In a `pr` project with dirty local trunk it leaves `merged_at` empty, shows Merge blocked (`pr-dirty-trunk`) with the table's buttons, and a button press stamps it.
48. mark-merged on an incomplete or running epic prints the refusal (`--json`: `refused` true, exit non-zero).
49. gx-cleanup calls the verb (grep of the skill and its fixture run).
50. PR chip and "Open on GitHub" show after the PR exists. Mock states: `card-draft-ci-running`, `page-a-ci-failing`.
51. The card PR line shows number chip, check summary and link. Mock states: `card-draft-ci-running`, `card-ready-for-review`.
52. The activity list is newest first with the fix entry linking to its ticket. Mock states: `page-a-ci-failing`.
53. A scripted `gh` failure adds "GitHub unreachable", keeps state, retries and never pauses.
54. Restart the server with an open PR: the PR chip and watching continue with no `gh` lookup for identity.
55. Change `delivery` in `project.json`: the epic pill updates on the next tick.
56. `pr` to `merge` with an open PR: tracking continues, no Merge button; after the PR closes the epic is Awaiting merge.
57. `auto-pr` off during a running `pr-text` ticket: the PR still opens.
58. `pr` to `merge` on an Awaiting PR epic with a running text ticket: the ticket is cancelled and no PR opens; `merge` to `pr` on Awaiting merge becomes Awaiting PR.
59. `merge` to `pr` with a running Merge ticket: it finishes, main does not move, and the epic is Awaiting PR.
60. `auto-pr: true` in a `merge` project opens no PR; the manual Create PR still opens one.
61. A one-off shows no Create PR and opens no PR.
62. The pause-to-verb table test passes with the "CI failing" and "PR text failed" rows, and Try again clears each.
63. While the `pr-text` ticket runs the epic shows "Writing PR text" in Running, not in Needs you and not Awaiting PR.
64. Each Merge blocked way in a `pr` project shows its kind's buttons from the table above, and `EpicMergeBlocked` does not match `ci-failing` or `pr-text-failed`.

**Mock states**

- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-draft-ci-running`, width 440, fixture scenario `pr-flow-draft-ci-running`, parts: card, ring, title, status, pr-line.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-fixing`, width 440, fixture scenario `pr-flow-fixing`, parts: card.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-ready-for-review`, width 440, fixture scenario `pr-flow-ready`, parts: card.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-ci-failing`, width 440, fixture scenario `pr-flow-ci-failing`, parts: card, pill, reason, try-again.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-merge-blocked`, width 440, fixture scenario `pr-flow-merge-blocked`, parts: card.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-awaiting-pr`, width 440, fixture scenario `pr-flow-awaiting`, parts: card, create-pr.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `card-merge-project-create-pr`, width 440, fixture scenario `pr-flow-merge-project`, parts: card, create-pr.
- `/Users/david/.local/share/gx/tickets/gx/pr-flow/mocks/02-epic-card-and-page-for-a-pr-epic.html`, `page-a-ci-failing`, width 1000, fixture scenario `pr-flow-ci-failing`, parts: header, actions, pause-notice, activity.

The fixture scenarios above do not exist yet and are implementation tickets.

## Out of Scope

- Code hosts other than GitHub: the flow uses `gh` only.
- Answering PR review comments: the fix agent fixes CI failures only.
- gx merging the PR, or turning on GitHub's own auto-merge: the person merges, gx watches.
- One-offs.
- GitHub events in the PR activity list (reviews, comments, every CI run): it lists gx's own actions and the CI results they reacted to.
- Updating the PR title or body after opening.
- Re-running code-review or verify after a fix.

## Further Notes

- Research behind the `gh` decisions is on branch `research/pr-flow-gh-facts`
  (`docs/research/pr-flow-04-gh-facts.md`).

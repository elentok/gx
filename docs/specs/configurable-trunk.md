# Configurable trunk

Handoff spec from the `gx:configurable-trunk` map (`configurable-trunk/ticket.md` in the ticket
store), seeded from gx:follow-ups/36. Decisions come from the map's closed tickets 01–09; this spec
gathers them into one buildable whole. Terms follow `GLOSSARY.md`: **Trunk**, **Missing trunk**,
**Base** / **Target**, **Parked**, **Park kind**. See ADR 0036 for why trunk is resolved live and
never guessed, and ADR 0015 for the `gx merge` split.

## Problem Statement

gx assumes every project lands on the repo's default branch (`main` or `master`). A person whose
project integrates on `develop`, or on a release branch, cannot use the ralph-loop as intended: claimed
work starts from the wrong branch, finished epics land on the wrong branch, `gx merge` merges onto
the wrong branch, and the worktrees, log, status and commit views compare every branch against the
wrong one.

`project.json` already has a `trunk` key, so the person can write the branch name down, but nothing
reads it. The setting looks supported and silently does nothing. There is also no safe way to edit
`project.json`: the person opens the file by hand, and a typo either breaks the project or is
ignored, with no feedback.

When the intended branch is missing (a typo, a deleted branch, a branch that was never fetched),
today's code would quietly use some other branch. For landing, that is the worst outcome: work goes
to a branch the person did not choose.

## Solution

Every gx path that needs the trunk asks one resolver for it. The resolver reads the project's
`project.json` `trunk` on each use. When the key is unset, it detects the remote's default branch
(and `main` as a last resort), every time, without writing the result back. It then checks that the
branch exists as a local branch. If it does not, the caller gets one shared error that names the
project, the trunk value and `project.json`.

From the person's view:

- Setting `trunk` in `project.json` makes the ralph-loop start top-level work from that branch and
  land it there, makes `gx merge` merge onto it, and makes every TUI view compare against it. Edits
  apply at once: no restart, no reload.
- A missing or malformed trunk never sends work elsewhere. The server stops claiming for that
  project (**Missing trunk**) and says so once, in the event log, in chat and in the Queue tab. It
  carries on by itself once the branch exists. A landing that finds trunk missing parks the ticket
  `needs-repair`, to be re-landed after the fix. `gx merge`, `gx doctor` and the TUIs show the same
  error.
- `gx project edit-config [project]` opens a temp copy of the project's `project.json` in
  `$EDITOR`, the way `gx config edit` opens `config.json`, validates it on save with the server's own
  rules, offers to edit again on errors, and warns when a trunk change will redirect tickets already
  in flight.

## User Stories

### Setting trunk

1. As a project owner whose team integrates on `develop`, I want to set `trunk` in `project.json`,
   so that gx lands my project's work on `develop` instead of `main`.
2. As a project owner, I want trunk to be set per project, so that two projects on one machine can
   use different trunks.
3. As a project owner who never set `trunk`, I want gx to keep using my repo's default branch, so
   that nothing changes for me.
4. As a project owner who changes the repo's default branch on the remote, I want an unset trunk to
   follow the new default, so that I don't have to update gx by hand.
5. As a project owner, I want gx never to write a detected trunk into `project.json`, so that the
   file only holds what I chose.
6. As a project owner, I want a `trunk` edit to take effect at the next decision, so that I don't
   have to restart or reload the server.
7. As a project owner, I want the existing `trunk` key to be the one gx reads, so that I don't have
   to learn a second key.

### Ralph-loop claim and landing

8. As a project owner, I want claimed top-level tickets with no `base:` to start from trunk, so that
   the agent works on top of my integration branch.
9. As a project owner, I want a top-level ticket's **Target** to be trunk, so that finished work
   lands where my team integrates.
10. As a project owner, I want a top-level ticket with `base: release-1.2` to start from
    `release-1.2` but still land on trunk, so that `base:` never changes where work lands.
11. As a project owner, I want child tickets to keep landing on their parent's branch, so that
    trunk only affects top-level tickets.
12. As a project owner, I want a commitless top-level ticket's ref to be trunk, so that it behaves
    like any other top-level ticket.
13. As a project owner, I want the server to read trunk again at landing, so that a ticket lands on
    the trunk value that is current when it lands.
14. As a project owner who changed trunk to a branch that has diverged from an epic's base, I want
    the ff-only landing to refuse and report "needs rebase", so that no history is rewritten.

### Missing or invalid trunk

15. As a project owner who mistyped trunk, I want gx to refuse to use it rather than fall back to
    `main`, so that work never lands on a branch I did not choose.
16. As a project owner whose trunk is missing, I want the server to claim nothing for my project,
    so that no new work starts on a broken setting.
17. As a project owner whose trunk is missing, I want one event, one chat notice and a project-level
    reason in the Queue tab, so that I know why my project stopped without being spammed. A server
    restart may notify once more; a project with nothing queued shows nothing.
18. As a project owner, I want **Missing trunk** to clear by itself once the branch exists, with one
    event and one chat notice, so that I know claiming resumed without un-parking anything.
19. As a project owner, I want a missing trunk at claim to park no tickets, so that fixing the branch
    is the only repair.
20. As a project owner whose trunk disappears while an epic is running, I want the landing ticket to
    park `needs-repair` with park kind `trunk-missing` and a `## Needs Repair` section naming the
    trunk value, so that I know exactly what to fix.
21. As a project owner, I want to re-land such a ticket with `gx server tickets land` after the fix,
    so that recovery uses the normal path.
22. As a project owner whose trunk exists only as `origin/<trunk>`, I want the error to hint
    `git branch <trunk> origin/<trunk>`, so that I can fix it in one command.
23. As a project owner, I want gx never to fetch or create the trunk branch on its own, so that it
    never changes my repo without asking.
24. As a project owner with an unset trunk whose repo has no default branch and no `main`, I want
    the same Missing trunk error, so that detection failure is not silent.
25. As a project owner whose `project.json` is unreadable, I want gx to report the read error rather
    than fall back, so that a broken file is noticed; the server treats it like Missing trunk.
26. As a project owner, I want one error text everywhere (server, `gx merge`, TUIs, `gx doctor`), so
    that I recognise the problem wherever it shows up.
27. As any gx caller, I want merging into a branch that does not exist to fail, so that gx never
    fast-forwards some other checked-out HEAD by mistake.
28. As a project owner whose trunk is not checked out in any worktree, I want landing and
    `gx merge` to fast-forward the trunk ref itself, so that work lands without moving whatever
    branch the repo root has checked out.

### `gx merge`

29. As a developer, I want `gx merge <branch>` to merge onto the project's trunk, so that manual
    merges follow the same rule as the ralph-loop.
30. As a developer in a repo that is not a registered project, I want `gx merge` to use the repo's
    default branch, so that it still works without setup.
31. As a developer, I want `gx merge` to report the resolved branch as `target` in both text and
    `--json` output, never as "main", so that scripts and the `gx-merge` skill read one field.
32. As a developer whose trunk is missing, I want `gx merge` to exit with the shared error, so that
    I fix the setting before merging.
33. As a `gx-merge` skill user, I want the skill text to say trunk, so that the skill does not tell
    the agent to merge onto "main".

### Cleanup

34. As a developer whose trunk is missing, I want `gx cleanup` to stop with the shared error before
    removing anything, so that it never deletes a branch on a "landed?" check it can't trust.

### TUI views

35. As a worktrees-TUI user, I want the trunk row highlighted and sorted first, so that I find my
    integration branch at a glance.
36. As a worktrees-TUI user, I want the Base column, the preview's ahead/behind counts and the
    rebase action to use trunk, so that "rebased" means rebased on my trunk.
37. As a worktrees-TUI user, I want labels to say "rebased on `<trunk>`" / "needs rebase on
    `<trunk>`", so that I can see which branch is meant.
38. As a worktrees-TUI user, I want new worktrees to start from trunk, so that new work begins on my
    integration branch.
39. As a worktrees-TUI user, I want pull and push to keep using each branch's own upstream, so that
    trunk does not change where branches sync.
40. As a TUI user, I want the views to read trunk again on every reload, so that a `trunk` edit
    shows up without restarting the TUI.
41. As a log-view user, I want the base ref to be `origin/<trunk>`, so that the log compares against
    my trunk.
42. As a status-view user, I want the rebase target to be `origin/<trunk>` with no hard-coded
    `origin/main` fallback, so that I never rebase onto the wrong branch.
43. As a commit-view user, I want the trunk badge to colour my trunk, not `main`/`master`, so that
    the badge means the branch I land on.
44. As a TUI user whose trunk is missing, I want a warning in place of trunk-dependent parts while
    the worktree list still shows, so that the TUI stays usable.

### `gx project edit-config`

45. As a project owner, I want `gx project edit-config <name>` to open that project's
    `project.json` in `$EDITOR`, so that I can edit it like `gx config edit`.
46. As a project owner, I want `gx project edit-config` with no argument to pick the cwd's project,
    so that the common case needs no name.
47. As a project owner outside a registered repo, I want the existing "run `gx project add .`"
    error, so that I know how to register it.
48. As a project owner, I want the command never to create `project.json`, so that editing does not
    register a project by accident.
49. As a project owner, I want the editor to open a temp copy, so that the server never reads a
    half-written file.
50. As a project owner who saves without changes, I want "no changes" and no write, so that nothing
    is touched needlessly.
51. As a project owner, I want my save validated with the server's own loader (allowed keys, types,
    enums) and `trunk` checked as a valid branch name, so that anything the command accepts, the
    server accepts.
52. As a project owner who sets a well-formed trunk that names no branch yet, I want the save
    accepted, so that I can set trunk before creating the branch; it shows up later as Missing trunk.
53. As a project owner whose save is invalid, I want the error and an `Edit again? [Y/n]` prompt
    that reopens my edits, so that I don't lose them.
54. As a project owner who answers no, I want my edits discarded and the live file untouched, so
    that a bad edit never lands.
55. As a script running without a TTY, I want an invalid save to fail with no prompt, so that the
    script does not hang.
56. As a project owner whose `project.json` changed while I was editing (for example by
    `gx project set-path`), I want the write-back refused with the temp file's path, so that neither
    change is lost.
57. As a project owner, I want the command to work with the server stopped and send no reload, so
    that it has no server dependency.
58. As a project owner who changes trunk, I want a one-line warning "N tickets in flight will land
    on `<new>`", counting top-level roots only, so that I know running work will follow the new
    trunk.

### Doctor and developer-facing

59. As a project owner, I want `gx doctor` to report a missing trunk for the cwd repo's project,
    with the shared error and hint, so that diagnosis covers it.
60. As a project owner, I want `gx doctor` not to auto-fix trunk, so that it never guesses my branch.
61. As a gx developer, I want one resolver for trunk, so that a new feature cannot read trunk a
    different way.
62. As a gx developer, I want `git` helpers to refuse an empty trunk, so that a caller that forgot to
    resolve trunk fails loudly instead of using the repo default.
63. As a gx developer, I want the code to say `Trunk`, not `MainBranch`, so that names match the
    glossary.

## Implementation Decisions

### The resolver

- Lives in `tickets`. It imports `git` and `config` and owns project lookup (`RootFor`); `git`
  cannot (import cycle) and `config` has no repo access.
- `tickets.Trunk(projectDir, repoRoot) (string, error)`: reads `project.json` `trunk`; when unset,
  uses `git.RemoteDefaultBranch` (whose last resort stays `"main"`). It then checks that the result
  is a valid branch name and exists as local `refs/heads/<trunk>`.
- `tickets.TrunkFor(dir) (string, error)`: `RootFor(dir)` then `Trunk`. A dir in no registered
  project returns the detected repo default (same rule as unset trunk), never an error for that
  reason. The existence check still applies: a detected default with no local branch is
  `ErrTrunkMissing`, and its text names the repo instead of a project or `project.json`, and
  suggests `gx project add .` then setting `trunk`.
- Errors:
  - `ErrTrunkMissing`: a typed error carrying the project name (empty for an unregistered repo) and the trunk value, plus whether
    the trunk came from `project.json` or detection. Its message names the project, the trunk value
    and `project.json`, and adds the `git branch <trunk> origin/<trunk>` hint when only the remote
    branch exists. This is the one shared error for every caller.
  - An unreadable or unparsable `project.json` is returned as its own error; there is no fallback.
- No cache. The resolver reads `project.json` (`config.ReadProjectFile`) on every call, matching how
  the server reads every other project setting. If a cache is ever added, it needs its own
  invalidation.
- No `main`/`master` guesses anywhere else: drop the extra candidates in the remote-ref helper used
  by the log view and the `"origin/main"` fallback in the status view. Use `origin/<trunk>`, else the
  local trunk branch.

### `Repo.Trunk`

- Rename `Repo.MainBranch` → `Repo.Trunk`, `IsRebasedOnMain` → `IsRebasedOnTrunk`,
  `IsCommitMergedToMain` → `IsCommitMergedToTrunk` and related names. Mechanical, its own ticket,
  done first so later tickets use the new names.
- `git.FindRepo` stops filling `Trunk`. A new `tickets.OpenRepo(dir)` calls `FindRepo` and sets
  `Trunk` from the resolver. Every caller that needs trunk uses `OpenRepo`.
- When the resolver fails, `OpenRepo` returns a usable `Repo` with an empty `Trunk` plus the
  resolver's error. Callers that can degrade (TUI views) keep the `Repo`; the rest fail with the
  error. Only a `FindRepo` failure returns no `Repo`.
- `git` helpers that compare against or merge into trunk refuse an empty `Repo.Trunk` with "trunk
  not resolved".
- Only two sites may read the repo default directly: `gx doctor`'s repair of a broken bare repo and
  the first worktree after clone (no project exists yet).

### Ralph-loop claim and landing

- Today one server function (`rootBaseRef`) returns a single ref used both as the claim base and as
  the landing target, so an explicit `base:` also changes where work lands. Split it into a **root
  base** (claim base and commitless ref) and a **root target** (landing). Both resolve trunk with
  `tickets.Trunk(projectDir, repoRoot)` (the server already has the project). Base derivation keeps
  `""` as the "trunk" token.
- Root base: the derived base, or trunk when it is `""`. Claim resolves trunk on every claim, even
  when the base is not `""`, so a missing trunk always puts the project in Missing trunk.
- Root target:
  - Explicit `base:` on a top-level ticket → trunk. `base:` changes only where work starts.
  - Derived stacked root (on-done landing, blocker not yet landed) → today's rule stays: the
    blocker's branch until that branch is reachable from trunk, then trunk. Only the "landed?"
    check changes, from the remote default to trunk.
  - Otherwise → trunk.
- At claim, `ErrTrunkMissing` (or an unreadable `project.json`) puts the project in **Missing
  trunk**: the server claims nothing for it, publishes one event and one chat notice on entering the
  state, and the Queue tab's explain output shows a project-level reason. No ticket is parked. The
  state clears by itself on the first pass that resolves trunk, with one event and one chat notice
  ("trunk found, claiming resumes"). The state lives in server memory only: after a restart a
  still-missing trunk notifies once more. It is checked only for projects with queued roots, in the
  claim loop; a project with nothing queued has nothing to block and shows no reason. Today a missing base ref at claim
  only logs and retries silently; that path is replaced.
- At landing, the server resolves trunk again (it already re-derives the target at landing). On
  `ErrTrunkMissing` it parks the ticket `needs-repair` with park kind `trunk-missing` and a
  `## Needs Repair` section naming the trunk value. Recovery is `gx server tickets land` after the
  fix. The project is also Missing trunk, so no new claims start.
- Nothing records trunk at claim. A ticket lands on the trunk read at landing time. Landing stays
  ff-only, so a diverged new trunk fails as "needs rebase". If the new trunk is an ancestor of the
  old one, the ff succeeds and the new trunk gains the old trunk's commits up to the epic; this is a
  known, accepted edge (ADR 0036).
- `MergeBranchInto` refuses a target branch that does not exist, for every caller. Today it
  fast-forwards the root's checked-out HEAD when no worktree has the target branch, which can move
  the wrong branch (trunk `develop`, root on `main`). New rule when the target exists:
  - A worktree has the target checked out → ff-merge there (unchanged).
  - Else the root's HEAD is the target → ff-merge in the root.
  - Else → fast-forward the target ref directly: check the target is an ancestor of the branch,
    then `git update-ref refs/heads/<target> <branch> <old>`. Not an ancestor → the usual "needs
    rebase" outcome. No worktree is touched.
  - It never moves a branch other than the target.

### `gx merge`

- Always merges onto `TrunkFor(cwd)`. No `--into` flag. An unregistered repo gets the detected
  default. `ErrTrunkMissing` exits non-zero with the shared error.
- Text and `--json` output report the resolved branch as `target`, never "main". The `gx-merge`
  skill and `gx-cleanup`'s merge step say trunk and read `target`.
- Landing a child onto its parent's branch stays with server landing.

### Cleanup

- `gx cleanup`'s worktree scan skips the trunk branch and uses trunk for "landed?" checks, through
  `OpenRepo`.
- On `ErrTrunkMissing` or an unreadable `project.json`, cleanup exits non-zero with the shared error
  before removing anything. It never skips the "landed?" checks and carries on.

### TUI views

- Worktrees, log, status and commit views open their repo through `OpenRepo` and resolve trunk again
  on every reload.
- Worktrees: trunk row highlight and sort, Base column, preview ahead/behind, rebase action,
  new-worktree start point and the refresh after pulling trunk all use trunk. Pull and push keep each
  branch's own upstream. Preview labels show the trunk name.
- Log: base ref `origin/<trunk>`. Status: rebase target `origin/<trunk>`. Commit: the badge colours
  trunk.
- On `ErrTrunkMissing` or an unreadable `project.json`, a view shows the shared error as a warning
  in place of trunk-dependent parts, marks no row as trunk, and still lists worktrees. No crash, no
  fallback.

### `gx doctor`

- New report-only check for the cwd repo's project (via `RootFor`), using the shared error and hint.
  No auto-fix.

### `gx project edit-config [project]`

- `[project]` is a bare project name, like `gx project set-path`. Omitted → the cwd's project.
  Not registered, or no `project.json` → the existing "no ticket-store project for this repo; run
  `gx project add .`" error. Never creates `project.json`.
- Copies `project.json` to a temp file and opens it in `$EDITOR`, reusing `gx config edit`'s editor
  launch (`$EDITOR` only; error if unset). Unchanged bytes → "no changes", nothing written.
- Validation: the server's own `project.json` loader (key whitelist, types, enums) plus
  `git check-ref-format --branch` on `trunk`. Branch existence is not checked.
- On error: print it, then `Edit again? [Y/n]`. Yes reopens the same temp copy with the edits; no
  discards it. Non-TTY stdin: no prompt, exit non-zero, live file untouched.
- Before an atomic write-back, compare the live file with the bytes the copy started from. If they
  differ, refuse ("project.json changed while you were editing"), keep the temp file and print its
  path.
- When the save changes `trunk`, print one line: "N tickets in flight will land on `<new>`", where
  N counts the project's top-level roots in flight in the local ticket store: claimed, or with a
  feature branch not yet landed. Children are not counted; they land on their parent's branch. A hand edit of `project.json`
  gets no warning.
- Local only: works with the server down, sends no reload.

### Suggested ticket order

1. Rename `MainBranch` → `Trunk` (mechanical).
2. Resolver + `OpenRepo` + `ErrTrunkMissing`; `FindRepo` stops filling trunk; `git` helpers refuse
   an empty trunk; `MergeBranchInto` refuses a missing target and ffs an unchecked-out target by
   ref update.
3. Server root base / root target split, claim (Missing trunk state, event, chat, Queue reason) and landing (park `trunk-missing`).
4. `gx merge`, cleanup and the `gx-merge` / `gx-cleanup` skill text.
5. TUI views (worktrees, log, status, commit) with reload and degraded mode.
6. `gx doctor` check.
7. `gx project edit-config`.

## Testing Decisions

- Good tests drive gx from its outside: a real temp git repo, a real ticket store with a real
  `project.json`, and the public entry point (server runner pass, CLI command, TUI model update).
  They assert on what a person sees (where a branch landed, ticket status and park kind, events,
  command output, rendered labels), not on which helper was called.
- Seams, highest first. These follow the map's decisions and existing test patterns; they were not
  confirmed with the person in a separate step.
  1. **Server runner** (existing seam): claim and landing with `trunk` set, unset, missing,
     malformed and edited between claim and landing. Covers base, Target, Missing trunk (event,
     chat notice once, Queue explain reason, self-clearing) and the `trunk-missing` park. Prior art:
     the server runner tests that show a project cap edit applying live, landing an iteration and
     closing the root, and parking `needs-repair` on a failed finish; land-recovery tests for park
     kind assertions.
  2. **CLI commands** (existing seam): `gx merge` run function (prior art: the clean ff, diverged
     and text-output merge tests) for trunk, unregistered repo and Missing trunk, including the
     `target` field. `gx doctor` (prior art: the doctor tests). `gx project edit-config` alongside
     the `gx project set-path`/`remove` tests, with the editor and TTY made injectable so tests can
     script edits, the edit-again loop, the non-TTY failure, the concurrent-change refusal and the
     in-flight warning.
  3. **Resolver** (new, small seam): `tickets.Trunk` / `TrunkFor` / `OpenRepo` against temp repos:
     set, unset (remote default, `main` last resort), unregistered, missing local branch with and
     without `origin/<trunk>`, malformed value, unreadable `project.json`. Most behaviour is also
     covered through seams 1 and 2; this seam pins the shared error text.
  4. **TUI model reload** (existing seam): worktrees model reload and preview tests for trunk row,
     labels, ahead/behind, a `trunk` edit picked up on reload, and degraded mode. Log, status and
     commit views get one test each for the trunk ref or badge.
- `git` package tests (prior art: the merge fast-forward tests) cover `MergeBranchInto` refusing a
  missing target, ff-ing a target with no worktree by ref update while the root's HEAD stays put,
  and helpers refusing an empty trunk.
- Server runner tests also cover a top-level `base: release-1.2` ticket starting from
  `release-1.2` and landing on trunk, and a stacked root retargeting to trunk once its blocker
  lands on trunk.
- Run tests per package with `rtk go test ./<pkg>/...`; never the whole repo at once.

## Out of Scope

- Changing trunk while an epic is running is unsupported. gx's behaviour is defined (land on the
  live trunk, warn in `gx project edit-config`), but it is not a supported workflow.
- Syncing between branches (for example merging `develop` back into `main`).
- A per-epic or per-run trunk override, for release branches.
- Giving `gx config edit` the same temp-copy validation as `gx project edit-config`; tracked as a
  follow-up.
- The PR base for PR landing: no PR-creation path exists today; tracked as gx:follow-ups/34.
- An `--into` flag on `gx merge`.
- Fetching or creating the trunk branch automatically.

## Further Notes

- `Config.WithProject` has no production caller, so other `project.json` overrides (`agents`,
  `skills`, `execution-queue.max-agents-per-epic`) are also parsed and ignored. This spec only wires
  `trunk`, through its own resolver, and does not fix those.
- Research notes behind tickets 01 and 02 are in `docs/research/trunk-usage-inventory.md` and
  `docs/research/server-project-json-cache.md` (uncommitted).
- Glossary: **Trunk**, **Missing trunk** and the "Merge onto Trunk" section are already in
  `GLOSSARY.md`; `base:` was removed from **Epic**.

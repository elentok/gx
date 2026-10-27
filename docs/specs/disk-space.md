# Disk space

Handoff spec from the gx:disk-space map. The decisions behind it are the closed tickets
gx:disk-space/01 to 04. The research is in `docs/research/disk-space-leftover-worktrees.md`.

## Problem Statement

A queue run filled the disk twice. The first time, the Go build cache had grown to 60G and 259M was
left free. `gx brief go-test` failed at link time with "No space left on device", and the ticket
paused until a person cleared space by hand. The second time, one ticket failed to link with 157M
free and another ticket's full `server/` test run had 21 failures, all "no space left on device".

Nothing in gx watches free disk space. The person learns about a full disk only when a ticket fails.
gx also leaks disk space on its own:

- gx never removes an epic's feature worktree, not even after the epic merges into trunk. Each one
  keeps a full checkout with its `node_modules`.
- Each Playwright e2e run builds `gx`, `claude` and `gxfixture` into a new `gx-e2e-bin-*` temp dir
  and never removes it. These dirs held 28 GiB.
- `claudeenv.Isolate` gives each test package a temp `HOME`. Tests that run `go` there download the
  Go modules into it, Go makes those files read-only, and the cleanup fails without a word. These
  dirs held 2 GiB.

## Solution

The gx server checks free disk space for every project while it runs. It checks two disks per
project: the disk that holds the project's repo, and the disk that holds the Go build cache. Two
server-wide thresholds in `config.json` set when gx acts. The defaults are 20G for the warn
threshold and 10G for the clean threshold.

- Under the warn threshold, every web page shows a banner. The banner names the disk, shows its
  free space and has a "Clean now" button. gx sends one chat notice. When space is fine, the banner
  is gone.
- Under the clean threshold, gx runs the cleanup commands of the projects on that disk, once per
  crossing. The default command is `go clean -cache`, set server-wide in `config.json`. A project
  can replace it in its `project.json`, or set it empty to turn cleanup off. gx sends a chat notice
  that says how much the cleanup freed, how much is free now, and whether dispatch is held.
- If the disk is still under the clean threshold after the cleanup, gx starts no new tickets for
  the projects that use that disk. Running tickets keep running. Held tickets show a "disk low"
  verdict. Dispatch resumes on its own once free space climbs back above the clean threshold, and gx
  sends a chat notice when it does. The banner's "Clean now" button is the web way out
  (ADR 0042).

gx also stops the three leaks:

- Right after an epic merges, gx removes the epic's feature worktree when the worktree has no
  changes and no live agent session uses it. The epic branch stays.
- Playwright e2e builds its binaries into one fixed path per worktree.
- `claudeenv.Isolate` points the Go caches at the real ones, and its cleanup removes the temp
  `HOME` fully or reports why it could not.

## User Stories

**Settings**

1. As a person, I want a warn threshold and a clean threshold for free disk space in `config.json`,
   so that I can tune when gx warns me and when it acts.
2. As a person, I want the thresholds on by default at 20G (warn) and 10G (clean), so that I get
   protection without setting anything up.
3. As a person, I want a threshold of zero to turn that threshold off, so that I can opt out of the
   warning or the automatic clean, the same way a zero budget limit turns the budget off.
4. As a person, I want a server-wide cleanup command in `config.json` with the default
   `go clean -cache`, so that every project can free the shared Go build cache with no setup.
5. As a person, I want a project's `project.json` to replace the cleanup command, so that a project
   with other rebuildable caches cleans those instead.
6. As a person, I want an empty cleanup command in `project.json` to turn cleanup off for that
   project, so that gx never runs a command I don't want on that project.
7. As a person, I want an edit to the thresholds or the cleanup command to apply without a server
   restart, so that I can react while a queue runs.
8. As a person, I want a threshold pair where the clean threshold is above the warn threshold to be
   corrected rather than rejected, so that a typo does not stop gx from starting.

**Watching free space**

9. As a person, I want gx to check free space whenever the gx server runs, so that a full disk
   never surprises a queue run, even with no queue running.
10. As a person, I want gx to check each project's repo disk and the Go build cache's disk, so that
    a full disk in either place is seen before a build fails.
11. As a person, I want gx to check free space every minute and right before it starts a ticket, so
    that a disk that fills between checks still stops the next start.
12. As a person with several projects on one disk, I want gx to treat that disk once, so that one
    low disk gives one banner, one clean and one set of notices.

**Warning**

13. As a person, I want a banner on every web page when a disk is under the warn threshold, so that
    I see the problem wherever I am in the web UI.
14. As a person, I want the banner to name the disk and show its free space, so that I know how bad
    it is.
15. As a person, I want the banner to say when dispatch is held on that disk, so that I know why no
    new tickets start.
16. As a person, I want no banner when every disk is above the warn threshold, so that the web UI
    stays clean when nothing is wrong.
17. As a person with two low disks, I want one banner per low disk, so that each one has its own
    free space and its own "Clean now" button.
18. As a person, I want a chat notice when a disk crosses under the warn threshold, so that I hear
    about it even when the web UI is closed.
19. As a person, I want only one warn notice per crossing, so that a disk that stays low does not
    send a notice every minute.

**Automatic clean**

20. As a person, I want gx to run the cleanup commands of the projects on a disk when that disk
    crosses under the clean threshold, so that gx frees what it can rebuild before a build fails.
21. As a person, I want gx to run each distinct command once per disk, so that ten projects with
    `go clean -cache` clean the shared cache once, not ten times.
22. As a person, I want gx to clean only when a disk crosses under the clean threshold, and again
    only after free space has climbed back above it, so that every build does not start from a cold
    cache.
23. As a person, I want gx never to clean after each iteration or before each queue, so that the
    shared Go cache stays warm in normal use.
24. As a person, I want a chat notice after an automatic clean that says how much it freed, how
    much is free now and whether dispatch is held, so that I know if I need to act.
25. As a person, I want a failed cleanup command to be logged with its error and to leave the hold
    in place, so that a broken command never lets dispatch fill the disk.
26. As a person, I want a cleanup command to run with the project's repo as its working directory
    and with a time limit, so that a hung command does not block the server.

**Holding dispatch**

27. As a person, I want gx to start no new tickets for a project while its repo disk or the Go
    cache's disk is still under the clean threshold after the clean, so that a queue run does not
    fail ticket after ticket on a full disk.
28. As a person, I want running tickets to keep running during a hold, so that work in progress is
    not lost.
29. As a person, I want projects on other disks to keep dispatching during a hold, so that one full
    disk does not stop unrelated work.
30. As a person, I want queued one-offs and retros held the same way as epic tickets, so that
    nothing that goes through the queue fills the disk further.
31. As a person, I want held tickets to show a "disk low" verdict in the web UI, the TUI Queue tab
    and `gx server explain`, so that every place that explains a waiting ticket names the cause.
32. As a person, I want dispatch to resume on its own once free space climbs back above the clean
    threshold, so that freeing space by hand is enough and I don't need a second step.
33. As a person, I want a chat notice when dispatch resumes, so that I know the queue moves again.

**Clean now**

34. As a person, I want a "Clean now" button on the banner, so that I can free space from the web
    UI without a terminal.
35. As a person, I want "Clean now" to run the same commands an automatic clean runs for that disk,
    so that the button and the automatic clean behave the same.
36. As a person, I want "Clean now" to check free space again right after it finishes, so that a
    hold ends at once when the clean freed enough.
37. As a person, I want the button disabled while the clean runs, so that I don't start it twice.
38. As a person, I want "Clean now" to tell me when it failed, and why, so that I know to free
    space another way.
39. As a person, I want a CLI verb that runs the same clean and one that prints disk status, so that
    the server API and the CLI stay in step (ADR 0028).

**Feature worktree removal**

40. As a person, I want gx to remove an epic's feature worktree right after the epic merges into
    trunk, so that merged epics stop holding a full checkout with its `node_modules`.
41. As a person, I want gx to remove the worktree only when it has no staged, unstaged or untracked
    changes, using plain `git worktree remove` and never `--force`, so that gx never deletes work.
42. As a person, I want gx to keep the worktree when a live agent session uses it, so that no
    session loses its working directory.
43. As a person, I want the epic branch kept after the removal, so that every commit and later reads
    of the branch stay possible.
44. As a person, I want gx to log why it kept a worktree when a check fails, so that I can tell why
    a merged epic's worktree is still there.
45. As a person, I want the removal on every path where gx records an epic as merged: its own merge,
    an epic with nothing to merge, an epic merged elsewhere, and an epic I merged by hand, so that no
    merge path leaks a worktree.
46. As a person, I want gx to keep the feature worktree of an epic that never merges (the one-offs
    root) and of any epic not yet merged, so that live work keeps its checkout.

**Temp-dir leaks**

47. As a developer, I want Playwright e2e to build its binaries into one fixed path per worktree, so
    that repeated or killed runs leave at most one copy.
48. As a developer, I want `claudeenv.Isolate` to point `GOCACHE` and `GOMODCACHE` at the real Go
    caches, so that tests stop downloading modules into each temp `HOME`.
49. As a developer, I want the `claudeenv.Isolate` cleanup to make files writable before it removes
    the temp `HOME`, and to report an error when the removal fails, so that the temp dir is gone or
    the failure is visible.

## Implementation Decisions

**Disk watcher (server)**

- A new disk watcher in the server owns all of this. It keeps, per disk, the last free-space
  reading, the warn state, the clean state (armed or spent) and the hold. The web snapshot, the
  claim path, the explain path and the clean route all read from it.
- A disk is a volume, identified by its device id, not by path. gx finds the disk of each project's
  repo path and the disk of the Go build cache. The Go cache path comes from `go env GOCACHE`. If
  `go` is missing or the command fails, gx logs it once and watches only the repo disks.
- Every project watches two disks: its repo disk and the Go cache's disk. Usually both are the same
  disk. Projects with `vcs: none` (the scratch project) watch their repo path's disk the same way.
- The watcher runs on its own one-minute timer, separate from the claim loop's 2-second tick. Before
  each claim, the claim path reads free space for that project's disks again. A free-space read is
  one statfs call, so this is cheap.
- A project's cleanup commands for a disk are the distinct effective cleanup commands of every
  project that watches that disk. With the defaults, that is one `go clean -cache` run.
- State changes per disk:
  - Free space drops under the warn threshold: mark the disk warned and send the warn notice. Clear
    the warn mark when free space climbs back above the warn threshold.
  - Free space drops under the clean threshold while the clean is armed: run the disk's cleanup
    commands, mark the clean spent, read free space again, set the hold if still under the clean
    threshold, and send the clean notice. Re-arm the clean, and clear the hold, when free space
    climbs back above the clean threshold. Clearing a hold sends the resume notice and kicks the
    claim loop.
  - With no cleanup command at all for a disk (every project there set it empty), the drop under
    the clean threshold sets the hold at once. The clean notice then says that no cleanup command
    is set.
- The cleanup runs with the project's repo as its working directory (for a bare repo, gx's usual
  trunk checkout), outside the server's request path, with a timeout. A failure logs the command,
  its exit code and the tail of its output, and the hold stays.
- Only one clean runs per disk at a time. A "Clean now" while an automatic clean runs for that disk
  waits for it or is refused with a clear reason; the implementer picks which.
- The watcher's thresholds and the cleanup command reload with `config.json` and `project.json`,
  like the budget limits and the project cap today.

**Holding dispatch**

- The hold sits in the per-root claim path, next to the project cap and the "project unavailable"
  check. A root whose project watches a held disk claims nothing. Other roots keep claiming.
  One-offs and retros go through the same claim path, so they are held too. Design sessions and
  web "new session" starts are person-initiated and not held.
- Running agents, merges and landings continue during a hold.
- The explain path gets a new verdict, "disk low", for an unclaimed, queued ticket whose project
  watches a held disk. Its reason names the disk and its free space. The TUI Queue tab and
  `gx server explain` already print verdict text, so the TUI gets no new element.

**Why the hold is allowed (ADR 0042)**

- gx cannot recover on its own because it has already freed what it can safely rebuild. What is
  left (worktrees, caches it does not own, the person's files) needs a person's judgement.
- The banner names the fix: free space on that disk. "Clean now" on the banner runs the cleanup
  again and checks free space at once. Dispatch also resumes on its own once free space returns,
  without any click.
- The hold is not a ticket pause or a merge block. No ticket changes status, so it adds no row to
  the pause-to-verb table. It works like the "project unavailable" and budget holds on new starts.

**Settings**

- `config.json` gets a new section for disk space with a warn threshold, a clean threshold and a
  cleanup command. The command is an argv list, not a shell string. The implementer picks the key
  names and the size format (a human size such as `20G`, or a number of bytes), and updates
  `docs/config-schema.json`.
- `project.json` gets one new whitelisted key for the cleanup command, layered over the global one
  through the existing project overrides. An empty list turns cleanup off for that project; an
  absent key keeps the global command. `docs/project-schema.json` is updated.
- A zero threshold turns that threshold off. If the clean threshold is above a non-zero warn
  threshold, gx raises the warn threshold to match it, the same "clamp, don't reject" rule the
  budget config uses.

**Server API and CLI**

- The snapshot gets a server-filled disk status field, like `herdr_unavailable` and `budget`. It
  lists only disks under the warn threshold. Each entry has a disk id, a display name (mount point
  or volume name), free bytes, whether dispatch is held on it, the projects that watch it and the
  last clean result. When every disk is fine, the list is empty.
- A disk status read route and a "clean now" write route, each with a matching `gx server` CLI verb.
  The write takes a disk id and returns the bytes freed, the free space after, the hold state, or a
  refusal with a reason (unknown disk, clean already running, command failed). Both routes go in
  the route table, the CLI parity list, the web mux (the write in the web writes list) and the web
  allowlist test. The implementer picks route paths and verb names.
- A disk status change (warn, clean, hold, resume) publishes a server event so the web and TUI
  refresh without polling. The implementer picks the event name.

**Chat notices**

- Three new server notice kinds, sent through the existing chat path like the "project
  unavailable" notice:
  - warn: the disk, its free space, the warn threshold;
  - clean: the disk, the bytes freed, the free space now, and whether dispatch is held (or that no
    cleanup command is set, or that the command failed);
  - resume: the disk, its free space, that dispatch resumed.
- One notice per state change, never one per minute. A "Clean now" from the web sends the clean
  notice too, so the chat log matches what happened.

**Web banner**

- The banner reuses the existing `Strip` component in the shared page frame, below the "Offline"
  strip, so it shows on every page. One strip per low disk.
- A warned disk uses the waiting tone. A held disk uses the paused tone. The text names the disk and
  its free space; a held disk adds that new tickets are held. The implementer writes the exact
  copy, short enough for phone width.
- The "Clean now" button sits in the strip. It is disabled while the call runs. On success the
  strip updates from the snapshot. On a refusal or failure the strip shows the reason; the
  implementer picks between the existing write-error pattern and inline text in the strip.
- The banner follows the page cache rule: after a reload it repaints from the cached snapshot.

**Feature worktree removal**

- A new server step, "remove the merged epic's feature worktree", runs after gx stamps `merged_at`
  on every path that stamps it: the fast-forward merge, the nothing-to-merge case, the
  merged-elsewhere case, and the hand-merge stamping.
- The step skips the trunk checkout and the one-offs root. It checks that no live agent session or
  pane uses the worktree, the same check the design cleanup uses. Then it calls plain
  `git worktree remove` without `--force`, so git refuses a worktree with changes. It takes the
  ralph-loop worktree lock while it removes.
- A failed check or a git refusal logs the epic, the worktree path and the reason, and keeps the
  worktree. It never pauses the epic and never fails the merge.
- The epic branch stays. gx already recreates a missing feature worktree if the epic is queued
  again.
- The ralph-loop feature code says the feature worktree always stays. That comment and its `keep`
  marker change to say gx removes it after the merge.

**Temp-dir leaks**

- The Playwright global setup builds into one fixed directory inside the worktree, under an ignored
  build path. The implementer picks the path and adds it to `.gitignore` if needed. A build
  overwrites the binaries in place.
- `claudeenv.Isolate` reads the real `GOCACHE` and `GOMODCACHE` (through `go env`) before it swaps
  `HOME`, and sets both env vars to those paths. If `go` is missing, it leaves them unset. Its
  cleanup walks the temp `HOME` and makes every entry writable before `os.RemoveAll`, and returns
  or prints the error when removal still fails. The implementer picks the cleanup's signature,
  keeping every `TestMain` caller working.

**Not done on purpose**

- The watcher does not count `$TMPDIR` on its own. On macOS it is on the same disk as the repos, so
  the free-space check already sees it.
- The cleanup command does not sweep old `$TMPDIR/gx-*` dirs. With the leaks fixed, a sweep would
  only hide new ones.

## Testing Decisions

A good test checks what a person or a client sees: the snapshot, the explain verdict, the chat
notices, the files on disk. It does not check the watcher's internal fields.

Seams:

1. **Server, through servertest.** One new seam: a free-space source on the server `Config`. The
   default reads the real disk. Tests pass a fake that returns set free bytes per path and can
   change them between checks, and a fake cleanup runner (or a real small command in a temp dir).
   A Config field also sets the watcher interval, so tests run in milliseconds. Tests use
   `servertest.StartWithStore`, a temp store and `testutil.TempRepo`, and a fake chat webhook. Prior
   art: `TestUnavailable_MissingPathExplainsNotifiesOnceAndSetPathRecovers` in
   `server/unavailable_test.go` (explain verdict, one notice per crossing, recovery), the budget
   tests (`server/budget_limits_test.go`, `server/configreload_budget_test.go`) for holds and config
   reload, and `server/handmerged_test.go` for the hand-merge path.
2. **Config.** Table tests for the new keys: defaults, zero, clamping, project override, empty
   override, unknown-key rejection. Prior art: the project overrides tests in `config/`.
3. **Web.** Vitest for the banner component and the frame (no banner, one warned disk, two disks,
   held disk, pending button, failed clean). Prior art: `webui/src/routes.test.tsx` and the
   `Strip` gallery entry. One e2e flow on a fixture server for the full click path.
4. **Fixture.** A new `gxfixture` scenario, `disk-low`, whose server uses a fake free-space source
   the flow can change at runtime (the implementer picks the mechanism, for example a file the fake
   reads), and a cleanup command that "frees" space by changing that value.
5. **claudeenv.** A unit test in `testutil/claudeenv` that creates a read-only tree inside the temp
   `HOME`, runs the cleanup and checks the dir is gone, and one that checks `GOCACHE` and
   `GOMODCACHE` point outside the temp `HOME`.
6. **Feature worktree removal.** Server tests with a real temp repo and a real feature worktree:
   removed after merge when clean and idle; kept with an untracked file; kept with a live session;
   kept for the one-offs root; branch still present in every case.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test" names the seam
from Testing Decisions that already covers the story; the verify part reruns it. New flows take the
next free numbers in `webui/e2e/CATALOG.md` (69 onward); the implementer assigns them.

New scenario to write: `disk-low` (see Testing seam 4). New flows to write:

- **Flow A, "Disk banner and Clean now":** scenario `disk-low` with free space set under the warn
  threshold. Open the Epics page, check the strip names the disk and free space and shows "Clean
  now". Open an epic page and a ticket page and check the strip is there too. Click "Clean now" with
  the call held, check the button is disabled, release it, and check the strip goes away once the
  fake reports space above the warn threshold.
- **Flow B, "Disk hold":** scenario `disk-low` with free space under the clean threshold and a
  cleanup that frees too little. Queue `ready-epic` (the scenario includes one). Check the strip
  says new tickets are held and the ticket shows "disk low". Raise free space above the clean
  threshold and check, without a reload, that the strip leaves the held state and the ticket starts.
- **Flow C, "Clean now fails":** scenario `disk-low` with a cleanup command that exits non-zero.
  Click "Clean now" and check the strip shows the failure reason and stays.

Entries:

1. CLI check: in a temp config, set both thresholds, start a fixture server with `--config`, run the
   disk status verb; it prints the set thresholds. Go test (seam 2).
2. CLI check: with no disk section in config, the disk status verb prints 20G and 10G. Go test
   (seam 2).
3. Go test (seam 1): warn threshold 0 sends no warn notice and shows no banner entry; clean
   threshold 0 never cleans or holds.
4. Go test (seam 2): the effective command with no setting is `go clean -cache`. Go test (seam 1):
   with the default, a crossing runs `go clean -cache` (fake runner records the argv).
5. Go test (seam 2): a project override replaces the command for that project only. Go test
   (seam 1): the fake runner records the override's argv.
6. Go test (seam 2): an empty list turns cleanup off. Go test (seam 1): a disk whose only project
   has cleanup off holds at once and the clean notice says no command is set.
7. Go test (seam 1): edit `config.json` thresholds while the server runs; the next check uses the
   new values with no restart. Prior art: `server/configreload_budget_test.go`.
8. Go test (seam 2): clean 15G with warn 10G loads as warn 15G, clean 15G, and the server starts.
9. CLI check: on a fixture server with an empty queue, the disk status verb shows a reading taken
   in the last minute. Go test (seam 1): the watcher checks with no queued root.
10. Go test (seam 1): a project whose repo path and Go cache path report different free space
    shows both disks; a low Go cache disk alone holds the project.
11. Go test (seam 1): with a long watcher interval, drop free space under the clean threshold and
    queue a ticket; the claim path rereads free space and does not start it.
12. Go test (seam 1): two projects on one disk give one snapshot entry, one clean run and one notice
    each of warn, clean and resume.
13. Flow A (strip on the Epics page, an epic page and a ticket page).
14. Flow A (disk name and free space in the strip). Vitest (seam 3): the strip text.
15. Flow B (strip says new tickets are held).
16. Flow A (strip gone once space is above the warn threshold). Vitest (seam 3): no strip with an
    empty disk list.
17. Vitest (seam 3): two low disks give two strips, each with its own button.
18. Go test (seam 1): a crossing under the warn threshold sends one warn notice with disk and free
    space.
19. Go test (seam 1): free space stays under the warn threshold for several checks; one warn notice
    only. A climb above and a second drop sends a second one.
20. Go test (seam 1): a crossing under the clean threshold runs the disk's cleanup commands.
21. Go test (seam 1): three projects on one disk with the same command run it once; a fourth with a
    different command runs that one once too.
22. Go test (seam 1): free space stays under the clean threshold across several checks; the cleanup
    runs once. It runs again only after a climb above the clean threshold and a new drop.
23. Go test (seam 1): a finished iteration and a queue add with free space above the clean
    threshold run no cleanup.
24. Go test (seam 1): the clean notice carries bytes freed, free space now and the hold state, for
    both the "held" and "not held" outcomes.
25. Go test (seam 1): a failing cleanup command logs its error, the clean notice says it failed,
    and the hold stays. Flow C.
26. Go test (seam 1): the cleanup runs with the project's repo as its working directory; a command
    that sleeps past the timeout is killed and handled as a failure.
27. Flow B (ticket shows "disk low" and does not start). Go test (seam 1): a held project claims
    nothing.
28. Go test (seam 1): an agent launched before the hold keeps running and finishes normally during
    the hold.
29. Go test (seam 1): a project on a disk with enough space still dispatches while another disk is
    held.
30. Go test (seam 1): a queued one-off under a held disk does not start and shows "disk low".
31. Flow B (verdict in the web UI). CLI check: `gx server explain <ticket>` on the held fixture
    prints "disk low" with the disk and free space. The TUI Queue tab prints the same verdict
    text, so no TUI check is needed beyond the explain output.
32. Flow B (ticket starts after space returns, no click). Go test (seam 1).
33. Go test (seam 1): the hold clearing sends one resume notice.
34. Flow A.
35. Go test (seam 1): "Clean now" for a disk runs the same distinct commands as the automatic
    clean, recorded by the fake runner.
36. Go test (seam 1): "Clean now" that frees enough clears the hold in its own response, with no
    wait for the next minute check. Flow A (strip goes away).
37. Flow A (button disabled while the call is held). Vitest (seam 3).
38. Flow C. CLI check: the clean verb against the failing fixture exits non-zero and prints the
    reason.
39. CLI check: on a fixture server, the disk status verb prints the snapshot's disk list, and the
    clean verb with a disk id prints bytes freed and free space after. `TestRouteTableHasCLIVerbs`
    and `TestWeb_EverythingElseIs404` pass with the new routes.
40. Go test (seam 6): after an epic merges, its feature worktree is gone and `git worktree list`
    no longer shows it.
41. Go test (seam 6): a feature worktree with an untracked file is kept after the merge, and the
    log names the reason.
42. Go test (seam 6): a feature worktree with a live session is kept after the merge, and the log
    names the reason.
43. Go test (seam 6): the epic branch exists after the removal.
44. Go test (seam 6): the log line for a kept worktree names the epic, the path and the reason.
45. Go test (seam 6): one test per path that stamps `merged_at`: fast-forward merge, nothing to
    merge, merged elsewhere, and hand-merge stamping. Each removes the clean, idle worktree.
46. Go test (seam 6): the one-offs root's worktree stays; a completed but unmerged epic (auto merge
    off) keeps its worktree.
47. Shell check: run `make web-e2e` twice with one flow (`gx brief -C webui playwright epics.e2e.ts
    -g "^1\."`); count `$TMPDIR/gx-e2e-bin-*` before and after (no new dirs), and the fixed bin
    path in the worktree exists with `gx`, `claude` and `gxfixture`.
48. Go test (seam 5): after `Isolate`, `go env GOCACHE GOMODCACHE` point outside the temp `HOME`.
49. Go test (seam 5): a read-only tree in the temp `HOME` is removed by the cleanup; a forced
    failure is reported. Shell check: `go test ./testutil/claudeenv/ -timeout 60s` leaves no new
    `$TMPDIR/gx-claudeenv*` dir.

## Out of Scope

- The web cleanup section: archive epics, delete branches, remove unmerged worktrees, list sizes,
  and the `gx cleanup scan` fixes. That is gx:follow-ups/41.
- Deleting epic branches or iteration branches. Branches cost almost no space.
- Removing session worktrees and other worktrees gx does not own.
- Counting `$TMPDIR` as its own disk, and sweeping old `$TMPDIR/gx-*` dirs.
- A new TUI banner or indicator. The TUI shows the hold through the explain verdict.
- Percentage thresholds and per-project thresholds. Both thresholds are absolute and server-wide.
- Cleaning after each iteration or before each queue.

## Further Notes

- The Go build cache is one shared folder (`~/Library/Caches/go-build` on this machine), so a clean
  makes the next build in every worktree cold. That is why gx cleans only on a crossing under the
  clean threshold.
- On APFS, `du` overstates what removing a worktree frees, because pnpm clones files from its
  store. The clean notice reports the change in free space from statfs, not a `du` sum.
- The research measured 28 GiB of `gx-e2e-bin-*` dirs and 2 GiB of `gx-claudeenv*` dirs in
  `$TMPDIR`. They were the largest leaks; feature worktrees came next.
- gx's own project repo is a bare repo, so gx cannot tell a Go project from its repo root. That is
  why the cleanup command is a server-wide default with a per-project override, not a language
  detection.

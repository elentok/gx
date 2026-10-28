# Disk space

Handoff spec from the gx:disk-space map. The decisions behind it are the closed tickets
gx:disk-space/01 to 04, plus the person's answers to the design review of this spec. The research
is in `docs/research/disk-space-leftover-worktrees.md`.

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
server-wide thresholds in `config.json` set when gx acts. The defaults are `"20G"` for the warn
threshold and `"10G"` for the clean threshold.

- Under the warn threshold, every web page shows a banner. The banner names the disk, shows its
  free space and has a "Clean now" button. gx sends one chat notice. The banner and the warning
  clear once free space is 2G above the warn threshold, so a disk near the line does not flicker.
- Under the clean threshold, gx holds dispatch for the projects on that disk at once and runs their
  cleanup commands in the background. The default command is `go clean -cache`, set server-wide in
  `config.json`. A project can replace it in its `project.json`, or set it empty to turn cleanup
  off. gx sends a chat notice that says how much the cleanup freed, how much is free now, and
  whether dispatch is still held. gx cleans again only after free space has climbed back above the
  higher of the warn threshold and the clean threshold plus 2G.
- If the disk is still under the clean threshold after the cleanup, the hold stays. Running tickets
  keep running. Held tickets show a "disk low" verdict. Dispatch resumes on its own once free space
  climbs back above the clean threshold, and gx sends a chat notice when it does. The banner's
  "Clean now" button is the web way out (ADR 0042). A held disk always has a banner.

gx also stops the three leaks:

- After an epic merges, gx removes the epic's feature worktree when the worktree has no changes, no
  live agent session or pane works in it, the epic is not queued and it has no open tickets. The
  epic branch stays.
- Playwright e2e builds its binaries into one fixed path per worktree.
- `claudeenv.Isolate` points the Go caches at the real ones, and its cleanup removes the temp
  `HOME` fully or reports why it could not.

## User Stories

**Settings**

1. As a person, I want a warn threshold and a clean threshold for free disk space in `config.json`,
   written as human sizes such as `"20G"`, so that I can tune when gx warns me and when it acts.
2. As a person, I want the thresholds on by default at `"20G"` (warn) and `"10G"` (clean), so that I
   get protection without setting anything up.
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
11. As a person, I want gx to check free space every minute and again right before it starts a
    ticket, with both readings going through the same rules, so that a disk that fills between
    checks still stops the next start.
12. As a person with several projects on one disk, I want gx to treat that disk as one disk, so
    that one low disk gives one banner and one set of notices.

**Warning**

13. As a person, I want a banner on every web page when a disk is under the warn threshold, so that
    I see the problem wherever I am in the web UI.
14. As a person, I want the banner to name the disk and show its free space, so that I know how bad
    it is.
15. As a person, I want the banner to say when dispatch is held on that disk, so that I know why no
    new tickets start.
16. As a person, I want a held disk to have a banner and a "Clean now" button even when the warn
    threshold is off, so that every hold has a web way out (ADR 0042).
17. As a person, I want the banner to go away only once free space is 2G above the warn threshold
    and the disk is not held, so that the web UI stays clean when nothing is wrong and does not
    flicker near the line.
18. As a person with two low disks, I want one banner per low disk, so that each one has its own
    free space and its own "Clean now" button.
19. As a person, I want a chat notice when a disk crosses under the warn threshold, so that I hear
    about it even when the web UI is closed.
20. As a person, I want no second warn notice until free space has climbed 2G above the warn
    threshold and dropped under it again, so that free space moving up and down with each build
    does not send a notice each time.

**Automatic clean**

21. As a person, I want gx to hold dispatch for a disk's projects as soon as a reading puts that
    disk under the clean threshold, and to run their cleanup commands in the background, so that
    no ticket starts on a nearly full disk while the clean runs.
22. As a person, I want gx to run each distinct pair of cleanup command and project repo once per
    clean, so that a command with relative paths runs in every project's repo, and a repeated
    `go clean -cache` costs almost nothing after the first run.
23. As a person, I want gx to clean once when a disk crosses under the clean threshold, and again
    only after free space has climbed back above the higher of the warn threshold and the clean
    threshold plus 2G, so that free space moving near the clean threshold does not empty the Go
    cache again and again, even when both thresholds are equal or warn is off.
24. As a person, I want gx never to clean after each iteration or before each queue, so that the
    shared Go cache stays warm in normal use.
25. As a person, I want a chat notice after an automatic clean that says how much it freed, how
    much is free now and whether dispatch is still held, so that I know if I need to act.
26. As a person, I want a failed cleanup command to be logged with its error and to leave the hold
    in place, so that a broken command never lets dispatch fill the disk.
27. As a person, I want a cleanup command to run with the project's repo as its working directory
    and with a time limit, so that a hung command does not block the server.
28. As a person, I want a ticket whose build fails because the clean removed the Go cache under it
    to be handled like any other failed build, so that the clean needs no special recovery.

**Holding dispatch**

29. As a person, I want gx to start no new tickets for a project while its repo disk or the Go
    cache's disk is held, so that a queue run does not fail ticket after ticket on a full disk.
30. As a person, I want running tickets to keep running during a hold, so that work in progress is
    not lost.
31. As a person, I want projects on other disks to keep dispatching during a hold, so that one full
    disk does not stop unrelated work.
32. As a person, I want queued one-offs and retros held the same way as epic tickets, so that
    nothing that goes through the queue fills the disk further.
33. As a person, I want held tickets to show a "disk low" verdict in the web UI, the TUI Queue tab
    and `gx server explain`, so that every place that explains a waiting ticket names the cause.
34. As a person, I want dispatch to resume on its own once free space climbs back above the clean
    threshold, so that freeing space by hand is enough and I don't need a second step.
35. As a person, I want a chat notice when dispatch resumes, so that I know the queue moves again.

**Clean now**

36. As a person, I want a "Clean now" button on the banner, so that I can free space from the web
    UI without a terminal.
37. As a person, I want "Clean now" to run the same commands an automatic clean runs for that disk,
    and to end through the same end-of-clean rule, so that the button and the automatic clean
    behave the same.
38. As a person, I want "Clean now" to check free space again right after it finishes, and to send
    one clean notice that carries the hold state and no separate resume notice, so that a hold ends
    at once when the clean freed enough and the chat log shows one message per clean.
39. As a person, I want "Clean now" on a disk with no cleanup command to skip the run and only check
    free space again, so that the button ends the hold once I have freed space by hand.
40. As a person, I want the button disabled while the clean runs, so that I don't start it twice.
41. As a person, I want no clean of either kind to start on a disk while another clean runs there,
    and "Clean now" refused with "clean already running" in that case, so that two cleans never run
    at once and I know why nothing new happened.
42. As a person, I want a failed "Clean now" to show its reason in the banner strip, so that I know
    to free space another way.
43. As a person, I want a CLI verb that runs the same clean and one that prints disk status, so that
    the server API and the CLI stay in step (ADR 0028).

**Feature worktree removal**

44. As a person, I want gx to remove an epic's feature worktree after the epic merges into trunk,
    so that merged epics stop holding a full checkout with its `node_modules`.
45. As a person, I want gx to remove the worktree only when it has no staged, unstaged or untracked
    changes, using plain `git worktree remove` and never `--force`, so that gx never deletes work.
46. As a person, I want gx to keep the worktree when any live agent session or pane the server
    tracks works inside it, so that no session loses its working directory.
47. As a person, I want gx to keep the worktree when the epic's root is queued, and never to remove
    a worktree that a claim is setting up at the same time, so that an epic I queued again keeps its
    checkout.
48. As a person, I want gx to keep the worktree when the epic has open tickets, so that work still
    to do keeps its checkout.
49. As a person, I want the epic branch kept after the removal, so that every commit and later reads
    of the branch stay possible.
50. As a person, I want gx to log why it kept a worktree when a check fails, so that I can tell why
    a merged epic's worktree is still there.
51. As a person, I want the removal to follow every path where gx records an epic as merged: its own
    fast-forward merge, an epic with nothing to merge, and an epic I merged by hand, so that no
    merge path leaks a worktree.
52. As a person, I want the removal to run in the background after the merge has finished, so that
    a slow `git worktree remove` never holds up a merge.
53. As a person, I want gx to keep the feature worktree of an epic that never merges (the one-offs
    root) and of any epic not yet merged, and to keep it through every ticket finish before the
    merge, so that live work keeps its checkout.

**Temp-dir leaks**

54. As a developer, I want Playwright e2e to build its binaries into one fixed path per worktree, so
    that repeated or killed runs leave at most one copy.
55. As a developer, I want `claudeenv.Isolate` to point `GOCACHE` and `GOMODCACHE` at the real Go
    caches, so that tests stop downloading modules into each temp `HOME`.
56. As a developer, I want the `claudeenv.Isolate` cleanup to make files writable before it removes
    the temp `HOME`, and to report an error when the removal fails, so that the temp dir is gone or
    the failure is visible.

**Server restart**

57. As a person, I want a server restart while a disk is low to hold dispatch from its first
    reading, before any claim, and to send one warn notice and run one clean, so that a restart
    never starts tickets on a full disk and never loses the clean.

## Implementation Decisions

**Disk watcher (server)**

- A new disk watcher in the server owns all of this. It keeps, per disk, the last free-space
  reading, the warn mark, a "clean running" flag, the automatic-clean state (armed or spent) and
  the hold. The two clean fields are separate: both kinds of clean (automatic and "Clean now") set
  and clear the running flag, and only the automatic clean changes armed or spent. The web
  snapshot, the claim path, the explain path and the clean route all read from the watcher.
- Nothing persists. On server start the watcher begins with no disks known, every warn mark clear,
  every clean armed, no clean running and no hold. The first reading of a disk, from the timer or
  the claim path, goes through the state machine before the claim path decides. A restart while a
  disk is low therefore holds dispatch before any claim, sends one warn notice and runs one clean.
  That clean is cheap, since the cache was already emptied before the restart.
- A disk is a volume, identified by its device id, not by path. gx finds the disk of each project's
  repo path and the disk of the Go build cache. The Go cache path comes from `go env GOCACHE`. If
  `go` is missing or the command fails, gx logs it once and watches only the repo disks.
- Every project watches two disks: its repo disk and the Go cache's disk. Usually both are the same
  disk. Projects with `vcs: none` (the scratch project) watch their repo path's disk the same way.
- The watcher reads free space on its own one-minute timer, separate from the claim loop's 2-second
  tick. The claim path also reads free space for a project's disks before each claim. A free-space
  read is one statfs call, so this is cheap. Every reading, from either source, goes through the
  same state machine below.
- A disk's cleanup work is the set of distinct pairs (cleanup argv, project repo path) over every
  project that watches that disk and has a non-empty command. Each pair runs once per clean, with
  the repo path as its working directory. The repo path is the `repo` value from `project.json`
  (for gx, its bare repo). With the default, `go clean -cache` runs once per project; every run
  after the first finds an empty cache and returns at once.

**State machine per disk**

- Warn mark. A reading under the warn threshold, with the mark clear, sets the mark and sends the
  warn notice. The mark clears only when a reading is above the warn threshold plus a fixed 2G
  margin. The margin is not a setting.
- Clean and hold. A reading under the clean threshold sets the hold at once.
  - If the automatic clean is armed and no clean is running, gx marks the clean spent, sets the
    running flag, starts the cleanup work in the background, and sends nothing yet. When the work
    ends, the end-of-clean rule below runs.
  - If the automatic clean is spent, or a clean of either kind is running, the hold stays and
    nothing new runs. An armed clean stays armed while a "Clean now" runs; the next reading after
    it ends decides.
  - If the disk has no cleanup work (every project on it set an empty command), the clean is marked
    spent at once and the clean notice says no cleanup command is set.
- End of clean. This one rule ends both kinds of clean. gx clears the running flag, reads free
  space again, feeds that reading to the warn mark, and sets the hold from it: held if under the
  clean threshold, not held otherwise. It sends exactly one clean notice, which carries the bytes
  freed, the free space now and the hold state. It sends no resume notice, even when the hold
  clears, and kicks the claim loop when it does. A clean never holds a disk that was not already
  held: a "Clean now" on a warned, unheld disk leaves dispatch running while it runs, and its end
  sets the hold only if the new reading is under the clean threshold.
- Re-arm. The automatic clean re-arms when a reading is above the higher of the warn threshold and
  the clean threshold plus 2G. The same 2G margin as the warn mark, fixed. This one rule covers
  warn off and warn equal to clean (which the clamp in story 8 can produce).
- Resume. A held disk whose reading is at or above the clean threshold, with no clean running,
  clears the hold, sends the resume notice and kicks the claim loop. Readings while a clean runs
  never clear the hold; the end-of-clean rule does.
- Nothing is claimed for a project while any of its disks is held. An automatic clean always runs
  on a held disk, so dispatch stays held for the whole automatic clean.
- Running builds are not stopped during a clean. `go clean -cache` can delete cache entries that a
  running `go build` or `go test` is reading. gx accepts that. Such a failure looks like any failed
  build, and the ticket's normal retry and pause handling covers it.
- The cleanup runs outside the server's request path, with a timeout. A failure logs the command,
  the working directory, its exit code and the tail of its output. The clean notice says it failed,
  and the hold stays.
- The thresholds and the cleanup commands reload with `config.json` and `project.json`, like the
  budget limits and the project cap today.

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
  again, or with no cleanup command only checks free space again. Either way it ends the hold once
  enough space is free. Dispatch also resumes on its own once free space returns, without any
  click.
- A held disk always has a banner, also with the warn threshold off, so the button is always
  reachable.
- The hold is not a ticket pause or a merge block. No ticket changes status, so it adds no row to
  the pause-to-verb table. It works like the "project unavailable" and budget holds on new starts.

**Settings**

- `config.json` gets a new section for disk space with a warn threshold, a clean threshold and a
  cleanup command. Thresholds are human size strings such as `"20G"` and `"10G"`. The command is an
  argv list, not a shell string. The implementer picks the key names and the exact size units
  accepted, and updates `docs/config-schema.json`.
- `project.json` gets one new whitelisted key for the cleanup command, layered over the global one
  through the existing project overrides. An empty list turns cleanup off for that project; an
  absent key keeps the global command. `docs/project-schema.json` is updated.
- A zero threshold turns that threshold off. If the clean threshold is above a non-zero warn
  threshold, gx raises the warn threshold to match it, the same "clamp, don't reject" rule the
  budget config uses.

**Server API and CLI**

- The snapshot gets a server-filled disk status field, like `herdr_unavailable` and `budget`. It
  lists every disk whose warn mark is set or that is held. Each entry has a disk id, a display name
  (mount point or volume name), free bytes, whether dispatch is held on it, whether a clean is
  running, the projects that watch it and the last clean result (bytes freed, or the failure
  reason). When no disk is warned or held, the list is empty.
- A disk status read route and a "clean now" write route, each with a matching `gx server` CLI verb.
  The write takes a disk id. It refuses with a reason for an unknown disk, and with "clean already
  running" while the disk's running flag is set. Otherwise it sets the running flag, runs the
  disk's cleanup work (or none, if the disk has none), and ends through the end-of-clean rule, so
  it sends one clean notice and no resume notice. It returns the bytes freed, the free space after
  and the hold state. A "Clean now" does not change whether the automatic clean is armed. Both routes go in
  the route table, the CLI parity list, the web mux (the write in the web writes list) and the web
  allowlist test. The implementer picks route paths and verb names.
- A disk status change (warn, clean start, clean end, hold, resume) publishes a server event so the
  web and TUI refresh without polling. The implementer picks the event name.

**Chat notices**

- Three new server notice kinds, sent through the existing chat path like the "project
  unavailable" notice:
  - warn: the disk, its free space, the warn threshold;
  - clean: the disk, the bytes freed, the free space now, and whether dispatch is held (or that no
    cleanup command is set, or that the command failed);
  - resume: the disk, its free space, that dispatch resumed.
- One notice per state change, never one per reading. Each clean, automatic or "Clean now", sends
  exactly one clean notice and no resume notice, so the chat log matches what happened.

**Web banner**

- The banner reuses the existing `Strip` component in the shared page frame, below the "Offline"
  strip, so it shows on every page. One strip per disk in the snapshot's disk list.
- A warned disk uses the waiting tone. A held disk uses the paused tone. The text names the disk and
  its free space; a held disk adds that new tickets are held. The implementer writes the exact
  copy, short enough for phone width.
- The "Clean now" button sits in the strip. It is disabled while the call runs and while the
  snapshot says a clean is running on that disk. On success the strip updates from the snapshot. A
  refusal or a failed clean shows its reason inline in the strip, and the strip stays.
- The banner follows the page cache rule: after a reload it repaints from the cached snapshot.

**Feature worktree removal**

- `stampEpic` is the one place every merge path stamps `merged_at`. Today three paths do: the
  fast-forward merge in `mergeEpic`, the nothing-to-merge case in `mergeEpic`, and the hand-merge
  stamping. The "merged elsewhere" case in `mergeEpic` stamps only `completed_at`, because
  `merged_at` was already stamped by one of the three. When `stampEpic` stamps `merged_at`, it
  requests the removal of that epic's feature worktree. A merge path added later gets the removal
  with no extra code.
- The removal runs in the background (through the server's settle group, as other post-merge work
  does), never under the trunk lock. It must start only after the stamping path has released its
  trunk lock, so the dequeue that follows the stamp has happened and the "not queued" check sees
  the final queue. The implementer picks how (for example, the background task takes the same
  trunk-lock key the stamping path held).
- The removal checks, in order, and keeps the worktree on the first failure. Checks 2 to 5 run
  inside one hold of the ralph-loop worktree lock, the same lock the claim path takes when it
  creates or reuses a feature worktree (`withWorktreeLock`, used by `PrepareFeature`). A claim then
  either sets up the worktree before the checks run (the root is queued, so the worktree is kept)
  or after the remove (gx recreates the missing worktree). Queue adds do not take the trunk lock,
  so the trunk lock cannot give this guarantee.
  1. The worktree is not the trunk checkout, and the root is not the one-offs root.
  2. No live agent session or pane that the server tracks has its working directory inside the
     worktree. This covers every kind: ralph-loop iteration and merge-ticket agents, design
     sessions, one-off agents and web sessions. It is a new check; the design cleanup's check sees
     only design sessions.
  3. The epic's root is not in the queue.
  4. The epic has no open tickets.
  5. `git worktree remove` without `--force` succeeds, so git refuses a worktree with staged,
     unstaged or untracked changes.
- The worktree lock is an unexported package variable in ralphloop today, so ralphloop exports a
  small helper that runs a function under it. The server runs checks 2 to 5 inside one call of
  that helper.
- A failed check or a git refusal logs the epic, the worktree path and the reason, and keeps the
  worktree. It never pauses the epic and never fails the merge.
- The epic branch stays. gx already recreates a missing feature worktree if the epic is queued
  again.
- `ResumeFeature` keeps `keep: true` on the feature worktree, so ticket finishes before the merge
  never remove it. Only the comment on `FinishMerge` that says "The feature worktree always stays"
  changes, to say the post-merge removal is the one place gx removes it.

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
   change them between readings, and a fake cleanup runner that records each (argv, working dir)
   pair and can block until the test releases it. A Config field also sets the watcher interval,
   so tests run in milliseconds. Tests use `servertest.StartWithStore`, a temp store,
   `testutil.TempRepo` and a fake chat webhook. Prior art:
   `TestUnavailable_MissingPathExplainsNotifiesOnceAndSetPathRecovers` in
   `server/unavailable_test.go` (explain verdict, one notice per crossing, recovery), the budget
   tests (`server/budget_limits_test.go`, `server/configreload_budget_test.go`) for holds and config
   reload, and `server/handmerged_test.go` for the hand-merge path.
2. **Config.** Table tests for the new keys: defaults, size strings, zero, clamping, project
   override, empty override, unknown-key rejection. Prior art: the project overrides tests in
   `config/`.
3. **Web.** Vitest for the banner component and the frame (no banner, one warned disk, two disks,
   held disk, clean running, pending button, failed clean, refusal). Prior art:
   `webui/src/routes.test.tsx` and the `Strip` gallery entry. Three e2e flows on a fixture server
   for the click paths.
4. **Fixture.** A new `gxfixture` scenario, `disk-low`, whose server uses a fake free-space source
   the flow can change at runtime (the implementer picks the mechanism, for example a file the fake
   reads), and a cleanup command that "frees" space by changing that value. The scenario has one
   unqueued epic for the hold flow.
5. **claudeenv.** A unit test in `testutil/claudeenv` that creates a read-only tree inside the temp
   `HOME`, runs the cleanup and checks the dir is gone, and one that checks `GOCACHE` and
   `GOMODCACHE` point outside the temp `HOME`.
6. **Feature worktree removal.** Server tests with a real temp repo and a real feature worktree. One
   test per removal rule (trunk and one-offs root, live session or pane, queued root, open tickets,
   uncommitted changes), one per merge path that stamps `merged_at`, and one that the epic branch
   stays.

## Verification

A verify ticket runs these against a temporary gx server from `gxfixture`. "Go test" names the seam
from Testing Decisions that already covers the story; the verify part reruns it. New flows take the
next free numbers in `webui/e2e/CATALOG.md` (69 onward); the implementer assigns them.

New scenario to write: `disk-low` (Testing seam 4). New flows to write:

- **Flow A, "Disk banner and Clean now":** scenario `disk-low` with free space set under the warn
  threshold. Open the Epics page, check the strip names the disk and free space and shows "Clean
  now". Open an epic page and a ticket page and check the strip is there too. Click "Clean now" with
  the call held, check the button is disabled, release it. Set free space to just above the warn
  threshold and check the strip stays; set it 2G above and check the strip goes away.
- **Flow B, "Disk hold":** scenario `disk-low` with free space under the clean threshold and a
  cleanup that frees too little. Queue the scenario's epic. Check the strip says new tickets are
  held and the ticket shows "disk low". Raise free space above the clean threshold and check,
  without a reload, that the strip leaves the held state and the ticket starts.
- **Flow C, "Clean now fails":** scenario `disk-low` with a cleanup command that exits non-zero.
  Click "Clean now" and check the strip shows the failure reason inline and stays.

Entries:

1. CLI check: start a fixture server with `--config` setting both thresholds as size strings, run
   the disk status verb; it prints the set thresholds. Go test (seam 2): size strings parse.
2. CLI check: with no disk section in config, the disk status verb prints 20G and 10G. Go test
   (seam 2).
3. Go test (seam 1): warn threshold 0 sends no warn notice; clean threshold 0 never cleans or holds.
4. Go test (seam 2): the effective command with no setting is `go clean -cache`. Go test (seam 1):
   with the default, a crossing runs `go clean -cache` (the fake runner records the argv).
5. Go test (seam 2): a project override replaces the command for that project only. Go test
   (seam 1): the fake runner records the override's argv.
6. Go test (seam 2): an empty list turns cleanup off. Go test (seam 1): a disk whose only project
   has cleanup off holds at once and the clean notice says no command is set.
7. Go test (seam 1): edit `config.json` thresholds while the server runs; the next reading uses the
   new values with no restart. Prior art: `server/configreload_budget_test.go`.
8. Go test (seam 2): clean `"15G"` with warn `"10G"` loads as warn 15G, clean 15G, and the server
   starts.
9. CLI check: on a fixture server with an empty queue, the disk status verb shows a reading taken
   in the last minute. Go test (seam 1): the watcher reads with no queued root.
10. Go test (seam 1): a project whose repo path and Go cache path report different free space
    watches both disks; a low Go cache disk alone holds the project.
11. Go test (seam 1): with a long watcher interval, drop free space under the clean threshold and
    queue a ticket; the pre-claim reading holds the disk, starts the clean, and the ticket does not
    start.
12. Go test (seam 1): two projects on one disk give one snapshot entry, one warn notice, one clean
    notice and one resume notice.
13. Flow A (strip on the Epics page, an epic page and a ticket page).
14. Flow A (disk name and free space in the strip). Vitest (seam 3): the strip text.
15. Flow B (strip says new tickets are held).
16. Go test (seam 1): with warn 0 and a held disk, the snapshot lists the disk as held. Vitest
    (seam 3): a held disk with no warn renders a strip with "Clean now".
17. Flow A (strip stays just above the warn threshold, goes away 2G above). Vitest (seam 3): no
    strip with an empty disk list.
18. Vitest (seam 3): two disks in the list give two strips, each with its own button.
19. Go test (seam 1): a crossing under the warn threshold sends one warn notice with disk and free
    space.
20. Go test (seam 1): free space moves between just under and just over the warn threshold for
    several readings; one warn notice only. A climb 2G above and a new drop sends a second one.
21. Go test (seam 1): a reading under the clean threshold holds the disk before the clean ends
    (the fake runner blocks); a claim attempt while the clean runs claims nothing; after release, a
    reading above the clean threshold clears the hold with the clean notice saying "not held".
22. Go test (seam 1): three projects on one disk with the same command record three
    (argv, repo) pairs, one per repo; two projects that share a repo path and command record one.
23. Go test (seam 1): free space moves around the clean threshold, staying under the warn threshold;
    the cleanup runs once. It runs again only after a reading above the warn threshold and a new
    drop under the clean threshold. With warn 0, and with warn equal to clean, a reading just above
    the clean threshold does not re-arm; one 2G above it does.
24. Go test (seam 1): a finished iteration and a queue add with free space above the clean
    threshold run no cleanup.
25. Go test (seam 1): the clean notice carries bytes freed, free space now and the hold state, for
    both the "held" and "not held" outcomes.
26. Go test (seam 1): a failing cleanup command logs its error, the clean notice says it failed,
    and the hold stays. Flow C.
27. Go test (seam 1): the cleanup runs with the project's repo as its working directory; a command
    that sleeps past the timeout is killed and handled as a failure.
28. No runtime check: this is an accepted risk with no new code. Spec review confirms the
    Implementation Decisions say a build failure during a clean takes the normal failed-build path.
29. Flow B (ticket shows "disk low" and does not start). Go test (seam 1): a held project claims
    nothing.
30. Go test (seam 1): an agent launched before the hold keeps running and finishes normally during
    the hold.
31. Go test (seam 1): a project on a disk with enough space still dispatches while another disk is
    held.
32. Go test (seam 1): a queued one-off under a held disk does not start and shows "disk low".
33. Flow B (verdict in the web UI). CLI check: `gx server explain <ticket>` on the held fixture
    prints "disk low" with the disk and free space. The TUI Queue tab prints the same verdict text,
    so no TUI check is needed beyond the explain output.
34. Flow B (ticket starts after space returns, no click). Go test (seam 1).
35. Go test (seam 1): a hold that clears on a later reading sends one resume notice.
36. Flow A.
37. Go test (seam 1): "Clean now" for a disk runs the same (argv, repo) pairs as the automatic
    clean, recorded by the fake runner. A "Clean now" leaves an armed automatic clean armed.
38. Go test (seam 1): "Clean now" on a held disk that frees enough clears the hold in its own
    response, with no wait for the next minute reading, and the fake chat gets exactly one clean
    notice and no resume notice. "Clean now" on a warned, unheld disk: a claim while it runs
    (runner blocked) still starts the ticket, and after it ends one clean notice goes out with
    "not held" and no resume notice. Flow B covers the hold clearing in the web UI.
39. Go test (seam 1): on a held disk whose only project has cleanup off, "Clean now" runs nothing;
    with free space raised by the fake first, it clears the hold in its response.
40. Flow A (button disabled while the call is held). Vitest (seam 3): disabled while the snapshot
    says a clean is running.
41. Go test (seam 1): with the fake runner blocking an automatic clean, "Clean now" is refused with
    "clean already running"; a second "Clean now" during a first one is refused the same way. With
    a "Clean now" blocked in the fake runner on an armed disk, a reading under the clean threshold
    holds the disk but records no second run; after the "Clean now" ends, the disk is still armed
    and the next low reading starts the automatic clean. CLI
    check: the clean verb against a disk with a running clean prints that reason and exits
    non-zero.
42. Flow C. Vitest (seam 3): a refusal and a failure each show their reason inline in the strip.
43. CLI check: on a fixture server, the disk status verb prints the snapshot's disk list, and the
    clean verb with a disk id prints bytes freed and free space after. `TestRouteTableHasCLIVerbs`
    and `TestWeb_EverythingElseIs404` pass with the new routes.
44. Go test (seam 6): after an epic merges, its feature worktree is gone and `git worktree list`
    no longer shows it.
45. Go test (seam 6): a feature worktree with an untracked file is kept after the merge, and the
    log names the reason.
46. Go test (seam 6): a feature worktree with a live agent pane inside it is kept, and one with a
    live web session inside it is kept; the log names the reason.
47. Go test (seam 6): a merged epic whose root is queued again before the removal runs keeps its
    worktree. A second test checks the normal merge, where the dequeue follows the stamp, does
    remove it (the removal does not race the dequeue). A third test holds the worktree lock, lets
    the removal start and wait on it, re-queues the epic and lets a claim set up the feature
    worktree under the lock first; the worktree survives the removal.
48. Go test (seam 6): a merged epic with an open ticket keeps its worktree.
49. Go test (seam 6): the epic branch exists after the removal.
50. Go test (seam 6): the log line for a kept worktree names the epic, the path and the reason.
51. Go test (seam 6): one test per path that stamps `merged_at`: fast-forward merge, nothing to
    merge, and hand-merge stamping. Each removes the clean, idle worktree.
52. Go test (seam 6): with the ralph-loop worktree lock held by the test, the merge completes and
    the root is dequeued; the worktree is removed only after the test releases the lock.
53. Go test (seam 6): the one-offs root's worktree stays; a completed but unmerged epic (auto merge
    off) keeps its worktree; a ticket finish in the feature worktree before the merge keeps it.
54. Shell check: run one flow twice (`gx brief -C webui playwright epics.e2e.ts -g "^1\."`); count
    `$TMPDIR/gx-e2e-bin-*` before and after (no new dirs), and the fixed bin path in the worktree
    exists with `gx`, `claude` and `gxfixture`.
55. Go test (seam 5): after `Isolate`, `go env GOCACHE GOMODCACHE` point outside the temp `HOME`.
56. Go test (seam 5): a read-only tree in the temp `HOME` is removed by the cleanup; a forced
    failure is reported. Shell check: `go test ./testutil/claudeenv/ -timeout 60s` leaves no new
    `$TMPDIR/gx-claudeenv*` dir.
57. Go test (seam 1): stop the server with free space under the clean threshold (after its clean
    ran), start a new server on the same store with the fake still low and a ticket queued. The
    first claim is held, the ticket shows "disk low", and the fake chat gets one warn notice and
    one clean notice from the new server.

## Out of Scope

- The web cleanup section: archive epics, delete branches, remove unmerged worktrees, list sizes,
  and the `gx cleanup scan` fixes. That is gx:follow-ups/41.
- Deleting epic branches or iteration branches. Branches cost almost no space.
- Removing session worktrees and other worktrees gx does not own.
- Counting `$TMPDIR` as its own disk, and sweeping old `$TMPDIR/gx-*` dirs.
- A new TUI banner or indicator. The TUI shows the hold through the explain verdict.
- Percentage thresholds, per-project thresholds and a configurable hysteresis margin.
- Cleaning after each iteration or before each queue.
- Pausing or stopping running builds during a clean.

## Further Notes

- The Go build cache is one shared folder (`~/Library/Caches/go-build` on this machine), so a clean
  makes the next build in every worktree cold. That is why gx cleans only on a crossing under the
  clean threshold and re-arms only well above it.
- On APFS, `du` overstates what removing a worktree frees, because pnpm clones files from its
  store. The clean notice reports the change in free space from statfs, not a `du` sum.
- The research measured 28 GiB of `gx-e2e-bin-*` dirs and 2 GiB of `gx-claudeenv*` dirs in
  `$TMPDIR`. They were the largest leaks; feature worktrees came next.
- gx's own project repo is a bare repo, so gx cannot tell a Go project from its repo root. That is
  why the cleanup command is a server-wide default with a per-project override, not a language
  detection.

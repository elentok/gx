# Epic diff: a read-only Diff page

## Problem Statement

Agents write an epic's code on a branch, and I cannot read that code in the web UI. To review it before I merge or send a PR, I switch to a terminal and run git. After the merge the branch is gone, so I cannot easily read what the epic changed. A squashed PR makes that worse, because the agent's commits disappear from trunk. gx stores no record of where an epic started or ended.

## Solution

A read-only **Diff page** in the web UI. It takes a project and two refs and shows the changes on the second ref since it forked from the first (three-dot). It lists the range's commits grouped by ticket, and it shows one file's diff at a time.

- The epic page, the archived epic page and a done ticket's page link to it. The link opens the **Epic range**, before the merge and after it.
- At merge time gx stamps the Epic range (base SHA and tip SHA) on the epic. A start-up backfill stamps epics that merged earlier, where trunk's reflog gives an exact answer. Where gx cannot know the range, the page says "No diff recorded" and lets me type two refs.
- Each diff line stays addressable by file, side and line number, so the next milestone (human code review) can attach focus highlights and comments. This milestone builds none of that.

## User Stories

1. As a person who runs epics, I want a Diff page that takes a project and two refs, so that I can read the code the agents wrote in the web UI.
2. As a reviewer, I want the two refs to be editable text fields filled from the link, so that I can change either side without leaving the page.
3. As a reviewer, I want any branch, tag or SHA to work as a ref, so that I am not limited to epic branches.
4. As a reviewer, I want the comparison to be three-dot, so that I see only what the second ref changed since it forked from the first.
5. As a reviewer, I want the "..." between the two fields to tell me the comparison is three-dot, so that I do not mistake it for two-dot.
6. As a person with an unmerged epic, I want its page to link to the Diff page with the epic's derived base and the epic branch, so that I can read the work before merging.
7. As a person with a stacked epic whose blocker is unmerged, I want the base to be the blocker's branch (or the epic's explicit `base:` when one is set), so that I see only this epic's changes.
8. As a person with a stacked epic whose blocker has merged, I want the base to be trunk (or the explicit `base:`), so that the link follows the real target.
9. As a person with a running epic, I want "View diff" once its branch exists, so that I can read work in progress.
10. As a person with a merged epic, I want "View diff" to open the stamped Epic range, so that I can read it after the branch is gone.
11. As a person with an archived epic, I want "View diff" on the archived epic page too, so that old epics are readable.
12. As a person with a done ticket, I want "View diff" on its page that opens the epic's Diff page narrowed to that ticket, so that I can read just that ticket's code.
13. As a person with an epic that has no commits, I want no "View diff" link, so that I do not open an empty page. A merged epic with no commits has a stamped empty range (base equal to tip). An unmerged epic has no commits while its branch has none past its base.
14. As a person with a merged epic that has no stamped range (base and tip both empty), I want "View diff" to open the page with empty ref fields and the "No diff recorded" notice, so that I can type two refs myself.
15. As a person, I want gx to stamp the Epic range in the same step that sets `merged_at`, so that range and merge time never disagree.
16. As a person with a fast-forward merge, I want the base to be the target tip before the merge and the tip to be the target tip after it, so that the range is exact.
17. As a person with a stacked epic that merges into its blocker's branch, I want the same rule against the epic's derived target (the blocker's branch), so that its range is exact too.
18. As a person whose project delivers by PR, I want gx to stamp the range once trunk holds GitHub's merge commit, so that the squash case works.
19. As a person with a PR merged as a merge commit, I want base to be its first parent and tip to be the merge commit, so that the range matches the PR.
20. As a person with a squash-merged PR, I want base to be the first parent and tip to be the squash commit, so that I can still read the whole change as one commit.
21. As a person with a rebase-and-merge PR, I want gx to find the rebased commits on trunk by commit headline, so that base is the commit before the first and tip is the last.
22. As a person whose PR commit headlines do not match trunk, I want gx to stamp nothing and the page to say "No diff recorded", so that I never see a guessed range.
23. As a person whose stacked epic's blocker was squash-merged, I want the epic's stamped diff while its commits still exist in this clone, and "Commits not available in this clone" once they are gone, so that I am never shown a wrong diff.
24. As a person with epics that merged before this feature, I want a start-up backfill that stamps an exact range from trunk's reflog, so that old epics open on the right diff.
25. As a person, I want the backfill to stamp only an exact, unambiguous match (every trailer commit of the epic inside one reflog entry's range), so that no range is guessed.
26. As a person, I want the backfill to run at server start-up on every machine and for every registered project, so that each machine reads its own reflog.
27. As a person, I want the backfill to log each epic it refused and why, so that I can tell a refusal from a bug.
28. As a person with an epic that merged into trunk more than once, I want "No diff recorded", so that a union of ranges never mixes in other epics' commits.
29. As a person with an epic that has no commits, I want "No changes" when I open the page with equal refs, and no link, so that the page does not pretend there is a diff.
30. As a person on a machine whose reflog has no matching entry, I want "No diff recorded" and editable ref fields, so that the page still helps.
31. As a person whose stamped SHA is missing in this clone, I want the notice "Commits not available in this clone" and no diff, so that I know the cause.
32. As a reviewer, I want the commit list grouped by ticket, using the `Ralph-Loop-Ticket` trailer, so that I read the work ticket by ticket.
33. As a reviewer, I want a group to show the ticket's title when the ticket still exists, so that I recognize it.
34. As a reviewer, I want groups in the order of their first commit in the range, so that the order follows the work.
35. As a reviewer, I want trailer-less commits whose subject starts with `<map>:` in a "Design" group, so that spec and ADR commits are separate from code.
36. As a reviewer, I want every other trailer-less commit in an "Other commits" group, so that nothing is hidden.
37. As a reviewer, I want "Design" and "Other commits" after the ticket groups, so that code comes first.
38. As a reviewer opening two typed refs with no epic, I want no "Design" group, so that the group appears only where an epic gives it meaning.
39. As a reviewer, I want an empty commit to still appear in its group, so that the list matches git.
40. As a reviewer, I want the commit list collapsed by default to one line, "COMMITS BY TICKET (n)", so that the file tree stays the main panel.
41. As a reviewer, I want that line to show the current narrowing at its right, so that I always know what I am looking at.
42. As a reviewer, I want to click a ticket, "Design", "Other commits" or a single commit to narrow the diff, so that I read exactly that part.
43. As a reviewer, I want narrowing to filter the file tree to the files those commits changed, so that the tree matches the diff.
44. As a reviewer, I want a narrowed file to show each commit's own patch stacked in commit order with the commit subject as the label, so that every line stays tied to a real commit.
45. As a reviewer, I want narrowing to a ticket to show only commits that carry that ticket's trailer, so that other tickets' commits between them do not appear.
46. As a reviewer, I want a "Showing: Ticket 03 x" chip under the ref fields and a way to clear it, so that I can return to all commits.
47. As a reviewer, I want "All commits" at the top of the opened commit list, so that clearing the narrowing is one click.
48. As a reviewer of a very large epic, I want the commit list capped at 500 commits with the notice "Showing the first 500 commits", so that the page stays fast.
49. As a reviewer of a capped list, I want the file list and patches to still cover the whole range, so that no change is hidden by the cap.
50. As a reviewer, I want the file list in a tree built like the TUI's tree (directories first, sorted by name, single-child chains folded, a file count per directory), so that it feels familiar.
51. As a reviewer, I want each file row to show a status letter and a change bar, so that I see what changed at a glance.
52. As a reviewer, I want directories to collapse, so that I can focus on part of the tree.
53. As a reviewer, I want a file filter box that matches a path substring, case-insensitive, and rebuilds the tree from the matches, so that I find a file fast.
54. As a reviewer, I want "No files match" when the filter has no result, so that an empty tree is explained.
55. As a reviewer, I want the page to show one file's diff at a time, so that it stays fast on a 50-file epic.
56. As a reviewer, I want `p` and `n` and Prev and Next buttons to move through files in tree order with wrap-around, and a "3 of 12" counter, so that I can page through quickly.
57. As a reviewer typing in a field, I want `p` and `n` ignored while an input has focus, so that typing never moves the file.
58. As a reviewer, I want the selected file in the URL, so that I can bookmark or share a link to one file.
59. As a reviewer, I want the refs, the narrowing and the selected file all in the URL, so that a reload or a shared link shows the same view.
60. As a reviewer, I want renames detected and shown as renames, so that a moved file does not look like a delete and an add.
61. As a desktop reviewer, I want split view by default and a Split/Unified switch, so that I choose what reads best.
62. As a phone reviewer, I want unified view by default, so that code is readable at 390px.
63. As a phone reviewer, I want two tabs, Files and Diff, so that the tree and the diff each get the full screen.
64. As a reviewer, I want large files (over 2000 lines or 60 KB of patch) collapsed with "Large diff, collapsed", the changed-line count and a Load diff button, so that one file never freezes the page.
65. As a reviewer, I want lock files and other generated files collapsed with "Generated file, collapsed" and a Load diff button, so that noise stays out of the way.
66. As a reviewer, I want a loaded large file to render in a virtualized box 70vh tall, so that a 5000-line file scrolls smoothly.
67. As a reviewer, I want unmodified lines expandable to full context in the un-narrowed view, so that I can read around a change.
68. As a reviewer of a narrowed view, I want the expand rows fixed, so that stacked per-commit patches stay true to each commit.
69. As a reviewer, I want an added or changed image (png, jpg, gif, webp, up to 2 MB) shown as a before box and an after box with sizes, so that I see the visual change.
70. As a reviewer, I want other binary files, svg files and images over 2 MB shown as "Binary file, N KB" with no preview, so that I know a file changed.
71. As a reviewer, I want "No changes" for an empty range, so that I know the range is valid and empty.
72. As a reviewer, I want `Unknown ref "x"` with a red border for a ref that does not resolve, so that I find the typo.
73. As a reviewer, I want a "No common history" notice, in the same style as "No changes", when two refs share no ancestor, so that I do not read it as an error.
74. As a reviewer, I want the ref fields to stay editable under every notice, so that I can fix the refs and go on.
75. As a reviewer, I want a Reload button, so that I can pick up new commits, since the page loads once and does not update itself.
76. As a reviewer on the dark app theme, I want the diff to use the app's colors and mono font at 12px/18px, so that it looks like the rest of the UI.
77. As a security-minded owner, I want every route behind the existing password and TOTP login, read-only, so that nothing new is exposed.
78. As a security-minded owner, I want typed refs checked so that they cannot inject git options or reach another project's repo, so that LAN and Tailscale access stays safe.
79. As a security-minded owner, I want file contents read only from git's object store, never from the filesystem, so that a path cannot reach files outside the repo.
80. As the person building the review milestone, I want each diff line addressable by file, side and line number, so that comments and highlights can attach later.
81. As a person on a machine with a pre-existing store, I want the new epic fields added by a named migration step, so that the schema guard and old stores stay valid.
82. As an operator, I want the backfill to run once per project on each machine and to be safe to re-run (it skips epics that already have a range), so that a re-run changes nothing.
83. As a person whose epic branch gx finds already merged into its target (hand-merge detection), I want gx to run the backfill's reflog matcher on the target at detection time and stamp the range only on an exact match, so that a hand merge never gets a guessed range. With no exact match gx stamps nothing and the page shows "No diff recorded".
84. As a person who uses "Mark as merged" on an epic with no PR number, I want the same rule as story 83, even in a `pr` delivery project. An epic with a PR number that I mark as merged by hand uses the PR rules (stories 18 to 22).
85. As a person whose ticket has several commits, I want the ralph-loop merge step (`StampMerged`) to stamp the `Ralph-Loop-Ticket` trailer on every commit of the ticket's pick, so that grouping and narrowing find all of them.
86. As a reviewer of an epic with tickets merged before this change, I want those tickets' earlier commits (which have no trailer) to stay in "Other commits", so that gx does not rewrite old history to group them.
87. As a reviewer, I want the page to group commits by one canonical ticket address even when a commit carries the legacy `<epic>/<id>` trailer form or the epic's old name, so that a ticket never splits into two groups.
88. As a person with an unmerged epic whose branch has no commits past its base, I want no "View diff" link, so that I do not open an empty page.
89. As a person with a done ticket, I want "View diff" only when the ticket is not `commitless` and its epic shows "View diff". When the narrowing matches no returned commit, I want an empty tree and a notice that says why: the 500-commit cap notice only when the list is truncated, otherwise "No commits of this ticket in this range" (as in a squash-merged PR epic). The notice means the narrowing never looks broken.
90. As a reviewer, I want merge commits (such as "Merge branch '<map>-design' into <map>-impl") listed with no files and no patch, and labelled as merges, so that they match what `git log -p` shows by default.
91. As a person looking at a map epic, I want no range and no "View diff" link on it, so that I read its commits as "Design" in the impl epic's diff. gx copies the impl epic's `merged_at` onto the map, but stamps no range there.
92. As a person whose hand-merged epic came in one `git pull` together with other epics, I want gx to refuse the reflog entry and show "No diff recorded", so that the stamped range never holds other epics' code.
93. As a reviewer of a squash-merged PR epic, I want a commit whose message holds more than one `Ralph-Loop-Ticket` line to have no trailer, so that a squash body never groups the whole squash under one ticket.

## Implementation Decisions

### Epic range, stamping and migration

- Two new epic fields hold the base SHA and the tip SHA. gx writes them in the same step that sets `merged_at`. A merged epic with both empty has no recorded range. An epic with no commits gets an empty range (base equal to tip), at merge and in the backfill. The page tells "no commits" (base equals tip, no link) from "no range recorded" (fields empty, link and "No diff recorded"). An unmerged epic counts as having no commits while its branch has none past its base. Field names are the implementer's choice.
- Three other paths set `merged_at`:
  - Hand-merge detection (`stampMerged` in `server/merge/handmerged.go`) finds an epic branch that is an ancestor of its target. At detection time gx runs the backfill's reflog matcher on the target and stamps only an exact match. Otherwise it stamps nothing and the page shows "No diff recorded".
  - "Mark as merged" for an epic with no PR number (`EpicMarkMerged` in `server/merge/markmerged.go`) follows the same rule. The split is on `pr_number`, not on delivery mode: in a `pr` project an epic with no PR number still uses the reflog rule, even though `syncTrunk` and `stampAndDequeueMerged` (`server/merge/prtrunk.go`) run. An epic with a PR number uses the PR rules.
  - `NothingToMerge` (`server/merge/epicmerge_root.go`) stamps an epic whose branch is missing or already an ancestor of its target. It stamps an empty range only when no commit on the target carries the epic's trailer. Otherwise it uses the reflog rule.
  - In all three, an epic with no trailer commits is checked first: gx stamps base = tip = the target tip and skips the matcher (an empty trailer set would match every entry).
- Map epics get no range and no "View diff" link. `tickets/ticketmigrate/map_done.go` copies the impl epic's `merged_at` onto the map, and the stamp and the link skip epics of kind `map`.
- Ticket trailer: `StampMerged` (`ralphloop/merge/merge.go`) cherry-picks a ticket's range and today amends only HEAD, so only the last commit of a multi-commit ticket has `Ralph-Loop-Ticket`. From now on it takes the pick's start and stamps the trailer on every commit in `start..HEAD`. All four callers pass the start: `MergeTicket`, `ralphloop/conflict.go` (two calls) and `repair/merge.go`. After a conflict the pick is `PrePickHead..HEAD` (the `MergeMarker`). The tokens, elapsed and cost trailers stay on HEAD only, so nothing double-counts. Older tickets keep their earlier commits without a trailer, so those commits land in "Other commits". gx does not rewrite old history.
- Line counts: `EpicTrailerLines` sums only trailer commits and sets `lines_added` and `lines_removed` for hand-merged and PR epics and in `migrations/helpers.go`. With every commit stamped, new epics count every commit and old epics keep their lower counts. This is intended, with no recount.
- "Has commits past its base" and "its epic shows View diff": the epic and ticket responses gain a boolean for each, computed by one `git rev-list --count` per request and never polled.
- Schema change, so a new named Migration step, never a rename or reuse of a released step. The schema snapshot is regenerated. One step both adds the fields and runs the backfill; its name is new and fixed.
- Fast-forward and stacked epics: base is the target tip before the merge, tip is the target tip after it. The target is the epic's derived target, as `DeriveRootBase` (`tickets/basederive.go`) and `EpicRefs` compute it: trunk, the blocker's branch while the blocker is unmerged, or an explicit `base:` (which can be a raw branch). The link and the stamp both use this derived base and target.
- PR delivery: gx stamps once trunk holds GitHub's merge commit. It tells the merge styles apart by comparing the PR's commit headlines with trunk's commits.
  - Merge commit (two parents): base is its first parent, tip is the merge commit.
  - Squash: base is the first parent, tip is the squash commit. The commit list shows one entry.
  - Rebase-and-merge: gx finds the PR's N rebased commits on trunk by headline. Base is the commit before the first, tip is the last.
  - Headlines that do not match: stamp nothing.
- A stacked epic whose blocker is squash-merged was stamped when it merged into the blocker's branch, and those SHAs stay in the local clone until gc. gx keeps the stamp. The page shows the diff while the objects exist and "Commits not available in this clone" once they are gone.
- gx keeps no pin ref. The decision reverses the map's earlier "keep a gx ref forever", because the full diff is needed only after a squash and the stamped tip is enough while the commits exist.
- Backfill, a named Migration step run at server start-up:
  - It runs once per machine for every registered project, and reads that machine's trunk reflog.
  - Each reflog entry is a fast-forward with `old new`, which is exactly the base and the tip. An entry matches an epic when every trailer commit of the epic lies in `old..new`. The match is by trailer containment, never by branch name, because a rename keeps the old name in the trailer. The matcher accepts any entry where `old` is an ancestor of `new`, so a hand `git merge` with a merge commit counts. It refuses an entry where `old..new` holds a trailer commit of another epic (a `git pull` that brought several epics), and the epic then shows "No diff recorded".
  - It refuses any ambiguous match and any epic that merged more than once. It stamps nothing for them and logs each refused epic with the reason.
  - Epics with no commits get an empty range (base equal to tip). Epics with no run log and epics whose commits are not on trunk are refused and logged.
  - It skips epics that already have a range. A named step runs once per project on each machine (it is then recorded in `applied-migrations.txt`), and it is safe to re-run.
  - There is no reflog protection such as `gc.reflogExpire`. The first reflog entries expire around 2027-01-10, so this step must ship before that date.
  - In this repo it recovers 110 of 201 archived epics exactly. The rest show "No diff recorded" or "No changes".

### Server

- Every route takes the project in the request (query `project=`, like the existing routes) and resolves the repo from it, so a ref of another project cannot resolve. Routes are GET only, behind the existing login, with no new pause kind. Each route also gets its CLI verb and its web-mode entry, as the repo's route rules require.
- Ref check: reject a ref that starts with `-`. One `git rev-parse --verify --end-of-options <ref>^{commit}` call per ref (`--verify` takes exactly one revision), so the response also says which field is unknown, for the red border. Only the resolved SHAs reach `git diff` and `git log`. An unknown ref returns 400 "Unknown ref". Two refs with no merge base return a "no common history" result, not an error. Responses echo the resolved SHAs.
- File list call: for the three-dot range, returns for each file its status, rename source, added and deleted line counts, and flags `large`, `binary` and `generated`, plus a `truncated` flag. At most 5000 files. Renames are detected.
  - `large`: patch over 2000 lines or 60 KB.
  - `generated`: lock files, by a list the implementer keeps.
- Patch call: one unified patch for one file. It has no size cap. The page asks for a `large` or `generated` file only after "Load diff". Binary files get no patch. The server may cache patches keyed by the resolved SHAs.
- Commit call: at most 500 commits in range order, each with SHA, subject, author time, the `Ralph-Loop-Ticket` trailer as a canonical address (or null; null also when the message holds more than one `Ralph-Loop-Ticket` line, as in a GitHub squash body), a merge marker and its changed files, plus a `truncated` flag. A merge commit has no changed files and no patch, as `git log -p` shows by default. The file list and patch calls accept a `commits=` filter for narrowing. In a narrowed view the patch call returns one patch per selected commit for the file, in commit order. Narrowing works only on returned commits.
- The server turns each trailer into the canonical address `<project>:<epic>/<id>` before it sends it, the way `MergedTickets` does. It reads the legacy `<epic>/<id>` form (`TicketTrailerValue`) and the old name of a renamed epic. The page's grouping, the ticket title lookup and the done ticket's narrowing all match on that address.
- Blob call: returns the old (merge-base) and new blob of one file. The page sends the resolved base and tip SHAs that the file-list call returned, never the typed refs, so the patch and the blobs always come from the same pair (a branch that moves between two calls cannot mix them). For a renamed file the file list carries the old path, and the call reads the old blob at the old path. It reads with `git cat-file blob <sha>:<path>` from the object store only, and serves only paths that appear in the range's file list. It rejects a path that is absolute, contains `..`, starts with `-` or holds a NUL byte. A symlink returns its target text and is never followed. Text blobs over 2 MB return a flag and no text. For png, jpg, gif and webp up to 2 MB it returns raw bytes. Other binaries, svg and larger images return a flag only.
- The page groups commits by the canonical trailer address `<project>:<epic>/<id>`. "Design" is a trailer-less commit whose subject starts with `<map>:` (the epic name without `-impl`). All other trailer-less commits, including a merge commit and the earlier commits of tickets merged before the every-commit trailer, are "Other commits". Design exists only when the page is opened with an epic. A merge commit is labelled as a merge.
- A stamped SHA missing from the clone returns a "commits not available" result, which the page shows as its own notice.

### Web UI

- One new hash route in its own lazy chunk. The URL holds the project, both refs, the epic (when present), the narrowing and the selected file.
- Library: `@pierre/diffs`, pinned to an exact version (1.6.4 at research time; 1.7.0 betas break). It takes the server's per-file unified patch. Its code lives only in the lazy Diff chunk.
- Layout (approved mock "Sidebar"):
  - Desktop: a 300px left column and one file's diff on the right. Left column: the commits disclosure, the file filter, then the file tree. Header row: two ref inputs with "..." between, Reload and the Split/Unified switch. A chip row shows the current narrowing.
  - Phone: the Files and Diff tabs. Unified is the default.
  - Dark theme only, because the app has no light theme.
- Exactly one file is mounted at a time. A loaded large file uses Pierre's `Virtualizer` in a 70vh box. The page never renders the whole range. A later milestone may mount more than one file, and the per-file mount and the tree order keep that open.
- Theming: Pierre's `unsafeCSS` maps its background to the app's surface token and its font to the app's mono token, and sets 12px/18px on `pre` and `code` (the font-size variable alone did not take effect). Pierre's own dark theme colors the syntax.
- Line anchors: the diff exposes each line by file, side and line number through Pierre's `DiffLineAnnotation` model. No annotation UI ships now.
- Notices replace the diff area and keep the ref fields editable: "No changes", `Unknown ref "x"` (red border on the unknown field), "No diff recorded" (amber), "No common history", "Commits not available in this clone" (amber, in the same area as "No diff recorded"). "No diff recorded" is also what a merged epic without a range shows. Both amber notices use a reason-neutral text: "gx has no exact range for this epic. Type two refs above to compare them." The mock's text "This epic merged before gx stored its range" is wrong for non-matching PR headlines, multi-merge epics and a squash-merged blocker, and is replaced.
- Entry points: the epic page, the archived epic page and a done ticket's page. A running or complete epic links from its branch. "View diff" shows on every merged epic except empty ones (base equal to tip). An unmerged epic shows it once its branch has commits past its base. An epic with no commits shows "No changes" and no link. A done ticket shows "View diff" only when it is not `commitless` and its epic shows "View diff". A narrowing that matches no returned commit shows the cap notice and an empty tree.
- Prototype note: React 19 logs `flushSync was called from inside a lifecycle method` on every page of the app, so it is not a Diff page defect.

### Measured in the mock (`@pierre/diffs` 1.6.4, dev build, Chromium)

- Mounting 50 diffs at once is sluggish, which is why one file mounts at a time.
- A 5000-line file in split view without virtualization took 2.1 s with 10,000 DOM rows and about 365 MB heap. In `Virtualizer` it took 0.6 s with 400 rows and about 200 MB.
- A typical small diff renders in 46 to 350 ms, grammar load included.
- Not tried: split view on a phone and image diffs with real blobs.
- Expand context (map ticket gx:epic-diff/10, prototype branch `prototype/epic-diff-mock`, commit `62eec7a15`): story 67 works as designed. Pierre's `loadDiffFiles` option takes the blob call: it gets the file's diff metadata (`name`, `prevName`, `type`) and returns `{ oldFile, newFile }` as `{ name, contents }`, with `oldFile: null` for a pure rename. Pierre calls it on the first click on an expand row and caches the result for that file.
  - Real blobs of a 207-line file: the expand rows grew to the exact line counts and the file read whole after the last click, in split and unified view.
  - A 4773-line file with 42 changes, in `Virtualizer`: 20 to 60 ms per expand click, also in the middle of the file after a scroll. The DOM stayed at 200 to 400 lines. Not virtualized, expanding every row gave 9545 DOM lines and 8 s of clicks, so a loaded large file must stay in `Virtualizer`.
  - A failed blob call (HTTP 500) left the diff as it was, and the next click retried and worked.
  - Before the blobs load, the trailing row after the last hunk reads "More unchanged context may be available" with no count. After the load it shows the exact count.
  - A patch and blobs from different SHAs make Pierre throw (`deletionLine and additionLine are null`) and the file's diff area goes empty. The patch and the blob call must use the same resolved SHA pair.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response of a route, the DOM the person sees, the epic's stored fields. It does not check helper functions or internal state.
- Seams (one new seam at the highest level, plus the existing ones):
  - The server routes, tested with `servertest` against a real repo from `testutil.TempRepo`. This covers the file list, patch, commit and blob calls, ref rejection, the size flags, narrowing, and the stamping at merge time (fast-forward, stacked, and PR styles with the fake gh).
  - The migration, tested against a temp repo whose trunk reflog is built by real fast-forward merges. It covers exact matches, an ambiguous match, a double-merge epic, an epic with no commits, a missing run log and a second run that changes nothing.
  - The web page, with vitest on grouping, tree building, filter, narrowing and notices, and one Playwright flow for the round trip and the layout (the repo's rule is no new flow unless a real server round trip or browser layout is needed, and this page needs both). The suite has 143 flows and `maxFlows` is 143 (`playwrightguard/budget_test.go`), so the ticket that adds the flow raises it from 143 to 144, and its title says why (the Diff page needs a server round trip and browser layout). The flow keeps only the round trip and layout checks: links, URL reload, keys, phone tabs, the virtualized scroll and computed styles. Render-only checks (the notices, "No files match", "Binary file, N KB", the `...`) go to vitest, per `gx-playwright-e2es`, which also keeps the flow under the 45 s slow limit.
- Prior art: the `servertest` fixtures and `WriteTicketWith`; the migration tests and schema snapshot guard; the existing Playwright flows for epic pages and the phone layout; the `gxfixture` scenarios.
- No test reads a developer's real repo or the live server's state.

## Verification

A verify ticket runs these against a temporary gx server. All stories need the new scenario `epic-diff` in `gxfixture`, with shapes below, and one new flow "Diff page" (new file) that runs the shapes and needs `maxFlows` raised from 143 to 144 (see Testing Decisions). Entries that say "vitest" are render checks and stay out of the flow. The Mock states list at the end holds the look checks. Stories refer to a Mock states line by its state name.

New scenario `epic-diff`, shapes (implementation tickets):
- `merged` (default): a merged epic with a stamped range, 15 commits across 3 tickets (one ticket with the trailer on every commit, one with the legacy `<epic>/<id>` trailer form, one with only its last commit stamped as an older ticket), a design commit (`<map>:` subject), a trailer-less other commit, a "Merge branch" commit, a rename, an added image, a lock file and a 5000-line source file, plus one ticket's commits not in one run. It also holds an archived merged epic (story 11) and a done commitless ticket.
- `running`: an unmerged epic with its branch; a stacked epic whose blocker is unmerged; a second stacked epic whose blocker has merged (the link uses trunk, story 8); an epic with an explicit `base:` (story 6); an unmerged epic with no commits past its base.
- `unstamped`: a merged epic with no range.
- `empty`: a merged epic with no commits (stamped empty range).
- `missing-sha`: a merged epic whose stamped tip is absent from the clone.
- `pr`: PR-delivery epics merged by merge commit, squash and rebase-and-merge, one with non-matching headlines (uses the fake gh), a done ticket in a squash-merged epic whose squash body holds several trailer lines, and a stacked epic behind a squash-merged blocker (its stamped objects exist, and a variant with them removed).
- `backfill`: a store with merged epics and no stamped range, more than one project, a real trunk reflog, one ambiguous match, one entry holding two epics (from one `git pull`), one double-merge epic, a map epic copied from its impl epic, an epic with no run log, an epic whose commits are not on trunk, and an epic with no commits.

Entries:

Entry 1: typed refs and page shell (stories 1 to 5, 74 to 74, 75 to 75)

flow "Diff page" on `merged`: open the page with two typed refs, change a field, press Reload. Vitest: the `...` is shown between the fields. CLI: the file-list verb with the two refs prints the same files as the page. Mock states `desktop-default`.

Entry 2: any ref, injection (stories 3 to 3, 78 to 78)

server test: a branch, a tag and a SHA resolve; a ref starting with `-` and an unknown ref return 400 "Unknown ref" (one `--verify` call per ref); the response carries SHAs only. CLI: the file-list verb with `--output=x` as a ref exits non-zero.

Entry 3: links before the merge (stories 6 to 9, 88 to 88)

flow "Diff page" on `running`: the epic page link opens the derived base and branch; the first stacked epic's link uses the blocker's branch; the second stacked epic (blocker merged) uses trunk; the epic with an explicit `base:` uses it; the unmerged epic with no commits past its base has no link. Server test: the epic response's "has commits past its base" boolean is false for that epic and true for the others. Mock states `desktop-default`.

Entry 4: links after the merge (stories 10 to 10, 11 to 11, 14 to 14)

flow "Diff page" on `merged` and `unstamped`: the merged epic and the archived epic page open the stamped range; the unstamped epic opens empty ref fields. Vitest: the "No diff recorded" notice with the reason-neutral text. Mock states `desktop-no-diff-recorded`.

Entry 5: done ticket link (stories 12 to 12, 89 to 89)

flow "Diff page": a done ticket's "View diff" opens the epic page's Diff page narrowed to that ticket, and the done commitless ticket has no link. Server tests: the epic and ticket responses carry the "shows View diff" boolean. Vitest: a narrowing that matches no returned commit shows an empty tree, with the cap notice when `truncated` and "No commits of this ticket in this range" otherwise. Mock states `desktop-commits-narrowed`.

Entry 6: empty epic (stories 13 to 13, 29 to 29)

flow "Diff page" on `empty`: no "View diff" link on the epic page. Vitest: typed equal refs show "No changes". Mock states `desktop-empty-range`.

Entry 7: stamp at merge (stories 15 to 17, 83 to 83, 84 to 84, 91 to 91)

Go test with `servertest`: merging a fast-forward epic and a stacked epic sets base SHA and tip SHA together with `merged_at`; both equal the derived target tip before and after, including an epic with an explicit `base:`. A merged epic with no commits is stamped with base equal to tip. Hand-merge detection, `NothingToMerge` and "Mark as merged" with no PR number (also in a `pr` project) stamp only on an exact reflog match and otherwise stamp nothing; an empty epic on any of these paths stamps base = tip; `NothingToMerge` on an epic merged outside gx uses the reflog rule, not an empty range; an entry holding another epic's commits is refused; a map epic gets no range; an epic with a PR number marked merged by hand uses the PR rules. CLI: `gx tickets show` of the epic lists both fields.

Entry 8: PR styles (stories 18 to 23, 93 to 93)

Go tests on the `pr` shape: merge commit, squash and rebase-and-merge each stamp the stated base and tip; non-matching headlines stamp nothing and the page shows "No diff recorded"; in the squash epic the commit call returns a null trailer for a squash body with several `Ralph-Loop-Ticket` lines, and a done ticket's narrowing there shows "No commits of this ticket in this range"; a stacked epic behind a squash-merged blocker keeps its stamp and shows the diff while its objects exist, and "Commits not available in this clone" once they are removed. Mock states `desktop-no-diff-recorded`.

Entry 9: migration and backfill (stories 24 to 28, 30 to 30, 81 to 81, 82 to 82, 92 to 92)

Go test on the `backfill` shape: the step stamps exact matches, stamps an empty range for the epic with no commits, refuses the ambiguous and double-merge epics and the entry holding two epics, stamps nothing on the map epic, the epic with no run log and the epic not on trunk, logs each refusal with its reason, and skips an epic that already has a range. A re-run with the applied list skipped (as `archivefields_test.go` does with `SkipApplied`) changes nothing. `go test ./migrations -run TestSchemaSnapshot` passes after the snapshot update. Flow "Diff page" on `unstamped`: the ref fields are empty. Vitest: "No diff recorded". Mock states `desktop-no-diff-recorded`.

Entry 10: missing SHA (stories 31 to 31)

flow "Diff page" on `missing-sha`: no diff and the ref fields stay editable. Vitest: the notice "Commits not available in this clone", amber, in the same area as "No diff recorded". Mock states `desktop-no-diff-recorded` (same notice area, amber).

Entry 11: grouping (stories 32 to 39, 86 to 86, 87 to 87, 90 to 90)

server test: the commit call returns the canonical address for a modern trailer, a legacy `<epic>/<id>` trailer and a renamed epic's old name, and marks a merge commit with no files. Vitest on grouping: trailer groups with titles, order by first commit, Design by `<map>:` subject, Other commits (including the older ticket's earlier commits and the merge commit, labelled as a merge), empty commit shown, no Design without an epic. CLI: the commit verb prints the trailer address for each commit. Mock states `desktop-commits-narrowed`.

Entry 12: commit list and narrowing (stories 40 to 47)

flow "Diff page": the list is collapsed at load; opening it shows All commits, Design and tickets; clicking a ticket narrows the tree and shows the chip; clearing the chip restores the full tree. Vitest: narrowing to "Design", to "Other commits" and to a single commit each filters the tree and sets the chip. Mock states `desktop-commits-narrowed`.

Entry 13: narrowed patches (stories 44 to 44, 45 to 45, 90 to 90)

server test: with `commits=` the patch call returns one patch per selected commit in commit order, a ticket's non-adjacent commits omit the commits between them, and a merge commit returns no patch. Vitest: a narrowed file shows commit subjects as labels. Mock states `desktop-commits-narrowed`.

Entry 14: 500 cap (stories 48 to 48, 49 to 49)

server test with 501 commits: the commit call returns 500 and `truncated`, the file list still lists every file. Vitest: the notice "Showing the first 500 commits". Mock states `desktop-default`.

Entry 15: file tree and filter (stories 50 to 54)

vitest on tree building (order, folding, counts), collapsing a directory hides its rows (story 52), and a file row shows its status letter and change bar (story 51). Flow "Diff page": typing in the filter rebuilds the tree. Vitest: a string with no match shows "No files match". Mock states `desktop-filtered` and `desktop-filter-no-match`.

Entry 16: one file, keys, URL (stories 55 to 59)

flow "Diff page": `p` and `n` move and wrap, the counter updates, keys are ignored while the filter has focus, the URL holds the file and narrowing, and a reload at that URL shows the same file. This flow declares one reload. Mock states `desktop-default`.

Entry 17: renames (stories 60 to 60)

server test: a renamed file is listed with its source and status R. Mock states `desktop-default`.

Entry 18: view mode and theme (stories 61 to 61, 62 to 62, 76 to 76)

flow "Diff page" at 1200px and 390px: split by default on desktop, unified on phone, the switch changes it; the diff background equals the app's surface color and the code font is 12px/18px (computed style). Mock states `desktop-default` and `phone-diff`.

Entry 19: phone tabs (stories 63 to 63)

flow "Diff page" at 390px: the Files and Diff tabs, with no sideways page scroll. Mock states `phone-files` and `phone-diff`.

Entry 20: large and generated (stories 64 to 66)

server test: a patch over 2000 lines or 60 KB is flagged `large`, a lock file `generated`, and the patch call sends them on request. Flow "Diff page": Load diff on the 5000-line file renders in a 70vh box and scrolling reaches the middle. Mock states `desktop-large-file`.

Entry 21: expand (stories 67 to 67, 68 to 68)

flow "Diff page": expanding context works un-narrowed (a real blob round trip). Vitest: the narrowed view shows no expand rows. Mock states `desktop-default` and `desktop-commits-narrowed`.

Entry 22: images and binaries (stories 69 to 69, 70 to 70)

server test: the blob call returns bytes for a png of 2 MB or less and a flag for svg, other binaries and a png over 2 MB. Vitest: the image file shows before and after boxes with sizes; a non-image binary shows "Binary file, N KB". Mock states `desktop-image-file`.

Entry 23: empty notice (stories 71 to 71, 74 to 74)

see entry 6. Mock states `desktop-empty-range`.

Entry 24: unknown ref (stories 72 to 72, 74 to 74)

server test: an unknown ref returns 400 and names the unknown field. Vitest: a typed unknown ref shows `Unknown ref "x"` with a red border on that field and the fields stay editable. Mock states `desktop-unknown-ref`.

Entry 25: no common history (stories 73 to 73)

server test and vitest: two refs with no common ancestor show "No common history" with editable fields. Mock states `desktop-empty-range` (same notice style).

Entry 26: auth (stories 77 to 77)

`TestWeb_EverythingElseIs404` covers the new routes; a signed-out API call gets 401, and a non-GET method is refused. CLI: each new route has its verb (`TestRouteTableHasCLIVerbs`).

Entry 27: object store only (stories 79 to 79)

server test: a blob call for a path outside the file list, an absolute path, a `..` path, a leading `-` and a NUL byte is refused; a symlink returns its target text; a dirty file on disk is not read.

Entry 28: line addressing (stories 80 to 80)

vitest: the diff component exposes a line annotation by file, side and line number for an added and a deleted line.

Entry 29: trailer on every commit (stories 85 to 85, 86 to 86)

Go test in `ralphloop/merge`: `StampMerged` on a multi-commit ticket leaves `Ralph-Loop-Ticket` on every commit of the pick, not only HEAD, including a pick resolved after a conflict (`PrePickHead..HEAD`); the tokens, elapsed and cost trailers stay on HEAD only. Server test: a commit without the trailer, from a ticket merged before the change, comes back with a null trailer and lands in "Other commits".


**Mock states**

- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-default`, 1200px, the `epic-diff` scenario `merged`, split view, first file open, commits collapsed: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree, file-nav, file-card, diff-body.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-commits-narrowed`, 1200px, the `epic-diff` scenario `merged` narrowed to one ticket: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree, file-nav, file-card, diff-body.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-filtered`, 1200px, the `epic-diff` scenario `merged` with a filter typed: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree, file-nav, file-card, diff-body.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-filter-no-match`, 1200px, the `epic-diff` scenario `merged` with a filter that matches nothing: ref-bar, view-toggle, commits-disclosure, file-filter, file-nav.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-large-file`, 1200px, the `epic-diff` scenario `merged` with the 5000-line file selected before Load diff: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree, file-nav, file-card.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-image-file`, 1200px, the `epic-diff` scenario `merged` with the added image selected: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree, file-nav, file-card.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-empty-range`, 1200px, the `epic-diff` scenario `empty` opened with equal refs: ref-bar, view-toggle, notice.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-unknown-ref`, 1200px, the `epic-diff` scenario `merged` with a typed unknown ref: ref-bar, view-toggle, notice.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `desktop-no-diff-recorded`, 1200px, the `epic-diff` scenario `unstamped`: ref-bar, view-toggle, notice.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `phone-files`, 390px, the `epic-diff` scenario `merged`, Files tab: ref-bar, view-toggle, commits-disclosure, file-filter, file-tree.
- `/Users/david/.local/share/gx/tickets/gx/epic-diff/mocks/04-mock-the-diff-page.html`, `phone-diff`, 390px, the `epic-diff` scenario `merged`, Diff tab in unified view: ref-bar, view-toggle, file-nav, file-card, diff-body.

## Out of Scope

- Human code review: focus highlights and comments sent to the agent. The next milestone. Only the line addressing is kept.
- A diff view in the TUI, which has its own log and diff views.
- Live update while tickets merge. The page loads once and has a Reload button.
- A two-dot comparison and an "ignore whitespace" switch.
- A light theme for the Diff page. The app has only a dark theme.
- A pin ref or any other protection that keeps squashed commits reachable, and any reflog protection.
- Ranges for epics that merged more than once, whose commits are not on trunk, or that have no matching reflog entry. They show "No diff recorded".
- Rewriting the trailers of tickets merged before the every-commit trailer. Their earlier commits stay in "Other commits".
- Rendering more than one file at a time.

## Further Notes

- The step that stamps old epics must ship before about 2027-01-10, when trunk reflog entries start to expire. After that the exact range for older epics is lost.
- Route names, query names, wire field names, the lock-file list and the patch cache are the implementer's choices.
- Research: `docs/research/epic-diff-rendering-library.md` and `docs/research/epic-diff-merged-epic-ranges.md`. Terms **Diff page** and **Epic range** are in `GLOSSARY.md`.
- The stored mock is a standalone HTML file with 11 states. The prototype code on branch `prototype/epic-diff-mock` is throwaway and was never merged.

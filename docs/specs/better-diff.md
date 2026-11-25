# Better diff: a calmer Diff page and Show diff

## Problem Statement

The **Diff page** is cluttered. The ref bar, a Reload button, Prev and Next buttons, a counts line, a narrow chip, a commits disclosure, a file filter and a card header all compete for space. The file tree uses the mono font and takes much room. The diff opens split by default, the colors are hard to read, and the diff lines do not use the gx mono font because the current font rule matches nothing. The page scrolls as one block, so picking a file low in the tree moves the diff out of view.

I also cannot look at what an agent is doing right now. The page only compares two refs, so a chat's working directory, a Design worktree or a claimed ticket's worktree has no diff in the web UI. I have to open a terminal and run git.

## Solution

A calmer Diff page, plus a **Show diff** entity action that opens a **Worktree diff**.

- One header bar holds the title, one range chip, the counts and the Unified/Split toggle. Reload, Prev/Next, the counts line, the narrow chip and the file card header are gone.
- Commits sit in one collapsed row above the file tree. The file tree uses Saira with status dots and counts. The file filter shows only above 15 files, or when I press `/`.
- The diff opens unified on every device. The toggle stays and the browser remembers my choice.
- The tree and the diff scroll on their own.
- The diff uses new colors and the gx mono font.
- **Show diff** is one entity action. It is in the action menu of a chat, a Design, a ticket and an epic, and in the command palette's "This page" section. It opens the **Epic range** for an epic and a done ticket, and a **Worktree diff** for a chat, a Design and a claimed or paused ticket with an iteration worktree. There is no global picker.

A Worktree diff shows the commits since a base plus uncommitted edits. Untracked files that git does not ignore show as added.

## User Stories

1. As a reviewer, I want the Diff page to have one header bar, so that the page is not cluttered.
2. As a reviewer, I want the header to show the title, one range chip, the file and line counts and the Unified/Split toggle, so that I see everything I need in one row.
3. As a reviewer, I want no Reload button, so that there is one control fewer to ignore.
4. As a reviewer, I want the page to refetch when the browser window gets focus and after I edit the refs, so that the diff stays current without a button.
5. As a reviewer, I want the commit narrowing and the selected file to stay after a refetch, and the first file of the new list selected when my file is gone, so that I do not lose my place and never see a blank diff.
6. As a reviewer, I want no Prev and Next buttons and no counts line, so that the toolbar is quiet, and I want the `n` and `p` keys to keep working.
7. As a reviewer, I want the commits in one collapsed row above the tree, so that they take no room until I open them.
8. As a reviewer, I want the picked commit or ticket group highlighted in the commits list, so that I know what the view is narrowed to without a chip.
9. As a reviewer, I want the file filter hidden until the range has more than 15 files or I press `/`, so that small diffs have no filter box.
10. As a reviewer, I want the file tree in Saira with a status dot and `+n −n` counts per file, so that the tree is narrow and readable.
11. As a reviewer, I want the diff to open unified on every device, so that I read it in one column.
12. As a reviewer, I want the Unified/Split choice remembered in my browser, so that I choose it once.
13. As a reviewer, I want the tree and the diff to scroll on their own, so that picking a file low in the tree never moves the diff out of view.
14. As a reviewer, I want a sticky path line above the diff instead of a card header, so that I always see which file I read, and I want a renamed file shown as "old path → new path".
15. As a reviewer on a phone, I want the Files and Diff tabs kept, and picking a file to switch to the Diff tab, so that the small screen works.
16. As a reviewer, I want new colors for added and deleted lines, their gutters, the word highlights and the syntax tokens, so that the diff is easy to read.
17. As a reviewer, I want the diff lines in the gx mono font at 12.5px with a 19px line height, so that the code matches the rest of the app.
18. As a reviewer, I want an Epic range's chip to read `main → <tip>` with an edit link, so that I can still compare other refs.
19. As a reviewer, I want the edit link to swap the chip for the two ref inputs, Enter in either input to apply and close them, and Esc or the edit link again to cancel, so that I change refs without an Apply button.
20. As a reviewer, I want an unknown ref marked invalid in its input, so that I see which ref to fix.
21. As a reviewer, I want a failed load to show a "Try again" button inside its notice, so that I can retry without Reload.
22. As a reviewer, I want the unknown-ref notice to say "Fix the ref with the edit link" and the commits-not-available notice to say "Fetch them, then return to this window", so that the notices match the controls that exist.
23. As a person running agents, I want a Show diff action on a chat, so that I see what that chat changed in its working directory.
24. As a person running agents, I want Show diff on a Queue agent chat and on a Fix agent chat, so that I see their worktrees too. A Queue agent shows its running iteration since the iteration's own base, and a Fix agent shows its directory since the trunk merge-base.
25. As a person running agents, I want Show diff on a Design, so that I see what the Design worktree changed since trunk.
26. As a person running agents, I want Show diff on a claimed or paused ticket whose iteration worktree exists, so that I see that worktree since the base the iteration was created from.
27. As a person running agents, I want a stacked ticket's diff to hide its blocker's commits, so that I see only the ticket's own work.
28. As a person running agents, I want Show diff on a done ticket to open its commits, as the "View diff" link does today.
29. As a person running agents, I want Show diff on an epic to open its Epic range, as the "View diff" link does today.
30. As a person running agents, I want Show diff in each page's action menu and in the command palette's "This page" section, so that I reach it with the mouse or the keyboard.
31. As a person running agents, I want the existing "View diff" links on the epic page and a done ticket's header to stay and open the same page the action opens, so that nothing I know disappears.
32. As a person running agents, I want Show diff hidden on a chat whose directory is not a worktree of the project's repo, on a ticket that has no iteration worktree and no commits, and on an epic with no range, so that the action never leads nowhere.
33. As a reviewer, I want a Worktree diff's chip to read "Worktree · <target> since <base>@<sha>" with no ref inputs and no edit link, so that the header does not offer refs that make no sense.
34. As a reviewer, I want a Worktree diff to include commits since the base and uncommitted edits, so that I see everything the agent changed.
35. As a reviewer, I want untracked files git does not ignore shown as added, and ignored files never shown, so that new files appear and build output does not.
36. As a reviewer, I want a Worktree diff's commit list to end with one "Uncommitted changes" row, so that I can narrow the view to the edits that are not committed yet.
37. As a reviewer, I want a Worktree diff to leave the worktree's index and files untouched, so that a running agent is never disturbed.
38. As a reviewer, I want a link to a Worktree diff to reopen the same target, so that a reload or a shared link works, and I want the link to show the current diff, not the one from when it was copied.
39. As a reviewer, I want a target with no worktree (an unclaimed ticket, a deleted chat) to show an empty state naming the target, so that I know why nothing shows, and I never want a ref diff in its place.
40. As a reviewer, I want a worktree with no changes to say "No changes since <base>", so that an empty diff is not mistaken for a failure.
41. As a reviewer, I want a worktree with more than 500 untracked files, or an untracked file over 2 MiB, to leave those files out and say so, so that a huge untracked tree never makes the page slow.
42. As a reviewer, I want images, renamed, deleted and binary files in a Worktree diff to show as they do in an Epic range, so that both modes behave alike.

## Implementation Decisions

**Page structure**

- The Diff page has one mode switch. A ref mode reads the project and two refs, as today. A worktree mode reads the project and a worktree target. The URL names the mode, and the page shows one header in both.
- The header chip differs by mode. Epic range or typed refs show `<base> → <tip>` and an edit link. A Worktree diff shows "Worktree · <target> since <base>@<sha>". The target is the chat name, the Design name or the ticket address.
- The counts in the header sum the files the view shows (files, added, deleted). The old counts line is removed and its numbers live only in the header.
- The page height is the viewport. The tree column and the diff pane each scroll on their own, and the path line is sticky in the diff pane. On a phone the Files and Diff tabs stay, and each tab scrolls on its own.
- The commits row reads "N commits" and opens to the grouped list. The narrow chip is removed. The picked group or commit row is highlighted, and "All commits" clears the narrowing. Narrowing, the selected file and the existing `n` and `p` keys keep working.
- The file filter renders only when the unfiltered file list has more than 15 files, or after `/` is pressed outside a text field.
- The file tree row is a status dot (added uses the done tone, anything else uses the waiting tone), the name in Saira, and `+n −n` in the tone colors. Directories are plain dim labels, and the change bar is gone. The selected row uses the sidebar's current color.
- The path line shows the file's path, or "old path → new path" for a rename. The status letter and counts of the old card header are not shown; the tree row carries them. Large and generated files keep their collapsed "Load diff" body, and a failed patch keeps its Retry button.
- The refetch runs on window focus and after a ref edit. It applies to both modes and keeps the narrowing. It keeps the selected file when that file is still in the current list, and otherwise selects the first file of the current list. A Worktree diff builds a new snapshot on each refetch.
- Unified is the default on every device. The Unified/Split choice is stored in the browser. A missing or unreadable store falls back to unified.
- Notices change where they named Reload. The failed-load notice has a "Try again" button. The unknown-ref notice says to fix the ref with the edit link. The commits-not-available notice says to fetch them and return to the window. The ref inputs keep their invalid mark.
- On an Epic range, the edit link replaces the chip with the two ref inputs. Enter in either input applies and closes them. Esc, or the edit link again, cancels. There is no Apply button.

**Rendering**

- Colors come from a custom CSS-variable theme registered once at module load with `registerCustomCSSVariableTheme`, plus one `:host` block in `unsafeCSS`. There is no `!important`. The syntax tokens, the added and deleted accents, the word-highlight overrides, the context gutter and the font variables are set there from the app's Chakra tokens. The exact token mapping and values are the ones ticket 04 probed and recorded:
  - foreground uses `text.muted`, comment `text.faint`, parameter `text.muted`, punctuation `text.dim`
  - keyword uses `kind.design.fg`, string and string-expression use `kind.implementation.fg`, function uses `tone.running.fg`, constant uses `tone.waiting.fg`
  - addition accent uses `tone.done.fg`, deletion accent uses `tone.deadlocked.fg`
  - word emphasis is `color-mix(in srgb, var(--chakra-colors-tone-done-fg) 30%, transparent)` for additions and the same mix with `tone-deadlocked-fg` for deletions, because the variable takes a final color
  - background and context gutter use `surface`
  - row and gutter tints stay the library's own mix of the accent and the surface
- The font rule moves from `.diffs-host` (it matches no element, so the mono font never applied) to `:host`: `--diffs-font-family` is the mono token, `--diffs-font-size` 12.5px, `--diffs-line-height` 19px. A `pre, code` rule sets the same size and line height, because the variable alone leaves `code` at 16px. This came from a browser probe in ticket 04.
- The diff renderer stays behind the one diff-body module. Only that module touches `@pierre/diffs`.

**Show diff action**

- It is one entry in the entity action registry, in the "open" group, with no chord, no confirm and no bulk rule. Its menu and palette rows come from the registry, so the palette's "This page" section needs no new code.
- Availability reads the entity snapshot only. The chat and ticket responses (and the entity built from them) gain the fields the rule needs: whether a chat's directory qualifies (the git-common-dir check runs once per session directory and is cached while the session lives, never per list request), whether a ticket has an iteration worktree, and the ticket's status and commit state. The epic response already says whether today's link shows. The ticket page's open entity must carry its status. The implementer picks the field names.
- By kind:
  - chat (plain, Queue or Fix agent): its directory, only when `git rev-parse --git-common-dir` of that directory equals the project repo's. A separate clone is hidden, because its snapshot objects are not in the project's object store. Builder, Triage and other managed sessions do not get the action.
  - Design: its Design worktree.
  - ticket: shown when the ticket is claimed or paused and its iteration worktree exists. Any other ticket falls back to the done-ticket rule: shown when today's link shows, which opens the commits. Otherwise hidden.
  - epic: its Epic range, where today's link shows.
- Running the action navigates to the Diff page URL for the entity. The existing "View diff" links stay and navigate to the same URL.

**Worktree diff URL and server**

- The URL is `/diff?project=<p>&worktree=<kind>:<id>` with kind `chat`, `design` or `ticket`, for example `worktree=chat:abc123`, `worktree=design:better-diff`, `worktree=ticket:gx:better-diff-impl/03`. The base is not in the URL, so a shared link shows the current diff. URL parsing and building stay in the one URL module.
- One new read route resolves a worktree target and builds the snapshot. It takes the project and the target. It returns the base SHA, the HEAD SHA, the snapshot SHA, whether uncommitted changes exist, the base's label, and a reason when the target has no worktree. The file list, patch, blob and commit routes stay as they are and take SHAs; the page calls them with the returned SHAs.
- The base is the merge-base of HEAD with trunk for a plain chat, a Fix agent chat and a Design.
- For a claimed or paused ticket the target is its iteration worktree. The base is the iteration worktree's own base, the one persisted in the run entry (`IterationWorktree.Base()`), whenever the run is tracked. It is not the ticket's resolved base. A stacked ticket therefore does not show its blocker's commits.
- When the run entry is gone (a paused ticket whose run is no longer tracked), the server derives the base without new persistence. For a stacked ticket it is the merge-base of the worktree's HEAD with the blocker's iteration branch, if that branch still exists. Otherwise it is the merge-base of HEAD with the epic's feature branch.
- A Queue agent chat whose run is not tracked has no base to derive from. It shows the empty state naming the target.
- The chip's base label is the epic's feature branch name for an iteration base, the blocker ticket's address for a stacked ticket, and the trunk name for a trunk-based target, each followed by `@<sha>`.
- `chat:<id>` for a Queue agent resolves to its running iteration and uses the same base as `ticket:`. `chat:<id>` for a Fix agent uses its directory and the trunk merge-base.
- The snapshot (from the worktree research, ticket 03):
  1. Copy the worktree's index to a private temporary file.
  2. With the copy named in `GIT_INDEX_FILE` on the child process only, run `add -u` (tracked changes and deletions), then `add --` with only the kept untracked paths from `git ls-files -o --exclude-standard` (after the 500-file and 2 MiB rules below), then `write-tree`.
  3. Run `commit-tree` on that tree with HEAD as parent, which gives the snapshot SHA S.
  4. Diff `base..S` with the existing range code.
- The snapshot commit is never signed, and the server gives it its own author and committer, so the user's git config can neither prompt nor fail it. The worktree's own index, lock files and file times are never touched. The only side effect is loose objects that `git gc` prunes later. Objects land in the repo's shared store, so the existing routes read S from the project's repo.
- The kept untracked files are in S as added files, so no separate untracked path exists. Renames pair across commits and uncommitted edits. Binary files and images read from S through the existing blob route.
- The commit list is read for `base..HEAD`, never `base..S`. The server's response flags uncommitted changes, and the page appends one "Uncommitted changes" row in the waiting tone. Narrowing to that row shows the `HEAD..S` changes.
- Before the snapshot, the server lists unignored untracked files with `git ls-files -o --exclude-standard`. The cap is fixed at 500 files. Above it, no untracked file goes into the snapshot. Independently of the count, a single untracked file larger than the blob route's size limit (2 MiB, the point where a blob reads as too large) is left out. The page shows one notice listing the count of left-out files, "Untracked files not shown (N). Add them to .gitignore or commit them." Tracked and committed changes still show. The file list's own cap of 5000 files is a separate limit and is unchanged.
- A target with no worktree (an unclaimed ticket, a deleted chat, a missing Design worktree) answers with the empty-state reason, and the page shows an empty state naming the target. It never falls back to a ref diff. A worktree with no changes shows "No changes since <base>".

**Reuse**

- The glossary already defines **Diff page**, **Worktree diff** and **Show diff**; the spec and the code use those words. No new term is needed.

## Testing Decisions

A good test checks what a person sees or what a route returns, not how a component or a function is built.

- **Highest seam, the web e2e flows.** The Diff page flow in `diff-page.e2e.ts` and the registry rows in the jump flow cover the page, the action, the menu and the palette against a fixture server. New flows are listed in Verification. This is the main seam.
- **Server seam.** The new worktree route is tested at its HTTP route with real git repos in a temp dir, like `diff_test.go`. Cases are committed plus uncommitted edits, untracked and ignored files, a stacked ticket's iteration base, a paused ticket with no run entry (a stacked and an unstacked one), a Queue agent chat and a Fix agent chat, a chat in a separate clone, more than 500 untracked files, an untracked file over 2 MiB, a missing worktree, and that the worktree's index sha, mtime and lock state do not change. Tests follow the repo's Go test rules (bounded processes, readiness waits).
- **Registry seam.** `registry.test.ts` covers Show diff availability per entity kind and state, in the style of the existing rules tests.
- **Component tests** with Vitest stay where they exist (`DiffPage`, `FileTree`, `CommitList`, `DiffBody`). Update them for the removed controls and for the unified default, the remembered toggle and the 15-file filter rule. The `DiffBody` test also covers that the `:host` block carries the font and that no rule uses `!important`.
- **Prior art.** `diff_test.go`, `viewdiff_test.go`, `registry.test.ts`, `DiffPage.test.tsx` and the `epic-diff` fixture scenario.

## Verification

A verify ticket runs these against a temporary gx server after the epic is built. The page checks use flows of `diff-page.e2e.ts` (scenario `epic-diff`) and the registry rows of `jump.e2e.ts`. The Worktree diff needs a new fixture scenario, named here `worktree-diff`. It seeds a project with a chat, a Queue agent on a claimed ticket and a Design worktree, each with real git history, uncommitted edits, an untracked file and an ignored file. It also seeds a ticket that is not claimed, and a worktree with more than 500 untracked files, one with an untracked file over 2 MiB, a Fix agent chat, a Queue agent chat, a paused ticket with a tracked run, a paused ticket with no run entry (stacked on a blocker whose iteration branch exists, and unstacked), a Queue agent chat with no tracked run, a rename across a commit and an uncommitted edit, a deleted file, and an untracked image. The implementer picks the final name.

Calmer page (stories 1 to 15, 18 to 22)
- Flows: the **Diff page** flow in `diff-page.e2e.ts`, updated. It no longer asserts the ref bar, Reload, Prev/Next, the narrow chip or the file card header. It asserts one header with the range chip and the toggle. It checks the unified default and the remembered toggle after a reload, the 15-file filter rule and `/`, the tree and the diff scrolling on their own, `n` and `p`, the highlighted commit row, the edit link (Enter applies, Esc cancels, an unknown ref marks the input), the focus refetch keeping the narrowing and the file (or selecting the first file when it is gone), and the 390 px Files and Diff tabs. Look check: the Mock states `a-desktop-range` and `a-phone-diff`.
- Checks: `gx brief -C webui playwright diff-page.e2e.ts <flow numbers>` exits 0. On the `missing-sha` shape, the commits-not-available notice has no "Reload" text. A forced failed load shows a "Try again" button that reloads the page data.
- New flows to write: the focus refetch (dispatch a window focus after the fixture changes the range, once with the selected file still present and once after it was removed), the edit link keyboard rules, and the failed-load "Try again". These become implementation tickets.

Colors and font (stories 16, 17)
- Flows: the `diff-page.e2e.ts` flow that today asserts "the code is 12px/18px", updated to 12.5px/19px. A new step asserts the diff line's computed font family equals the app's mono font, and the rendered added and deleted rows use the new tints. Look check: the Mock states `a-desktop-range` and `a-desktop-worktree`.
- Checks: `rg '!important' webui/src/diff` finds nothing in the theme block. No step uses the `.diffs-host` selector.
- New flows: none beyond the updated step.

Worktree diff (stories 23 to 27, 33 to 42)
- Flows: new flows in `diff-page.e2e.ts` on scenario `worktree-diff`. For a chat, a Design and a claimed ticket, open the page and check the chip text, that committed and uncommitted files are both in the tree, that the untracked file shows with the added dot and the ignored file does not, that the commit list ends with "Uncommitted changes", and that narrowing to it shows only the uncommitted changes. Also check that a stacked ticket hides its blocker's commits, that a paused ticket with no run entry shows the derived base in its chip (the blocker's address when stacked, the feature branch name otherwise) and its sha, that a Queue agent chat with no tracked run shows the empty state naming it, the empty state for the unclaimed ticket (naming it, with no ref inputs), the "No changes since" notice, the untracked-cap notice (both the count case and the oversize-file case, each listing N), and that a reload keeps the same worktree. Story 42: the renamed file's path line reads "old path → new path", the deleted file's tree row has the waiting-tone dot, and the untracked image shows an image body. Story 24: run Show diff from the Queue agent chat page and from the Fix agent chat page, and assert each chip and base (the iteration's own base for the Queue agent, the trunk merge-base for the Fix agent). Look check: the Mock states `a-desktop-worktree` and `a-phone-files`.
- Checks: after the page loads, `git -C <worktree> status --porcelain` equals its output before, the index file's mtime and sha are unchanged, and no `index.lock` exists. `curl` of the worktree route for a chat id that does not exist answers with the empty-state reason, not a 5xx.
- New flows: all of the above. They become implementation tickets, with the scenario.

Show diff action (stories 23 to 32)
- Flows: the registry rows step of the `jump.e2e.ts` flow (Cmd/Ctrl+P rows), extended. On a chat, a Design, a claimed ticket, a paused ticket, a done ticket and an epic, "Show diff" is in the palette's "This page" section and in the page's action menu, and running it opens the matching Diff page URL. The epic page and the done ticket keep their "View diff" links, which open the same URL.
- Checks: the action is present on a claimed ticket and on a paused ticket with an iteration worktree. It is absent on a chat whose directory is a separate clone or not a git repo, on a ticket with no iteration worktree and no commits, on an epic with no range and on a Builder session. `registry.test.ts` passes.
- New flows: the menu and palette steps above, in the existing flows.

Mock states (stored mock `04-prototype-calmer-diff-page.html`, the look check for the stories above)
- `$(gx tickets root)/better-diff/mocks/04-prototype-calmer-diff-page.html`, state `a-desktop-range`, 1280 px, scenario `epic-diff` (shape `merged`), parts: header, range, counts, view-toggle, left, commits-button, file-tree, diff-pane.
- `$(gx tickets root)/better-diff/mocks/04-prototype-calmer-diff-page.html`, state `a-desktop-worktree`, 1280 px, scenario `worktree-diff` (a claimed ticket, new), parts: header, range, counts, view-toggle, left, commits-button, commits-list (open, ends with "Uncommitted changes"), file-tree, diff-pane.
- `$(gx tickets root)/better-diff/mocks/04-prototype-calmer-diff-page.html`, state `a-phone-files`, 390 px, scenario `worktree-diff` (a claimed ticket, new), parts: header, range, counts, view-toggle, left, commits-button, file-tree.
- `$(gx tickets root)/better-diff/mocks/04-prototype-calmer-diff-page.html`, state `a-phone-diff`, 390 px, scenario `epic-diff` (shape `merged`), parts: header, range, counts, view-toggle, diff-pane.

## Out of Scope

- A global Show diff command or picker. The action is per entity only.
- Polling or live updates of a Worktree diff. The refetch on window focus is the only refresh.
- Human code review, comments and focus highlights on diff lines.
- Light theme for the diff.
- Any server change beyond the new worktree route and the chat and ticket response fields needed for Show diff availability.
- Diffs for Builder, Triage and other managed sessions.

## Further Notes

- The throwaway prototype is on the branch `prototype/better-diff-calmer-diff-page` (commit e8be55a43). It never folds into main. The stored mock is the look reference, and its probed color and font values are in the Rendering decisions.
- The research behind the renderer and the snapshot is in the tracker tickets `better-diff/02` and `better-diff/03`, which name `docs/research/better-diff-pierre-styling.md` and `docs/research/better-diff-worktree-git.md` on local branches.

# PRD: Status page stash command

## Problem Statement

When working on the status page, I often want to set aside some of my working-tree changes
without committing them — either everything, or just what I've staged. Today gx has no way
to stash from the status page, so I drop to a terminal, type the `git stash` invocation by
hand, and remember the flags. That breaks my flow and the resulting stashes are unnamed and
hard to find later.

## Solution

Add a stash command to the status page under a new `S` (shift+s) chord prefix:

- `Sa` — stash **all** changes (staged + unstaged)
- `Ss` — stash **staged** changes only

Pressing either opens an input modal asking for an optional stash name, then performs the
stash and refreshes the file list. If there is nothing to stash, gx tells me so instead of
opening the modal. The diff-pane toggles `s` (render mode) and `w` (wrap) are left untouched.

## User Stories

1. As a gx user on the status page, I want to press `Sa` to stash all my changes, so that I
   can return to a clean working tree without leaving the app.
2. As a gx user, I want to press `Ss` to stash only my staged changes, so that I can set
   aside a curated subset while keeping the rest of my work in progress.
3. As a gx user, I want a modal to ask me for a stash name, so that I can label the stash and
   find it again later.
4. As a gx user, I want the stash name to be optional, so that I can stash quickly with
   `Sa Enter` when I don't care about the label.
5. As a gx user, when I submit an empty name, I want git to auto-generate the usual
   `WIP on <branch>` message, so that the stash is still valid and descriptive enough.
6. As a gx user, I want to press Esc in the stash modal to cancel, so that I can back out
   without stashing anything.
7. As a gx user, I want to be told "nothing to stash" when my working tree is clean and I
   press `Sa`, so that I don't waste time typing a name for a no-op.
8. As a gx user, I want to be told "nothing staged to stash" when I press `Ss` with an empty
   index, so that I understand why nothing happened.
9. As a gx user, I want the file list to refresh after a stash, so that the status page
   immediately reflects the now clean (or partially clean) tree.
10. As a gx user, I want a success notification after stashing, so that I get confirmation the
    operation worked.
11. As a gx user, I want stash to work regardless of whether the filetree or the diff pane is
    focused, so that I don't have to think about focus for a working-tree operation.
12. As a gx user, I want the `s` (render mode) and `w` (wrap) toggles to keep working exactly
    as before, so that adding stash doesn't disrupt my muscle memory.
13. As a gx user, I want the `S` prefix to sit alongside the other capital-letter git ops
    (`P` push, `A` amend, `B` bump, `R` refresh), so that the keymap stays consistent.
14. As a gx user, if a stash fails (e.g. old git without `--staged`), I want the error shown
    in the normal error modal, so that I can see what went wrong.
15. As a gx user, I want the stash name modal to look and behave like the existing credential
    modal, so that the interaction feels familiar.

## Implementation Decisions

- **Variants:** Two only — `Sa` (all) and `Ss` (staged). "Stash unstaged" was dropped: git
  has no clean single-command implementation, and the `--keep-index` behavior would be
  misleading under that name.
- **Prefix:** `S` (shift+s), chosen over rebinding the existing single-key `s`/`w` diff
  toggles. `S` is a sibling of the existing capital-letter working-tree ops and leaves the
  diff-pane keymap untouched. (`s` render mode and `w` wrap live in two different key
  managers — status and diffview — so rebinding them would touch three registrations across
  two modules for no benefit.)
- **Scope:** Global within the status page — dispatched regardless of filetree/diff focus,
  like the other git ops.
- **Deep module — git layer:** A named/staged-aware stash function with the shape
  `StashPush(dir, name string, stagedOnly bool) (string, error)`. It builds the git args
  (`stash push`, optional `--staged`, optional `-m <name>`) and reuses the existing
  `index.lock` busy-retry loop that the current `Stash()` already implements (extracted into
  a shared helper rather than duplicated). This is the one piece with real logic and is unit
  testable in isolation against a temp repo.
- **Modal:** Mirrors the existing credential-prompt pattern in the status model — a
  `textinput`-backed modal with open flag, the entered name, and a flag recording which
  variant (all vs staged). Rendered via the existing `RenderInputModal` component. Title and
  prompt vary by variant ("Stash all changes" / "Stash staged changes").
- **Empty-state pre-check:** Before opening the modal, the dispatch handler checks the
  in-memory status file list — no extra git call. `Sa` with no changes and `Ss` with nothing
  staged short-circuit to a `notify.Info` and do not open the modal. The modal only ever
  opens when a stash can succeed.
- **Post-stash:** On success, emit `notify.Success` and refresh the status file list. On
  error, route through the existing git-error modal path. No auto-pop, stash list, or
  follow-up prompt — those are separate future features.
- **git version:** `git stash push --staged` requires git ≥ 2.35. Older git will error, and
  that error surfaces through the normal error modal. No version detection.

## Testing Decisions

Good tests here exercise observable behavior, not internals: given a repo in a known state,
running a stash produces the expected stash contents and leaves the working tree in the
expected state; given a UI key sequence, the model opens/closes the right modal and emits the
right commands/notifications.

- **`git.StashPush` (unit):** Highest-value tests. Against a temp repo, cover: stash all with
  a name, stash all without a name (auto-message), stash staged only with a name, stash
  staged only without a name, and that staged-only leaves unstaged changes intact. Prior art:
  existing `git/stash_test.go`, `git/stage_test.go`.
- **Status model / e2e:** `Sa` opens the modal; typing a name + Enter fires the stash and the
  tree refreshes; Esc cancels without stashing; `Ss` opens with the staged variant. Prior
  art: existing status model/e2e tests covering the credential modal and other git ops.
- **Empty-state pre-check:** `Sa` on a clean tree shows the "nothing to stash" notify and does
  not open the modal; `Ss` with nothing staged shows "nothing staged to stash" and does not
  open the modal.

## Out of Scope

- Stash unstaged-only (`su`) — dropped, no clean implementation.
- Listing, viewing, applying, popping, or dropping stashes.
- Stashing untracked or ignored files (`--include-untracked` / `--all`).
- Partial/interactive (`--patch`) stashing.
- Rebinding or otherwise changing the `s` (render mode) and `w` (wrap) toggles.
- git version detection or fallback for `--staged` on git < 2.35.

## Further Notes

The implementation rides almost entirely on existing patterns: `git/stash.go` already has the
`index.lock` retry loop to reuse, and the status model's credential modal
(`credentialInput`/`credentialModalView`/`handleCredentialKey`) is a near-exact template for
the stash-name modal. Work is split into two vertical slices so `Sa` is shippable and
reviewable before `Ss` is added.

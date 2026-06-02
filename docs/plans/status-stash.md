# Status page: stash command

Add a stash command to the status page under a new `S` (shift+s) chord prefix, with an
in-status name-input modal. Two variants only — "stash unstaged" was dropped (no clean
native git implementation).

## Decisions

- **Variants:** `Sa` = stash all (staged + unstaged), `Ss` = stash staged only. No `su`.
- **Prefix:** `S` (shift+s), a sibling of the existing capital-letter git ops (`P` push,
  `A` amend, `B` bump, `R` refresh, `L` lazygit). `s` (render mode) and `w` (wrap) are
  left untouched.
- **Scope:** global — dispatched regardless of filetree/diff focus (like `P`/`p`/`B`/`A`).
- **Name:** optional. Enter-with-text → `git stash push [-m <name>]`; Enter-empty → bare
  `git stash push` (git auto-names); Esc → cancel.
- **Empty pre-check:** before opening the modal, use the in-memory file list to verify
  there is something to stash. `Sa` on clean tree → `notify.Info("nothing to stash")`;
  `Ss` with nothing staged → `notify.Info("nothing staged to stash")`. Modal only opens
  when a stash can succeed.
- **Post-stash:** `notify.Success(...)` + `m.refresh()`. No auto-pop / stash-list (future).
- **git version:** `git stash push --staged` needs git ≥ 2.35; older git errors surface
  via the existing error modal.

## Tasks

### git layer (`git/stash.go`)
- [x] Add a named/staged-aware stash function, e.g.
      `StashPush(dir, name string, stagedOnly bool) (string, error)`, building args
      (`stash push` [`--staged`] [`-m <name>`]).
- [x] Reuse the existing `index.lock` retry loop from `Stash()` (extracted
      `runGitWithIndexLockRetry` helper, shared by `Stash` and `StashPush`).
- [x] Add/extend tests in `git/stash_test.go` covering: all vs staged, with/without name.

### status model state (`ui/status/model_state.go`)
- [x] Add fields mirroring the credential modal: `stashOpen bool`,
      `stashInput textinput.Model`, `stashStagedOnly bool`.
- [x] Initialize `stashInput` (textinput) lazily on open via `newStashInput()` (mirrors
      `newCredentialInput`; the credential input is likewise not constructed up front).

### keybindings (`ui/status/model_keys.go`)
- [x] Add binding ID `bindingStashAll` (w26.1). `bindingStashStaged` deferred to w26.2.
- [x] Register `S`-prefix chords: `{Seq: ["S","a"]}` + `{Seq: ["S","esc"]}` cancel-chord,
      category "Git". (`{Seq: ["S","s"]}` deferred to w26.2.)
- [x] Dispatch `Sa` in `dispatchBinding` → `m.openStash(false)`: runs the pre-check, then
      opens the modal or emits the "nothing to stash" notify.

### modal handling (`ui/status/model_update.go`)
- [x] In `handleKeyPress`, add an `if m.stashOpen { return m.handleStashKey(msg) }` guard
      alongside the existing `credentialOpen` guard.
- [x] Implement `handleStashKey`: Esc cancels (clear `stashOpen`); Enter submits → fire a
      stash command with the trimmed input value + `stashStagedOnly`; otherwise delegate
      the keystroke to `stashInput.Update`.
- [x] Add a `stashFinishedMsg` + handler: on success `notify.Success(...)` + `m.refresh()`;
      on error route through `showGitError`.

### modal view (`ui/status/stash.go` + view wiring)
- [x] Add `stashModalView()` mirroring `credentialModalView()` —
      `components.RenderInputModal(title, prompt, m.stashInput.View(), ui.HintSubmitCancel(), ...)`.
      Title/prompt vary by variant ("Stash all changes" / "Stash staged changes").
- [x] Wire `stashModalView()` into the status view's modal-overlay selection (alongside
      `credentialModalView`).

### tests
- [x] Model/e2e test: `Sa` opens modal, typing a name + Enter triggers stash, tree
      refreshes.
- [x] Test: `Sa` on a clean / untracked-only tree shows the notify and does not open the
      modal. (`Ss` staged pre-check → w26.2.)
- [x] Test: Esc cancels without stashing.

### docs
- [ ] Update `CHANGELOG.md` (via the changelog skill) with the stash command. (Deferred to
      w26.2 so both variants land in one entry.)

## Implementation notes (w26.1)

- **Empty pre-check refinement:** untracked files are excluded from "stashable" — the stash
  runs without `--include-untracked` (out of scope), so `git stash push` ignores them. A tree
  of only-untracked files therefore reads as "nothing to stash" rather than opening a modal
  that would no-op. Captured in `Model.hasStashableChanges`.

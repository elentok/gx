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

## Extract stash into its own `ui/stash` model (decided via /grill)

Refactor: the `Sa` interaction shipped embedded in the status model (`ui/status/stash.go` +
`stashOpen`/`stashInput`/`stashStagedOnly` fields). Move it into a standalone sub-model under
`ui/stash`, following the `push`/`pull`/`bump` convention, so it stops polluting the status
model. Done before w26.2 so `Ss` is built once on the final shape.

### Decisions

- **Convention:** `ui/stash` exposes `New() Model`, `Open(root string, stagedOnly bool) tea.Cmd`,
  `Update(msg) (Model, tea.Cmd, Result)`, `View(width int) string`, exported `IsOpen`, and
  `InputFocused()`. First sub-model to own a `textinput`.
- **Status integration (mirror `pull`):** `stash stash.Model` field constructed with
  `stash.New()`; top-of-`Update` guard `if m.stash.IsOpen { return m.handleStashUpdate(msg) }`;
  overlay branch in `view.go`; new glue file `ui/status/model_stash.go` consuming `Result`.
  `InputFocused()` in `diff_search.go` also delegates to `m.stash.InputFocused()`.
- **Pre-check stays in status:** `hasStashableChanges` + the `"nothing to stash"` /
  `"nothing staged to stash"` notify remain in the status dispatch. Status calls
  `m.stash.Open(...)` only when stashable; the sub-model never models an empty state.
- **Phases:** `phaseInput` → `phaseStashing` (spinner, like `bump`'s `phaseTagging`) → done,
  plus `phaseFailed` (red frame, esc/enter/q to dismiss). The model owns failure rendering
  like `pull`; status does NOT call `showGitError`.
- **Result:** `Result{Done bool, Outcome Outcome, StagedOnly bool, Err error}`, `Outcome` ∈
  `{OutcomeNone, OutcomeStashed, OutcomeCancelled}`. Status glue: `OutcomeStashed` →
  `notify.Success(...)` + `refresh()`; `OutcomeCancelled` → nothing; `Err` → `notify.Error(...)`
  + record output to `m.output`.
- **Title/prompt** vary by `StagedOnly` ("Stash all changes" / "Stash staged changes").
- **git layer unchanged:** `git.StashPush` stays in `git/`; the sub-model calls it (via a
  `components.CommandRunner`, matching how `pull` runs git, or a plain cmd — implementer's
  call, but reuse `git.StashPush` so the retry loop is preserved).
- **No ADR:** follows the established sub-model convention (cf. ADR 0002), cheaply reversible,
  unsurprising.

### Tasks (extraction)

- [x] Create `ui/stash/stash.go`: `Model`, `New`, `Open(root, stagedOnly) tea.Cmd`,
      `Update` (phases input/stashing/failed), `View(width)`, `InputFocused`, `Result` + `Outcome`.
- [x] Reuse `git.StashPush(root, name, stagedOnly)` inside the stashing cmd.
- [x] Remove `ui/status/stash.go` and the `stashOpen`/`stashInput`/`stashStagedOnly` fields
      from `model_state.go`; remove the old `handleStashKey`/`stashFinishedMsg`/`stashModalView`.
- [x] Wire status: `stash stash.Model` field + `stash.New()`; top guard in `Update`;
      overlay in `view.go`; new `model_stash.go` glue consuming `Result`;
      `diff_search.go` `InputFocused` delegation.
- [x] Keybinding dispatch `bindingStashAll` → `m.stash.Open(m.worktreeRoot, false)`
      (keep the `Sa` + `S,esc` chords and the `hasStashableChanges` pre-check in status).
- [x] Move deep model tests to `ui/stash/stash_test.go` (mirror `ui/pull/pull_test.go`):
      open → type → Enter drives input→stashing→Done `OutcomeStashed`; Esc → `OutcomeCancelled`;
      error → `phaseFailed`.
- [x] Slim `ui/status/stash_model_test.go` to the seam: pre-check notices (clean /
      untracked-only) + `Result{OutcomeStashed}` → notify+refresh. Drop the in-modal mechanics tests.
- [x] `go build ./...`, `go vet`, `go test ./git/ ./ui/stash/ ./ui/status/`.

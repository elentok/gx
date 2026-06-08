# Image diffs in the commit detail panel (log + stash tabs)

Generalize the status-only inline image-diff overlay (ADR 0010) to the commit detail panel shared by
the log and stash split views.

## Decisions (from grilling session)

1. **Extract a generic overlay controller** — move `ui/status/imagediff` up to `ui/imagediff` and add
   a reusable `Overlay` controller holding the full ADR-0010 lifecycle, parameterized by host
   callbacks. `status.Model` and `commit.Model` each embed one. No duplication.
2. **Inject screen origin** — `splitview` exposes `DetailOrigin() (col,row)`; the container pushes it
   into `commit.Model` via `WithScreenOrigin`. The panel computes its diff-pane rect relative to
   (0,0) and adds the origin. Status keeps origin (0,0).
3. **Setters return the disrupt cmd** — `WithRef`/`WithScreenOrigin` return `(commit.Model, tea.Cmd)`;
   the container batches the returned eager-clear+settle cmd. Internal commit events (scroll,
   file-select, modal open/close, resize) use the same deferred-`Update` wrapper status uses.
4. **Shared blob endpoint resolver** — factor the old/new treeish+path decision out of
   `singleFileDiffArgs` so a new `git.CommitImageDiffBlobs` and the text diff agree (incl. stash
   `^3`-untracked + rename old-path). Full parity in v1 (near-free with the shared resolver).
5. **Forward settle msg explicitly** — overlay tick is an exported `imagediff.SettleMsg`; log and
   stashlist add one switch case forwarding it to `commitDetail.Update`.
6. **Amend ADR 0010** (done) and generalize CONTEXT.md `Image diff` + add `Screen origin` (done).

## Tasks

### 1. Extract reusable overlay package

- [x] Move `ui/status/imagediff/*` to `ui/imagediff/` (pure layout module, update import paths).
- [x] Add `ui/imagediff/overlay.go`: `Overlay` controller holding `activeIDs`, `nextID`, `settleSeq`,
      `fallbackPath`, cached `capability`, `dirty`. Define a host interface / callback struct:
      fetch blobs, absolute body geometry, `ModalOpen() bool`, write bytes, detect capability.
      (`SettleHost` interface + `NewOverlay(writeBytes, detectCapability)`; `HasImageExtension` shared.)
- [x] Port `disrupt`, `handleSettle`, `cmdPlaceImageDiff`, `cmdClearImagePlacements`,
      `ansiCursorPosition`, settle-debounce const, and `SettleMsg` (exported) into the controller.
- [x] Unit tests for the controller (port `image_diff_test.go` coverage; host callbacks faked).

### 2. Reparent status onto the controller

- [x] Replace status' `imageDiffState` + `image_diff.go` machinery with an embedded `imagediff.Overlay`
      + thin adapter callbacks (`selectedStatusFile`, `diffarea.ActiveSection`, `imageDiffPanelGeometry`,
      `git.ImageDiffBlobs`, `writeImageDiffBytes`). (status `Model` implements `imagediff.SettleHost`.)
- [x] Keep status' deferred-`Update` dirty/modal wrapper; delegate to `overlay`.
- [x] Confirm status e2e/image tests still pass unchanged. (Lifecycle tests moved to `ui/imagediff`;
      status keeps integration tests through `renderDiffPane` + public accessors.)

### 3. Git ref-based blobs

- [x] Factor old/new endpoint resolution out of `singleFileDiffArgs` into a shared helper
      (`commitDiffEndpoints` + `stashTargetRef`) covering: regular `ref^1`/`ref`, stash `^1`/`^3`,
      rename path. `singleFileDiffArgs` now shares `stashTargetRef`.
- [x] Add `git.CommitImageDiffBlobs(repoRoot, ref string, file CommitFile) (old,new []byte, oldOK,newOK bool)`
      using the shared resolver + `gitObjectBytes`.
- [x] Tests: regular modify/add/delete, stash tracked, stash untracked (`^3`), rename.

### 4. splitview screen origin

- [x] Add `splitview.Model.DetailOrigin() (col,row int, visible bool)` from orientation + list dims
      (vertical: col=listW,row=0; horizontal: col=0,row=listH; fullscreen-detail: 0,0;
      collapsed / fullscreen-list: not visible).
- [x] Unit tests across orientation × visibility states.

### 5. commit.Model overlay integration

- [x] Embed `imagediff.Overlay`; add `screenCol/screenRow/screenVisible` fields.
- [x] `WithScreenOrigin(col,row,visible)` and `WithRef(ref)` return `(Model, tea.Cmd)` (disrupt cmd).
- [x] `diffPaneBodyRect()` relative to (0,0); `PanelGeometry()` = rect + screen origin (SettleHost).
- [x] Reserve blank lines in the diff pane when the selected file is image-eligible (`binaryDiffLines`);
      else keep the `binary file` summary line.
- [x] Deferred-`Update` wrapper via an `overlaySignature` snapshot (scroll/selection/focus/expand/
      resize/modal); route `imagediff.SettleMsg` to the overlay.
- [x] `OnDeactivate` entry point the container can call.
- [x] Adapter callbacks: selected file, `git.CommitImageDiffBlobs(worktreeRoot, ref, file)`,
      `imagediff.WriteToStdout`, `imagediff.DefaultDetectCapability`.

### 6. Wire log + stash containers

- [x] After every Update, a deferred `withSyncedDetailOrigin()` reads `split.DetailOrigin()` and calls
      `commitDetail.WithScreenOrigin(...)`, batching the returned disrupt cmd (no-ops when unchanged).
- [x] On selection change, batch the cmd returned by `commitDetail.WithRef(...)` (log
      `handleSelectionChange` + `openSelected`; stash `navigateList` + `stashLoadedMsg` +
      `SelectionChangedMsg`).
- [x] Add a switch case forwarding `imagediff.SettleMsg` to `commitDetail.Update` (log + stash).
- [x] Implement `OnPageDeactivated()` → `commitDetail.OnDeactivate()` (both log and stashlist).
- [x] Collapsed / fullscreen-list states report not-visible origin (`splitview.DetailOrigin`); a
      container modal also forces not-visible so the overlay clears.

### 7. Verify

- [x] `go build ./...` and full `go test ./...` (1448 pass; controller, git blobs, splitview origin,
      and commit end-to-end placement all covered).
- [ ] Manual: log tab split, j/k through commits with an image change → side-by-side renders, no
      drift on scroll/resize/orientation/fullscreen/tab-switch. *(needs a real kitty terminal)*
- [ ] Manual: stash tab, tracked + untracked image; added/deleted image (centered single).
      *(needs a real kitty terminal)*
- [ ] Manual: non-kitty terminal / `image-diffs: false` → falls back to `binary file`.
      *(needs a real kitty terminal)*

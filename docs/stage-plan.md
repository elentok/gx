# `gx stage` Implementation Plan

## Goal

Add a new `gx stage` command that opens an interactive two-pane staging UI inspired by Lazygit, with syntax-highlighted diffs via `git diff` + `delta`, and supports hunk/line stage and unstage flows.

## Confirmed product decisions

- Active selection indicator uses a **left gutter marker** (no background override) to preserve syntax highlighting readability.
- In diff **line mode**, `<space>` applies to the selected changed line only.
- In diff **hunk mode**, `<space>` applies to the selected hunk.
- `enter` moves focus from status pane to diff pane.
- `esc` and `q` move focus from diff pane back to status pane.
- `q` while focused on status exits the stage UI.
- Untracked files are listed and can be staged from the UI.

## Scope

- Add a new CLI command entrypoint: `gx stage`.
- Build a dedicated Bubble Tea model for staging UI (not mixed into worktrees UI).
- Add git plumbing for status/diff parsing and patch apply operations.
- Add tests for command wiring, layout behavior, navigation/focus, and stage/unstage actions.

## Architecture overview

### 1) Command integration

- Update `cmd/cmd.go`:
  - Add `stage` command in `execute(...)` switch.
  - Include usage help line for `gx stage`.
  - Implement `runStage(d deps) error` similar to existing TUI commands (resolve repo from cwd, then launch Bubble Tea program).

### 2) New stage domain package(s)

- Create a new UI package for this feature (proposed: `ui/stage`).
- Create supporting git helpers (proposed additions under `git/`) to avoid embedding command logic in UI code.

### 3) Data model

- Track status entries from porcelain output, including both staged and unstaged states per file.
- Track selected file index in status pane.
- Track focus: `status` vs `diff`.
- Track diff section focus: `unstaged` vs `staged`.
- Track navigation mode: `hunk` vs `line` (toggle with `a`).
- Track active target within each diff section:
  - active hunk index
  - active changed line index (line mode)
- Track independent scroll offsets for staged/unstaged sections.

### 4) Layout behavior

- Use terminal width threshold of `100` columns:
  - `>100`: horizontal split (status left, diff right).
  - `<=100`: vertical split (status top, diff bottom).
- Status pane consumes 30% of split dimension.
- Diff pane always contains two sections:
  - unstaged (top)
  - staged (bottom)
- Collapse an empty section when the selected file has no lines in that group.

### 5) Diff acquisition and rendering

- For selected file, collect:
  - unstaged patch (`git diff -- <path>`)
  - staged patch (`git diff --cached -- <path>`)
- Render through `delta` for syntax highlighting:
  - use one-shot git overrides so side-by-side is disabled for this UI path.
  - disable paging interaction issues by capturing command output programmatically.
- Preserve delta ANSI styles and prepend a narrow gutter column for active item markers.

### 6) Active item highlighting strategy

- Do not mutate syntax-highlighted line colors.
- Add left gutter indicator:
  - active hunk/line row: strong marker (for example `▌`)
  - non-active row: blank or dim space
- Apply indicator consistently in both staged and unstaged sections.

### 7) Parsing for navigation and patch actions

- Parse raw unified diff (non-colorized) to build structural metadata:
  - hunks (`@@ ... @@` boundaries)
  - changed lines (`+`/`-` only; ignore context lines and file headers)
- Keep mapping from visible changed-line targets to source line numbers needed for patch operations.
- Rebuild metadata whenever selected file or index changes.

### 8) Stage/unstage operations

- Hunk mode:
  - unstaged section: stage active hunk (`git add -p` equivalent via `git apply --cached` with generated hunk patch).
  - staged section: unstage active hunk (apply reverse to index or use reset patch flow).
- Line mode:
  - unstaged section: stage selected changed line via synthesized minimal patch.
  - staged section: unstage selected changed line via reverse/symmetric patching.
- After each `<space>` action:
  - refresh file status list
  - refresh both diff sections for selected file
  - preserve nearest valid cursor target where possible

## Implementation phases

- [x] Phase 1: Command wiring and basic model skeleton
  - Add `gx stage` command routing and usage text.
  - Add initial `ui/stage` model with startup, size handling, focus state, and quit behavior.

- [x] Phase 2: Status pane
  - Parse porcelain status for tracked/untracked files.
  - Implement file list rendering and navigation (`j/k` + arrows).
  - Keep selected file synced with diff fetch.

- [x] Phase 3: Diff fetching and structural parsing
  - Add git helpers for staged/unstaged file diffs.
  - Build hunk/line metadata for navigation and patch actions.
  - Add diff section collapsing logic.

- [x] Phase 4: Diff rendering and interaction
  - Pipe diff rendering through delta while preserving ANSI output.
  - Implement focus transfer (`enter`, `esc`, `q`) and section switch (`tab`).
  - Implement mode toggle (`a`) and active gutter indicators.
  - Implement per-section scrolling.

- [x] Phase 5: Stage/unstage actions
  - Implement `<space>` actions for hunk mode and line mode.
  - Handle untracked file staging flow.
  - Refresh data model after each action and maintain stable selection.

- [x] Phase 6: Tests and verification
  - Add/extend command tests in `cmd` for `gx stage` dispatch/help.
  - Add focused unit tests for diff parser + navigation target rules.
  - Add model tests for layout threshold behavior (`100` boundary), focus rules, and section collapsing.
  - Add tests for stage/unstage patch generation and git helper behavior (with temp repos).
  - Run targeted tests, then full suite.

## Verification checklist

- `gx stage` launches from a repo and exits cleanly.
- Status pane shows expected files and supports `j/k` and arrows.
- Layout switches exactly at width `100`.
- Diff pane shows staged/unstaged sections and collapses empty section.
- Delta-based syntax highlighting is visible in diff rendering.
- `tab` changes active section in diff focus.
- `a` toggles hunk/line mode.
- Navigation in diff mode only lands on actual changed lines/hunks.
- `<space>` stages/unstages correctly for both hunk and line modes.
- `esc`/`q` return to status from diff; `q` exits from status.

## Risks and mitigations

- Delta output parsing complexity:
  - Mitigation: use raw unified diff for structural parsing and delta only for display.
- Single-line patch correctness across file states:
  - Mitigation: build patch synthesis with tests against add/remove/adjacent changes.
- ANSI width and gutter alignment issues:
  - Mitigation: rely on ANSI-aware width helpers and snapshot tests for rendering.

## Review

- What changed:
  - Added `gx stage` command wiring in `cmd/cmd.go` with a dedicated stage runner and usage entry.
  - Added stage git plumbing in `git/stage.go`:
    - porcelain status parsing (`ListStageFiles`)
    - staged/unstaged and untracked diff retrieval (plain + delta-rendered)
    - index patch application (`ApplyPatchToIndex`) and full-file stage helper (`StagePath`)
    - worktree root resolution (`WorktreeRoot`)
  - Added a new Bubble Tea model in `ui/stage/model.go` implementing:
    - two-pane layout with 100-column responsive split and 30% status pane
    - status/diff focus model, section switching, hunk/line mode toggling
    - active-row left gutter marker without overriding syntax colors
    - per-section independent scroll offsets
    - hunk/line stage and unstage via generated patches
    - untracked file staging from the unstaged section
  - Added unified-diff parsing and patch synthesis in `ui/stage/diffparse.go`.

- Test commands and results:
  - `go test ./cmd ./git ./ui/stage -count=1` ✅
  - `go test ./... -count=1` ✅

- Follow-up improvements:
  - Add richer integration tests using the local Bubble Tea harness to exercise end-to-end key flows (`enter`, `tab`, `a`, `<space>`, `esc`) in one scenario.
  - Improve line-level patch synthesis for edge cases with very complex multi-edit hunks and binary files.

## Part 2 plan

- [x] Add active-pane border highlighting in orange and apply catppuccin-themed accents in stage UI.
- [x] Add status-pane `<space>` toggle for whole-file stage/unstage.
- [x] Switch status collection to list untracked files individually, and build a collapsible directory tree for nested paths.
- [x] Add directory-row actions: collapse/expand with left/right (and h/l), `<space>` stages/unstages all descendants.
- [x] In diff view, mark the full active hunk with side-column indicators while in hunk mode.
- [x] Add `J/K` diff scrolling independent from active hunk/line cursor.
- [x] After stage/unstage in diff view, move focus to the destination section, keep moved target visible, and animate marker briefly.
- [x] Add targeted tests for status-space toggles, directory tree behavior, and diff scroll behavior; then run full suite.

## Part 2 review

- What changed:
  - Updated stage styling with catppuccin-inspired accents and active pane orange border highlight.
  - Status pane now renders a collapsible directory tree, supports `h/l` and left/right collapse-expand, and supports `<space>` stage/unstage on file or directory rows.
  - Changed status collection to `git status --porcelain=v1 --untracked-files=all -z` so nested untracked files are directly actionable.
  - Diff pane now supports hunk-wide side-column markers in hunk mode and `J/K` viewport scrolling without moving active target.
  - After diff `<space>`, focus moves to destination section, target is auto-focused/kept visible, and a short marker animation is shown.
  - Added git helper `UnstagePath` for full-path index unstage operations.

- Test commands and results:
  - `go test ./git ./ui/stage ./cmd -count=1` ✅
  - `go test ./... -count=1` ✅

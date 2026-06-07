# PRD — Inline image diffs via the kitty graphics protocol

## Problem Statement

When a changed file is an image (PNG, JPG, GIF, etc.), gx's status diff panel shows nothing useful
— just a generic `"binary file (prev size: X, new size: Y)"` line (`binarySummaryLine`). To see what
actually changed, the user has to leave gx and open the files in an external viewer, losing the
in-place review flow that the diff panel exists to support.

## Solution

When the user is on a kitty-capable terminal and the selected changed file is an image, the diff
panel renders a **side-by-side comparison** of the old and new versions directly inline, using the
kitty graphics protocol. Everywhere else — unsupported terminals, decode failures, oversized files,
tmux-over-non-kitty, or a user opt-out — the existing binary summary line is shown, unchanged.

## User Stories

1. As a developer reviewing unstaged changes to a logo file, I want to see the old and new versions
   side by side in the diff panel, so that I can tell at a glance what visually changed without
   leaving gx.
2. As a developer reviewing staged changes to an icon, I want the comparison to reflect exactly what
   will be committed (HEAD → index), so that the image diff matches the text-diff semantics I
   already trust for the staged section.
3. As a developer who just added a new screenshot asset, I want to see it rendered large and
   centered (not awkwardly split against an empty "old" pane), so that an add looks like an add.
4. As a developer who deleted an image asset, I want to see the deleted version rendered large and
   centered, so that a delete is visually unambiguous from a modification.
5. As a developer working in a terminal that doesn't support the kitty graphics protocol, I want to
   see the same binary-summary line gx already shows today, so that nothing breaks or renders
   garbled escape codes.
6. As a developer working inside tmux on a kitty-hosted session, I want image diffs to still render
   (via tmux passthrough), so that my usual multiplexed workflow isn't a second-class experience.
7. As a developer working inside tmux on a non-kitty host terminal, I want a graceful fallback to
   the summary line, so that gx doesn't attempt something it can't reliably do.
8. As a developer with an oversized or corrupt image in the diff, I want gx to fall back to the
   summary line rather than hang, crash, or show a half-broken render.
9. As a developer who finds the image-rendering feature misbehaving in my specific terminal setup, I
   want a config toggle to turn it off entirely, so that I have an escape hatch without waiting for
   a code fix.
10. As a developer rapidly moving through the file list with `j`/`k`, I want gx to clear stale image
    placements immediately and only redraw once I settle on a file, so that I never see an overlay
    drift out of sync with the panel beneath it.
11. As a developer resizing my terminal or toggling fullscreen on the diff panel, I want any visible
    image comparison to be cleared and correctly redrawn at the new dimensions, so that the overlay
    never overlaps the wrong screen region.
12. As a developer comparing a resized image (e.g. a logo that changed from 120×80 to 200×80), I
    want each side rendered at its own true aspect ratio, so that the size difference is visually
    accurate rather than stretched to fit a shared box.

## Implementation Decisions

This PRD implements the design captured in **ADR 0010** (`docs/adr/0010-kitty-image-diff-overlay.md`)
and the **Image diff** glossary term in `CONTEXT.md`. Key points repeated here for context:

- Bubbletea's `View()` cannot host persistent graphics, so images are placed as a side effect of a
  `tea.Cmd`, not embedded in the rendered string. `View()` always reserves blank cells for the image
  area so layout math accounts for the space.
- Lifecycle: any disrupting event (file selection change, scroll, resize, focus change, fullscreen
  toggle, tab switch away) **immediately and unconditionally clears** existing placements.
  Re-placement happens only after a short debounce (~80ms) once the model settles, and only if the
  currently selected file is still an image diff.
- Detection (kitty graphics support, host pixel-per-cell size, tmux passthrough capability) is
  queried once and cached, mirroring `ui.DetectTerminal`'s `$KITTY_*`/`$TMUX` env-based caching.
- There is exactly **one** fallback path for every failure mode (unsupported terminal, tmux-over-
  non-kitty, decode error, oversized image, user opt-out): render `binarySummaryLine()`, unchanged
  from today.

### Modules

**1. `ui/kittygraphics`** (new package) — deep module wrapping the kitty graphics protocol.
   - Capability detection: whether the terminal supports the kitty graphics protocol, the host's
     pixel-per-cell size (for aspect-correct scaling), and whether tmux passthrough is viable
     (host-is-kitty-under-tmux). All I/O (env vars, ioctl/winsize queries, escape-sequence probes)
     is injected so detection is unit-testable without a real terminal.
   - Protocol encoding: turn raw image bytes + a target cell-span into APC placement escape
     sequences, and produce the corresponding clear/delete sequences. Wraps in the tmux DCS
     passthrough envelope (`\033Ptmux;...\033\\`) when required.
   - Public surface stays small and stable: something like `DetectSupport(...) Capability`,
     `EncodePlacement(id, imageBytes, spanCols, spanRows) []byte`, `EncodeClear(id) []byte`.

**2. `git` package additions** — blob-fetch helpers alongside the existing `BinaryFileSizes`
   (`git/stage.go`), following the same old/new resolution rules:
   - Staged section: old = `HEAD:path` (or rename source), new = index blob (`:path`).
   - Unstaged section: old = index blob (`:path`), new = worktree file on disk.
   - Returns raw bytes (or a not-available signal) for each side; thin wrapper around `cat-file -p`
     / file reads, same shape and error handling as `BinaryFileSizes` / `gitObjectSize`.

**3. `ui/status/imagediff`** (new package) — deep, pure module that turns "old bytes, new bytes,
   available cell space, pixel-per-cell ratio" into a render plan:
   - Decodes both images (stdlib `image` package + format registrations for png/jpeg/gif/webp/bmp
     as needed).
   - Decides layout: side-by-side equal halves (each scaled to fit its half preserving its own
     aspect ratio) when both sides exist; single centered image when only one side exists (added or
     deleted file).
   - Computes each image's target column/row span from its pixel dimensions and the host's
     pixel-per-cell ratio, choosing the closest integer cell-span that preserves aspect ratio.
   - Returns a fallback signal (not an error the caller has to interpret) whenever decode fails,
     either side is empty/corrupt, or the combined size exceeds a size cap — the caller's job
     becomes a single `if plan.Fallback { show binarySummaryLine() }` branch.
   - No terminal I/O, no kitty-protocol knowledge — purely data in, layout decisions out. This is
     the natural seam for unit tests covering scaling math and layout choice.

**4. Glue inside `ui/status`** — wires everything together:
   - Detects whether the selected file is an image (extension allowlist: `.png`, `.jpg`, `.jpeg`,
     `.gif`, `.webp`, `.bmp`) and whether the `image-diffs` config toggle and capability detection
     both allow rendering.
   - Reserves blank cells in the diff panel's rendered output where the comparison goes.
   - Owns the lifecycle state machine: tracks "dirty" (needs clear), "settling" (debounce timer
     running), "placed" (active placement IDs + their screen coordinates). Emits clear commands
     eagerly on every disrupting `Update`, and a debounced place command on settle.
   - Computes the panel's actual on-screen position (row/col offset) at place-time, since that's
     when the layout is guaranteed stable.
   - Replaces the `len(diff.ViewLines) == 0 && diffcore.HasBinaryDiff(...)` branch in
     `ui/status/diff_view.go` with: try image diff (if eligible) → reserve cells; else →
     `binarySummaryLine()` (unchanged).

### Configuration

Add `image-diffs` (bool, default `true`) to `config.Config` (`config/config.go`), following the
existing `UseNerdFontIcons`-style boolean flag pattern (default value in `Default()`, override
parsing in `Load()`'s `raw` struct, schema entry in `docs/config-schema.json`).

## Testing Decisions

Good tests here exercise external behavior — given inputs, what decision/output comes out — not
internal plumbing. Per the codebase's existing style (e.g. `ui/diffview/diffcore/core_test.go`,
`git/stage_test.go`-style table tests, `ui/status/model_test.go` for Update/Cmd flows):

- **`ui/kittygraphics`** — table-driven tests over injected env/ioctl/probe-response inputs,
  asserting the detected `Capability` (graphics support, pixel-per-cell, tmux-passthrough-viable).
  Separate table-driven tests asserting the byte-exact escape sequences produced by
  `EncodePlacement`/`EncodeClear` for given inputs, including the tmux-wrapped variant. No real
  terminal needed — this is the highest-value seam in the feature because it's the part most prone
  to silent protocol-formatting bugs.
- **`ui/status/imagediff`** — table-driven tests over (old bytes, new bytes, available cells,
  pixel-per-cell) → expected `RenderPlan` (layout choice, per-side spans, fallback signal). Covers:
  both-sides-present scaling, added/deleted single-centered layout, decode-failure fallback,
  oversized-input fallback, and aspect-ratio edge cases (very wide/very tall images). Pure
  data-in/data-out — no terminal, no git, no bubbletea.
- **`ui/status` glue** — extend the existing `ui/status/model_test.go` / `e2e_test.go`-style
  Update/Cmd tests to cover: a disrupting event (selection change/resize) immediately produces a
  clear command; settling after the debounce window produces a place command only if the selection
  is still an image diff; the `image-diffs: false` config short-circuits straight to
  `binarySummaryLine()`; capability-detection-says-unsupported also short-circuits. These are
  necessarily more verbose Update/Cmd-style tests, but they're the only way to pin down the
  trickiest, most stateful part of the feature (the lifecycle state machine).
- **`git` blob-fetch additions** — no dedicated tests, matching the precedent set by
  `BinaryFileSizes` (untested thin wrapper around `run`/`cat-file`).

## Out of Scope

- Rendering images anywhere other than the status diff panel (e.g. commit detail view, log).
- Supporting graphics protocols other than kitty (Sixel, iTerm2 inline images, etc.).
- Animated image playback (GIFs render as a static frame).
- Zoom/pan/fullscreen interaction with the rendered images themselves.
- Configurable layout (e.g. user-chosen split ratio, vertical stacking instead of side-by-side).
- Detecting images by content-sniffing rather than file extension.

## Further Notes

- `ui/worktrees/kitty.go` already shells out to `kitten @ action goto_session` for terminal-session
  management — a *simpler* "launch an external kitty action" pattern that this feature deliberately
  does not follow (see ADR 0010's rejected-alternatives section for why).
- `ui.DetectTerminal`/`ui.Terminal` (`ui/terminal.go`) already distinguishes
  `TerminalKitty`/`TerminalKittyRemote`/`TerminalTmux`; the new capability detection in
  `ui/kittygraphics` should compose with (not duplicate) that existing detection.
- The tmux passthrough path requires the user's tmux to have `set -g allow-passthrough on`; this
  should be called out wherever the feature is documented for end users (changelog, README, etc.),
  since gx cannot set that for them.

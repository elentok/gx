# ADR 0010 — Inline image diffs render as kitty graphics overlays, not external launches

## Status
Accepted

## Context

Status's diff panel shows binary files as a one-line summary (`binarySummaryLine`, `ui/status/diff_view.go`):
`"binary file (prev size: X, new size: Y)"`. For images, a side-by-side render of the old and new
versions would be far more useful — but bubbletea (gx's TUI framework) only knows how to draw
styled text via `View() string`. It has no concept of a persistent graphic tied to a screen region.

The kitty graphics protocol renders images by writing raw APC escape sequences directly to the
terminal. Those placements are **independent of bubbletea's redraw cycle**: they persist on screen
at fixed cell coordinates until explicitly deleted, and must be explicitly recomputed and
re-emitted whenever the underlying layout changes (selection, scroll, resize, focus, fullscreen
toggle, tab switch).

Two approaches were considered:

- **A — external viewer launch.** On a keypress over an image diff, shell out to `kitten icat` (or
  similar), the same way `ui/worktrees/kitty.go` already shells out to `kitten @ action
  goto_session`. Simple and robust — no fighting the render loop — but it isn't an inline diff
  view; it's a "launch a tool" action, and breaks the "stay in gx" flow that the diff panel exists
  to support.
- **B — overlay placement as a side effect of View().** `View()` reserves blank cells where the
  images belong (so bubbletea's layout math accounts for the space); a `tea.Cmd` then computes the
  panel's actual screen position and emits/clears kitty placements directly, bypassing bubbletea's
  string-diffing renderer entirely.

Option B was chosen: it keeps the comparison inline, in the same panel, alongside the file list and
text diffs — consistent with how every other diff type (text, symlink, binary-summary) is shown in
place rather than launched externally.

## Decision

Image diffs render as **kitty graphics overlays positioned as a side effect**, with the following
rules to keep stale placements from drifting out of sync with the panel:

- `View()` always renders blank reserved cells for the image area; it never embeds graphics escape
  codes in the returned string (bubbletea's diff-based renderer would mangle them).
- Any event that can invalidate a placement's position or content — file selection change, scroll,
  resize, focus change, fullscreen toggle, tab switch away — **immediately emits a clear-placements
  command**. Clearing is eager and unconditional; it never waits to see whether a new placement will
  follow.
- Re-placement happens only once the model **settles**: a short debounce (~80ms) after the last
  disrupting event, and only if the currently selected file is still an image diff. This avoids
  placement thrash when the user holds `j`/`k` to move quickly through the file list.
- Detection of kitty-graphics support, the host terminal's pixel-per-cell size (for aspect-correct
  scaling), and tmux passthrough capability are all queried once and cached, mirroring how
  `ui.DetectTerminal` already caches `$KITTY_*`/`$TMUX` checks.
- Any failure in this pipeline — unsupported terminal, tmux-over-non-kitty host, decode error,
  oversized image, or a user opt-out via config (`image-diffs: false`) — falls back to the existing
  `binarySummaryLine()` text. There is exactly one fallback path, not a matrix of degraded states.

## Consequences

- The diff panel gains a rendering path that writes to the terminal **outside** bubbletea's
  string-based render loop — a deliberate, documented exception to the framework's model. Anyone
  touching diff-panel layout must remember that resizing/scrolling the panel requires emitting a
  clear, not just recomputing `View()`.
- The feature is inherently best-effort: detection can produce false positives in unusual
  terminal/multiplexer combinations, which is why a config escape hatch (`image-diffs`) exists
  alongside auto-detection.
- `ui/worktrees/kitty.go` remains the example of the *simpler* "shell out to kitty" pattern (option
  A) for cases where launching an external tool is the right shape (terminal sessions). This ADR
  documents why the diff-panel feature deliberately does not follow that simpler precedent.

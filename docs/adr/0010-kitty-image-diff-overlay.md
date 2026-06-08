# ADR 0010 — Inline image diffs render as kitty graphics overlays, not external launches

## Status
Accepted (amended — see "Generalization to the commit detail panel" below)

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
  disrupting event, and only if the currently selected file is still an image diff *and* no modal
  is open. The modal check matters because a modal is composed into `View()`'s text output
  (`ui.OverlayCenter`) — the kitty placement paints over that at the terminal's graphics layer
  regardless, so placing while a modal is open would occlude it. (Opening or closing a modal is
  itself treated as a disrupting event, so the overlay reappears once the modal closes.) This also
  avoids placement thrash when the user holds `j`/`k` to move quickly through the file list.
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

## Generalization to the commit detail panel (amendment)

The feature first shipped only in status's diff panel. It is now also available in the **commit
detail panel** (`ui/commit`), the shared right/bottom panel of the log and stash split views — so
image diffs render for committed and stashed changes, not just the working tree.

The original implementation lived in `ui/status/image_diff.go`, coupled to the status `Model`. The
generalization splits it into reusable and host-specific parts:

- **`ui/imagediff`** — the pure layout module (moved up from `ui/status/imagediff`) plus a reusable
  **`Overlay` controller** that owns the entire ADR-0010 lifecycle: the eager-clear/debounced-
  replace state machine, the settle timer, the cached terminal capability, and the place/clear
  `tea.Cmd`s. The controller is parameterized by host callbacks (fetch old/new blobs, report the
  panel's body geometry in **absolute** screen cells, report whether a modal is open, write bytes
  to the terminal, detect capability). `status.Model` and `commit.Model` each embed one and supply
  those callbacks.

- **Two host adapters.** Status keeps its working-tree-vs-index blob source (`git.ImageDiffBlobs`).
  The commit detail panel uses a ref-based source whose old/new endpoints are resolved by the *same*
  helper that builds the text-diff arguments (`git/commit_diff.go`), so the image and the unified
  diff can never disagree about which two versions they compare — including the stash `^3`-untracked
  case and rename old-paths.

The hard part is **absolute screen coordinates**. Kitty placements are positioned at absolute cell
coordinates, but a detail panel composed into a split view via `lipgloss.Join*` only receives its
width/height — never its origin. Two shapes were possible: (a) move the controller up into the
log/stash container, which knows the absolute layout, and reach into the detail for selection/blob
state; or (b) keep the controller in the detail panel (consistent with status, where the panel owns
its overlay) and **inject the detail panel's absolute screen origin** from the container.

Shape (b) was chosen. `splitview` gains `DetailOrigin() (col, row)`; the container pushes it into
`commit.Model` (`WithScreenOrigin`) on every layout change, and the panel computes its body rect
relative to (0, 0) then adds the injected origin. This keeps "the panel that draws the overlay also
owns its lifecycle" true for both hosts, at the cost of a new contract: **a detail panel that paints
outside bubbletea must be told its screen origin** (see the *Screen origin* glossary entry).

Two lifecycle consequences specific to the embedded case:

- **List-selection changes are out-of-band.** Moving the list cursor (`j`/`k`/Enter) is a key routed
  to the *list* panel; it never enters `commit.Model.Update`, yet it swaps the detail's ref and is
  the most common disrupting event. So the detail's mutating setters (`WithRef`, `WithScreenOrigin`)
  **return the disrupt `tea.Cmd`** for the container to batch — the controller still owns the
  lifecycle, the container only relays the command it gets back.

- **The settle tick must be routed back in.** `commit.Model` was synchronous before this; the
  debounce is its first async round-trip. log/stashlist drop unhandled messages rather than
  broadcasting them, so they add one explicit case forwarding the overlay's settle message
  (`imagediff.SettleMsg`) to `commit.Model.Update`. Tab-switch-away clearing reuses the existing
  `OnPageDeactivated` hook, forwarded by the container to the detail's `Overlay`.

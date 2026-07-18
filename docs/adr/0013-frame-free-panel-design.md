# ADR 0013 — Persistent layout panels render frame-free, not bordered

## Status
Accepted

## Context

Every persistent layout panel (status's filetree/staged/unstaged, log's commit list/detail, commit
view, stashlist, worktrees) rendered with a full rectangular border via `ui.RenderPanelFrame`,
active/inactive and semantic meaning (staged=green, unstaged=orange, filetree=blue) carried by
border- and title-color. The user judged this too visually busy for panels that are on screen at
all times, and asked for a border-free alternative distinguishing panels by background/spacing
instead — while keeping borders for transient overlays (modals, menus, confirms).

A survey of other bordered-panel TUIs (lazygit, k9s, gitui, yazi, superfile, tig) found full
borders are still the norm there, with focus shown via a border-color swap. The closest frame-free
precedent is Charm's own `crush` (same bubbletea v2 + lipgloss v2 stack): most panels have no
rectangular border, using a 4-tier background elevation scale and `Focused`/`Blurred` style pairs
for active/inactive state instead of brightness alone. Every tool surveyed, including `crush`,
still borders its floating modals/dialogs even with a frame-free base layout — a convergent signal
that modal borders are worth keeping regardless of the persistent-panel decision.

The design was iterated live (real rendered output via the `run` skill, not static mockups) across
several rounds before landing on the rules below.

## Decision

Persistent layout panels render via `ui.RenderPanel` (`ui/frame.go`, options built by
`ui.PanelOptionsFor`) — **no border glyphs**. Bordered rendering (`ui.RenderPanelFrame` /
`ui.RenderModalFrame`) is retained only for popups/modals; this is now the *only* rendering path
for persistent panels, not a toggle (the prototype's `GX_FRAMELESS` env gate was removed once the
design was validated).

Concrete rules:

- **No panel-level margin.** Panels render edge-to-edge. Separation between adjacent panels is
  drawn by the *composing view*, not the panel: a 1-cell seam (`ui.RenderSeamRow` /
  `ui.RenderSeamColumn`, filled with `ui.SeamColor` = `ui.ColorBase`) inserted between panels by
  `ui/log/view.go`, `ui/status/diff_view.go`, etc. A single full-screen panel therefore gets no
  seam automatically, since the seam belongs to the composition, not the panel.
- **Padding:** `PaddingX: 1, PaddingY: 0` inside every panel (left/right only).
- **Active/inactive state** is carried by the **header background**: `ui.ColorSurface` when
  inactive, `ui.ColorSurface1` when active, plus bold title text when active. No left accent bar or
  corner glyph. The panel body background is uniform (`ui.ColorBase`) regardless of active state —
  only the header distinguishes it, per HITL feedback favoring a distinct header over a shifted
  body.
- **Sidebar mode:** a panel's *body* background darkens to `ui.ColorMantle` when it is shown
  alongside a detail/preview panel (vs. `ui.ColorBase` standalone), via the `sidebar bool` param on
  `PanelOptionsFor` (driven by `splitview.Model.IsSplit()`). This is independent of active/inactive
  — see CONTEXT.md's **Sidebar mode** glossary entry.
- **Semantic color** (staged=green, unstaged=orange, filetree=blue) is unchanged from the bordered
  design: each call site passes its own `titleColor`/`accent` into `PanelOptionsFor`, rendered as
  title/accent color against the flat header instead of a border.
- Filetree title overflow (title + branch + worktree didn't fit) was resolved by moving
  branch/worktree info to two dim lines at the bottom of the panel instead of the title row.

## Naming

The prototype-era `Frameless`-prefixed names were renamed once this became the permanent design,
not a variant: `RenderFramelessPanel` → `RenderPanel`, `FramelessPanelOptions(For)` →
`PanelOptions(For)`, `FramelessSeamColor` → `SeamColor`, `ui/frameless.go` → `ui/panel.go`. The
worktrees package's stale `sidebar` naming for its read-only preview area (`ui/worktrees/sidebar.go`
et al.) was also renamed to `preview` (`ui/worktrees/preview.go`) as part of this effort, freeing
"sidebar" for the four actual list-driving sidebars — see CONTEXT.md's **Preview panel** /
**Sidebar** / **Sidebar mode** entries.

## Considered Options

- **Keep borders, swap border color for focus** (the lazygit/k9s/gitui/yazi norm). Rejected by the
  user as still too visually busy for always-on panels.
- **Background-only distinction with no header treatment** (Variant B: `ColorSurface`/`ColorSurface1`
  body backgrounds). Tried first; the user preferred the header-background variant (Variant A) from
  the first comparison and it was not iterated further.
- **Left accent bar or corner glyph for active state** (closer to `crush`'s accent-bar approach).
  Not adopted — header background + bold title was judged sufficient on its own.

## Out of Scope

- Terminal background variance (light-background / transparent terminals) — no existing detection
  for this; standing preference is dark-theme only, anchored to the existing Catppuccin-inspired
  palette in `ui/styles.go`.
- Modal/menu/confirm border removal — every frame-free TUI surveyed, including `crush`, still
  borders its modals/floating dialogs, so `ui.RenderModalFrame` keeps its border.
- Tab bar (`ui/app/model_tabs.go`) — already a flat, non-bordered idiom, not part of this change.
- `ui/scrollbar.go` rendering relative to a bordered frame — not revisited; no issue was found in
  practice, but it wasn't specifically re-verified against the frame-free layout either.

## Consequences

- All persistent layout panels (status, log, commit, stashlist, worktrees) now share one rendering
  path (`ui.RenderPanel`) with no feature flag; any new persistent panel should use it directly
  rather than reaching for `ui.RenderPanelFrame`.
- **Resolved:** `ui/commit/image_diff.go`'s `diffPaneBodyRect()` was flagged as a follow-up on the
  assumption that the frame-free header renders a header row plus a 1-cell margin row before the
  body, which would have shifted the kitty image-diff overlay's vertical offset from 1 to 2. That
  assumption doesn't hold: `PanelOptionsFor` (`ui/panel.go`) always sets `PaddingY: 0`, and
  `RenderPanel` (`ui/frame.go`) only inserts `PaddingY` margin rows, so no margin row exists between
  header and body for any panel. The existing `paneY+1` offset is correct as-is; no code change or
  terminal verification was needed.

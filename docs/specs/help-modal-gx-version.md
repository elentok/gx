# Help modal: show the gx version in the bottom-right border

## Problem Statement

The help modal (opened via the `?` keybinding, `ui/help/help.go`) shows the gx version nowhere.
The version string is already computed for the `gx --version` CLI flag (`cmd/version.go`), but
it's unexported to package `cmd` and never surfaced anywhere inside the TUI. A user who wants to
know which gx build they're running (e.g. to compare against a changelog entry, or when reporting
a bug) has no in-app way to see it short of quitting and running `gx --version` from the shell.

## Solution

Show the resolved version string in the bottom-right corner of the help modal's border, mirroring
the existing top-border title convention (`╭─ Keybindings ─── 2/5 ─╮`) but on the bottom edge
instead: `╰─────────────────────── v0.28.16 ─╯` (the exact string `gx --version` prints after
`"gx "` — no separately-added prefix; see Implementation Decisions).

## User Stories

1. As a gx user, I want to see the running gx version while the help modal is open, so that I can
   confirm which build I'm on without leaving the TUI.
2. As a gx user filtering the help modal, I want the version label to stay visible in the bottom
   border regardless of scroll position or filter query, so that it doesn't get pushed off-screen
   by keybinding content.
3. As a gx user on a build without an embedded version (a `go run`/`go install` build with no
   `-ldflags` version stamp), I want the modal to fall back to the same `dev` label the CLI already
   uses, so that the border never shows a blank or malformed corner.

## Implementation Decisions

- Extract the version-resolution logic currently in `cmd/version.go` (`getVersion`) into a new
  leaf package, `github.com/elentok/gx/version`, exporting a `Get() string` function with the same
  resolution order: build-time `-ldflags`-injected value, then `debug.ReadBuildInfo()`, then
  `"dev"` fallback. This extraction is **required, not a style preference**: `cmd/*.go` already
  imports `github.com/elentok/gx/ui` extensively, so `ui/help` cannot import `cmd` (import cycle) —
  a shared leaf package is the only way for both `cmd` and `ui/help` to reach the same logic.
  `cmd/version.go` calls into this package instead of holding its own copy; the
  `-ldflags "-X ...version=..."` build flag target moves to the new package's variable.
- Update every place that stamps the version at build time to target the new package's variable
  instead of `cmd.version`: `Makefile` (the `-ldflags "-X github.com/elentok/gx/cmd.version=..."`
  lines) and `.goreleaser.yaml` (same `-X` target). Missing this silently drops the stamped version
  from release and `make install` binaries (falls back to `debug.ReadBuildInfo()`/`"dev"`) — both
  `gx --version` and the new modal label depend on it.
- `ui/frame.go`: extend `injectBorderTitle` (or add a sibling function it shares logic with) to
  also embed a right-aligned label on the **bottom** border line, not just the top. Add a
  `BottomRightTitle` field to `ModalFrameOptions`, rendered only when `TitleInBorder` is set and
  `BottomRightTitle` is non-empty — mirrors how `RightTitle` is optional on the top border today.
  Unlike the current `injectBorderTitle`, truncate `Title`/`RightTitle`/`BottomRightTitle` when
  they don't fit the frame width (matching the truncation `RenderPanelFrame`'s `topInner` logic
  already does), so a narrow frame can't overlap or overflow a border corner.
- `ui/help/help.go`: `View()` passes `BottomRightTitle: version.Get()` — the exact string
  `version.Get()` returns, with **no added prefix**. This matches the CLI exactly: `runVersion`
  prints `fmt.Fprintf(w, "gx %s\n", getVersion())`, where any leading `v` comes from the git tag
  content itself (`git describe`), not a hardcoded prefix. Prepending `"v"` unconditionally would
  diverge from the CLI on a tag-less fallback build (e.g. `git describe --always` producing a bare
  commit hash) — the modal must never show something the CLI wouldn't. Only the help modal changes
  in this pass — other `RenderModalFrame`/`PanelFrameOptions` callers are unaffected since the new
  field defaults to empty/unused.
- Bottom-border label styling matches the existing top-right title: `ui.ColorGray`, no bold,
  space-padded on both sides. (Author's judgment call — not independently requested; free to
  bikeshed at implementation time since it's cosmetic and low-risk.)
- Package path `github.com/elentok/gx/version` is also an author's judgment call, not a requirement
  — any leaf location that avoids the `cmd`↔`ui` cycle works equally well.
- No caching/memoization decision needed: `version.Get()` is called once per `View()` render, same
  cost profile as the existing `getVersion()` call for `--version`.

## Testing Decisions

- Unit-test the new bottom-border injection in `ui/frame_test.go`, following the existing table
  style used by `TestRenderModalFrameIncludesTitleBodyAndHint`: assert the rendered frame's last
  line contains the bottom-right label, is still bounded by `╰`/`╯`, and that omitting
  `BottomRightTitle` leaves the bottom border unchanged (regression guard for the other
  `RenderModalFrame` callers). Include a narrow-frame case where `Title`/`RightTitle`/
  `BottomRightTitle` combined exceed the frame width, asserting the labels truncate rather than
  overlapping or overflowing the corners.
- Unit-test `version.Get()`'s three-way fallback (ldflags value set / build-info version present /
  neither present → `"dev"`) in the new package's own test file, following the same
  table-driven style as existing small-package tests in this repo (e.g. `ui/frame_test.go`).
- Do not test `ui/help` itself for the version string — the frame-injection test already covers the
  rendering mechanics, and `help.View()`'s job is just to pass the value through, not compute it.
- Prior art: `ui/frame_test.go` for `RenderModalFrame`/`injectBorderTitle` coverage style.

## Out of Scope

- Adding the version label to `PanelFrameOptions`/`panel.go` or any other bordered surface (status
  bar, other modals) — help modal only, per the request.
- Any new keybinding, copy-to-clipboard, or update-check affordance tied to the version label — it
  is a static, read-only label.
- Changing how the version is computed or injected at build time (the `-ldflags` mechanism itself
  is unchanged, only relocated to the new package).

## Further Notes

This was scoped directly to a lightweight spec rather than a full wayfinder map — the change is
confined to one existing rendering seam (`injectBorderTitle`/`RenderModalFrame`) plus a small,
low-risk extraction of already-working version logic into its own package. No open decisions or
fog remain.

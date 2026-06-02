# ADR 0005 — `gx term` directional split flags (`--right`/`--below`)

## Status
Accepted

## Context

`gx term` exposes the `ui/terminalrun` launcher (previously TUI-only) as a CLI
command: open a command into a tmux/kitty split or tab, falling back to running
in place when no multiplexer is available. The headline use case is launching
things from neovim (e.g. `gx term --below lazygit`) into a split.

The launcher must name the split *direction* on the command line. The internal
`SplitType` enum and the tmux/kitty vocabulary both use `hsplit`/`vsplit`, but
those terms mean **opposite things** across the two terminals:

- **tmux** `split-window -h` = side-by-side (left/right); `-v` = stacked
  (top/bottom).
- **kitty** `--location=hsplit` = stacked (top/bottom); `vsplit` = side-by-side.

The existing TUI code maps `HSplit → tmux -h` *and* `HSplit → kitty hsplit`,
so the same `SplitType` already produces opposite visual layouts depending on
the terminal. Inside the TUI this is obscured (the user picks from a menu and
sees the result), but on a CLI flag it would be a documented promise that lies
on one of the two terminals.

Separately, the user asked whether to expose `--left`/`--above` in addition to
`--right`/`--below`. tmux supports all four directions (`-b`/before gives left
and above); kitty cannot choose a *side* at all — `--location` selects only the
split *axis*, and kitty decides placement.

## Decision

Name the CLI direction flags by **visual outcome**, not by the tmux/kitty
keyword:

- `--right` — new pane side-by-side (tmux `-h`, kitty `vsplit`)
- `--below` — new pane stacked underneath (tmux `-v`, kitty `hsplit`); this is
  the **default** when no direction is given
- `--tab` — new tab
- `--here` — run in place (exec-replace), even when a multiplexer is available

Do **not** expose `--left` or `--above`. They are real on tmux but kitty cannot
honor a side, so they would degrade to the same-axis split on kitty
(`--left ≡ --right` visually) — reintroducing exactly the cross-terminal lie
this decision removes, only more quietly.

The internal `SplitType` enum keeps its `HSplit`/`VSplit` names; the mapping
from the user-facing directional flags to `SplitType` lives in the CLI layer.

## Considered Options

- **Mirror the internal `hsplit`/`vsplit` vocabulary.** Rejected: keeps the CLI
  aligned with the package's own terms but inherits the tmux-vs-kitty
  orientation inconsistency as a user-facing promise.
- **`--split`/`--vsplit` (the original sketch).** Rejected: `--split` is vague
  about orientation and the pairing is asymmetric.
- **Add `--left`/`--above`, documented as tmux-only.** Rejected: a flag that
  silently does something different on kitty is precisely the surprise the
  directional naming exists to avoid. Two axes both terminals honor identically
  is a smaller, honest surface.

## Consequences

- The CLI flag names (`--right`/`--below`) deliberately do **not** match the
  internal `SplitType` names (`HSplit`/`VSplit`). A future reader touching the
  flag→`SplitType` mapping needs this ADR to understand why they diverge and why
  there is no `vsplit`/`hsplit`/`left`/`above` flag.
- Users can build neovim mappings on `--right`/`--below` and trust the layout is
  identical on tmux and kitty.
- Adding `--left`/`--above` later remains possible if kitty ever gains side
  selection, without churning the existing flags.

# Unified Diff Cleanup Plan

## Goal

Keep unified diff fully interactive, but make it as easy to scan as the side-by-side view:

- hide literal `+` / `-` markers in changed rows
- keep delta syntax highlighting
- extend add/remove row background to the full rendered width

## Constraints

- Unified mode is still the source of truth for hunk and line operations.
- We should not parse delta's side-by-side output for this work.
- Search no longer matching literal `+` / `-` markers is acceptable.

## Approach

Use a custom unified display renderer on top of the existing unified diff data:

1. Keep raw unified diff parsing unchanged for:
   - hunk/line mapping
   - stage/unstage/discard patch synthesis
   - selection and cursor behavior
2. Keep using delta `--color-only` in unified mode for syntax-colored content.
3. Transform each unified display row before rendering:
   - classify rows as plain, add, remove, or hunk header
   - strip the visible change marker from added/removed rows
   - preserve `displayToRaw` mappings
4. In pane rendering, pad added/removed rows with matching background color so the tint reaches the right edge of the panel.

## File Areas

- `ui/status/model_state.go`
  - add display-row metadata to section state
- `ui/status/model_diffstate.go`
  - build unified display rows with row kinds
  - strip visible `+` / `-` markers from changed rows
  - preserve row kinds through wrapping
- `ui/status/view_panes.go`
  - apply full-width padding background for add/remove rows
- `ui/status/model_test.go`
  - cover hidden markers
  - cover full-width background padding
  - keep stage/unstage behavior coverage in unified mode

## Non-Goals

- rewriting unified mode around delta's side-by-side renderer
- changing hunk/line patch logic
- changing search semantics to preserve `+` / `-` matches

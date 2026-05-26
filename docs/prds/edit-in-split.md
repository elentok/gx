# Edit File in Split/Tab

## Problem Statement

When reviewing a diff in the status or commit view, opening a file for editing always takes over the
current terminal session (in-place). There is no way to open the file in a new split pane or tab
without leaving gx entirely. This forces a context switch and interrupts the review workflow.

## Solution

Add an `e`-prefix chord family to both the status view and the commit view that opens the selected
file in the editor in four ways: in-place (`ee`), horizontal split (`es`), vertical split (`ev`),
and new tab (`et`). All variants pass the correct line number to the editor so the cursor lands at
the relevant position. On plain terminals, split/tab variants display a warning and do nothing.

## User Stories

1. As a developer reviewing unstaged changes in the status view, I want to press `ee` to open the
   selected file in-place, so that the current "e" muscle memory keeps working.
2. As a developer reviewing unstaged changes in the status view, I want to press `es` to open the
   selected file in a horizontal split, so that I can edit while keeping the diff visible.
3. As a developer reviewing unstaged changes in the status view, I want to press `ev` to open the
   selected file in a vertical split, so that I can use screen width more efficiently.
4. As a developer reviewing unstaged changes in the status view, I want to press `et` to open the
   selected file in a new terminal tab, so that my current layout is preserved.
5. As a developer with the diff panel focused in the status view, I want the editor to open at the
   line corresponding to the active hunk or changed line, so that I land in the right place
   immediately.
6. As a developer with the filetree panel focused in the status view, I want the editor to open at
   the top of the selected file, so that opening for editing is always available regardless of focus.
7. As a developer reviewing a commit in the commit view, I want to press `ee`/`es`/`ev`/`et` to
   open the selected file in the corresponding editor mode, so that the same editing workflow is
   available when browsing history.
8. As a developer with the diff panel focused in the commit view, I want the editor to open at the
   line of the active hunk or changed line, so that I land at the relevant position in the file.
9. As a developer with the header panel focused in the commit view, I want a warning when I press
   any edit chord, so that I know why no editor opened.
10. As a developer on a plain terminal (no tmux or kitty remote), I want a warning when I press
    `es`, `ev`, or `et`, so that I understand splits and tabs are not available in my environment.
11. As a developer on a plain terminal, I want `ee` to still work, so that in-place editing is
    always available regardless of terminal type.
12. As a developer, I want the editor to receive the correct line number regardless of which open
    mode I use, so that I always land at the right position in the file.
13. As a developer using tmux, I want `es` to open a horizontal split and `ev` to open a vertical
    split, so that the axis matches standard tmux orientation conventions.
14. As a developer using kitty with remote control, I want `et` to open a new kitty tab, so that
    the file opens without disturbing my current window layout.

## Implementation Decisions

### `terminalrun` package extended with `SplitType`

The `terminalrun` package is the single place that owns "run a command in a given terminal context."
It will gain a `SplitType` value type with four variants: `InPlace`, `HSplit`, `VSplit`, `Tab`. The
main entry point becomes `CommandWithSplit(worktreeRoot, terminal, splitType, program, args, done)`
(name subject to change). The existing `Command` and `CommandCustom` functions are preserved for
callers that only need the original in-place/hsplit behaviour.

Terminal × SplitType compatibility matrix:

| SplitType | Plain | Tmux | Kitty | KittyRemote |
|-----------|-------|------|-------|-------------|
| InPlace   | ✓     | ✓    | ✓     | ✓           |
| HSplit    | warn  | ✓    | warn  | ✓           |
| VSplit    | warn  | ✓    | warn  | ✓           |
| Tab       | warn  | ✓    | warn  | ✓           |

Tmux mappings: HSplit → `split-window -h`, VSplit → `split-window -v`, Tab → `new-window`.
Kitty remote mappings: HSplit → `@launch --type=window --location=hsplit`, VSplit →
`@launch --type=window --location=vsplit`, Tab → `@launch --type=tab`.

### `editorLaunchArgs` moved to `ui` package

The function that maps `(editorBin, args, target, line)` to the correct editor-specific
argument list is currently unexported in the `status` package. It is moved to the `ui` package as
an exported function so both the `status` and `commit` packages can use it without duplication.

### Status view: `e` becomes a chord prefix

The single-key `e` binding (`bindingEdit`) is replaced by four chords under the `e` prefix:
`ee` (in-place), `es` (hsplit), `ev` (vsplit), `et` (tab). An `e+esc` cancel-chord entry is added
following the pattern already used by `y`, `c`, `g`, and `m` prefixes. The dispatch handler passes
the appropriate `SplitType` to `terminalrun`.

Line number resolution in the status view is unchanged: diff focus → active hunk/line position,
filetree focus → 0 (open at top of file).

### Commit view: `e`-prefix chords added

The commit view gains the same four `e`-prefix chords. The commit model implements its own
`editorLineForCurrentSelection` method (mirroring the status model) using `focusDiff` and
`diffModel`. Focus rules:
- Filetree focus: open selected file at line 0.
- Diff focus: open at active hunk/line position.
- Header focus: warn "no file selected", no-op.

### Worktrees migration (final task)

The split and tab command functions currently private to the `worktrees` package (`cmdTmuxHSplit`,
`cmdTmuxVSplit`, `cmdTmuxNewWindow`, `cmdKittySplit`, `cmdKittyNewTab`) duplicate logic now owned by
the extended `terminalrun`. After the status/commit work is complete, the `worktrees` model is
migrated to call `terminalrun` for these actions and the duplicate functions are removed.

## Testing Decisions

**What makes a good test:** test external behaviour, not implementation. Construct real or minimal
model state, trigger an action, assert on the resulting `tea.Cmd` or message type. Do not assert on
internal fields.

### Modules to test

**`terminalrun` (extended):** Table-driven unit tests covering each `Terminal × SplitType`
combination. For unsupported combinations, assert the returned `tea.Cmd` produces a `notify`
warning message. Prior art: existing `TestCommand_ReturnsCmd` and `TestCommandCustom_KeepOpen` in
`terminalrun_test.go`.

**`ui.editorLaunchArgs`:** The existing table-driven tests in `status/model_test.go` move alongside
the function to a new `ui` package test file. Coverage stays the same.

**Status view — `editorLineForCurrentSelection`:** The existing
`TestEditorLineForCurrentSelectionInDiffMode` test stays in the `status` package; no new cases
needed unless the logic changes.

**Status view — chord dispatch:** Existing `model_test.go` tests that cover `bindingEdit` are
updated to exercise `ee`; spot-check `es`/`ev`/`et` produce the expected `SplitType` (by
inspecting the message type returned by the cmd). Prior art: status model key dispatch tests.

**Commit view — `editorLineForCurrentSelection`:** New test mirroring the status model test,
asserting line resolution for diff-focus and returning 0 for filetree-focus. Prior art:
`TestEditorLineForCurrentSelectionInDiffMode` in `status/model_test.go`.

## Out of Scope

- A modal/menu UI for split selection (decided against during design).
- Supporting `TerminalKitty` (non-remote) for splits/tabs — requires enabling kitty remote control;
  users see the existing "enable kitty remote control" notice.
- Adding split/tab open modes to the log view or worktrees view beyond the worktrees migration.
- Any changes to the `e`-prefix namespace beyond the four edit chords defined here.

## Further Notes

The `e` prefix is intentionally left open for future chords beyond the four defined here. The
cancel-chord entry (`e+esc`) must be added alongside the four bindings to match the existing chord
cancellation pattern used by `y`, `c`, `g`, and `m`.

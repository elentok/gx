# Go Migration Plan

Migrate git-helpers from a Deno/TypeScript CLI to a Go TUI using BubbleTea. The first feature
is a Worktrees page that replaces the current `status` command with an interactive, always-visible
worktree dashboard.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/charmbracelet/bubbletea` | TUI framework (model-update-view loop) |
| `github.com/charmbracelet/bubbles` | Pre-built TUI components (see below) |
| `github.com/charmbracelet/lipgloss` | Terminal styling and layout |
| `github.com/charmbracelet/x/exp/teatest` | E2E testing for BubbleTea programs |

### Bubbles components we use

- **`bubbles/table`** - Worktree list. Provides columns, rows, keyboard navigation (up/down/j/k
  are built in via KeyMap), selected row tracking, and header/cell/selected styles.
- **`bubbles/viewport`** - Sidebar. Scrollable content pane for commit log and changed files.
- **`bubbles/textinput`** - Rename and clone dialogs. Single-line input with default value,
  cursor, placeholder support.
- **`bubbles/spinner`** - Loading indicator for async git pull/push operations.
- **`bubbles/help`** - Help bar at the bottom. Auto-generates help text from `bubbles/key`
  bindings, supports short (single-line) and full (multi-line) modes.
- **`bubbles/key`** - Keybinding definitions. All our keys go through this so they integrate
  with the help component automatically.

### Lipgloss usage

- **Styling** - borders (rounded for sidebar, normal for table), colors (green/yellow/red for
  sync status), bold/faint for emphasis.
- **Layout** - `lipgloss.JoinHorizontal` to compose table + sidebar side by side,
  `lipgloss.JoinVertical` to stack content + help bar. `lipgloss.Place` for centering dialogs.
- **Dimensions** - `Width`/`Height` constraints to make the layout responsive to terminal size.

## Architecture Overview

```
gx/
├── main.go                  # Entry point, initialize app
├── go.mod
├── git/                     # Git operations (pure library, no TUI dependency)
│   ├── repo.go              # Repo discovery, bare repo detection
│   ├── worktree.go          # Worktree list/add/remove/move
│   ├── branch.go            # Branch create/delete/rename, local & remote
│   ├── remote.go            # Remote list/update/prune
│   ├── status.go            # Sync status (ahead/behind/diverged), uncommitted changes
│   ├── log.go               # Commit log retrieval
│   ├── run.go               # Low-level git command executor
│   ├── repo_test.go
│   ├── worktree_test.go
│   ├── branch_test.go
│   └── status_test.go
├── ui/                      # TUI layer (BubbleTea)
│   ├── app.go               # Root model, page routing, global keybindings
│   ├── worktrees/            # Worktrees page
│   │   ├── model.go         # Page model, Update, View
│   │   ├── table.go         # Wraps bubbles/table with our columns and row data
│   │   ├── sidebar.go       # Wraps bubbles/viewport for commit log + changes
│   │   ├── keys.go          # Key map definitions (bubbles/key bindings)
│   │   ├── delete.go        # Delete confirmation dialog
│   │   ├── rename.go        # Rename dialog (wraps bubbles/textinput)
│   │   ├── clone.go         # Clone dialog (wraps bubbles/textinput)
│   │   ├── yank.go          # File selection (checkboxes) for yank
│   │   └── paste.go         # Paste action
│   ├── styles.go            # Shared lipgloss styles
│   └── components/          # Custom components (only what bubbles doesn't provide)
│       └── checklist.go     # Multi-select checkbox list (not in bubbles)
└── testutil/                # Test helpers
    ├── repo.go              # Create temp bare repos with worktrees
    └── commit.go            # Create dummy commits
```

### Key design decisions

- **`git/` has no TUI imports.** It returns Go structs; the UI layer formats them. This makes
  the git layer independently testable.
- **Lean on bubbles for standard components.** Table, text input, viewport, spinner, and help
  are all provided by bubbles - we wrap them with our data, not reimplement them. The only
  custom component is the checkbox list for yank (bubbles/list doesn't support multi-select
  with checkboxes).
- **Each interaction (delete, rename, clone, yank) is a sub-model** within `ui/worktrees/`. The
  page model delegates to the active sub-model when one is open.
- **Clipboard is page-level state**, not global. It holds a list of file paths and source worktree.
  The sidebar or a status bar shows "N files in clipboard" when non-empty.

## Testing Strategy

### Unit tests (`git/` package)

Standard Go tests using temp directories. The `testutil/` package creates throwaway bare repos
with worktrees so we can test git operations against real repos without mocking.

### E2E tests (`teatest`)

Use `github.com/charmbracelet/x/exp/teatest` to drive the full TUI. Each test:

1. Creates a temp bare repo with worktrees (via `testutil/`).
2. Starts the app with `teatest.NewTestModel()`, pointed at the temp repo.
3. Sends keys and asserts screen content.

```go
func TestDeleteWorktree(t *testing.T) {
    repo := testutil.CreateBareRepoWithWorktrees(t, "feature-a", "feature-b")

    m := newModel(repo)
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

    // Navigate to feature-a and delete it
    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("feature-a"))
    })

    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("Delete"))  // confirmation prompt
    })

    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return !bytes.Contains(bts, []byte("feature-a"))  // row gone
    })

    tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
    tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}
```

Key patterns:
- **`WaitFor`** to assert screen content after each action (avoids flaky timing issues).
- **`testutil` temp repos** so tests are isolated and don't touch real repos.
- **Golden file snapshots** (`teatest.RequireEqualOutput`) for visual regression tests on key
  screens (initial load, sidebar content, dialogs). Run with `-update` to regenerate.

### What to E2E test

Each milestone adds E2E tests for its features:

| Milestone | E2E tests |
|-----------|-----------|
| 2 - Table | App starts, shows worktrees, j/k navigation updates sidebar, active worktree highlighted |
| 3 - Delete/Rename | d+y deletes and removes row, r+type+enter renames and updates row |
| 4 - Clone | c+type+enter creates new row in table |
| 5 - Yank/Paste | y opens checklist, space toggles, enter yanks, p on another worktree pastes |
| 6 - Pull/Push | gpl shows spinner then updates status, gps same |

---

## Milestone 1: Project scaffold + git library

Set up the Go module, implement the git operations layer, and verify with tests. No TUI yet.

### Steps

1. `go mod init`, add dependencies (`bubbletea`, `lipgloss`, `bubbles`, `x/exp/teatest`).
2. Implement `git/run.go` - execute git commands, capture stdout/stderr, return structured errors.
3. Implement `git/repo.go`:
   - `FindRepo(path) (Repo, error)` - walk up to find `.git` or bare repo.
   - `IdentifyDir(path) (DirInfo, error)` - detect bare repo root vs worktree root.
   - Repo struct: `Root string`, `IsBare bool`, `MainBranch string` (detect main vs master).
4. Implement `git/worktree.go`:
   - `ListWorktrees(repo) ([]Worktree, error)` - parse `git worktree list --porcelain`.
   - Worktree struct: `Path string`, `Name string`, `Branch string`, `Head string`.
5. Implement `git/branch.go`:
   - `DeleteLocalBranch`, `DeleteRemoteBranch`, `RenameBranch`, `CreateBranch`.
6. Implement `git/remote.go`:
   - `UpdateRemotes`, `PruneRemote`.
7. Implement `git/status.go`:
   - `SyncStatus(repo, branch) (Status, error)` - ahead/behind/diverged relative to main branch.
   - `UncommittedChanges(worktreePath) ([]Change, error)` - parse `git status --porcelain`.
8. Implement `git/log.go`:
   - `CommitsSinceMain(repo, branch) ([]Commit, error)` - log from HEAD to main branch merge-base.
9. Write unit tests for all of the above using temp repos (`testutil/`).

### Done when

- `go test ./git/...` passes.
- Can programmatically list worktrees, get sync status, and get commit logs from a real bare repo.

---

## Milestone 2: Basic TUI - worktree table

Render the worktrees page with the table, navigation, and sidebar. Read-only, no interactions yet.

### Steps

1. Implement `ui/app.go`:
   - Root BubbleTea model that holds the current page.
   - On init: detect repo from cwd, load worktrees, determine active worktree.
   - Global quit: `q`, `ctrl+c`.
2. Implement `ui/worktrees/table.go`:
   - Wrap `bubbles/table.Model` with three columns: Name, Branch, Sync Status.
   - Configure `table.DefaultKeyMap()` - it already supports up/down/j/k, page up/down, home/end.
   - Use `table.DefaultStyles()` as base, customize `Selected` style with lipgloss for the
     active row highlight.
   - Sync status column shows: "synced", "N behind", "N ahead", "diverged" with lipgloss
     color (green/yellow/magenta/red).
   - Set initial cursor to worktree matching cwd via `table.SetCursor()`.
3. Implement `ui/worktrees/sidebar.go`:
   - Wrap `bubbles/viewport.Model` for scrollable content.
   - Render content as a string: commits section (abbreviated hash + message) then changed
     files section (modified/added/deleted with lipgloss colors).
   - Set content via `viewport.SetContent()` whenever the selected table row changes.
   - Style with `lipgloss` rounded border on the left side.
4. Implement `ui/worktrees/model.go`:
   - Compose table + sidebar using `lipgloss.JoinHorizontal(lipgloss.Top, tableView, sidebarView)`.
   - On `tea.WindowSizeMsg`: allocate ~60% width to table, ~40% to sidebar, resize both via
     `table.SetWidth()`/`table.SetHeight()` and `viewport.Width`/`viewport.Height`.
5. Implement `ui/styles.go` - shared lipgloss styles (borders, status colors, section headers).
6. Implement `main.go`:
   - Parse cwd, find repo, launch BubbleTea program with worktrees page.
   - Wire up `bin/gx` to run the Go binary (or just `go run .` during dev).

### Done when

- Running `gx` from a bare repo or worktree shows the table with real data.
- Can navigate rows with j/k, sidebar updates.
- Active worktree is highlighted when launched from inside one.
- E2E tests: app starts with correct rows, navigation updates sidebar, golden snapshot for
  initial screen.

---

## Milestone 3: Delete and rename

Implement the `d` and `r` interactions.

### Steps

1. Implement `ui/worktrees/keys.go`:
   - Define all key bindings using `bubbles/key.NewBinding()` with `key.WithKeys()` and
     `key.WithHelp()` so they auto-integrate with the `bubbles/help` component.
   - Implement `ShortHelp()` and `FullHelp()` (the `help.KeyMap` interface) to control what
     shows in the help bar.
2. Implement `ui/worktrees/delete.go`:
   - `d` on a worktree shows a confirmation prompt inline (simple "Delete X? [y/N]" rendered
     at the bottom - no need for a full component, just intercept y/n/esc keys).
   - On `y`: call `git/worktree.Remove` + `git/branch.DeleteLocalBranch` +
     `git/branch.DeleteRemoteBranch`.
   - Refresh table via `table.SetRows()` after deletion.
   - Show error inline if deletion fails.
3. Implement `ui/worktrees/rename.go`:
   - `r` opens a `bubbles/textinput.Model` pre-filled with current name via
     `textinput.SetValue()`. Render it inline below the table.
   - On enter: call `git worktree move`, rename directory, rename branch.
   - Port the rename logic from the existing `rename-worktree.ts` (gitdir/`.git` file fixups).
   - Refresh table after rename.

### Done when

- Can delete a worktree with `d`, confirm, see it removed from the table.
- Can rename a worktree with `r`, type new name, see it updated.
- E2E tests cover both flows end-to-end against a temp repo.

---

## Milestone 4: Clone worktree

Implement the `c` interaction.

### Steps

1. Implement `ui/worktrees/clone.go`:
   - `c` opens a `bubbles/textinput.Model` pre-filled with current worktree name.
   - On enter: create new worktree as a copy.
   - Clone strategy: `git worktree add` with new branch, then `cp -r` working tree files
     (including untracked) from source to destination.
   - Refresh table after clone.

### Done when

- Can clone a worktree with `c`, specify name, see new worktree in table.
- Cloned worktree has all files including untracked ones from the source.

---

## Milestone 5: Yank and paste

Implement the `y` (yank files) and `p` (paste files) interactions.

### Steps

1. Implement `ui/components/checklist.go` - multi-select checkbox list with toggle and toggle-all.
2. Implement `ui/worktrees/yank.go`:
   - `y` shows a checklist of uncommitted + untracked files in the selected worktree.
   - All items checked by default.
   - Navigate with j/k, toggle with space, confirm with enter.
   - Store selected file paths + source worktree path in page-level clipboard state.
3. Update `ui/worktrees/model.go`:
   - Show clipboard indicator in status bar: "N files in clipboard" (or empty).
4. Implement `ui/worktrees/paste.go`:
   - `p` copies files from clipboard source to current worktree destination.
   - Preserve relative paths. Create directories as needed.
   - Clear clipboard after paste.
   - Show success/error message.

### Done when

- Can yank files from one worktree, navigate to another, paste them.
- Clipboard indicator shows file count, clears after paste.

---

## Milestone 6: Git pull and push

Implement `gpl` and `gps` chained key interactions.

### Steps

1. Add chained key support to the key handling in `ui/worktrees/model.go`:
   - Track a key buffer with a short timeout (e.g. 500ms).
   - `g` starts a chain, `gp` continues, `gpl` triggers pull, `gps` triggers push.
2. Implement pull/push as async commands (`tea.Cmd`) that run `git pull`/`git push` in the
   selected worktree directory.
3. Show a `bubbles/spinner.Model` in the status bar while the operation is running.
4. On completion: stop spinner, refresh sync status via `table.SetRows()`, show result message.

### Done when

- `gpl` runs git pull in the selected worktree, updates sync status.
- `gps` runs git push in the selected worktree, updates sync status.
- Status bar shows progress and result.

---

## Milestone 7: Polish and ship

### Steps

1. Error handling pass: ensure all git errors surface as user-visible messages, not panics.
2. Handle edge cases: empty repo, no worktrees, detached HEAD, worktree with no branch.
3. Wire up `bubbles/help.Model` at the bottom of the layout:
   - Renders short help (one-line) by default from the key bindings defined in `keys.go`.
   - `?` toggles full help (multi-line) showing all available keybindings.
   - Compose with `lipgloss.JoinVertical(lipgloss.Left, mainContent, helpView)`.
4. Update `bin/gx` to build and run the Go binary.
5. Add a `Makefile` or `go install` instructions.
6. Update `README.md`.

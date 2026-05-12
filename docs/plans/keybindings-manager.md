# Keybindings Manager

## Goal

Replace three parallel, duplicated structures in each UI model:

1. Manual chord tracking (`keyPrefix` field + `handleChordKey`)
2. `ChordHints(prefix string)` method (duplicates chord structure)
3. `var stageKey* = key.NewBinding(...)` + `var keySections = []help.KeySection{...}` (duplicates all bindings for help display)

With a `keybindings.Manager` per model that is the single source of truth for that model's binding metadata, chord dispatch, and help sections.

## Architecture

Each model (parent and child alike) owns its own `Manager`. Models are self-contained: they register their own bindings, handle their own chords, and generate their own help sections. The parent assembles the final help view by combining sections from all relevant managers:

```go
// Status model builds help from its own bindings + child model bindings
m.help = help.NewModel(slices.Concat(
    m.keys.HelpSections(),                        // global: quit, help, goto-log, chords, etc.
    m.fileTreeModel.Keybindings().HelpSections(), // filetree: j/k cursor, collapse, search, etc.
    m.diff.Keybindings().HelpSections(),          // diff: scroll, hunk nav, visual, etc.
))
```

Child models expose their Manager via a `Keybindings() keybindings.Manager` method. They use their own Manager internally in their `Update()` for chord handling and dispatch — exactly the same pattern as the parent.

**Boundary rule:** the status model's Manager covers bindings the *status model* handles (global actions, git operations, navigation between views, chords). Child model Managers cover bindings the *child model* handles internally (cursor navigation, search input, etc.). Keys not matched by the status model's Manager are passed to `delegateToChild`, which forwards the raw `KeyPressMsg` to the focused child's `Update()`.

## Package: `ui/keybindings`

### Types

```go
type Binding struct {
    ID         string     // dispatch identifier, e.g. "goto-log"
    Seq        []string   // key sequence: ["g","l"] for chord, ["j"] for single key
    Categories []string   // help sections this binding appears in (can be multiple)
    Title      string     // description shown in help, e.g. "goto log"
    Display    string     // optional key display override, e.g. "↑/k" (defaults to Seq joined with "/")
}

// Manager is a value type — embed it in the bubbletea model so prefix state is
// copied correctly on each update.
type Manager struct {
    bindings []Binding
    prefix   []string // accumulated key sequence in progress
}
```

### API

```go
func New(bindings []Binding) Manager

// Process feeds a key press and returns:
//   match != nil, consumed=true  → sequence complete, dispatch on match.ID
//   match == nil, consumed=true  → chord in progress, call ChordHints() for status bar
//   match == nil, consumed=false → key not registered, fall through to child delegation
func (m *Manager) Process(key string) (match *Binding, consumed bool)

// ChordHints returns completions for the current internal prefix.
func (m Manager) ChordHints() []key.Binding

// Bindings returns all registered bindings. The caller groups and renders them
// (e.g. by passing to help.NewModel).
func (m Manager) Bindings() []Binding

// Reset clears the accumulated prefix.
func (m *Manager) Reset()
```

### Notes

- Bindings with `len(Seq) == 1` match immediately; no prefix state is stored.
- Bindings with `len(Seq) > 1` accumulate prefix until the full sequence matches or a non-matching key cancels.
- The `keybindings` package has no dependency on the `help` package — it only returns `[]Binding`. The `help` package owns grouping and rendering.
- `G` (shift+G) currently requires complex detection in `handleChordKey` due to bubbletea inconsistencies — investigate whether `msg.String()` is reliable; may need to register `["G", "shift+g"]` as alternatives.
- If a background message opens a modal while the user is mid-chord, the Manager's prefix is left dirty. Call `m.keys.Reset()` wherever modal flags are set.

## Phase 1: Create `ui/keybindings` package

- [ ] Create `ui/keybindings/keybindings.go` with `Binding`, `Manager`, and all methods above
- [ ] Write unit tests in `ui/keybindings/keybindings_test.go`:
  - Single-key binding matches immediately
  - Two-key chord: first key → consumed, no match; second key → match
  - Cancellation: unrecognized second key → not consumed, prefix cleared
  - `ChordHints()` returns correct completions for current prefix
  - `Bindings()` returns all registered bindings in registration order

## Phase 2: Migrate `status` model

### 2a. Status model Manager

- [ ] Define the status model's `Manager` covering only status-level bindings:
  - Global: quit, help, `?`, chord prefixes and their completions (goto-log, git-commit, yank-*, etc.)
  - Status-owned context-specific actions available in filetree and/or diff focus: pull, push, rebase, amend, context-inc/dec, refresh, render-mode, etc.
  - Use `Categories []string` for bindings that apply in both filetree and diff focus
  - Do NOT include j/k navigation, search input, or other bindings owned by child models
- [ ] Add `keys keybindings.Manager` to `Model` struct; remove `keyPrefix string`
- [ ] Call `m.keys.Reset()` wherever modal flags are set (credential, running, output, confirm, error)

### 2b. Child model Managers

- [ ] Add `keys keybindings.Manager` to `filetree.Model` covering its internal bindings (cursor nav, collapse, search, etc.); expose via `Keybindings() keybindings.Manager`
- [ ] Add `keys keybindings.Manager` to the diff model covering its internal bindings (scroll, hunk nav, visual, wrap, etc.); expose via `Keybindings() keybindings.Manager`

### 2c. Wire up help

- [ ] Update `help.NewModel` to accept variadic `[]keybindings.Binding` slices, grouping by category internally; same category name from multiple slices merges into one section
- [ ] Replace `help.NewModel(keySections)` with:
  ```go
  help.NewModel(
      m.keys.Bindings(),
      m.fileTreeModel.Keybindings().Bindings(),
      m.diff.Keybindings().Bindings(),
  )
  ```
- [ ] Delete `stageKey*` vars and `keySections` from `model_help.go`

### 2d. Simplify key dispatch

- [ ] Rewrite `handleKeyPress` to use `m.keys.Process()`:
  ```go
  match, consumed := m.keys.Process(msg.String())
  switch {
  case match != nil:
      return m.dispatchBinding(match.ID, msg)
  case consumed:
      return m, nil // chord in progress
  }
  return m.delegateToChild(msg)
  ```
- [ ] Add `dispatchBinding(id string, msg tea.KeyPressMsg)` — the single dispatch point for all status-model-owned bindings, replacing `handleChordKey` + `handleFiletreeKey` + `handleDiffKey`
- [ ] Add `delegateToChild(msg tea.KeyPressMsg)` — forwards unmatched keys to the focused child model's `Update()`; replaces `handleFocusedChildKey` with a clearly-scoped function
- [ ] Delete `handleChordKey`, `handleFiletreeKey`, `handleDiffKey`, `handleFocusedChildKey`
- [ ] Delete `ChordHints(prefix string)` method; update the `ChordHinter` interface usage to call `m.keys.ChordHints()` directly

### 2e. Verify

- [ ] `go build ./...` and `go test ./...`

## Out of scope (follow-up)

- Migrate `log`, `worktrees`, `commit` models (same pattern once status is proven)

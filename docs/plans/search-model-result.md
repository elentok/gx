# search.Result: richer return from search.Model.Update()

## Problem

`search.Model.Update()` previously returned `(Model, tea.Cmd, bool)` — the bare `bool`
only signals whether the key was handled. Callers that need to react to *what changed*
(query updated? cursor moved? search just started?) had to snapshot state before the
call and diff afterwards:

```go
wasActive := m.search.IsActive()
prevQuery  := m.search.Query()
prevMode   := m.search.Mode()
newSearch, _, handled := m.search.Update(msg)
m.search = newSearch
if handled {
    if !wasActive && m.search.IsActive() { /* set scope */ }
    if m.search.Mode() == search.SearchModeInput && m.search.Query() != prevQuery { /* recompute */ }
    if prevMode == search.SearchModeResults && m.search.Mode() == search.SearchModeResults { /* jump */ }
}
```

This is noisy and error-prone: the caller must know which mode transitions matter and
reconstruct them manually.

## Solution

Replace the bare `bool` with `search.Result`:

```go
type Result struct {
    Handled       bool  // key was consumed by search
    Activated     bool  // transitioned None → Input (search session just started)
    QueryChanged  bool  // query text changed this update
    CursorChanged bool  // match cursor moved this update (n / N)
}
```

`Update()` now returns `(Model, tea.Cmd, Result)`. The Result is computed inside
`Update()` by snapshotting `mode`, `query`, and `cursor` before delegating to the
existing inner handlers, then diffing.

The caller at `ui/commit/model_update.go` becomes:

```go
newSearch, cmd, result := m.search.Update(msg)
m.search = newSearch
if result.Handled {
    if result.Activated {
        m.searchScope = searchScopeSidebar
        if m.focusDiff { m.searchScope = searchScopeDiff }
    }
    if result.QueryChanged {
        m.search.SetMatches(m.computeSearchMatches(m.search.Query()))
    }
    if result.QueryChanged || result.CursorChanged {
        m.jumpToCurrentMatch()
    }
    return m, cmd
}
```

No snapshotting. Every branch reads as intent.

## Scope of change

`ui/search/search_update.go` — change `Update()` return type; compute Result from
before/after diff. Inner handlers (`handleKeyPress*`) are unchanged (still return bool).

`ui/diffview/model.go`, `ui/filetree/model.go` — trivial: `handled` → `result.Handled`.
These callers don't need the richer signal yet; the change is just keeping them
compatible with the new signature.

`ui/commit/model_update.go` — replace snapshotting block with Result-driven logic
(see above). Now returns `cmd` (previously discarded as `_`) so the textinput cursor
blink animation is preserved.

## Why not async messages?

The message-driven approach (`SearchQueryUpdatedMsg` / `JumpToMatchMsg`) is what
`ui/status` uses and it works well there: status has separate sub-models (filetree,
staged diff, unstaged diff), each with its own embedded `search.Model`. Keys are
routed through one of those sub-models; results bubble back as messages.

`ui/commit` has a single model that owns both the sidebar and the diff pane, and its
search spans both. There's no sub-model to delegate through — commit handles search
directly. Synchronous computation (compute matches, jump) is simpler and keeps tests
working without a cmd-pipeline runner. The Result struct gives commit the signals it
needs without async indirection.

The `SearchQueryUpdatedMsg` / `JumpToMatchMsg` cmds are still emitted by
`search.Update()` (status relies on them). `ui/commit` receives them harmlessly
(they're not handled so they no-op) and still gets the textinput cursor cmd via the
returned `tea.Cmd`.

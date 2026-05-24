# Extract ui/navstate — Pure Navigation State Machine

**Beads**: main-2ol.1

## Goal

Extract a `ui/navstate` package that owns all routing *state* (active tab, live tab, history stack,
tab memory, transition logic) as a pure Go struct with no `tea.Cmd`/`tea.Model` dependencies. Then
rewire `app.Model` to hold a `navstate.State` and delegate to it.

## What navstate owns

- `activeTab`, `liveTab`
- `stack []nav.ViewState` (no `historyEntry` — no model references)
- `lastViewStateByTab map[nav.TabID]nav.ViewState`
- Transition methods: `Open`, `Switch`, `Back`, `ApplyViewStateChanged`
- Resolution helpers: `tabViewStateForViewContext`, `resolveTabID`, `sameViewContext`
- Default worktree path (needed for fallback resolution)

## What app.Model keeps

- `livePageByTab map[nav.TabID]livePage`
- `historyEntry{viewState, model}` (the model side of the stack)
- All `tea.Cmd` batching, `newHistoryEntry`/`newLivePage`, `restoreLogSelectionFromPoppedPage`
- View rendering

## Transition outputs (no tea.Cmd in navstate)

Each mutating navstate method returns a `Transition` value that describes what happened, so
app.Model can decide which tea commands to fire:

```go
type TransitionKind int
const (
    TransitionNone TransitionKind = iota
    TransitionPushed      // Open: new entry pushed, init + resize needed
    TransitionPopped      // Back: entry popped, resize needed
    TransitionSwitched    // Switch: tab changed, resize needed
    TransitionQuit        // Back on empty stack
    TransitionStateUpdate // ViewStateChanged: no page rebuild needed
)

type Transition struct {
    Kind         TransitionKind
    ActiveTab    nav.TabID
    ViewState    nav.ViewState // the new current view state
    PoppedEntry  nav.ViewState // set on TransitionPopped
}
```

## Plan

- [ ] Create `ui/navstate/navstate.go` with `State` struct and `Transition` types
- [ ] Implement `NewState(defaultWorktreePath string) State`
- [ ] Implement `State.Open(vs nav.ViewState) Transition`
- [ ] Implement `State.Switch(vs nav.ViewState) Transition`
- [ ] Implement `State.Back() Transition`
- [ ] Implement `State.ApplyViewStateChanged(vs nav.ViewState)`
- [ ] Implement `State.Active() nav.ViewState` and `State.ActiveTab() nav.TabID`
- [ ] Extract `tabViewStateForViewContext`, `resolveTabID` into navstate
- [ ] Write `ui/navstate/navstate_test.go` covering all transition edge cases
- [ ] Rewire `app.Model` to embed `navstate.State`, remove raw fields (`liveTab`, `activeTab`, `stack`, `lastViewStateByTab`)
- [ ] Update `app.Model.Update` to drive navstate and consume `Transition` outputs
- [ ] Run `go build ./... && go test ./...` clean

## Invariants to preserve (verified by existing tests)

1. `Back` on empty stack → `TransitionQuit`
2. `Switch` clears the stack
3. `ApplyViewStateChanged` updates top-of-stack iff same tab
4. Empty `WorktreeRoot` falls back to `defaultWorktreePath`
5. All context fields empty → restore from `lastViewStateByTab`
6. `TabCommit` with no ref → `"HEAD"`
7. Unknown tab → `TabWorktrees`

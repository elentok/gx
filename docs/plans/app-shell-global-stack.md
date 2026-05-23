# App-Shell Global Stack Refactor

Rewrite app-shell navigation state to a single global stack + `lastViewStateByTab`.
Removes per-tab history, commit→log tab coupling, and `routerState` duplication.

## Goals

- One global `stack []historyEntry` instead of per-tab `histories`
- `liveTab TabID` — base tab shown when stack is empty
- `activeTab` = top-of-stack tab or `liveTab`
- `commit` is a first-class tab (footer, `gc`, `4` keybindings)
- Remove `routerState` duplication; inline into `Model`

## Navigation Rules

1. `Open(r)`: push onto stack, `activeTab = r.Tab`
2. `Switch(target)`: clear stack, `liveTab = activeTab = target.Tab`
3. `Back()`: pop stack; if empty → quit; else `activeTab = stack[top].Tab` or `liveTab`
4. `ViewStateChanged(r)`: update `lastViewStateByTab[r.Tab]`; update top of stack if same tab

## Tasks

- [x] Write plan
- [x] Delete `router_state.go` and `router_state_test.go`
- [x] Rewrite `model.go`: new fields (`stack`, `liveTab`), `New()`, `Update()`, `activePage()`, `setActivePage()`
- [x] Update `model_tabs.go`: `switchTab()`, `resolveTabID()`, `orderedTabs()`, `tabsView()`, keybindings, `tabViewStateForViewContext()`
- [x] Update `keys.go`: add `BindingGotoCommit`, `gc`, `4` bindings
- [x] Update `model_test.go`: remove commit→log tests, add new scenarios
- [x] Run `go test ./ui/app/...` green

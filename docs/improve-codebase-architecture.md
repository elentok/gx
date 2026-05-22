# Architecture Deepening Opportunities

Generated 2026-05-21.

---

## 1. Modal lifecycle: shallow duplication across `amend/`, `bump/`, `push/`, `pull/`

- **Files:** `ui/amend/amend.go`, `ui/bump/bump.go`, `ui/push/push.go`, `ui/pull/pull.go`
- **Problem:** Each modal reimplements the same three-phase state machine: confirm → running → done. Each has its own `Result` struct with slightly different fields and its own spinner/step-list rendering. The confirm-running-done logic is spread across four modules rather than living in one. Deletion test: if you deleted any one modal, you'd still have three copies of the lifecycle logic; it's not earning its keep in any single place.
- **Solution:** Extract a shared `modal/executor` module owning the confirm→running→done state machine. Amend, bump, push, pull each describe *what steps to run* (a plan) and *what to display* — but the lifecycle of running those steps lives in one place.
- **Benefits:** Locality: lifecycle bugs (e.g., spinner not dismissed) are fixed once. Leverage: adding a new modal means writing a plan, not reimplementing a state machine. Tests for the executor cover all four modals implicitly.

---

## 2. Git calls embedded in modal `Open()` — no seam between planning and execution

- **Files:** `ui/amend/amend.go:Open()`, `ui/push/push.go`, `ui/pull/pull.go`, `ui/bump/bump.go`
- **Problem:** Each modal's `Open()` directly calls `git.*` functions to compute whether preconditions are met and which steps to enqueue. The git calls and the step-building logic are intertwined with UI state initialization. There is no seam between "compute the plan for this operation" and "display and execute that plan." This makes modals untestable without a real repository and makes the planning logic invisible to callers.
- **Solution:** Extract a pure planning function per modal (e.g., `amend.Plan(hash, stagedFiles, isPushed)`) that takes precomputed git data and returns steps. The caller fetches git data, calls the planner, gets back a plan, and passes it to the modal. The modal becomes pure UI.
- **Benefits:** Locality: all amend planning logic in one place. Leverage: plan functions are testable with zero git infrastructure. The modal itself has a simpler, narrower interface (just "display this plan").

---

## 3. `filetree/` generic structure hides a shallow seam: no separation between tree state and display

- **Files:** `ui/filetree/model.go`, `ui/filetree/view.go`, `ui/filetree/search.go`
- **Problem:** `filetree.Model[T]` owns navigation state, search state, key handling, and rendering all in one struct. The generic `T` is a clean idea, but the module isn't deep: callers must know about `Entry[T]`, `EntryKind`, `Leaves`, `DisplayName`, `Expanded` — almost as much as the implementation. There's no real seam between "tree navigation state" and "what gets rendered." The search integration is also repeated differently in `log/` and `filetree/`.
- **Solution:** Deepen the filetree module by making `Entry` construction the caller's only concern. Push collapse/expand, path rendering, and search-match highlighting fully inside. Callers should be able to call `filetree.SetItems(items)` and `filetree.View()` without knowing about `Entry`, `DisplayName`, or `Leaves`. The interface becomes a list of domain items in, rendered rows out.
- **Benefits:** Leverage: callers (status, commit) stop threading display concerns down into their models. Locality: collapse/expand logic, search highlighting, and row building live in one place. Tests: filetree can be tested by asserting on rendered output given a flat item list.

---

## 4. `diffview/` nav logic spread across `nav.go`, `search.go`, `yank_action.go`, `model.go` — no single seam

- **Files:** `ui/diffview/model.go`, `ui/diffview/nav.go`, `ui/diffview/search.go`, `ui/diffview/yank_action.go`
- **Problem:** The diffview module has good layering at the bottom (diffcore → diffrender → diffview), but within diffview the navigation and search concerns bleed into each other across four files. Understanding "what happens when I press `n` for next search hit" requires following the call through model → search → nav → render. There is no module that says "given a parsed diff and a search query, here are the reachable positions."
- **Solution:** Extract a `diffview/navigator` module that owns hunk/line position arithmetic and search cursor advancement. The diffview model holds a navigator and an offset; rendering is a pure projection from `(navigator.State, viewportHeight)`. Nav mode (hunk vs line), search hits, and yank targets are all computed inside navigator.
- **Benefits:** Locality: diff navigation bugs live in one module. Leverage: the navigator can be tested purely (no lipgloss, no bubbles). Adding a new nav mode (e.g., word-diff) means extending navigator, not hunting across four files.

---

## 5. `app/` tab management is a shallow pass-through: no real seam between routing and view lifecycle

- **Files:** `ui/app/model.go`, `ui/app/model_tabs.go`, `ui/nav/nav.go`
- **Problem:** `app.Model` holds a `history []pageState` and a `tabs map[RouteKind]tabPageState`. Tab switching, history push/pop, and page activation are mixed in `model_tabs.go`. The `pageActivationAware` interface is the only seam (2 lines). If you want to understand what happens on a Back navigation, you read `model_tabs.go:handleBack` → `pageState` fields → `loadPage` → then diverge into view-specific initialization. The routing is real logic, but it's not deep: callers (nav messages) must know about Push/Replace/Back at the message level while the tab lifecycle details are exposed as fields.
- **Solution:** Extract a `ui/router` module with a clean `Router` struct: `Push(route)`, `Back()`, `ActiveView()`. It owns the history stack and view cache, emitting lifecycle events (activated, deactivated) that views subscribe to. `app.Model` becomes a coordinator that forwards tea messages to the active view and displays the active view's output.
- **Benefits:** Locality: routing bugs (history corruption, wrong view activated) live in router. Leverage: views don't need to know they're being cached — they just implement `Activate()/Deactivate()`. Tests: router is testable without rendering anything.

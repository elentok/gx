# Splitting the ui packages

Ticket: `gx:split-server-pkg/12`. Question: how do `ui/tickets`, `ui/status`, `ui/worktrees`, `ui/agents` and `ui` (root) split into packages under the agreed limits, combined with the model splits in `split-large-types.md` so each package is split once? What is the import direction and where are the cycle risks?

Limits: 50 files (tests included), 4,500 source lines, 5,000 test lines, 180 `Test` funcs, 1,000 lines per file, 50 methods per type.

Method: the code itself. File, line and `Test` counts come from `go/ast` over one directory (raw `\n` counts, top-level `func Test...` only, like `split-size-baseline.md`). The file-to-file reference graph comes from `go/types` (every identifier use that resolves to a declaration in another file of the same package). Exported-symbol use from outside comes from `go/ast` over the whole repo. `go list` gave the import lists. `gopls implementation` and `gopls references` checked who implements and uses `ServerAPI` and `ServerLink`. File-to-package assignments below are whole files. A file that holds both a child and glue for a model is counted where most of it goes, so line numbers are within a few percent. Test counts that say "drive" are name-based (the test calls `Update`, `New...` or builds a `Model`), so they are upper bounds. Method counts are the plus or minus 5 numbers from `split-large-types.md`.

Read against `main` at `e039fe8e8`.

## Summary

| Package | Now (files / src / test lines / tests) | Over the limit | Proposed |
| --- | --- | --- | --- |
| `ui/tickets` | 114 / 8,764 / 12,293 / 434 | files, src, test lines, tests, 2 test files over 1000 | 4 required packages plus root, 3 optional |
| `ui/status` | 49 / 3,968 / 5,273 / 181 | test lines, tests, `model_test.go` 3,154 | 2 required packages plus root, 2 optional |
| `ui/worktrees` | 51 / 3,813 / 2,327 / 104 | files (by 1) | 2 required packages plus root, 1 optional |
| `ui/agents` | 33 / 3,668 / 3,902 / 181 | tests (by 1) | 4 required packages plus root (one is a small prerequisite leaf) |
| `ui` (root) | 57 / 2,326 / 1,538 / 98 | files | 4 small packages out, 3 optional |

- Every proposed package is a child that does not import its parent. The parent imports the child. The children take plain inputs and return a `Result`, which is the same rule `split-large-types.md` sets for the model children.
- Every package ends under all five count limits with room to spare (tightest: `ui/tickets` root at 4,669 of 5,000 test lines before the optional `sidebar` cut).
- `ui/tickets` is the only package that must be split by package just to meet the limits on source lines. The other four are over by a small margin on files or tests, but their model splits add files, so they get child packages too.
- The `herdr` import guard (`agentrunner/imports_test.go`), `envguard`, `oldwords` and `sleepguard` all list these packages or their files by path. Moves must edit those lists. See "Guard tests that name these paths".
- Public surface stays small. `ui/app` and `ui/agents` use 20 exported names from `ui/tickets`, 1 from `ui/status`, 1 from `ui/worktrees`, and `ui/app` uses 4 methods of `ui/agents.Model`. Only the `ui/tickets` names move (about 60 references).

## What is already true (so the plan can lean on it)

1. All five packages are one flat package of mostly `Model` methods. The file graph is a mesh around `Model`/`QueueModel`: in `ui/status` 33 of 36 source files reference `model_state.go`; in `ui/tickets` 33 of 54 reference `model.go` or `queue.go`. No package split works before the type split. Children must stop reading `Model` fields first. That order is in "Order of work".
2. Leaf files exist already. They are pure functions or small types with no `Model` reference. These move first: `ui/tickets` has 6 (`metrics_format`, `pause_labels`, `pause_reason`, `ticket_status`, `live_row`, `tree_icon_click`), `ui/worktrees` has 4 (`kitty`, `herdr`, `tmux`, `lazygit`), `ui` has 8 groups (below).
3. Public surfaces are narrow. `ui/app` calls `ticketsui.NewModel`, `NewQueueModel`, `statusui.NewModel`, `worktrees.NewWithSettings` and a handful of `ui/agents.Model` methods. The 20 `ui/tickets` names used from outside:
   - Types and funcs: `Model`, `QueueModel`, `NewModel`, `NewQueueModel`, `ServerAPI`, `ServerLink` (+ `ServerLinkUp`, `ServerLinkDown`, `ServerLinkReadOnly`), `QueueEpic`, `QueueStatus`, `QueueStatusOf`, `QueuedToast`, `ScratchDir`, `WatchStore`.
   - Messages: `ProbeRequestedMsg`, `ResnapshotRequestedMsg`, `StoreChangedMsg`, `StartSessionMsg`, `GoToSessionMsg`.
   - `ui/agents` imports `ui/tickets` only for `ServerLink`, `ServerLinkDown`, `QueueEpic` and `QueuedToast` (8 uses in `model.go`, `actions.go`, `attach.go`). So the agents tab depends on the whole tickets tab for four names.
4. `ServerAPI` (25 methods) is implemented by `apiclient.Client`, by the snapshot wrapper `iterationsSnapshot` (`server_answer.go:25`), by test fakes in `ui/tickets/server_mode_test.go` and by two `ui/app` types (`gopls implementation`). It is the contract the `apiclient` split will reshape (`split-large-types.md`: it splits into Queue, Tickets and Reads interfaces). Whatever package owns it takes that change.

## 1. ui/tickets

### Now

| Measure | Value | Limit |
| --- | --- | --- |
| Files | 114 (54 src, 60 test) | 50 |
| Source lines | 8,764 | 4,500 |
| Test lines | 12,293 | 5,000 |
| `Test` funcs | 434 | 180 |
| Files over 1000 | `queue_test.go` 1,867, `model_test.go` 1,387 | 1,000 |
| Methods | `QueueModel` 95, `Model` 90 | 50 |

No source file is over 1000 (largest: `queue.go` 761, `model.go` 636). 54 source files is already more than the whole file limit, so this package needs at least three children before the type split.

### Proposed packages

| Package | What goes in | Files | Src | Test lines | Tests |
| --- | --- | --- | --- | --- | --- |
| `ui/tickets/rowfmt` | `metrics_format`, `pause_labels`, `pause_reason`, `ticket_status`, `live_row`, `tree_icon_click` and their 7 test files | 13 | 391 | 333 | 14 |
| `ui/tickets/preview` | `previewPane` from `split-large-types.md`: `preview`, `preview_focus`, `preview_search`, `glamour`, `preview_frontmatter` and tests | 10 | 850 | 782 | 32 |
| `ui/tickets/rowactions` | `rowMenus` + `rowEditor` + the actions behind menu picks: `ticket_menu`, `epic_menu`, `status_menu`, `actions_menu`, `menu_dispatch`, `suggested_actions`, `server_status_menu`, `recovery_items`, `start_session`, `investigate`, `epic_merge`, `server_answer`, plus `watch_agent` and `transcript` | 26 | 1,857 | 2,261 | 84 |
| `ui/tickets/queue` | `QueueModel` and its children `queueRows`, `queueServer`, `serverKeys`, `harnessPicker`, `queueHeader`: all `queue_*.go`, `server_queue_keys`, the queue half of `hover` | 32 | 3,033 | 4,248 | 150 |
| `ui/tickets` (root) | `Model`, `serverBinding`, the sidebar, search, view, root `hover`, `server_mode`/`server_link`/`server_refresh` Model methods | 33 | 2,633 | 4,669 | 154 |

The five rows add up to the current totals (114 files, 8,764 src, 434 tests; test lines 12,293). `rowfmt` + `preview` + `rowactions` + `queue` + root are the minimum: remove any one and a limit fails (root without `rowactions` is 4,490 src, 6,930 test lines and 238 tests; without `queue` it is 5,666 src lines).

Three more packages are not needed for the limits. They give headroom or cut an import edge:

| Optional package | Why | Cut |
| --- | --- | --- |
| `ui/tickets/tabapi` (recommended) | The 20 names that `ui/app` and `ui/agents` use, minus `Model`/`QueueModel`/`New*`: `ServerAPI`, `ServerLink*`, the 5 messages, `QueueEpic`, `QueueAddFunc`, `QueueAddresses`, `QueueStatus*`, `QueuedToast`, `ScratchDir`, `WatchStore`. About 250 lines in 6 to 8 small files, taken from `server_mode`, `server_link`, `queue_epic`, `queue_status`, `server_refresh`, `start_session`, `epic_menu`, `queue_server_down`. Leaf. | Removes `ui/agents` -> whole tickets tab. About 60 references in 20 files change (`ticketsui.ServerLinkUp` -> `tabapi.ServerLinkUp`). Gives the `apiclient` split one place to put the `Queue`/`Tickets`/`Reads` interfaces. |
| `ui/tickets/sidebar` (recommended) | The 22-method sidebar from `split-large-types.md`: `model_data.go` 506 lines plus the tree parts of `model.go`; with `model_data_test`, `list_scrollbar_test` and about half of `model_test.go`. | Root drops to about 3,500 test lines and about 120 tests. Root is at 93% of the test-line limit without it. |
| `ui/tickets/queue/header` | `queueHeader` is view only (14 methods, `queue_header.go` 456 lines, `queue_header_test.go` 467 lines, 20 tests). It takes a value-typed input and has no state. | `queue` drops to about 2,500 src, 3,800 test lines, 130 tests. Not needed. |

### Import direction

Arrows point at the dependency. Nothing points up.

```
ui (root)  <-  rowfmt                       (leaf: ui, tickets, tickets/schema, tree)
ui (root)  <-  tabapi                       (leaf: server, tickets, viewmodel)
rowfmt     <-  preview
rowfmt, tabapi, preview  <-  rowactions
rowfmt, tabapi, preview, rowactions  <-  queue
rowfmt, tabapi, preview, rowactions, [sidebar]  <-  ui/tickets (root)
ui/tickets (root), queue, tabapi  <-  ui/app
tabapi  <-  ui/agents          (replaces the import of ui/tickets)
```

`queue` and the tickets root do not import each other. They share only children. This is why `QueueModel` moves down to a sibling package instead of staying in the root: if it stayed, the root would import its own children and `queue` would need the root's types.

### Cycle risks

1. `menu_dispatch`, `ticket_menu`, `watch_agent` and `transcript` reference each other (`menu_dispatch -> watch_agent: actionGoToPane, actionWatch, agentAddress, cmdWatchAgent`; `watch_agent -> menu_dispatch: serverNotRunning`; `watch_agent -> ticket_menu: noAgentLog`; `transcript -> watch_agent` and back through `menu_dispatch`). Keep them in one package (`rowactions`). Splitting the watch modal out would need `serverNotRunning` and `noAgentLog` moved to a lower leaf first. That is why `watchagent` is not a package here.
2. `ticket_menu.go`, `epic_menu.go`, `queue_ticket_menu.go` and `suggested_actions.go` read `Model` and `QueueModel` fields (`ticketMenuInput.onQueue`, `m.ticketMenu`). In `rowactions` they take `ticketMenuInput` and return a `Result`. If any menu file keeps a `Model` reference, `rowactions` imports the root and the root imports `rowactions`: a cycle. Check with `go list -deps` after every file move.
3. `epic_menu.go -> preview.go: epicTicketMDPath`. `rowactions` imports `preview`. `preview` must not import `rowactions`. `preview.go` today also calls `Model` methods (`model_data`, `model_keys`, `view.go`); those glue methods stay in the root (`model_preview_focus.go`, `queue_preview.go`).
4. `live_row.go -> view.go`, `queue_header.go -> live_row.go`. `live_row` must move with the symbols it needs from `view.go` (one helper), or take it as a parameter.
5. `server_mode.go` mixes the `ServerAPI` interface and 170 lines of `Model` methods. Split the file first: the interface to `tabapi`, the methods to `serverBinding`. `queue_server_down.go`, `queue_server_live.go` and `server_queue_keys.go` already reference both `server_mode.go` and `queue.go`, so they only compile in `queue` once `ServerAPI` lives in `tabapi`.
6. Shared test helpers. `writeTicket` (used in about 20 test files), `deliverLoad` (13), `loadQueueModel` (10), `writeMap`, `addStoreProject`, `expandOpenEpics` live in `model_test.go` and `queue_test.go`. Test files in other packages cannot see them. Put them in a non-test package under `ui/tickets/internal/tickettest` (importable by all children) before moving tests. Without that, helpers get copied.

### Files over 1000 lines

- `queue_test.go` (1,867, 50 tests) goes with `queue`. Natural cut by theme (from the test names): row tree and ordering (~560 lines), row render (~190), scroll, hover and mouse (~480), search (~210), edit chord (~65), header and clear keys (~130). Five files, each under 700.
- `model_test.go` (1,387, 36 tests) stays in the root, or splits with `sidebar`: sections and rendering (~560), selection and nav (~450), hide-done chord (~170), archived section (~180).

### Method counts

Same targets as `split-large-types.md`: `Model` ~34, `QueueModel` ~28. Children and their package: `rowMenus` ~14, `rowEditor` ~5 (both `rowactions`); `previewPane` ~9 (`preview`); `queueRows` ~18, `queueServer` ~9, `serverKeys` 7, `harnessPicker` 5, `queueHeader` 14 (`queue`); `sidebar` ~22 and `serverBinding` ~14 (root or `sidebar`). All under 50.

### Other risks

- No teatest or golden files in `ui/tickets`. Same as `split-large-types.md`: snapshot `View()` for a few states before moving render code.
- `agentrunner/imports_test.go` lists `ui/tickets` as an allowed importer of `herdr`. The only importing file is `server_answer.go` (`herdr.TabFocus`). In `rowactions` the entry is replaced by `ui/tickets/rowactions`. The list stays the same length ("shrink, never grow").
- `oldwords_test.go` allows `needsrepair` in `ui/tickets/pause_labels.go` and `ui/tickets/preview.go`. Both paths change.
- `QueueModel` has `entriesCache` as a pointer on purpose (`queue.go:81`). It stays one in `queue`.

## 2. ui/status

### Now

| Measure | Value | Limit |
| --- | --- | --- |
| Files | 49 (36 src, 13 test) | 50 |
| Source lines | 3,968 | 4,500 |
| Test lines | 5,273 | 5,000 |
| `Test` funcs | 181 | 180 |
| Files over 1000 | `model_test.go` 3,154 (106 tests) | 1,000 |
| Methods | `Model` 153 | 50 |

`ui/status` is the only package of the five where a test file is far past the line limit. The source side is fine today, but the model split adds children.

### Proposed packages

| Package | What goes in | Files | Src | Test lines | Tests | Required |
| --- | --- | --- | --- | --- | --- | --- |
| `ui/gitops` (new, shared with `ui/log`) | `gitops` from `split-large-types.md`: `model_bump`, `model_push`, `model_pull`, `model_stash` glue (4 files, 116 lines now) | 6 | 116 | 252 | 8 | yes |
| `ui/status/stagemodal` | `stageModals` and `stageActionRunner`, `stageActionKind`, `stageActionResult`, `stageConfirmAction`: `actions.go` (28 funcs, 423 lines) | 3 | 423 | 345 | 19 | yes |
| `ui/status/diffarea` (exists, grows) | The diff side: `diff_view`, `diff_state`, `diff_search`, `diff_keys`, `diff_runtime`, `diff_helpers`, `diff_types`, `search_counter`, `model_render_mode`, `model_diff_context`, `yank_diff`, `comment` (`diffLoader` + `diffActions`). Today it has 183 lines and 13 tests | 13 more | 1,008 more | 120 more | 6 more | no |
| `ui/status/filetree` | `filetreePane`: `filetree_actions`, `filetree_keys`, `filetree_nav`, `filetree_runtime`, `filetree_state`, `tree` (the confirm builders stay in the root, per `split-large-types.md`) | 7 | 685 | 94 | 3 | no |
| `ui/status` (root) | `Model`, routing, layout, `view*`, `image_diff` hooks, `e2e_test` | 20 | 1,736 | 4,462 | 145 | |

Why only two are required: after `gitops` and `stagemodal` leave with their tests, the root is 145 tests and 4,462 test lines, under both limits. The other two moves are the larger part of the model split, but they do not change the count limits. They are the "diff pane 26 + staging 18 + yank 7" and "filetree 29" tests below.

Root without the optional two (the `diff*` and `filetree*` files stay): 40 files, 3,429 source lines, 4,676 test lines, 154 tests. With all four moves: 20 files, 1,736 source lines, 4,462 test lines, 145 tests.

### What must be rewritten when tests move

126 of the 160 tests in the internal test files drive a whole `Model` (name-based: `newTestModel`, `Update`, `NewModel`). `actions_test.go` (9 of 9) and `status_page_actions_test.go` (7 of 10) open confirms by sending keys to `Model` and read `m.confirmOpen`, `m.runningContent`. When `stageModals` is its own package, those tests must be re-expressed against the modal's own `Update` and result, or they stay in the root and the root stays over 5,000 test lines. Plan for a rewrite of about 19 tests, not a move.

### model_test.go by theme (name-based)

| Theme | Tests | Lines | Destination |
| --- | --- | --- | --- |
| filetree (selection, icons, branch summary, collapse) | 29 | ~630 | `model_filetree_test.go` (root) or `filetree` |
| diff pane (render, side by side, hunk, wrap, binary) | 26 | ~830 | `model_diffpane_test.go` or `diffarea` |
| staging (line, visual, discard, unstage, apply selection) | 18 | ~600 | `model_staging_test.go` |
| yank | 7 | ~220 | with search below |
| search | 5 | ~200 | `model_search_yank_test.go` |
| layout, nav, mouse, shell keys | 21 | ~530 | `model_layout_test.go`, `model_keys_test.go` |

Seven or eight files, all under 900 lines. That is the fix for the 3,154-line file whatever happens to the packages. The shared helpers (`newTestModel`, `newTestModelDefault`, `pressKey`, `runStatusCmd`) go to one `helpers_test.go`.

### Import direction

```
ui, git, diffview, components, ...  <-  ui/gitops           (leaf: bump, push, pull, stash, output)
ui, git, components                 <-  stagemodal          (leaf)
diffview, keys                      <-  diffarea            (already the case)
ui, git, tree                       <-  filetree
gitops, stagemodal, diffarea, filetree  <-  ui/status (root)  <-  ui/app
```

### Cycle risks

1. The `Model` reference mesh. `actions.go` references `filetree_actions.go`, `model_state.go`, `view_chrome.go`; `diff_state.go` references `actions.go`, `filetree_runtime.go`, `model_runtime.go`, `view.go`. Each child must take its data as a parameter. `confirmAccept` (in `filetree_actions.go`) mixes modal and worktree state. It stays in the root and calls `stagemodal`; it does not move.
2. `diffarea` grows from 183 to about 1,200 lines. It must not import `ui/status`. Its inputs are the selection (`statusDiffSelection`, `statusDiffFileSelection` from `diff_helpers.go`) and render settings. Those two small types go with it.
3. `ModalOpen()` must keep the same set: `runningOpen`, `confirmOpen`, `errorOpen`, `credentialOpen`, bump, push, pull, stash, output, help. `Update` compares it before and after to mark the image overlay dirty (`model_update.go:31-60`). Missing one breaks image placement, and only a pixel test shows it.
4. `ui/gitops` is imported by `ui/status` and `ui/log` and imports neither.
5. `envguard_test.go` has `ui/status/model_runtime.go` (1 child-env use). That file stays in the root, so no change, unless `model_runtime.go` is split.

## 3. ui/worktrees

### Now

| Measure | Value | Limit |
| --- | --- | --- |
| Files | 51 (35 src, 16 test) | 50 |
| Source lines | 3,813 | 4,500 |
| Test lines | 2,327 | 5,000 |
| `Test` funcs | 104 | 180 |
| Files over 1000 | none (largest `model_update.go` 502) | 1,000 |
| Methods | `Model` 89 | 50 |

Only the file count is over, by one. But the `modal` interface in `split-large-types.md` makes 11 modal types. One file each would be 62 files in a single package. So the split by package is part of doing that plan.

### Proposed packages

| Package | What goes in | Files | Src | Test lines | Tests | Required |
| --- | --- | --- | --- | --- | --- | --- |
| `ui/worktrees/modal` | The `modal` interface and one type per mode: `model_confirm_modal`, `model_credential_prompt`, `model_error_modal`, `model_logs_modal`, `model_push_diverged`, `terminal_menu`, `model_help`, `search`, `rename`, `clone`, `worktree_create`, `paste`, `yank`, `delete` (progress part). Tests: help, paste, output-key | 17 | 1,474 | 433 | 16 | yes |
| `ui/worktrees/launch` | Terminal and editor openers: `kitty`, `herdr`, `tmux`, `lazygit`, `kitty_test` | 5 | 116 | 41 | 4 | yes |
| `ui/worktrees/tableview` | Table rendering and the preview text: `table`, `preview`, `icons`, with `preview_test`, `model_header_test`, `dirty_test` | 6 | 469 | 412 | 22 | no |
| `ui/worktrees` (root) | `Model`, `Update`, key and mouse dispatch, layout, `view`, the job commands (`pullpush`, `remoteupdate`, `track`, `model_commands`, `model_messages`), the e2e test | 23 | 1,754 | 1,441 | 62 | |

(Root combines the `ops` files, 311 lines, with the 1,443 lines of the model files. Without `tableview` the root has 3 more source files and 3 more test files: 29 files.)

`launch` is required because the `herdr` import and four `envguard` rules key on those files. It is the only part of the package that touches the process environment, so it is also the natural place to put the guard entries. `modal` is required because the model split adds files.

### Import direction

```
ui, git, herdr   <-  launch      (leaf)
ui, git, components, confirm, help, search, pull  <-  modal   (leaf; modals do not own worktrees, repo or spinner)
ui, git          <-  tableview
modal, launch, tableview  <-  ui/worktrees (root)  <-  ui/app
```

### Cycle risks

1. `modal` needs `Result` types (`modalResult`: command, close, show error, reload, start job). The root applies them. A modal must not call `Model` methods or read `repo`/`worktrees`. Today `model_keys.go` references 19 other files and `model_update.go` references 27. The `confirm.Model` stays a field in the root, because a confirm can open over another modal (`model_update.go:62`).
2. `delete.go` and `paste.go`/`yank.go` mix command builders and `Model` methods (`enterDeleteConfirm`, `handleDeleteProgressResult`). Only the progress state and view move; the command builders stay.
3. `jobRunner`, `jobKind`, `jobWorktree` and `finishPromptableJob` are read by both the credential modal and the logs modal. They live in a `jobs` type in `modal` (or a small `jobs.go` in `modal`), not in the root, or `modal` imports the root.
4. `terminal_menu.go` calls `herdr.go`, `kitty.go`, `tmux.go`. After the move `modal` imports `launch`. That is the one new edge between children; it points in the right direction.
5. `previewBase`, `renderPreviewContent` and `icons.go` (`uiIcons`) are used by the table and the preview. Keep them together in `tableview`.

### Guard tests that name these paths

- `agentrunner/imports_test.go`: replace `ui/worktrees` with `ui/worktrees/launch` in `herdrImporters`.
- `envguard_test.go` lines 76 to 79: `ui/worktrees/kitty.go` (2 rules), `lazygit.go`, `tmux.go` become `ui/worktrees/launch/...`.

## 4. ui/agents

### Now

| Measure | Value | Limit |
| --- | --- | --- |
| Files | 33 (15 src, 18 test) | 50 |
| Source lines | 3,668 | 4,500 |
| Test lines | 3,902 | 5,000 |
| `Test` funcs | 181 | 180 |
| Files over 1000 | none (largest `create.go` 609) | 1,000 |
| Methods | `Model` 123 | 50 |

### Proposed packages

| Package | What goes in | Files | Src | Test lines | Tests | Required |
| --- | --- | --- | --- | --- | --- | --- |
| `ui/agents/terminal` | `attachment` + `attachWriter` + `copyMode` + `terminalView`: `attach`, `attachwriter`, `copymode`, `terminal`; tests `attach_test`, `copymode_test`, `terminal_test`, `scroll_test` | 8 | 1,139 | 1,158 | 45 | yes |
| `ui/agents/create` | `createForm`: `create.go`, `create_test.go` | 2 | 609 | 320 | 17 | yes |
| `ui/agents/jump` | `jumpPopup`: `jump.go`, `jump_test.go` | 2 | 168 | 165 | 10 | yes |
| `ui/agents/agentstyle` | The shared style and label helpers now in `sidebar.go` and `terminal.go`: `fit`, `statusIcon`, `statusLabel`, `statusDetail`, `shortDir`, and the `style*` variables (26 declarations). About 150 lines. | 2 to 3 | ~150 | ~0 | 0 | yes (prerequisite) |
| `ui/agents` (root) | `Model`, leader keys, `sidebar`, `groups`, `hover`, `sessionActions`, `afk`, `focus`, `client` | 21 | 1,752 | 2,259 | 109 | |

(The 150 lines of `agentstyle` come out of `sidebar.go` and `terminal.go`, so the three child rows sum to 1,916 src and root drops to about 1,600 once they leave. The rows above are whole-file counts: root 1,752 + terminal 1,139 + create 609 + jump 168 = 3,668.)

`sidebar` stays in the root. It holds `sessions` and `cursor`, which almost every other group reads (`split-large-types.md`). Making it a package is optional and would need those two fields passed as input.

### Import direction

```
ui, server, nativerunner, agentrunner/screen   <-  terminal
ui, config, git, server                        <-  agentstyle (leaf)
agentstyle, SessionClient interface            <-  create, jump
terminal, create, jump, agentstyle, tabapi     <-  ui/agents (root)  <-  ui/app
```

`SessionClient` (`client.go`, 12 methods) stays a small interface in the root; `create` takes the one method it needs (`CreateSession`) as a func or a one-method interface. With `apiclient` sub-clients (`split-large-types.md`) this becomes the `Sessions` interface.

### Cycle risks

1. `attach.go` and `copymode.go` reference each other (`copymode -> attach: AttachStream, Copy, Wheel, ... 34 uses`; `attach -> copymode: writeCopied`; `terminal -> attach, copymode`). They are one cluster: keep all four files in `terminal`. `copyMode` takes a `screenSource` interface instead of `*attachment`, as in `split-large-types.md`.
2. `create.go` and `jump.go` use `styleCrashed`, `styleProjectHeader`, `styleTicket`, `fit`, `statusIcon`, `styleNameRead`, `shortDir`, `statusDetail`, `statusLabel` from `sidebar.go` and `terminal.go`. Without `agentstyle` these two children import the root (cycle) or `terminal` (wrong direction). `agentstyle` is the prerequisite and has no other dependency.
3. `actions.go` references `attach.go` (`attachment`, `closed`, `id`, `readOnly`). It stays in the root and uses `terminal`'s exported accessors.
4. `leader.go` reaches 9 of the 15 source files (`runLeaderAction` calls 16 methods). It is the glue and stays in the root. Keep its priority chain (`leader.go:92-128`) unchanged.
5. `sleepguard_test.go` lists `ui/agents/attach_test.go`, `fake_client_test.go`, `focus_test.go` by path. `attach_test.go` moves; `fake_client_test.go` is used by `actions_test.go` and `create_test.go` and may need a copy or an `internal/agentfake`.
6. `ui/agents` imports `ui/tickets` for four names today. With `tabapi` that edge becomes `ui/agents -> ui/tickets/tabapi` (leaf). Without it, the agents root still imports the tickets root, which now imports five children. No cycle, but a heavy edge (see the optional `tabapi` above).
7. 12 golden files in `testdata` cover sidebar, terminal and jump rendering. They must stay byte-identical with no `-update`. That is the free equivalence check for these moves.

## 5. ui (root)

### Now

| Measure | Value | Limit |
| --- | --- | --- |
| Files | 57 (32 src, 25 test) | 50 |
| Source lines | 2,326 | 4,500 |
| Test lines | 1,538 | 5,000 |
| `Test` funcs | 98 | 180 |

Only files are over, by 7. The package is a grab bag of three things: design tokens and render helpers (`styles`, `frame`, `badge`, `overlay`, `icons`, `scrollbar`, ...), domain helpers (`refrules`, `pushstate`, `command_output`, `fuzzy`) and settings (`settings.go` imports `ralphloop` and `config`). 224 files in 40 packages import `ui`. Splitting the render kit would be a very large churn for no limit gain, so the plan moves out only the domain helpers.

### Proposed packages

All four are small leaves that import `ui` and are not imported by it. Together they remove 10 files and bring the root to 47.

| Package | Files | Src | Test lines | Tests | Files that use it today |
| --- | --- | --- | --- | --- | --- |
| `ui/fuzzy` | `fuzzy.go`, `fuzzy_score.go` + 2 tests (`FuzzyMatch`, `FuzzyScore`, `HighlightMatches`) | 141 | 90 | 5 | 5 (`agents/create`, `agents/jump`, `components` x2, `contextmenu`) |
| `ui/refrules` | `refrules.go`, `color_resolve.go` + test (`CompileRefRules`, `MatchRefRule`, `SortDecorations`, `IsHiddenRef`, `ResolveColor`) | 173 | 57 | 5 | 3 (`commit/model`, `commit/view`, `log/ref_rules`) |
| `ui/cmdoutput` | `command_output.go` + test (`CommandOutput*`) | 138 | 96 | 9 | 8 (`pull`, `push`, `status/actions`, `worktrees` x3 and 2 tests) |
| `ui/pushstate` | `pushstate.go` (`PushState`, `CommitPushState`, 4 styles) | 41 | 0 | 0 | 7 (`commit`, `log` x3, `prs`, 2 tests) |

Root after: 47 files, 1,833 src lines, 1,295 test lines, 79 tests. `fuzzy` and `refrules` alone give exactly 50 files, the limit, so all four are needed for headroom.

Optional cuts, bigger churn, no limit gain:

| Package | What | Churn | Gain |
| --- | --- | --- | --- |
| `ui/settings` | `settings.go` + test (122 lines, `Settings`, `AboutLine`) | 78 files in 10 packages | Removes the `ralphloop` and `config` imports from the root of the whole ui tree. Every ui package currently compiles `ralphloop` through `ui`. |
| `ui/term` | `terminal.go` + test (`Terminal`, `DetectTerminal*`) | 23 files in 15 packages (`settings.go` calls `terminal.go`) | Moves the only OS probing out of the render kit. |
| `ui/relativetime`, `ui/harnesslabel` | `relative_time.go`, `harness_label.go` | 10 and 4 files | Neat but small. |

Move `settings` with `term` or move neither: `settings.go -> terminal.go` is the only edge between them, and `ui/settings` importing `ui` for `Terminal` would work, but `ui` importing `ui/settings` would not.

### Import direction

```
ui (root)  <-  fuzzy, refrules, cmdoutput, pushstate
ui and the four  <-  the tab and component packages that use them
config, git  <-  refrules, pushstate   (these two imports leave the root with them)
```

### Cycle risks

1. `fuzzy_score.go` uses `StyleFilterMatch` from `styles.go`. `refrules.go` uses `ResolveColor` (from `color_resolve.go`, which uses `styles.go`). `pushstate.go` uses the status styles. These stay one-way: the new packages import `ui`. The root must never import them back. The root has no reference to any of the four today (`go/types` check).
2. `envguard_test.go` keys `ui/openurl.go` by path. `openurl.go` stays, so no change.
3. `.ai/index.md` says "`ui/...` - TUI (`ui/app` is the root model; one package per tab or component)". Add one line that `ui` itself is the render kit and tokens, and that helpers with a domain (refs, push state, command output) get their own package.

## Combined view with split-large-types.md

Each package is split once. The model splits from `split-large-types.md` fall into the packages above like this:

| Model | Children from `split-large-types.md` | Where they land |
| --- | --- | --- |
| `status.Model` 153 | `stageModals`, `gitops`, `filetreePane`, `diffarea`+`diffLoader`+`diffActions` | `stagemodal`, `ui/gitops`, `filetree`, `diffarea` |
| `agents.Model` 123 | `copyMode`, `attachment`, `createForm`, `jumpPopup`, `sidebar`, `sessionActions`, `terminalView` | `terminal` (3), `create`, `jump`; `sidebar` and `sessionActions` stay in the root |
| `tickets.QueueModel` 95 | `queueHeader`, `queueRows`, `queueServer`, `serverKeys`, `harnessPicker`, shared menus/preview/editor | `queue` (own children), shared in `rowactions`, `preview` |
| `tickets.Model` 90 | `sidebar`, `serverBinding`, shared menus/preview/editor | root or `sidebar`; shared in `rowactions`, `preview` |
| `worktrees.Model` 89 | `modal` interface + 11 types, `jobs` | `modal` |

The shared children (`rowMenus`, `previewPane`, `rowEditor`) are built once in `rowactions` and `preview`. That matches the order in `split-large-types.md` ("shared children first").

## Guard tests that name these paths

These tests list packages or files by path. A move that forgets them fails the test, and the failure is not obvious from the move.

| Test | Entry | Change |
| --- | --- | --- |
| `agentrunner/imports_test.go` | `ui/tickets`, `ui/worktrees` as `herdr` importers | Replace with the one child that holds the `herdr` call (`rowactions`, `launch`). Do not add. |
| `envguard/envguard_test.go` | `ui/status/model_runtime.go`, `ui/worktrees/kitty.go` (x2), `lazygit.go`, `tmux.go`, `ui/openurl.go` | `kitty.go`, `lazygit.go`, `tmux.go` move to `launch/`. The others stay. |
| `oldwords/oldwords_test.go` | `ui/tickets/pause_labels.go`, `ui/tickets/preview.go` | `rowfmt/pause_labels.go`, `preview/preview.go` |
| `sleepguard/sleepguard_test.go` | `ui/agents/attach_test.go`, `fake_client_test.go`, `focus_test.go` | `attach_test.go` moves to `terminal/` |
| `ui/status/model_test.go:315` | writes a fixture file named `ui/status/model.go` into a temp repo | Not a source path. No change. |

## Order of work

Each step is a move with no behavior change, run with that package's tests, and goldens untouched.

1. Leaves that need no type split: the four `ui` root packages, `ui/worktrees/launch`, `ui/tickets/rowfmt`, `ui/agents/agentstyle`. Update the guard lists with each.
2. `tickettest` helpers, then split `queue_test.go` and `model_test.go` (tickets) and `model_test.go` (status) by theme inside the package. This fixes the over-1000 test files with no behavior risk.
3. The shared children, as in `split-large-types.md`: `ui/gitops`, `preview`, `rowactions`.
4. One model per ticket, smallest risk first, with the package move after the in-package type split: `ui/agents` (`terminal`, `create`, `jump`), `ui/worktrees` (`modal`, then `tableview`), `ui/status` (`stagemodal`, then `diffarea`, `filetree`), then `ui/tickets` (`queue`, then the root, then `sidebar`, `tabapi`).
5. `tabapi` after the `apiclient` split lands, so `ServerAPI` is reshaped once, not twice.

## Open questions for a person

1. Is the `ui/tickets/tabapi` cut wanted? It removes `ui/agents -> ui/tickets` and costs about 60 edits. It is the one place where the package split changes call sites outside these five packages.
2. `ui/status` and `ui/agents`: are test rewrites acceptable? About 19 `ui/status` tests (`stagemodal`) and some `agents` and `tickets` tests drive the whole model and cannot move as-is. If not, the root stays over the test limits and an exception list entry is needed.
3. Should `ui/settings` and `ui/term` move in this epic (78 and 23 files), or be filed as a follow-up? They do not affect any limit.
4. Does the guardrail count tests in `internal/tickettest` helper packages against the limits? They are non-test files, so they count as source.

# Status Nested Models Refactor Plan

## Goal

Refactor `ui/status` to use nested Bubble Tea child models with clear ownership:

1. file tree child model (`ui/filetree/`)
2. unstaged diff child model (`ui/diff/model.go`)
3. staged diff child model (`ui/diff/model.go`)

Each child owns its own `ui/search` model (independent query/cursor/results per view).

## Non-goals

- No user-visible behavior redesign beyond search ownership and display placement.
- No git side effects inside child models.

## Hard constraints

- Parent forwards key events only to the focused child (no key filtering by parent for child concerns).
- Search result counters are rendered by child views (frame right-title), not status footer.
- Search counter style: `{magnifying_glass_icon} {cursor}/{total}`.

## Target architecture

### Parent (`ui/status`)

Owns:

- app-level chrome/modals/help/runners
- global orchestration and focus switching
- git side effects and reload commands
- child model wiring and child intent handling

Does not own:

- filetree selection/search internals
- diff pane navigation/search internals

### Child: filetree (`ui/filetree`)

Owns:

- rows, collapse/expand, selection
- search input/results/jump behavior for list scope
- frame right-title search counter

Emits intent messages:

- `FileSelectedMsg`
- `OpenDiffMsg`
- optional action intent msgs as needed

### Child: diff (`ui/diff/model.go`, two instances)

Owns:

- section render/runtime state (viewport, nav mode, visual mode)
- search input/results/jump behavior for that pane
- frame right-title metadata (scroll %, search counter)

Emits intent messages:

- `ApplySelectionRequestedMsg`
- `DiscardSelectionRequestedMsg`
- `OpenFileRequestedMsg`
- etc.

## Execution plan

1. Extract status filetree into `ui/filetree` model with behavior parity.
2. Introduce `ui/diff/model.go` and migrate unstaged pane first.
3. Migrate staged pane to second `diff.Model` instance.
4. Move search ownership fully into each child model.
5. Route key events from parent to focused child only.
6. Remove footer search status; render counters in frame right-titles.
7. Delete legacy status-level explorer/search state and adapters.
8. Final cleanup and dedupe pass.

## Incremental rollout slices

### Slice A (safe UI ownership prep)

- Move search counter rendering from footer to pane frame right-title.
- Keep existing state backend temporarily.

### Slice B (filetree child extraction)

- Create `ui/filetree` model and migrate status pane logic.
- Parent consumes child msgs for reload/focus transitions.
- Follow-up cleanup in this slice:
  - Move `FileTree*` helper functions from `ui/explorer` into `ui/filetree`.
  - Make helpers operate directly on `[]filetree.Entry[T]`.
  - Remove `filetree -> explorer.FileTreeRow` adapter conversion from `ui/filetree/model.go`.
  - Keep temporary wrappers only during migration; delete wrappers after all call sites are switched.
  - Remove `ui/status/filetree_bridge.go` sync bridging (`syncFileTreeModel`) once `filetree.Model` is the source of truth for:
    - entries
    - collapsed dirs
    - selected index
  - Parent should read child state and orchestrate side effects only (no mirrored filetree state fields in `status.Model`).

### Slice C (diff child extraction)

- Create `ui/diff/model.go`, migrate unstaged then staged.
- Keep side effects in parent via intent msgs.

### Slice D (search split per child)

- Remove parent `search` field.
- Each child owns `search.Model`.
- Parent only routes keys/messages by focus.

## Testing strategy

- `ui/filetree`: selection/collapse/search tests.
- `ui/diff`: nav/visual/search/jump tests.
- `ui/status`: focus routing + child intent integration tests.
- Add explicit tests for independent search state:
  - filetree query unaffected by unstaged/staged query
  - staged/unstaged queries preserved when switching focus
- Add explicit tests for filetree helper ownership migration:
  - `ui/filetree` tests cover expand/collapse/parent/adjacent-file behavior without importing `ui/explorer`.

## API cleanup task

- Audit exported/public functions and types touched by this refactor in `ui/status`, `ui/filetree`, `ui/diff`, and `ui/explorer`.
- For each exported symbol, verify it is used outside its package.
- Convert symbols that are package-local in practice to unexported/private names.
- Keep exported API only where cross-package usage is intentional and documented.

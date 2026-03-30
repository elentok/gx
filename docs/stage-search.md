# Stage Search Plan

## Goal

Add vim-like search in `gx stage` for both status and diff views with two explicit modes:

- search-input mode
- search-results mode

Behavior target (from prompt):

- `/` enters search-input mode
- while typing, matches are highlighted live
- `enter` transitions to search-results mode
- `esc` exits search entirely (clears highlights)
- in search-results mode:
  - `n` jumps next
  - `N` or `p` jumps previous
  - `esc` exits and clears highlights while keeping cursor/focus at current match target

## Scope

- Status pane search over visible status entry labels.
- Diff-pane search over the currently active section (unstaged or staged).
- No cross-pane combined result list (active section only for diff).

## Design

### Search state in stage model

Add model fields:

- `searchMode` (`none`, `input`, `results`)
- `searchScope` (`status`, `diff-unstaged`, `diff-staged`)
- `searchQuery string`
- `searchInput textinput.Model`
- `searchMatches []searchMatch`
- `searchCursor int`

`searchMatch` stores target location:

- status: status entry index
- diff: display index + raw index in the active section

### Entering search

- `/` creates/focuses a textinput and sets scope from current focus/section.
- Existing overlays (running/confirm/error/help) keep priority and block search handling.

### Input mode

- Any typed character updates query.
- Recompute matches immediately.
- Highlight matches live.
- Keep selection anchored to first match when available.
- `enter` -> results mode.
- `esc` -> normal mode and clear search state/highlights.

### Results mode

- `n` -> next result (wrap).
- `N` or `p` -> previous result (wrap).
- `esc` -> clear search state/highlights and return to normal mode.

### Match navigation

- Status match jump updates `selected` and triggers debounced diff reload path.
- Diff match jump scrolls active section viewport so matched line is visible.
- Diff navigation does not switch hunk/line mode; it only positions viewport/cursor context.

### Highlight rendering

- Status rows: inline match highlight in display label.
- Diff rows: line-level highlight marker for matched lines (avoid mutating ANSI-colored tokens in delta output).
- Highlights active only during search-input/results modes.

### Footer text

- Search-input mode: `search: <query> <idx/total>` or `no matches`.
- Search-results mode: `search: <query> <idx/total> · n next · N/p prev · esc clear`.

## Test Plan

### Unit tests (`ui/stage/model_test.go`)

- Enter search with `/` in status and diff focus.
- Typing updates matches and highlights.
- `enter` transitions input -> results mode.
- `n` and `N`/`p` navigate correctly and wrap.
- `esc` clears search state while preserving current selection/viewport position.

### E2E tests (`ui/stage/e2e_test.go`)

- Status flow: `/` type query -> `enter` -> `n`/`p` -> `esc`.
- Diff flow: `/` type query in active section -> `enter` -> `n` -> `esc`.

## Rollout Steps

1. Add state + search handlers.
2. Add match computation/jump helpers.
3. Wire `/` and search mode key routing.
4. Add highlight rendering in status/diff views.
5. Add tests.
6. Run `go test ./ui/stage -count=1` and `go test ./... -count=1`.

# `gx log` / commit view implementation plan

## Goal

Add a unified multi-page TUI flow across worktrees, log, status, and commit:

- `worktrees` remains the entry page.
- `log` shows commit history for the active worktree/branch.
- `commit` shows a status-like diff explorer for a selected commit.
- `status` joins the same shell and navigation model.

The feature should feel like one application, not four disconnected programs.

## Product decisions from `docs/prompts/log.md`

- Add a new `log` page with commit table columns:
  - graph
  - hash
  - relative date
  - author initials
  - subject
  - tags and refs as badges, with distinct colors by kind
- Pressing `enter` on a log row opens the commit page for that commit.
- The commit page should visually match the status page, but without stage/unstage split semantics.
- Commit page needs a commit info frame with hash, subject, full date + relative date, and body.
- `b` toggles commit body collapse/expand.
- `r` opens a chained restore menu with:
  - restore local content to the version after the commit
  - restore local content to the version before the commit
- `d` deletes the selected change from the commit after confirmation.
- `y` opens the shared chained-key menu and adds yank targets for hash, subject, and message.
- Bottom tabs should show `worktrees`, `log`, `status`, with the active tab highlighted.
- Global page jumps:
  - `gw`
  - `gl`
  - `gs`
- In log mode with a custom starting ref, `gh` should reset the log root back to `HEAD`.
- `q` and `esc` should become contextual “back” instead of unconditional quit.
- The new popup-style chained keymap UX should be shared, not worktrees-only.

## What the codebase says now

- `ui/worktrees` and `ui/status` are currently separate Bubble Tea root models launched independently from `cmd/cmd.go`.
- There is no shared router, page shell, or back-stack yet.
- `ui/status` already contains most of the rendering and diff navigation behavior the commit page wants.
- `ui/status` is tightly coupled to live worktree state:
  - file list comes from `git.ListStageFiles`
  - diff content comes from staged/unstaged working-tree diff helpers
  - actions assume `HEAD`-based restore/discard/stage semantics
- `git/log.go` already has basic commit-history helpers, but the current `Commit` type is too small for the requested log page and commit header.
- Shared menu modal plumbing already exists in `ui/components/modal_menu.go`, but chained-key UX is still duplicated in per-page key-prefix logic.

## Architecture recommendation

Do not build this by cloning the status model into a second commit-only model.

The low-risk path is:

1. Introduce a top-level app shell that owns page routing, page history, footer tabs, and global page jumps.
2. Extract a reusable “diff explorer” core out of `ui/status` so both live status and historical commit review can render the same tree/diff interaction model.
3. Add a commit-data source abstraction so the explorer can be backed by:
   - live worktree staged/unstaged state
   - a single commit diff against parent
4. Layer commit-only actions (`restore`, `delete change`, commit-specific yank values) on top of that explorer.

This is more upfront refactor than a copy/paste, but it avoids two diverging implementations of the hardest UI in the repo.

## Root model recommendation

Use `ui/app/` for the new root model.

Why:

- `ui/` already contains multiple peer packages (`status`, `worktrees`, `components`, `menu`, `confirm`).
- The new shell is not a generic helper; it is the application entry TUI that owns routing, tabs, history, and global key handling.
- Putting it in `ui/app/` makes ownership clear:
  - `ui/app` owns top-level navigation and page composition
  - `ui/worktrees`, `ui/log`, `ui/status`, and later `ui/commit` own page-specific behavior
- This also leaves `ui/` itself free of a mixed bag of package-level files.

I do not recommend a flat `ui/root.go` / `ui/model.go` under `ui/`; that will blur the line between reusable UI helpers and the actual app shell quickly.

## Proposed milestones

- [x] Milestone 1: App shell, routing, and shared chained menus
  - Add a new root model under `ui/app/` that hosts child pages.
  - Move app launch in `cmd/cmd.go` to the root shell instead of directly launching worktrees/status models.
  - Define route/state concepts for:
    - worktrees
    - log
    - status
    - commit
  - Define launch intents for:
    - default app entry
    - `gx log [<hash-or-ref>]`
    - `gx show <hash-or-ref>`
    - direct status entry
  - Add navigation helpers:
    - `navigateTo(...)`
    - `navigateBack()`
  - Track route origin/history so `q` / `esc` can go back contextually.
  - Add bottom tab rendering and global `gw` / `gl` / `gs` handling.
  - Promote chained-key popup UX into a shared component/helper used by worktrees first.

- [x] Milestone 2: Git log data model and log page
  - Extend git log plumbing with a richer row model, likely separate from the existing lightweight `git.Commit`:
    - full hash
    - short hash
    - author name
    - author initials
    - subject
    - body
    - authored/committed date
    - refs/tags decorations, preserving ref kind
    - graph text
  - Add helpers to load commit history for:
    - current `HEAD`
    - an arbitrary `<hash-or-ref>` as the starting point
  - `gx log <hash-or-ref>` should start exactly at that resolved commit and walk ancestors from there, not merely select that row inside `HEAD` history.
  - Build `ui/log` with:
    - table rendering
    - search behavior matching worktrees/status
    - enter-to-open-commit
  - Add CLI entrypoints:
    - `gx log`
    - `gx log <hash-or-ref>`
  - In log view, when the shown ref is `HEAD` and the worktree has staged, unstaged, or untracked changes, inject a pseudo-commit row above history:
    - row should visually read as working tree / uncommitted state, not a real commit
    - pressing `enter` on it navigates to status view
  - Add `gh` in log view to reset a custom log root back to `HEAD`.
  - Wire worktrees `enter` to navigate to the selected worktree’s log view instead of opening terminal/lazygit behavior on that path.
  - Keep lazygit log available on its remapped key (`L`) as a separate action.

- [ ] Milestone 3: Extract reusable diff explorer from status
  - Split `ui/status` into:
    - page/shell concerns
    - reusable file-tree + diff-navigation + modal/action scaffolding
  - Introduce an interface or structured callbacks for the explorer data source:
    - list entries
    - load diff sections
    - commit header metadata optional
    - allowed actions
  - Preserve existing status behavior unchanged while moving it onto the shared explorer core.
  - Remove the temporary embedded-shell toggle (`EnableNavigation` or its equivalent) by separating:
    - app-shell navigation ownership
    - standalone page behavior/wrappers
  - This milestone is the real foundation for the commit page; do not skip it.

- [ ] Milestone 4: Commit page backed by historical commit diffs
  - Add git plumbing for commit inspection:
    - commit metadata with message body
    - per-file diff for a commit vs parent
    - support root commits cleanly
    - probably expose both raw unified diff and delta-rendered diff, mirroring status
    - resolve all refs/tags that point at the shown commit
  - Add CLI entrypoint:
    - `gx show <hash-or-ref>`
  - When `<hash-or-ref>` resolves to a branch name, open the commit page on that branch tip commit.
  - Model commit view as a single logical diff section instead of staged/unstaged.
  - Reuse status tree/diff navigation where possible:
    - hunk mode
    - line mode
    - visual selection
    - search
    - fullscreen diff
    - output/help/error modals
  - Add commit header frame and body collapse toggle.
  - Share the `log` tab highlight between log page and commit page.

- [ ] Milestone 5: Commit restore actions and yank extensions
  - Add restore flows for file/hunk/line/selection:
    - restore local content to state after commit
    - restore local content to state before commit
  - Extend yank popup with commit-specific targets.
  - Make action availability explicit:
    - disabled for binary/symlink cases where patch synthesis is unsafe
    - clear status/error copy for unsupported operations

- [ ] Milestone 6: Integration polish and verification
  - Finish contextual back behavior across all page transitions:
    - worktrees -> log -> commit
    - worktrees -> status
    - log -> status if added
  - Reconcile footer/help text per page so global vs local bindings are clear.
  - Add regression coverage for routing, tab state, menus, and commit restore actions.
  - Run targeted tests, then full suite.

- [ ] Milestone 7: Delete-change history rewrite
  - Implement `d` in commit view as actual history rewrite, not local reverse-apply.
  - Decide rewrite strategy explicitly before coding:
    - likely temporary branch / detached HEAD workflow
    - replay rewritten commit range after editing selected commit
  - Define behavior for:
    - target commit is `HEAD`
    - target commit is in the middle of the visible range
    - working tree is dirty
    - conflicts during rewrite
    - user abort / rollback
  - Add confirmation UX that clearly says history will be rewritten.
  - Add focused tests around commit selection, rewrite planning, and failure handling.
  - Run targeted tests, then full suite.

## Milestone details

### Milestone 1 notes

- This is the biggest UX unlock and the biggest architectural change.
- The root shell should own quit behavior; child pages should emit navigation intents instead of calling `tea.Quit` for `q`/`esc`.
- A small message protocol is likely enough:
  - `navigateMsg`
  - `backMsg`
  - `openCommitMsg`
  - `openLogMsg`
  - `openStatusMsg`
- The shell should accept an initial route/intention so `cmd/cmd.go` can launch:
  - default worktrees
  - log at `HEAD`
  - log at `<ref>`
  - show commit at `<ref>`
  - status for current worktree
- The first shell cut used route-push semantics for `gw` / `gl` / `gs`, but that caused repeated page re-creation and made terminal repaint glitches much harder to reason about.
- The better model is persistent cached top-level tabs:
  - `worktrees`
  - `log`
  - `status`
  - with history reserved for deeper pages such as `commit`
- Why we changed it:
  - tab switches should preserve local page state
  - tab switches should not pollute the back-stack
  - recreating child models on every tab hop amplified repaint/layout bugs and made shell behavior harder to reason about
- The shell now owns the `g{w,l,s}` tab chords before child pages see the leading `g`.
- Why we changed that:
  - letting children see the first `g` meant they could briefly enter their own chord-preview state before the shell switched tabs
  - that led to stale footer/chord UI and was one cause of the “line jump” / broken worktrees state during tab switches
- Footer tabs should be merged into the page footer/status line, not rendered as an extra line below it.
- Why we changed that:
  - a separate shell-owned tab row made child pages reserve one footer line while the shell also appended one, which created layout confusion
  - visually, tabs belong to the same footer/status region as the page hints
- Cached tabs should store explicit tab context rather than a full normalized `nav.Route`.
- Why we changed that:
  - storing full routes forced a `normalizeTopRoute` helper that was hard to explain and mixed route intent with cached tab state
  - explicit tab context (`kind`, `worktreeRoot`, `ref`, `initialPath`) is easier to compare, initialize, and reason about

### Milestone 2 notes

- The existing `git.Commit` should probably stay as the small generic type used by old callers.
- Add a richer type for log/commit UI instead of bloating every existing caller.
- Prefer git-native decorations/graph text from `git log --decorate --graph --format=...` rather than rebuilding graph logic in Go.
- The pseudo-commit row should be a UI model type, not a fake `git` commit object. Keep real commit data and synthetic navigation rows distinct.
- Keep “current log root” as explicit route state so `gh` is just a route update back to `HEAD`, not a special-case mutation hidden inside the table model.
- Log rows should carry typed badge decorations so UI can color branches, remotes, and tags differently rather than treating all decorations as plain strings.
- The initial log row renderer used ad-hoc `fmt.Sprintf`-style fixed fields.
- Next refactor should extract a small shared ANSI-aware fixed-column renderer from `ui/worktrees/table.go` and reuse it in log.
- Why:
  - log needs stable-width relative-time and initials columns
  - selected-row highlighting should cover the full visible line, which is easier if we avoid nested style resets inside the selected row
  - `worktrees` already solved the low-level “truncate/pad ANSI-safe cells to exact width” problem, but it is still trapped inside worktree-specific table rendering code

### Milestone 3 notes

- Today `ui/status` mixes:
  - status-specific actions
  - generic file-tree rendering
  - generic diff rendering/navigation
  - modal plumbing
- The current `EnableNavigation` flag is intentional transition glue, not the target architecture.
- By the end of this milestone, navigation mode should come from composition boundaries rather than a boolean living in page settings.
- Separate these before adding commit support or the commit page will either duplicate a lot of code or wedge historical behavior into status-only assumptions.
- Good extraction targets:
  - status tree building can be generalized from `git.StageFileStatus` to a smaller file-entry shape
  - diff pane state and parsing are already mostly generic
  - action handlers need capability flags

### Milestone 4 notes

- Commit page data source should expose one diff stream per file, but the explorer can still render it in the existing pane structure by treating the commit diff as one active section and hiding the second section entirely.
- Commit metadata rendering belongs above the file/diff area, not inside the footer/modal system.
- Need exact date formatting per prompt:
  - `YYYY.MM.DD`
  - relative time alongside it
- For `gx show <ref>`, show badges for all refs/tags pointing at the resolved commit, not just the originally provided ref string.
- Badge colors should differ by kind at minimum:
  - local branches
  - remote branches
  - tags
- Route state should still remember the originally requested ref string when useful for messaging/debugging, but badge rendering should reflect the resolved commit’s full decoration set.

### Milestone 5 notes

- `restore before/after commit` likely maps to `git restore --source=<rev>` for whole-file restores, but hunk/line restore needs patch synthesis against a chosen side of the commit.
- Keep restore work local to the current worktree; this milestone should not rewrite history.

### Milestone 7 notes

- `delete from commit` should rewrite history because that is the desired product behavior.
- This is meaningfully higher risk than the rest of the feature:
  - it changes commit identities
  - it can conflict
  - it needs rollback/abort semantics
- Allow rewrite only for commits reachable from current `HEAD`.
- If the viewed commit is not reachable from current `HEAD`, `d` should fail fast with a clear error explaining that rewrite is only supported for commits in the current `HEAD` history.
- Treat it as a separate final milestone so the main log/commit navigation and review flow can land first.

## Test plan

- [ ] `git`
  - log parsing for graph/decorations/body/tags
  - commit diff loading for normal commits and root commits
  - restore helper behavior for before/after commit sources
  - pseudo-commit eligibility detection for dirty `HEAD`
  - commit-decoration lookup for all refs/tags pointing at a commit
  - patch synthesis for restore actions
  - rewrite planning / safety checks for delete-history milestone

- [ ] `ui/log`
  - row rendering with graph/tags/refs
  - badge color distinctions by ref kind
  - pseudo-commit row rendering and placement
  - `gh` resets custom-root log back to `HEAD`
  - search next/prev behavior
  - enter emits open-commit intent

- [ ] `ui/status` / shared explorer
  - existing status behavior stays green after refactor
  - staged/unstaged switching still works
  - diff navigation/search behavior stays green

- [ ] root shell
  - `gw` / `gl` / `gs`
  - active tab highlighting
  - back-stack behavior for `q` / `esc`
  - commit view uses log tab highlight
  - initial-route launch for `gx log <ref>` and `gx show <ref>`

- [ ] commit page
  - body collapse toggle
  - restore menu flow
  - commit-specific yank targets
  - direct launch from `gx show <ref>`
  - branch-name launch opens branch tip commit
  - header badges show all refs/tags pointing at the commit with distinct colors

- [ ] rewrite milestone
  - delete confirm flow warns about history rewrite
  - unreachable commit from current `HEAD` shows explicit error and does not start rewrite
  - abort path restores original state
  - conflict path is handled explicitly

- [ ] end-to-end
  - worktrees -> enter -> log
  - log `HEAD` with dirty worktree -> pseudo-commit -> status
  - log -> enter -> commit
  - `gx log <ref>` opens on that ref
  - `gh` from custom-root log returns to `HEAD`
  - `gx show <ref>` opens directly on commit view
  - back all the way out

## Risks

- Status refactor risk:
  - `ui/status` has deep behavior and a large test surface.
  - Mitigation: extract incrementally and keep existing tests passing at each step.

- Commit action semantics risk:
  - History rewrite is materially riskier than local reverse-apply.
  - Mitigation: isolate it into the final milestone with explicit safety and rollback design.

- Synthetic-row UX risk:
  - pseudo-commit rows can leak fake commit assumptions into log code.
  - Mitigation: represent log rows as a discriminated union of real commit rows and synthetic action rows.

- Routing churn risk:
  - worktrees/status currently assume direct ownership of quit behavior.
  - Mitigation: introduce explicit navigation messages rather than ad-hoc callbacks.

- UX drift risk:
  - duplicated chained-key handling across pages will diverge.
  - Mitigation: land shared chained-menu component in Milestone 1 before adding more prefixes.

## Recommended implementation order

1. Milestone 1
2. Milestone 2
3. Milestone 3
4. Milestone 4
5. Milestone 5
6. Milestone 6
7. Milestone 7

Do not start commit actions before Milestones 3 and 4 are in place. Do not start history rewrite before the non-rewriting commit review flow is stable.

## Existing test coverage assessment

Current coverage is good for refactor safety in the areas we already have:

- `ui/status` has a large and valuable model test surface around navigation, rendering modes, search, diff actions, and regressions.
- `ui/worktrees` has solid integration-style coverage for key flows.
- `git` already has focused tests around commit history helpers and status/diff plumbing.
- `cmd` has command wiring coverage.

What that means in practice:

- We have enough existing coverage to refactor `ui/status`, `ui/worktrees`, and command dispatch with reasonable confidence if we move incrementally and keep tests green.
- We do not yet have coverage for:
  - app-level routing/history
  - log page rendering/search/navigation
  - pseudo-commit rows
  - direct `gx log <ref>` / `gx show <ref>` launch behavior
  - commit-page semantics
  - history rewrite flows

So: enough coverage to begin safely, but not enough to trust the new feature without adding new tests as part of each milestone.

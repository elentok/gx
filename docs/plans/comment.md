# Comment Flow Plan

## Goal
Add a comment capture flow from diff views that writes a markdown file under `~/.local/share/gx/comments/` and opens it in an editor with the same terminal-aware launch behavior as commit.

## Finalized Decisions

1. Keybinding shape
- Decision: keep existing `c c` for commit and add comment as `c m`.
- Reasoning: preserves existing muscle memory for commit while keeping comment in the same `c` chord namespace.

2. Scope of activation
- Decision: enable `c m` only in diff views.
- Reasoning: this matches the clarified UX intent and avoids accidental comment generation from non-diff contexts.

3. Location format
- Decision: use `@path/to/file L{line}` for a single line and `@path/to/file L{start}-{end}` for ranges.
- Reasoning: compact, human-readable, and consistent with current yank location semantics.

4. File/hunk location behavior
- Decision: for hunk scope include the hunk line range; for file scope omit `L...` entirely.
- Reasoning: retains precision where possible and avoids fake line precision for whole-file selection.

5. Diff content scope
- Decision: include only selected scope in code fence.
- Reasoning: keeps comments focused and avoids overlong files.

6. Output path and naming
- Decision: write to `~/.local/share/gx/comments/{YYYYMMDD-HHmmss-filename}.md`.
- Decision: sanitize filename component and append numeric suffix (`-2`, `-3`, ...) on same-second collisions.
- Reasoning: deterministic naming with collision safety.

7. Editor launch behavior
- Decision: reuse commit terminal behavior (`tmux` split, kitty remote split, fallback foreground process), but launch `$EDITOR <comment-file>`.
- Reasoning: consistent operator experience across commit and comment flows.

8. Fence language
- Decision: use fenced block as `diff`.
- Reasoning: improves readability of additions/removals.

9. Missing file context behavior
- Decision: if no file context exists in diff view, show status error and do nothing.
- Reasoning: fail-safe behavior with no silent incorrect output.

## Implementation Approach

1. Status view (`ui/status`)
- Add `c m` binding in key manager.
- Gate action to diff focus only.
- Build comment payload from selected diff scope:
  - file path from selected status file
  - location from focused selection/hunk/file rules
  - body from focused diff selection (line/range/hunk/file)
- Write markdown file to comments dir with timestamped filename.
- Open file via shared terminal-aware editor launcher.

2. Commit diff view (`ui/commit`)
- Add `c m` chord handling in existing prefix parser.
- Restrict behavior to diff focus only.
- Reuse same payload builder + writer pattern adapted to commit model selection APIs.
- Open with same shared terminal-aware editor launcher.

3. Shared helper extraction
- Extract reusable launcher for terminal-aware split behavior plus foreground fallback.
- Keep split-specific success status messaging contextual (e.g., opened tmux/kitty split).

4. Tests
- Status: verify `c m` chord behavior, diff-only gating, payload formatting by selection scope, filename pattern/collision logic.
- Commit: verify `c m` in diff focus only and payload formatting.
- Launcher: verify command formation for tmux, kitty remote, and fallback path.

## Task Checklist
- [x] Read and understand `docs/prompts/comment.md`
- [x] Confirm design decisions through grill session
- [x] Add `c m` keybinding in status diff flow
- [x] Add `c m` keybinding in commit diff flow
- [x] Implement comment payload formatter (location + fenced diff block)
- [x] Implement comment file writer (directory creation, timestamp naming, collision suffix)
- [x] Extract and apply shared terminal-aware editor launch helper
- [x] Add/update tests for status, commit, and launcher behavior
- [x] Run targeted tests and fix regressions

## Beads Breakdown
- `main-lp6`: implement `c m` in status diff
- `main-cph`: implement `c m` in commit diff
- `main-84v`: extract shared terminal-aware editor launcher
- `main-204`: implement comment file writer and naming rules
- `main-lkv`: tests for behavior and payload formatting (depends on implementation tasks)
- `main-jwq`: run targeted tests and fix regressions (depends on `main-lkv`)

# Git Worktrees: `.bare` Approach vs Claude Code Native

## Current Setup (`.bare` approach)

```
my-repo/
  .bare/          ← bare git repo
  .git            ← gitdir: ./.bare
  main/           ← initial worktree
  feature-a/      ← additional worktrees as siblings
```

This is a well-understood pattern for power users with manual worktree management.

## Claude Code Native Worktree Support

Claude Code has first-class worktree support via `claude --worktree <name>`, which creates worktrees at `.claude/worktrees/<name>/` relative to the repo root:

```
my-repo/
  .claude/
    worktrees/    ← add to .gitignore
      feature-a/
      feature-b/
  main/           ← primary worktree
```

## Comparison

| | `.bare` approach | Claude Code native |
|---|---|---|
| Worktree location | Siblings of `.bare/` | `.claude/worktrees/<name>/` |
| Session isolation | Manual | Automatic (each `--worktree` gets its own session) |
| Env file copying | Manual | `.worktreeinclude` auto-copies untracked files (`.env`, etc.) |
| Cleanup | Manual | Auto-cleans unchanged worktrees |
| PR checkout | Manual | `claude --worktree "#1234"` |
| Base branch config | Manual | `worktree.baseRef` in settings |

## Key Claude Code Behaviors

- Each worktree gets its own **independent Claude Code session** with a fresh context window
- `CLAUDE.md` is loaded normally in each worktree session
- Use `/resume` to switch between sessions across worktrees
- `--worktree` flag supports auto-generated names: `claude --worktree` → e.g. `bright-running-fox`
- Subagent worktrees (via `isolation: worktree`) are auto-cleaned after `cleanupPeriodDays`

## Useful Additions Regardless of Approach

- **`.worktreeinclude`** — `.gitignore`-syntax file that lists untracked files to auto-copy into new worktrees (e.g. `.env`, secrets)
- **`.claude/worktrees/` in `.gitignore`** — prevents worktree contents from polluting the main checkout

## When to Stick with `.bare`

- You prefer manual, explicit worktree management
- You use non-Claude tooling or your own scripts (like `gx wt`)
- You want worktrees as siblings at the top level for easy navigation

## When to Consider Claude Code Native

- You want automatic session isolation per worktree
- You want `claude --worktree "#1234"` for PR checkouts
- You want `.worktreeinclude` env file propagation
- You want automatic cleanup of stale worktrees

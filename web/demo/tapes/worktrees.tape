# WORKTREES — the parallelism layer: a sync/rebase-status table across
# worktrees, create a new one, switch tabs. Rendered to docs/demo-worktrees.gif
# by `make demos`.

Output "docs/demo-worktrees.gif"

Set FontFamily "Agave Nerd Font Mono"
Set FontSize 18
Set Theme "Catppuccin Mocha"
Set Width 1500
Set Height 850
Set Padding 24
Set TypingSpeed 60ms

# Launch gx on the worktrees tab (run from the .bare root).
Hide
Type "bash web/demo/run.sh ."
Enter
Sleep 5s
Show

Sleep 1500ms

# Walk the table to show the per-worktree ahead/behind/diverged status.
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "j"
Sleep 1200ms

# Create a new worktree.
Type "n"
Sleep 1200ms
Type "hotfix-cache"
Sleep 1000ms
Enter
Sleep 3000ms

# Switch to the log tab and back to worktrees.
Type "2"
Sleep 2200ms
Type "1"
Sleep 3s

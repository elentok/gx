# LOG — browse history, reword a commit, inspect a single commit's diff, and
# peek the stash. Rendered to docs/demo-log.gif by `make demos`.

Output "docs/demo-log.gif"

Set FontFamily "Agave Nerd Font Mono"
Set FontSize 18
Set Theme "Catppuccin Mocha"
Set Width 1500
Set Height 850
Set Padding 24
Set TypingSpeed 60ms

# Launch gx on the log tab for the main worktree.
Hide
Type "bash web/demo/run.sh main log"
Enter
Sleep 5s
Show

Sleep 1200ms

# Scroll down through the history.
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "k"
Sleep 1200ms

# Reword the selected commit: opens the message in an editor.
Type "rw"
Sleep 2500ms
Type "A"
Sleep 400ms
Type " (reviewed)"
Sleep 900ms
Escape
Type ":wq"
Enter
Sleep 3000ms

# Open the commit to show its full diff, then step back to the list.
Type "l"
Sleep 2800ms
Escape
Sleep 1200ms

# Peek the stash tab's split view.
Type "4"
Sleep 3s

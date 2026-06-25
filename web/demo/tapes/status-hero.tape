# STATUS HERO — the marquee demo: review an AI-authored change, hand a note back
# to the agent, then stage it hunk-by-hunk. Rendered to docs/demo-status.gif by
# `make demos`.

Output "docs/demo-status.gif"

Set FontFamily "Agave Nerd Font Mono"
Set FontSize 18
Set Theme "Catppuccin Mocha"
Set Width 1500
Set Height 850
Set Padding 24
Set TypingSpeed 60ms

# Launch gx on the status tab while hidden so the GIF opens on a rendered UI.
Hide
Type "bash web/demo/run.sh feature-auth status"
Enter
Sleep 5s
Show

Sleep 1200ms

# Navigate the tree to the changed file and open its diff.
Type "j"
Sleep 1s
Type "l"
Sleep 1800ms

# Yank the focused hunk as agent-ready markdown.
Type "ay"
Sleep 2500ms

# Ask AI: open the anchored diff in an editor, write an instruction, save — the
# handoff back to your agent.
Type "aa"
Sleep 2500ms
Type "o"
Sleep 400ms
Type "Make checkCredentials constant-time."
Sleep 1s
Escape
Type ":wq"
Enter
# Let gx settle after the editor closes and reloads the diff (focus returns to
# the filetree), then re-open the diff.
Sleep 3500ms
Type "j"
Sleep 800ms
Type "l"
Sleep 1800ms

# Stage the first hunk wholesale.
Space
Sleep 2200ms

# Drop into visual mode and stage a single line of the next hunk.
Type "v"
Sleep 1s
Space
Sleep 2200ms

# Toggle the side-by-side view to rest on the remaining changes.
Type "s"
Sleep 3s

# gx stage - part 2

## General

- Highlight the border of the active pane in orange
- Add some colors (use lipgloss), use the catppuccin theme (try to reuse colors
  from the gx wt command)

## Status view

- pressing `<space>` will stage/unstage the entire file
- when a directory is new it current appears as `{dir}/` but I can't stage the file inside,
  what would be a better approach:
  1. press `<space>` to stage the entire directory and then unstage what I don't want
  2. expand the directory in advance and show the new files allowing me to stage what I want from them

## Diff view

- Highlight the entire hunk when in hunk mode (with the side column)
- Add J/K to scroll the diff view without moving the cursor (if you move the
  cursor it should scroll to wherever you are)
- When staging/unstaging make sure the hunk/link that moved is visible in the other pane
  and add some animation when it's added

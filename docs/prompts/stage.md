# gx stage

I want to implement a new `gx stage` command, it should behave close to lazygit's stage UI but with syntax highlighting.

- It should show two panes: status view (`git status`) and diff view
- When the screen is more than 100 characters the status view will be a sidebar on the left
- When the screen is less (or equal) to 100 characters the status view will be on top and the diff view on bottom
- The status view should take 30% of the size
- The focus can be either on the status view or on the diff view
- When in the status view:
  - j/k/arrows moves up/down between files (the diff view shows the diff for that file)
  - pressing enter focuses on the diff view
- When in the diff view:
  - The diff view should be split to two sections (unstaged on top and staged on bottom)
  - If there are only lines in one group (staged/unstaged) the other one should be collapsed
  - Use `git diff` (with `delta` as the pager) to render the diff so we get syntax highlighting
  - tab moves between staged/unstaged
  - j/k/arrows move between hunks or lines (depends on the mode, can be toggled by pressing "a")
    - only lines with actual changes can be active
  - pressing <space> will stage/unstage
  - each diff view section should be scrollable on its own

Notes:

- Go over the delta documentation (`delta --help`)
- Go over the git diff documentation (`git diff --help`)

## Open questions that require research & design

- How should we highlight the active hunk/line?
  - highlighting the background could mess up the syntax highlighting
  - we can maybe have a column on the left to somehow indicate this hunk/link is active
  - what do you suggest?

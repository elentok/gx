# gx stage - add visual mode

In the staged/unstaged view allow selecting multiple lines by pressing "v" to
go to vim-like visual mode.

Pressing `<space>` will stage/unstage the block
Pressing `<esc>` will go back to normal mode

All actions that work on line/hunk should also work on line-range.

When in visual mode add an indication to the status bar ("VISUAL" to the top left).

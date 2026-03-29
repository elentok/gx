# gx stage - part 3

1. When entering the diff view unstaged should be in focus first
2. Highlight the title of every hunk with a dark background and clean it up a bit
   - Before: @@ -305,14 +305,30 @@ func (m \*Model) applySelection() tea.Cmd {
   - After: func (m \*Model) applySelection() tea.Cmd {
3. Add a blank line between hunks
4. No need for the diff prefix:
   ```
   diff --git a/ui/stage/model.go b/ui/stage/model.go
   index a3aee86..edccb8a 100644
   --- a/ui/stage/model.go
   +++ b/ui/stage/model.go
   ```
5. add "f" mapping to toggle the current staged/unstaged view to full screen (only one of them)
6. add "r" to refresh the view
7. there's a bug on the right border of the staged/unstaged views (for some lines some of the vertical border lines are missing)

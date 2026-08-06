package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
)

// Run launches the history browser TUI against the real ~/.claude/projects
// tree. It is a standalone program (not wired into gx's app/nav tab shell)
// since Claude session history isn't scoped to the current git worktree.
func Run() error {
	m := NewModel("", claudehistory.ListProjects, claudehistory.ListConversations)
	m.terminal = ui.DetectTerminal()
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

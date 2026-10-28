package history

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
)

// Run launches the history browser TUI against home's .claude/projects
// tree. It is a standalone program (not wired into gx's app/nav tab shell)
// since Claude session history isn't scoped to the current git worktree.
func Run(home string) error {
	root := filepath.Join(home, ".claude", "projects")
	listProjects := func(root string) ([]claudehistory.Project, error) {
		return claudehistory.ListProjects(root, home)
	}
	m := NewModel(root, listProjects, claudehistory.ListConversations, claudehistory.GrepTranscripts)
	m.terminal = ui.DetectTerminal()
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

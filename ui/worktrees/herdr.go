package worktrees

import (
	"github.com/elentok/gx/herdr"

	tea "charm.land/bubbletea/v2"
)

// cmdHerdrSession focuses the herdr workspace labeled name, creating one
// rooted at path if none exists yet.
func cmdHerdrSession(name, path string) tea.Cmd {
	return func() tea.Msg {
		_, err := herdr.FindOrCreateWorkspace(name, path)
		return terminalResultMsg{err: err}
	}
}

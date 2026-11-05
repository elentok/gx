package launch

import (
	"github.com/elentok/gx/herdr"

	tea "charm.land/bubbletea/v2"
)

// HerdrSession focuses the herdr workspace labeled name, creating one
// rooted at path if none exists yet.
func HerdrSession(name, path string) tea.Cmd {
	return func() tea.Msg {
		_, err := herdr.FindOrCreateWorkspace(name, path)
		return TerminalResultMsg{Err: err}
	}
}

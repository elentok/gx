package modal

import (
	"github.com/elentok/gx/ui/help"

	tea "charm.land/bubbletea/v2"
)

// Help wraps the help dialog.
type Help struct {
	model help.Model
}

// NewHelp opens a copy of the closed help template h for a page of the given
// size.
func NewHelp(h help.Model, width, height int) Help {
	h.Open(width, height)
	return Help{model: h}
}

// Model is the wrapped help dialog.
func (h Help) Model() help.Model { return h.model }

func (h Help) Update(msg tea.Msg) (Modal, tea.Cmd, Result) {
	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	return h, cmd, Result{Close: !h.model.IsOpen}
}

func (h Help) InputFocused() bool { return h.model.InputFocused() }

func (h Help) View(_, _ int) string { return h.model.View() }

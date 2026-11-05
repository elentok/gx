package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// olderServerNotice replaces every tab's ticket data when the server predates
// rendered status and epic summaries. There is no client-side fallback: the TUI
// and the server ship in one binary.
const olderServerNotice = "older server — update the server"

// serverBlockedBindings lists every binding that needs a writable server. A
// new server-backed binding needs an entry here — nothing else enforces that.
var serverBlockedBindings = map[keys.BindingID]bool{
	bindingTicketsFinishAndPause: true,
}

// serverKeyGuard is the shared no-op check for server-backed keys: blocked
// reports whether the caller should return cmd (a toast with the reason).
func (m Model) serverKeyGuard(id keys.BindingID) (cmd tea.Cmd, blocked bool) {
	reason := m.link.DisabledReason()
	if reason == "" || !serverBlockedBindings[id] {
		return nil, false
	}
	return notify.Info("disabled: " + reason), true
}

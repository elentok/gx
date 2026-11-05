package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// serverBlockedBindings lists every binding that needs a writable server. A
// new server-backed binding needs an entry here — nothing else enforces that.
var serverBlockedBindings = map[keys.BindingID]bool{
	bindingTicketsFinishAndPause: true,
}

// keyGuard is the shared no-op check for server-backed keys: blocked
// reports whether the caller should return cmd (a toast with the reason).
func (s serverBinding) keyGuard(id keys.BindingID) (cmd tea.Cmd, blocked bool) {
	reason := s.link.DisabledReason()
	if reason == "" || !serverBlockedBindings[id] {
		return nil, false
	}
	return notify.Info("disabled: " + reason), true
}

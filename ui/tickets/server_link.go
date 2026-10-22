package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// ServerLink is how the Tickets tab currently reaches the server. The zero
// value is "no restriction" (up).
type ServerLink int

const (
	ServerLinkUp ServerLink = iota
	// ServerLinkDown: the server is unreachable.
	ServerLinkDown
	// ServerLinkReadOnly: API version mismatch; the client must not write.
	ServerLinkReadOnly
)

// olderServerNotice replaces every tab's ticket data when the server predates
// rendered status and epic summaries. There is no client-side fallback: the TUI
// and the server ship in one binary.
const olderServerNotice = "older server — update the server"

// disabledReason is why server keys are off; empty when they are usable.
func (l ServerLink) disabledReason() string {
	switch l {
	case ServerLinkDown:
		return "server is down"
	case ServerLinkReadOnly:
		return "server is read-only — restart gx"
	}
	return ""
}

// serverBlockedBindings lists every binding that needs a writable server. A
// new server-backed binding needs an entry here — nothing else enforces that.
var serverBlockedBindings = map[keys.BindingID]bool{
	bindingTicketsReplaceQueue:   true,
	bindingTicketsAddToQueue:     true,
	bindingTicketsFinishAndPause: true,
}

// serverKeyGuard is the shared no-op check for server-backed keys: blocked
// reports whether the caller should return cmd (a toast with the reason).
func (m Model) serverKeyGuard(id keys.BindingID) (cmd tea.Cmd, blocked bool) {
	reason := m.link.disabledReason()
	if reason == "" || !serverBlockedBindings[id] {
		return nil, false
	}
	return notify.Info("disabled: " + reason), true
}

package supervise

import (
	"slices"

	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets/schema"
)

// BlockedPausedError ends a wait when the agent blocked and the ticket was
// paused blocked-pane. The agent is alive, waiting for a person.
type BlockedPausedError struct{ Reason string }

func (e *BlockedPausedError) Error() string {
	return "iteration paused: agent blocked on a request: " + e.Reason
}

// RecordLiveSession reads the native session of the agent still running under
// label and appends it to the ticket's session_ids once, as the in-process
// loop does when it launches or reattaches an iteration. The server launches
// without that step, so without this the finish path has no session to read
// the closing occupancy, elapsed time and cost from and writes none of them.
// Best-effort: "" (no live agent, no session yet) leaves the ticket as it was.
func RecordLiveSession(d deps.Deps, label, ticketPath string) string {
	s, ok, err := d.Runner.Find(label)
	if err != nil || !ok {
		return ""
	}
	status, err := d.Runner.Status(s)
	if err != nil || status.SessionID == "" {
		return ""
	}
	if t, err := schema.ParseTicket(ticketPath); err == nil && !slices.Contains(t.SessionIDs, status.SessionID) {
		_ = ticketops.AppendSessionID(ticketPath, status.SessionID)
	}
	return status.SessionID
}

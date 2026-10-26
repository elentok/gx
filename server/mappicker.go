package server

import (
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// designActivity is what a map's design sessions say about it.
type designActivity struct {
	// The fields below read only the sessions that are neither Exited nor failed.
	// running is set when one exists, asking when one waits on an answer (since
	// askedAt), countdownAt is the deadline of its auto-advance. endedAt is when
	// the newest Exited or failed session stopped.
	running, asking bool
	askedAt         time.Time
	countdownAt     time.Time
	endedAt         time.Time
}

// designActivities folds the sessions into one designActivity per map address.
// A session counts for a map when its Session ticket is the map or one of its
// tickets; live means not Exited.
func (s *Server) designActivities() map[string]designActivity {
	out := map[string]designActivity{}
	sessions, err := s.listSessions()
	if err != nil {
		s.log.Warn("design picker: list sessions", "err", err)
		return out
	}
	for _, info := range sessions {
		if !info.Design {
			continue
		}
		addr := epicAddress(info.Ticket)
		a := out[addr]
		exited := info.Status.State == SessionExited
		switch {
		case exited || info.Failure != nil:
			if info.StatusChangedAt.After(a.endedAt) {
				a.endedAt = info.StatusChangedAt
			}
		default:
			a.running = true
			if info.Actionable.Grilling {
				a.asking = true
				if info.StatusChangedAt.After(a.askedAt) {
					a.askedAt = info.StatusChangedAt
				}
			}
			if info.AutoAdvanceAt != nil {
				a.countdownAt = *info.AutoAdvanceAt
			}
		}
		out[addr] = a
	}
	return out
}

// mapNotStarted: the Destination is unconfirmed and none of the map's tickets
// is resolved.
func mapNotStarted(m tickets.Epic, step ProgressStep) bool {
	if step != ProgressChart {
		return false
	}
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusDone) {
			return false
		}
	}
	return true
}

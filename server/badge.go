package server

import (
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// badgeCount is the app icon badge: Actionable sessions plus Paused tickets.
// The session rule must match isActionable in webui/src/agents/rules.ts, so the
// badge and the top-bar count agree. Change one, change both.
func badgeCount(sessions []wire.SessionInfo, ts []wire.TicketInfo) int {
	n := 0
	for _, s := range sessions {
		attention := !s.Queue && !s.Design && s.Flow != string(tickets.FlowTriage)
		if attention && (s.Actionable.Reason != "" || s.Status.State == wire.SessionBlocked) {
			n++
		}
	}
	for _, t := range ts {
		if t.EpicIsMap {
			continue
		}
		switch t.Status {
		case string(schema.StatusPaused), string(schema.StatusLegacyAnswer), string(schema.StatusLegacyRepair):
			n++
		}
	}
	return n
}

// BadgeCount counts the badge from live state. The push sender calls it, and
// the snapshot handler calls badgeCount on the lists it already holds.
func (s *Server) BadgeCount() int {
	sessions, _ := s.queue.ListSessionsWithQueue()
	return badgeCount(sessions, s.idx.Snapshot().Tickets)
}

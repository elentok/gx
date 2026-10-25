package server

import (
	"cmp"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// pickerWindow is how long a finished Design stays in the picker.
const pickerWindow = 7 * 24 * time.Hour

// designActivity is what a map's design sessions say about it.
type designActivity struct {
	live, waiting bool
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
		if !exited {
			a.live = true
			a.waiting = a.waiting || info.Actionable.Grilling
		}
		out[addr] = a
	}
	return out
}

// mapPicker is a map's picker section, the time the section sorts by and the
// address of its implementation epic when it exists.
func (s *Server) mapPicker(project string, m tickets.Epic, step ProgressStep, byName map[string]tickets.Epic, act designActivity, now time.Time) (PickerSection, time.Time, string) {
	implName := m.Name + tickets.ImplEpicSuffix
	impl, hasImpl := byName[implName]
	implAddr := ""
	if hasImpl {
		implAddr = rootRef{Project: project, Epic: implName}.String()
	}
	if step == ProgressDone {
		return s.finishedPicker(project, m, impl, hasImpl, implAddr, now)
	}
	if mapNotStarted(m, step) && !act.live && !act.waiting {
		return PickerNotStarted, m.CreatedAt, implAddr
	}
	return PickerInProgress, cmp.Or(lastActivity(m), m.CreatedAt), implAddr
}

// finishedPicker places a Done map. A map with an impl epic is decided by the
// impl epic alone: completed_at is stamped when the impl epic is created, long
// before Branch.
func (s *Server) finishedPicker(project string, m, impl tickets.Epic, hasImpl bool, implAddr string, now time.Time) (PickerSection, time.Time, string) {
	if !hasImpl {
		if m.CompletedAt.IsZero() {
			return PickerInProgress, cmp.Or(lastActivity(m), m.CreatedAt), implAddr
		}
		return recentOrNone(m.CompletedAt, now), m.CompletedAt, implAddr
	}
	queuedAt := cmp.Or(impl.QueuedAt, impl.StartedAt, impl.MergedAt)
	if queued := s.rootQueued(rootRef{Project: project, Epic: impl.Name}); !queued && queuedAt.IsZero() {
		return PickerReadyToQueue, impl.CreatedAt, implAddr
	}
	queuedAt = cmp.Or(queuedAt, impl.CreatedAt)
	return recentOrNone(queuedAt, now), queuedAt, implAddr
}

func recentOrNone(at, now time.Time) PickerSection {
	if now.Sub(at) < pickerWindow {
		return PickerRecentlyFinished
	}
	return PickerNone
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

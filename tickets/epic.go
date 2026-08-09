package tickets

import "time"

// Epic is one immediate subdirectory of `.scratch/`. Discovery is dumb: an
// epic is counted regardless of which files exist inside it (spec.md,
// map.md, only issues/, or nothing yet).
type Epic struct {
	Name    string
	Path    string
	IsMap   bool   // has a map.md (wayfinder map)
	MapBody string // map.md's raw content, only set when IsMap
	Tickets []Ticket

	// StartedAt and CompletedAt come from the epic's optional epic.yaml
	// sidecar file (see loadEpicTiming). Zero when the epic has no
	// epic.yaml yet, or the file doesn't set that field.
	StartedAt   time.Time
	CompletedAt time.Time
}

// TotalCount is the epic's total ticket count.
func (e Epic) TotalCount() int {
	return len(e.Tickets)
}

// OpenCount is how many of the epic's tickets are not done — a ticket
// rendering as waiting-for-children (see Epic.RenderedStatus) counts as open
// here too, since its fork subtree is still outstanding work.
func (e Epic) OpenCount() int {
	open := 0
	for _, t := range e.Tickets {
		if e.RenderedStatus(t) != StatusDone {
			open++
		}
	}
	return open
}

// DoneCount is how many of the epic's tickets are done.
func (e Epic) DoneCount() int {
	return e.TotalCount() - e.OpenCount()
}

// CompletionDuration returns the epic's wall-clock span from epic.yaml's
// started_at to completed_at, and whether both are set — an epic missing
// either (not yet done, or predating this feature) reports ok=false.
func (e Epic) CompletionDuration() (duration time.Duration, ok bool) {
	if e.StartedAt.IsZero() || e.CompletedAt.IsZero() {
		return 0, false
	}
	return e.CompletedAt.Sub(e.StartedAt), true
}

// AllDone reports whether every one of the epic's tickets is done. A
// zero-ticket epic is not considered "all done" — it starts expanded, not
// collapsed, since "nothing here yet" is distinct from "everything closed".
func (e Epic) AllDone() bool {
	if len(e.Tickets) == 0 {
		return false
	}
	return e.OpenCount() == 0
}

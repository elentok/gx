package tickets

import (
	"os"
	"path/filepath"
	"time"
)

// KindMap is the ticket.md `kind:` of a wayfinder map epic. A map epic is
// hand-driven: its tickets are decisions a person resolves, so the scheduler
// never runs it and `tickets set` may claim and close its tickets directly.
const KindMap = "map"

// IsMapEpic reports whether the epic at epicPath is a wayfinder map, without
// loading its tickets. Same rule as Epic.IsMap.
func IsMapEpic(epicPath string) bool {
	epic := Epic{Path: epicPath}
	loadEpicTicketMD(&epic)
	if epic.IsMap {
		return true
	}
	_, err := os.Stat(filepath.Join(epicPath, "map.md"))
	return err == nil
}

// Epic is one immediate subdirectory of `.scratch/`. Discovery is dumb: an
// epic is counted regardless of which files exist inside it (spec.md,
// map.md, only issues/, or nothing yet).
type Epic struct {
	Name    string
	Path    string
	IsMap   bool   // a wayfinder map: ticket.md says kind: map, or an old-shape map.md exists
	MapBody string // the map's text (ticket.md body or map.md), only set when IsMap
	Tickets []Ticket

	// HasTicketMD is true when the epic uses the store shape: a ticket.md
	// entry file whose frontmatter supplies Status, BlockedBy, Base and
	// timing. Old-shape epics (epic.yaml sidecar) leave the first four zero.
	HasTicketMD bool
	Status      string
	BlockedBy   []string
	Base        string

	// StartedAt and CompletedAt come from ticket.md's frontmatter, or the
	// old-shape epic.yaml sidecar (see loadEpicTiming). Zero when neither
	// sets the field.
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
		if !e.RenderedStatus(t).Terminal() {
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

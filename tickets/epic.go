package tickets

import (
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// KindMap is the ticket.md `kind:` of a wayfinder map epic. A map epic is
// hand-driven: its tickets are decisions a person resolves, so the scheduler
// never runs it and `tickets set` may claim and close its tickets directly.
const KindMap = "map"

// IsMapEpic reports whether the epic at epicPath is a wayfinder map, without
// loading its tickets: its ticket.md says kind: map.
func IsMapEpic(epicPath string) bool {
	epic := Epic{Path: epicPath}
	loadEpicTicketMD(&epic)
	return epic.IsMap
}

// LoadEpicTicketMD reads the epic at epicPath's ticket.md, without loading
// its tickets.
func LoadEpicTicketMD(epicPath string) Epic {
	epic := Epic{Path: epicPath}
	loadEpicTicketMD(&epic)
	return epic
}

// Epic is one immediate subdirectory of `.scratch/`. Discovery is dumb: an
// epic is counted regardless of which files exist inside it (ticket.md,
// spec.md, only issues/, or nothing yet).
type Epic struct {
	Name    string
	Path    string
	IsMap   bool   // a wayfinder map: ticket.md says kind: map
	Body    string // ticket.md's body
	Tickets []Ticket

	// HasTicketMD is true when the epic has its ticket.md entry file, whose
	// frontmatter supplies Status, BlockedBy, Base and timing. An epic
	// without one fails ValidateProject.
	HasTicketMD bool
	Status      string
	BlockedBy   []string
	Base        string

	// StartedAt and CompletedAt come from ticket.md's frontmatter; zero when
	// it doesn't set the field.
	StartedAt   time.Time
	CompletedAt time.Time
	// CreatedAt is when the epic was created; zero on an epic that predates
	// the field and hasn't been migrated.
	CreatedAt time.Time
	// MergedAt is when the epic's branch merged into its target; a Merged epic
	// is one with it set.
	MergedAt time.Time
	// QueuedAt is when the epic was first added to the queue; zero on an epic
	// queued before the field existed.
	QueuedAt time.Time
	// The stats fields come from ticket.md's frontmatter. The pointers read nil
	// when unstamped, so a stamped zero differs from no stamp; ActualCost is
	// stamped whenever TotalTokens is.
	LinesAdded   *int
	LinesRemoved *int
	TotalTokens  *int64
	ActualCost   float64
	HadCodex     bool
	Friction     *schema.FrictionSummary
	// MergeBlockKind and MergeBlockReason are why the last merge paused the epic;
	// the merge step clears both before it decides. The kind is empty for a
	// needs-rebase pause.
	MergeBlockKind   string
	MergeBlockReason string
	// MergeTarget is the branch the epic merges into. Only a server snapshot
	// sets it; the loader leaves it empty.
	MergeTarget string

	// The evolve fields come from ticket.md's frontmatter; zero when unset.
	EvolvedAt              time.Time
	EvolveSkipped          bool
	EvolveTicket           string // address of the epic's evolve ticket
	DestinationUnconfirmed bool

	// FrontmatterErrs holds ticket.md field values that failed to parse, each
	// prefixed with its field name; ValidateProject reports them.
	FrontmatterErrs []string
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

// CompletionDuration returns the epic's wall-clock span from ticket.md's
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

// Package epicwaiting builds the epic-waiting fixture scenario. It lives outside
// the spec package to keep that package under its sizeguard limit.
package epicwaiting

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

const project = limitseed.Project

// Shapes are the --shape names of the epic-waiting scenario; the first is the default.
var Shapes = []string{"two-blockers", "blocker-not-queued", "blocker-paused", "picker-open", "cycle-refusal", "release"}

// Names the epic-waiting flows look at.
const (
	// Epic is the unqueued waiting epic (Ready to queue); Queued is
	// its twin in the queue (Implementing).
	Epic   = "notification-alignment-impl"
	Queued = "notification-alignment-queued-impl"
	// Deliver and Push are the blockers.
	Deliver = "deliver-primitive-impl"
	Push    = "push-notifications-impl"
	// Polish and UI are picker rows: open, not queued.
	Polish = "notification-polish"
	UI     = "notifications-ui"
	// Merged and Archived are the picker's omissions.
	Merged   = "notification-shipped-impl"
	Archived = "notification-old-impl"
	// Cycle is blocked by Epic, so it cannot be one of its blockers.
	Cycle = "waits-on-alignment-impl"
)

const doneAt = "2026-02-10T10:00:00Z"

// mergedMeta is the frontmatter of a merged epic; extra adds its fields.
func mergedMeta(extra string) string {
	return "status: open\ncompleted_at: " + doneAt + "\nmerged_at: 2026-02-10T11:00:00Z\n" + extra
}

func doneTicket(id, slug string) spec.Ticket {
	return spec.Ticket{ID: id, Slug: slug, Opts: servertest.TicketOpts{Status: "done", DoneAt: doneAt}}
}

// Scenario is the epic-waiting scenario in the given shape.
func Scenario(shape string) (spec.Scenario, bool) {
	if !slices.Contains(Shapes, shape) {
		return spec.Scenario{}, false
	}
	open := func(slug string) []spec.Ticket { return []spec.Ticket{{ID: "01", Slug: slug}} }
	// The mock's "2/3": two done, one ready row; 04 shows an in-epic "after 03".
	waitingTickets := []spec.Ticket{
		doneTicket("01", "first"),
		doneTicket("02", "second"),
		{ID: "03", Slug: "third"},
		{ID: "04", Slug: "fourth", Opts: servertest.TicketOpts{BlockedBy: []string{"03"}}},
	}
	blockers := []string{Deliver, Push}
	deliver := open("primitive")
	sc := spec.Scenario{EpicMeta: map[string]string{}, Hold: true}
	queue := func(names ...string) {
		for _, n := range names {
			sc.Queue = append(sc.Queue, project+":"+n)
		}
	}
	queue(Queued)
	switch shape {
	case "two-blockers":
		queue(Deliver, Push)
	case "picker-open", "cycle-refusal":
		blockers = blockers[:1]
		queue(Deliver)
	case "blocker-not-queued", "blocker-paused":
		blockers = blockers[:1]
	case "release":
		// The blocker is complete and unmerged, nothing of it queued: mark-merged releases the waiting epic.
		blockers = blockers[:1]
		deliver = []spec.Ticket{doneTicket("01", "primitive")}
		sc.EpicMeta[project+":"+Deliver] = "status: open\ncompleted_at: " + doneAt + "\n"
	}
	if shape == "blocker-paused" {
		deliver = []spec.Ticket{{ID: "01", Slug: "primitive", Opts: servertest.TicketOpts{PauseReason: "iteration-error"}}}
	}
	blockedMeta := "status: open\nblocked_by: [" + strings.Join(blockers, ", ") + "]\n"
	sc.EpicMeta[project+":"+Epic] = blockedMeta
	sc.EpicMeta[project+":"+Queued] = blockedMeta
	sc.EpicMeta[project+":"+Merged] = mergedMeta("")
	sc.Epics = []spec.Epic{
		{Project: project, Name: Epic, Tickets: waitingTickets},
		{Project: project, Name: Queued, Tickets: waitingTickets},
		{Project: project, Name: Deliver, Tickets: deliver},
		{Project: project, Name: Push, Tickets: open("push")},
		{Project: project, Name: Polish, Tickets: open("polish")},
		{Project: project, Name: UI, Tickets: open("ui")},
		{Project: project, Name: Merged, Tickets: []spec.Ticket{doneTicket("01", "shipped")}},
	}
	sc.Archived = []spec.ArchivedEpic{{
		Project: project, Name: Archived,
		Meta:    mergedMeta("archived_at: 2026-02-11T10:00:00Z\n"),
		Tickets: []spec.Ticket{doneTicket("01", "old-work")},
	}}
	if shape == "cycle-refusal" {
		sc.Epics = append(sc.Epics, spec.Epic{Project: project, Name: Cycle, Tickets: open("waits")})
		sc.EpicMeta[project+":"+Cycle] = "status: open\nblocked_by: [" + Epic + "]\n"
	}
	return sc, true
}

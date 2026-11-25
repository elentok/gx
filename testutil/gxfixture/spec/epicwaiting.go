package spec

import (
	"slices"
	"strings"

	"github.com/elentok/gx/server/servertest"
)

// EpicWaitingShapes are the --shape names of the epic-waiting scenario; the first is the default.
var EpicWaitingShapes = []string{"two-blockers", "blocker-not-queued", "blocker-paused", "picker-open", "cycle-refusal", "release"}

// Names the epic-waiting flows look at.
const (
	// EpicWaitingEpic is the unqueued waiting epic (Ready to queue); EpicWaitingQueued is
	// its twin in the queue (Implementing).
	EpicWaitingEpic   = "notification-alignment-impl"
	EpicWaitingQueued = "notification-alignment-queued-impl"
	// EpicWaitingDeliver and EpicWaitingPush are the blockers.
	EpicWaitingDeliver = "deliver-primitive-impl"
	EpicWaitingPush    = "push-notifications-impl"
	// EpicWaitingPolish and EpicWaitingUI are picker rows: open, not queued.
	EpicWaitingPolish = "notification-polish"
	EpicWaitingUI     = "notifications-ui"
	// EpicWaitingMerged and EpicWaitingArchived are the picker's omissions.
	EpicWaitingMerged   = "notification-shipped-impl"
	EpicWaitingArchived = "notification-old-impl"
	// EpicWaitingCycle is blocked by EpicWaitingEpic, so it cannot be one of its blockers.
	EpicWaitingCycle = "waits-on-alignment-impl"
)

const epicWaitingDoneAt = "2026-02-10T10:00:00Z"

// EpicWaitingScenario is the epic-waiting scenario in the given shape.
func EpicWaitingScenario(shape string) (Scenario, bool) {
	if !slices.Contains(EpicWaitingShapes, shape) {
		return Scenario{}, false
	}
	open := func(slug string) []Ticket { return []Ticket{{ID: "01", Slug: slug}} }
	// The mock's "2/3": two done, one ready row; 04 shows an in-epic "after 03".
	waitingTickets := []Ticket{
		{ID: "01", Slug: "first", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicWaitingDoneAt}},
		{ID: "02", Slug: "second", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicWaitingDoneAt}},
		{ID: "03", Slug: "third"},
		{ID: "04", Slug: "fourth", Opts: servertest.TicketOpts{BlockedBy: []string{"03"}}},
	}
	blockers := []string{EpicWaitingDeliver, EpicWaitingPush}
	deliver := open("primitive")
	sc := Scenario{EpicMeta: map[string]string{}, Hold: true}
	queue := func(names ...string) {
		for _, n := range names {
			sc.Queue = append(sc.Queue, project+":"+n)
		}
	}
	queue(EpicWaitingQueued)
	switch shape {
	case "two-blockers":
		queue(EpicWaitingDeliver, EpicWaitingPush)
	case "picker-open", "cycle-refusal":
		blockers = blockers[:1]
		queue(EpicWaitingDeliver)
	case "blocker-not-queued", "blocker-paused":
		blockers = blockers[:1]
	case "release":
		// The blocker is complete and unmerged, nothing of it queued: mark-merged releases the waiting epic.
		blockers = blockers[:1]
		deliver = []Ticket{{ID: "01", Slug: "primitive", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicWaitingDoneAt}}}
		sc.EpicMeta[project+":"+EpicWaitingDeliver] = "status: open\ncompleted_at: " + epicWaitingDoneAt + "\n"
	}
	if shape == "blocker-paused" {
		deliver = []Ticket{{ID: "01", Slug: "primitive", Opts: servertest.TicketOpts{PauseReason: "iteration-error"}}}
	}
	blockedMeta := "status: open\nblocked_by: [" + strings.Join(blockers, ", ") + "]\n"
	sc.EpicMeta[project+":"+EpicWaitingEpic] = blockedMeta
	sc.EpicMeta[project+":"+EpicWaitingQueued] = blockedMeta
	sc.EpicMeta[project+":"+EpicWaitingMerged] = epicDiffMergedMeta("")
	sc.Epics = []Epic{
		{Project: project, Name: EpicWaitingEpic, Tickets: waitingTickets},
		{Project: project, Name: EpicWaitingQueued, Tickets: waitingTickets},
		{Project: project, Name: EpicWaitingDeliver, Tickets: deliver},
		{Project: project, Name: EpicWaitingPush, Tickets: open("push")},
		{Project: project, Name: EpicWaitingPolish, Tickets: open("polish")},
		{Project: project, Name: EpicWaitingUI, Tickets: open("ui")},
		{Project: project, Name: EpicWaitingMerged, Tickets: []Ticket{{ID: "01", Slug: "shipped", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicWaitingDoneAt}}}},
	}
	sc.Archived = []ArchivedEpic{{
		Project: project, Name: EpicWaitingArchived,
		Meta:    epicDiffMergedMeta("archived_at: 2026-02-11T10:00:00Z\n"),
		Tickets: []Ticket{{ID: "01", Slug: "old-work", Opts: servertest.TicketOpts{Status: "done", DoneAt: epicWaitingDoneAt}}},
	}}
	if shape == "cycle-refusal" {
		sc.Epics = append(sc.Epics, Epic{Project: project, Name: EpicWaitingCycle, Tickets: open("waits")})
		sc.EpicMeta[project+":"+EpicWaitingCycle] = "status: open\nblocked_by: [" + EpicWaitingEpic + "]\n"
	}
	return sc, true
}

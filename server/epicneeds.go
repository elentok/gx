package server

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// The state pill words of a Needs-you loop card.
const (
	PillQuestion      = "Question"
	PillPermission    = "Permission"
	PillPaused        = "Paused"
	PillDeadlocked    = "Deadlocked"
	PillMergeBlocked  = "Merge blocked"
	PillAwaitingMerge = "Awaiting merge"
)

// needsYou is why a loop epic waits on the person: the pill word, the reason line,
// the button and when the wait began.
type needsYou struct {
	Pill   string
	Reason string
	Since  time.Time
	Action CardAction
	// More are the buttons after Action.
	More []CardAction
}

// ticketWait is one paused or asking ticket of an epic.
type ticketWait struct {
	needsYou
	ticket tickets.Ticket
}

// loopNeedsYou says whether the loop epic e waits on the person, and why. A
// waiting ticket comes first, oldest wait first, since it can hold a running epic
// too. An epic without one waits on its deadlock, its blocked merge, or an
// unmerged finish in a project with auto-merge off.
func (s *Server) loopNeedsYou(root rootRef, e tickets.Epic, state EpicListState) (needsYou, bool) {
	if e.IsMap || !e.MergedAt.IsZero() {
		return needsYou{}, false
	}
	fallback := cmp.Or(e.CompletedAt, lastActivity(e))
	if n, ok := s.ticketNeedsYou(root, e, fallback); ok {
		return n, true
	}
	switch {
	case state == EpicListMergeBlocked:
		n := needsYou{
			Pill: PillMergeBlocked, Reason: e.MergeBlockReason, Since: fallback,
			Action: CardAction{Kind: CardActionMerge, Label: "Merge", Target: root.String()},
		}
		if e.MergeBlockKind == string(schema.PauseDirtyTrunk) {
			n.Reason = schema.PauseDirtyTrunk.Label()
			n.Action = CardAction{Kind: CardActionStashMerge, Label: schema.VerbStashMerge.Label(), Target: root.String()}
			n.More = []CardAction{{Kind: CardActionMerge, Label: schema.VerbMergeAgain.Label(), Target: root.String()}}
		}
		return n, true
	case state == EpicListPaused && epicStateOf(e) == epicCycle:
		return needsYou{
			Pill: PillDeadlocked, Reason: cycleReason(root, e), Since: fallback,
			Action: CardAction{Kind: CardActionOpenEpic, Label: "Open epic", Target: root.String()},
		}, true
	case state == EpicListAwaitingMerge && !s.autoFFMerge(root.Project):
		return needsYou{
			Pill: PillAwaitingMerge, Reason: "Auto-merge is off for " + root.Project, Since: fallback,
			Action: CardAction{Kind: CardActionMerge, Label: "Merge", Target: root.String()},
		}, true
	}
	return needsYou{}, false
}

// ticketNeedsYou picks the oldest wait among e's asking, permission-blocked and
// paused tickets. A wait with no time of its own starts at fallback.
func (s *Server) ticketNeedsYou(root rootRef, e tickets.Epic, fallback time.Time) (needsYou, bool) {
	var waits []ticketWait
	for _, t := range e.Tickets {
		if n, ok := s.ticketWaitOf(root, e, t); ok {
			waits = append(waits, ticketWait{n, t})
		}
	}
	if len(waits) == 0 {
		return needsYou{}, false
	}
	for i := range waits {
		if waits[i].Since.IsZero() {
			waits[i].Since = fallback
		}
	}
	slices.SortStableFunc(waits, func(a, b ticketWait) int { return a.Since.Compare(b.Since) })
	n := waits[0].needsYou
	if more := len(waits) - 1; more > 0 {
		n.Reason += fmt.Sprintf(" (+%d more)", more)
	}
	return n, true
}

// ticketWaitOf says whether t is paused, asking or blocked on a permission. The
// wait's Since is zero when the log holds no time for it.
func (s *Server) ticketWaitOf(root rootRef, e tickets.Epic, t tickets.Ticket) (needsYou, bool) {
	addr := tickets.Address{Project: root.Project, Epic: root.Epic, ID: t.DisplayNumber()}
	status := e.RenderedStatus(t)
	label := t.DisplayNumber() + " " + t.Title
	switch {
	case isPaused(e, t) && status != tickets.StatusDraft:
		evs := readLogLenient(t)
		n := needsYou{
			Pill: PillPaused, Reason: label + ": " + tickets.PauseDetail(e, t),
			Action: CardAction{Kind: CardActionOpenTicket, Label: "Open ticket", Target: addr.String()},
		}
		if rounds := tickets.AllRounds(evs); len(rounds) > 0 {
			n.Since = rounds[len(rounds)-1].Question.TS
		}
		if open := tickets.OpenQuestion(evs); status == tickets.StatusPausedAnswer && open != nil {
			n.Pill, n.Reason = PillQuestion, label+": "+open.Summary
			n.Action = CardAction{Kind: CardActionAnswer, Label: "Answer", Target: addr.String()}
		}
		return n, true
	case status == tickets.StatusClaimed:
		card, err := s.permissionCard(addr)
		if err != nil || card == nil {
			return needsYou{}, false
		}
		return needsYou{
			Pill: PillPermission, Reason: label + " wants to run: " + cmp.Or(card.Description, card.Tool),
			Since:  permissionSince(readLogLenient(t)),
			Action: CardAction{Kind: CardActionAnswer, Label: "Answer", Target: addr.String()},
		}, true
	}
	return needsYou{}, false
}

// permissionSince is the time of the newest permission request in a ticket's log.
func permissionSince(evs []tickets.LogEvent) time.Time {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == tickets.EventPausePermission {
			return evs[i].TS
		}
	}
	return time.Time{}
}

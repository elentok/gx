package server

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// designPercent is the Designing progress in percent: the steps passed plus the
// closed map tickets, over the steps from Chart to Branch plus the map tickets,
// less one. It can go down when tickets are added.
func designPercent(m tickets.Epic, step ProgressStep) int {
	closed := 0
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusDone) {
			closed++
		}
	}
	total := len(tickets.DesignSteps) - 1 + len(m.Tickets) - 1
	if total <= 0 {
		return 0
	}
	return min(100, (slices.Index(tickets.DesignSteps, step)+closed)*100/total)
}

// reachedBranch reports whether a map's design is at Branch or later, which is
// when a joined card shows the -impl epic.
func reachedBranch(step ProgressStep) bool {
	return slices.Index(tickets.DesignSteps, step) >= slices.Index(tickets.DesignSteps, ProgressBranch)
}

// decorateDesignCard fills the card fields of a map row. impl is the card row of
// the map's -impl epic, or nil when the map has none. A joined card lists the
// active part's address first.
func decorateDesignCard(row *EpicListRow, m tickets.Epic, impl *EpicListRow, act designActivity) {
	row.CardKind = CardKindDesign
	row.Addresses = []string{row.Address}
	if impl != nil {
		row.Addresses = append(row.Addresses, impl.Address)
	}
	switch {
	case impl != nil && reachedBranch(row.ProgressStep):
		joinImplementation(row, impl, act)
	case row.ProgressStep == ProgressDone:
		row.Section = SectionFinished
		row.StatusWords = "Designed"
		row.RingProgress = 100
	default:
		row.RingProgress = designPercent(m, row.ProgressStep)
		fallback := cmp.Or(lastActivity(m), m.CreatedAt)
		placeDesign(row, act, mapNotStarted(m, row.ProgressStep), fallback)
		if row.ProgressStep == ProgressCreateTickets && impl != nil && row.StatusWords == "Designing" {
			// The ring has no fraction to show yet; the client draws its placeholder.
			row.RingProgress = 0
			row.StatusWords = fmt.Sprintf("Creating tickets (%d so far)...", impl.Total)
		}
	}
}

// joinImplementation turns a map row into the Implementation card of the joined
// pair: the -impl epic sets state, section and progress. A Design question still
// waiting on an answer wins the card for Needs you.
func joinImplementation(row *EpicListRow, impl *EpicListRow, act designActivity) {
	row.CardKind = impl.CardKind
	row.State, row.Done, row.Total = impl.State, impl.Done, impl.Total
	row.Queued, row.Queueable = impl.Queued, impl.Queueable
	row.MergedAt, row.BlockedByEpics = impl.MergedAt, impl.BlockedByEpics
	row.MergeBlockKind, row.MergeBlockReason = impl.MergeBlockKind, impl.MergeBlockReason
	row.Section, row.StatusWords, row.RingProgress = impl.Section, impl.StatusWords, impl.RingProgress
	row.Reason, row.StatePill, row.WaitingSince = impl.Reason, impl.StatePill, impl.WaitingSince
	row.Actions = make([]CardAction, len(impl.Actions))
	for i, a := range impl.Actions {
		a.Target = cmp.Or(a.Target, impl.Address)
		row.Actions[i] = a
	}
	row.Addresses = []string{impl.Address, row.Address}
	if impl.Section == SectionNeedsYou {
		return
	}
	var design EpicListRow
	design.Address = row.Address
	placeDesign(&design, act, false, act.askedAt)
	if design.StatePill == PillQuestion {
		row.Section, row.StatePill, row.Reason = SectionNeedsYou, design.StatePill, design.Reason
		row.WaitingSince, row.Actions = design.WaitingSince, design.Actions
		row.Addresses = []string{row.Address, impl.Address}
	}
}

// mapPendingDesignRow is the card of a Design session that runs before its map
// exists, keyed and titled by the session's epic name.
func mapPendingDesignRow(project, epic string, act designActivity) EpicListRow {
	row := EpicListRow{
		Address: rootRef{Project: project, Epic: epic}.String(), Project: project, Name: epic,
		Kind: EpicKindMap, State: EpicListDesigning, MapPending: true,
		CardKind: CardKindDesign, BlockedByEpics: []string{},
	}
	row.Addresses = []string{row.Address}
	placeDesign(&row, act, false, act.endedAt)
	return row
}

// placeDesign sets the section, status, pill, reason and button of a Design card
// from its sessions. fallback starts a Stopped wait when no session ended.
func placeDesign(row *EpicListRow, act designActivity, notStarted bool, fallback time.Time) {
	target := row.Address
	row.StatusWords = "Designing"
	switch {
	case !act.running && notStarted:
		row.Section = SectionReadyToDesign
		row.StatusWords = "Not started"
		row.Actions = []CardAction{{Kind: CardActionDesign, Label: "Design", Target: target}}
	case !act.running:
		row.Section = SectionNeedsYou
		row.StatePill = "Stopped"
		row.Reason = "No design session is running"
		row.WaitingSince = cmp.Or(act.endedAt, fallback)
		row.Actions = []CardAction{{Kind: CardActionContinue, Label: "Continue", Target: target}}
	case !act.countdownAt.IsZero():
		row.Section = SectionInProgress
		row.CountdownEndsAt = act.countdownAt
	case act.asking:
		row.Section = SectionNeedsYou
		row.StatePill = PillQuestion
		row.Reason = "The design session waits on your answer"
		row.WaitingSince = cmp.Or(act.askedAt, fallback)
		row.Actions = []CardAction{{Kind: CardActionAnswer, Label: "Answer", Target: target}}
	default:
		row.Section = SectionInProgress
		row.Actions = []CardAction{{Kind: CardActionOpen, Label: "Open", Target: target}}
	}
}

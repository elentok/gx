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

// decorateDesignCard fills the card fields of a map row.
func decorateDesignCard(row *EpicListRow, m tickets.Epic, byName map[string]tickets.Epic, act designActivity) {
	row.CardKind = CardKindDesign
	row.Addresses = []string{row.Address}
	impl, hasImpl := byName[m.Name+tickets.ImplEpicSuffix]
	switch {
	case row.ProgressStep == ProgressDone && !hasImpl:
		row.Section = SectionFinished
		row.StatusWords = "Designed"
		row.RingProgress = 100
	case row.ProgressStep == ProgressDone:
		// The joined card with the -impl epic is built in the next ticket.
	default:
		row.RingProgress = designPercent(m, row.ProgressStep)
		fallback := cmp.Or(lastActivity(m), m.CreatedAt)
		placeDesign(row, act, mapNotStarted(m, row.ProgressStep), fallback)
		if row.ProgressStep == ProgressCreateTickets && hasImpl && row.StatusWords == "Designing" {
			row.StatusWords = fmt.Sprintf("Creating tickets (%d so far)...", len(impl.Tickets))
		}
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

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
// less one. It can go down when tickets are added. A finished Design is 100.
func designPercent(m tickets.Epic, step ProgressStep) int {
	if step == ProgressDone {
		return 100
	}
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

// designSection is the section of the map's own Design card, whether or not a
// joined card currently shows the -impl epic instead. A finished Design only
// stays in Needs you while a question is still waiting on an answer.
func designSection(address string, m tickets.Epic, step ProgressStep, act designActivity) CardSection {
	p := placeDesign(address, act, mapNotStarted(m, step), time.Time{})
	if step == ProgressDone && p.section != SectionNeedsYou {
		return SectionFinished
	}
	return p.section
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
		// The impl epic's time pair carries over; joinImplementation resets it on a question.
		joinImplementation(row, impl, act)
		return
	case row.ProgressStep == ProgressDone:
		row.Section = SectionFinished
		row.StatusWords = "Designed"
		row.RingProgress = designPercent(m, row.ProgressStep)
	default:
		row.RingProgress = designPercent(m, row.ProgressStep)
		fallback := cmp.Or(lastActivity(m), m.CreatedAt)
		placeDesign(row.Address, act, mapNotStarted(m, row.ProgressStep), fallback).applyTo(row)
		if row.ProgressStep == ProgressCreateTickets && impl != nil && row.StatusWords == "Designing" {
			// The ring has no fraction to show yet; the client draws its placeholder.
			row.RingProgress = 0
			row.StatusWords = fmt.Sprintf("Creating tickets (%d so far)...", impl.Total)
		}
	}
	row.setCardTime(cardTimeSources{startedAt: m.DesignStartedAt})
}

// joinImplementation turns a map row into the Implementation card of the joined
// pair: the -impl epic sets state, section and progress. A Design question still
// waiting on an answer wins the card for Needs you.
func joinImplementation(row *EpicListRow, impl *EpicListRow, act designActivity) {
	m := *row
	*row = *impl
	row.Address, row.Name, row.Kind, row.ProgressStep = m.Address, m.Name, m.Kind, m.ProgressStep
	// The map keeps its own timestamps; the queue slot stays the impl epic's.
	row.CreatedAt, row.CompletedAt, row.QueuedAt, row.LastActivityAt = m.CreatedAt, m.CompletedAt, m.QueuedAt, m.LastActivityAt
	row.Actions = make([]CardAction, len(impl.Actions))
	for i, a := range impl.Actions {
		a.Target = cmp.Or(a.Target, impl.Address)
		row.Actions[i] = a
	}
	row.Addresses = []string{impl.Address, m.Address}
	if impl.Section == SectionNeedsYou {
		return
	}
	if p := placeDesign(m.Address, act, false, act.askedAt); p.pill == PillQuestion {
		row.Section, row.StatePill, row.Reason = SectionNeedsYou, p.pill, p.reason
		row.WaitingSince, row.Actions = p.waitingSince, p.actions
		row.setCardTime(cardTimeSources{})
		row.Addresses = []string{m.Address, impl.Address}
	}
}

func (p designPlacement) applyTo(row *EpicListRow) {
	row.Section, row.StatusWords, row.StatePill, row.Reason = p.section, p.statusWords, p.pill, p.reason
	row.WaitingSince, row.CountdownEndsAt, row.Actions = p.waitingSince, p.countdownEndsAt, p.actions
}

// designPlacement is where a Design card sits and what it shows.
type designPlacement struct {
	section         CardSection
	statusWords     string
	pill            string
	reason          string
	waitingSince    time.Time
	countdownEndsAt time.Time
	actions         []CardAction
}

// placeDesign places the Design card of the map at address from its sessions.
// fallback starts a question's wait when the session holds no time for it.
func placeDesign(address string, act designActivity, notStarted bool, fallback time.Time) designPlacement {
	p := designPlacement{statusWords: "Designing"}
	switch {
	case !act.running && notStarted:
		p.section = SectionReadyToDesign
		p.statusWords = "Not started"
		p.actions = []CardAction{{Kind: CardActionDesign, Label: "Design", Target: address}}
	case !act.running:
		// Nothing waits on the person: the Design is only paused until they continue it.
		p.section = SectionInProgress
		p.reason = "No design session is running"
		p.actions = []CardAction{{Kind: CardActionContinue, Label: "Continue", Target: address}}
	case !act.countdownAt.IsZero():
		p.section = SectionInProgress
		p.countdownEndsAt = act.countdownAt
	case act.asking:
		p.section = SectionNeedsYou
		p.pill = PillQuestion
		p.reason = "The design session waits on your answer"
		p.waitingSince = cmp.Or(act.askedAt, fallback)
		p.actions = []CardAction{{Kind: CardActionAnswer, Label: "Answer", Target: address, Link: "#/d/" + address}}
	default:
		p.section = SectionInProgress
		p.actions = []CardAction{{Kind: CardActionOpen, Label: "Open", Target: address}}
	}
	return p
}

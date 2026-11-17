package epics

import (
	"cmp"
	"fmt"
	"github.com/elentok/gx/server/wire"
	"slices"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// designPercent is the Designing progress in percent: the steps passed plus the
// closed map tickets, over the steps from Chart to Branch plus the map tickets,
// less one. It can go down when tickets are added. A finished Design is 100.
func designPercent(m tickets.Epic, step wire.ProgressStep) int {
	if step == wire.ProgressDone {
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
	return min(100, (slices.Index(designProgressSteps, step)+closed)*100/total)
}

// reachedBranch reports whether a map's design is at Branch or later, which is
// when a joined card shows the -impl epic.
func reachedBranch(step wire.ProgressStep) bool {
	return slices.Index(designProgressSteps, step) >= slices.Index(designProgressSteps, wire.ProgressBranch)
}

// designSection is the section of the map's own Design card, whether or not a
// joined card currently shows the -impl epic instead. A finished Design only
// stays in Needs you while a question is still waiting on an answer.
func designSection(address string, m tickets.Epic, step wire.ProgressStep, act designActivity) wire.CardSection {
	p := placeDesign(address, act, mapNotStarted(m, step), time.Time{}, step)
	if step == wire.ProgressDone && p.section != wire.SectionNeedsYou {
		return wire.SectionFinished
	}
	return p.section
}

// decorateDesignCard fills the card fields of a map row. impl is the card row of
// the map's -impl epic, or nil when the map has none. A joined card lists the
// active part's address first.
func decorateDesignCard(row *wire.EpicListRow, m tickets.Epic, impl *wire.EpicListRow, act designActivity) {
	row.CardKind = wire.CardKindDesign
	row.Addresses = []string{row.Address}
	if impl != nil {
		row.Addresses = append(row.Addresses, impl.Address)
	}
	switch {
	case impl != nil && reachedBranch(row.ProgressStep):
		// The impl epic's time pair carries over; joinImplementation resets it on a question.
		joinImplementation(row, impl, act)
		return
	case row.ProgressStep == wire.ProgressDone:
		row.Section = wire.SectionFinished
		row.StatusWords = "Designed"
		row.RingProgress = designPercent(m, row.ProgressStep)
	default:
		row.RingProgress = designPercent(m, row.ProgressStep)
		fallback := cmp.Or(lastActivity(m), m.CreatedAt)
		act.creating = impl != nil && row.ProgressStep == wire.ProgressCreateTickets
		placeDesign(row.Address, act, mapNotStarted(m, row.ProgressStep), fallback, row.ProgressStep).applyTo(row)
		if row.Working && act.creating {
			// The ring has no fraction to show yet; the client draws its placeholder.
			row.RingProgress = 0
			row.WorkingWord = designWords[wire.ProgressCreateTickets]
			row.StatusWords = fmt.Sprintf("Creating tickets (%d so far)...", impl.Total)
		}
	}
	setCardTime(row, cardTimeSources{startedAt: m.DesignStartedAt})
}

// joinImplementation turns a map row into the Implementation card of the joined
// pair: the -impl epic sets state, section and progress. A Design question still
// waiting on an answer wins the card for Needs you.
func joinImplementation(row *wire.EpicListRow, impl *wire.EpicListRow, act designActivity) {
	m := *row
	*row = *impl
	row.Address, row.Name, row.Kind, row.ProgressStep = m.Address, m.Name, m.Kind, m.ProgressStep
	// The map keeps its own timestamps; the queue slot stays the impl epic's.
	row.CreatedAt, row.CompletedAt, row.QueuedAt, row.LastActivityAt = m.CreatedAt, m.CompletedAt, m.QueuedAt, m.LastActivityAt
	row.Actions = make([]wire.CardAction, len(impl.Actions))
	for i, a := range impl.Actions {
		a.Target = cmp.Or(a.Target, impl.Address)
		row.Actions[i] = a
	}
	row.Addresses = []string{impl.Address, m.Address}
	if impl.Section == wire.SectionNeedsYou {
		return
	}
	if p := placeDesign(m.Address, act, false, act.askedAt, m.ProgressStep); p.pill == PillQuestion {
		row.Section, row.StatePill, row.Reason = wire.SectionNeedsYou, p.pill, p.reason
		row.WaitingSince, row.Actions = p.waitingSince, p.actions
		setCardTime(row, cardTimeSources{})
		row.Addresses = []string{m.Address, impl.Address}
		return
	}
	// At Branch the Design session still works while the -impl epic does not:
	// the joined card shows the Design state. A countdown wins over working.
	if p := placeDesign(m.Address, act, false, act.askedAt, m.ProgressStep); m.ProgressStep == wire.ProgressBranch &&
		!impl.Working && p.working && p.countdownEndsAt.IsZero() {
		row.Working, row.WorkKind = true, wire.WorkKindDesign
		row.WorkingWord = designWords[wire.ProgressBranch]
		row.StatusWords = row.WorkingWord + "..."
	}
}

func (p designPlacement) applyTo(row *wire.EpicListRow) {
	row.Section, row.StatusWords, row.StatePill, row.Reason = p.section, p.statusWords, p.pill, p.reason
	row.WaitingSince, row.CountdownEndsAt, row.Actions = p.waitingSince, p.countdownEndsAt, p.actions
	row.Working, row.WorkingWord, row.WorkKind = p.working, p.workingWord, ""
	if p.working {
		row.WorkKind = wire.WorkKindDesign
	}
}

// designWords is the bare working word of each Design step.
var designWords = map[wire.ProgressStep]string{
	wire.ProgressChart:          "Charting",
	wire.ProgressResolveTickets: "Researching",
	wire.ProgressSpec:           "Writing spec",
	wire.ProgressReview:         "Reviewing",
	wire.ProgressCreateTickets:  "Creating tickets",
	wire.ProgressBranch:         "Branching",
}

// reportedStepSteps maps a session's reported step to the Design step it names.
var reportedStepSteps = map[string]wire.ProgressStep{
	"chart": wire.ProgressChart, "resolve-tickets": wire.ProgressResolveTickets,
	"spec": wire.ProgressSpec, "review": wire.ProgressReview, "grill-findings": wire.ProgressReview,
	"create-tickets": wire.ProgressCreateTickets, "branch": wire.ProgressBranch,
}

// designWorkingWord is the bare word of a working Design: the working session's
// reported step, then the map's own step, then "Designing".
func designWorkingWord(act designActivity, step wire.ProgressStep) string {
	if w, ok := designWords[reportedStepSteps[act.reportedStep]]; ok {
		return w
	}
	return cmp.Or(designWords[step], "Designing")
}

// designPlacement is where a Design card sits and what it shows. working is set
// with workingWord (bare); statusWords then ends with "...".
type designPlacement struct {
	section         wire.CardSection
	statusWords     string
	working         bool
	workingWord     string
	pill            string
	reason          string
	waitingSince    time.Time
	countdownEndsAt time.Time
	actions         []wire.CardAction
}

// placeDesign places the Design card of the map at address from its sessions.
// fallback starts a question's wait when the session holds no time for it. step
// is the map's own step, which names the working word when no step is reported.
func placeDesign(address string, act designActivity, notStarted bool, fallback time.Time, step wire.ProgressStep) designPlacement {
	p := designPlacement{statusWords: "Designing"}
	working := func() {
		p.working = true
		p.workingWord = designWorkingWord(act, step)
		if act.creating {
			p.workingWord = designWords[wire.ProgressCreateTickets]
		}
		p.statusWords = p.workingWord + "..."
	}
	switch {
	case !act.running && notStarted:
		p.section = wire.SectionReadyToDesign
		p.statusWords = "Not started"
		p.actions = []wire.CardAction{{Kind: wire.CardActionDesign, Label: "Design", Target: address}}
	case !act.running:
		// Nothing waits on the person: the Design is only paused until they continue it.
		p.section = wire.SectionDesigning
		p.reason = "No design session is running"
		p.actions = []wire.CardAction{{Kind: wire.CardActionContinue, Label: "Continue", Target: address}}
	case !act.countdownAt.IsZero():
		p.section = wire.SectionDesigning
		p.countdownEndsAt = act.countdownAt
		if act.creating {
			working()
		}
	case act.asking:
		p.section = wire.SectionNeedsYou
		p.pill = PillQuestion
		p.reason = "The design session waits on your answer"
		p.waitingSince = cmp.Or(act.askedAt, fallback)
		p.actions = []wire.CardAction{{Kind: wire.CardActionAnswer, Label: "Answer", Target: address, Link: "#/d/" + address}}
	default:
		p.section = wire.SectionDesigning
		if act.working || act.creating {
			working()
		}
		p.actions = []wire.CardAction{{Kind: wire.CardActionOpen, Label: "Open", Target: address}}
	}
	return p
}

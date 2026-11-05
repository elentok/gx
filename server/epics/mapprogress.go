package epics

import (
	"fmt"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// progressLabels is the label of each Progress step.
var progressLabels = map[wire.ProgressStep]string{
	wire.ProgressChart:          "Chart",
	wire.ProgressResolveTickets: "Resolve tickets",
	wire.ProgressSpec:           "Spec",
	wire.ProgressReview:         "Review",
	wire.ProgressGrillFindings:  "Grill findings",
	wire.ProgressCreateTickets:  "Create tickets",
	wire.ProgressBranch:         "Branch",
	wire.ProgressDone:           "Done",
}

// mapSteps lists all Progress steps with their state. The steps before the
// current one are done, and all of them are once the map is Done.
func mapSteps(m tickets.Epic, byName map[string]tickets.Epic) []wire.MapStep {
	current := queue.MapProgress(m)
	steps := make([]wire.MapStep, 0, len(tickets.DesignSteps))
	state := wire.MapStepDone
	for _, step := range tickets.DesignSteps {
		label := progressLabels[step]
		switch {
		case step == current && current == wire.ProgressDone:
		case step == current:
			state = wire.MapStepCurrent
			label += currentStepNote(m, byName, current)
		case state == wire.MapStepCurrent:
			state = wire.MapStepTodo
		}
		steps = append(steps, wire.MapStep{Step: step, Label: label, State: state})
	}
	return steps
}

// currentStepNote is the suffix of the current step's label.
func currentStepNote(m tickets.Epic, byName map[string]tickets.Epic, step wire.ProgressStep) string {
	switch step {
	case wire.ProgressChart:
		return " · confirm destination"
	case wire.ProgressResolveTickets:
		if !tickets.HasOpenTicket(m) && schema.Section(m.Body, "Not yet specified") != "" {
			return " · fog left"
		}
	case wire.ProgressGrillFindings:
		checked, total := findingCounts(schema.Section(m.Body, "Review findings"))
		return fmt.Sprintf(" · %d of %d", checked, total)
	case wire.ProgressCreateTickets:
		if impl, ok := byName[m.Name+tickets.ImplEpicSuffix]; ok && impl.Status == string(schema.StatusDraft) {
			n := len(impl.Tickets)
			if n == 1 {
				return " · 1 ticket published"
			}
			return fmt.Sprintf(" · %d tickets published", n)
		}
	}
	return ""
}

// mapTicketFlags marks m's current ticket (the claimed one, else the lowest
// open ticket with no unresolved blocker) and the open tickets that are blocked.
func mapTicketFlags(m tickets.Epic) (current tickets.Ticket, blocked map[string]bool) {
	blocked = map[string]bool{}
	var claimed, frontier *tickets.Ticket
	for i, t := range m.Tickets {
		switch {
		case t.Status == string(schema.StatusClaimed) && (claimed == nil || t.Number < claimed.Number):
			claimed = &m.Tickets[i]
		case t.Status != string(schema.StatusOpen):
		case len(m.UnresolvedBlockers(t)) > 0:
			blocked[t.DisplayNumber()] = true
		case frontier == nil || t.Number < frontier.Number:
			frontier = &m.Tickets[i]
		}
	}
	switch {
	case claimed != nil:
		return *claimed, blocked
	case frontier != nil:
		return *frontier, blocked
	}
	return tickets.Ticket{}, blocked
}

// findingCounts counts the checked and all "- [ ]" or "- [x]" lines of a findings section.
func findingCounts(section string) (checked, total int) {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "- [ ]"):
			total++
		case strings.HasPrefix(line, "- [x]"), strings.HasPrefix(line, "- [X]"):
			checked++
			total++
		}
	}
	return checked, total
}

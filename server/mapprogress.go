package server

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// mapProgress is a map's current Progress step. It comes from the epic's
// design_step alone: status done always shows Done, and a missing field shows
// Chart. It never looks at files or branch refs.
func mapProgress(m tickets.Epic) ProgressStep {
	if m.Status == string(schema.StatusDone) {
		return ProgressDone
	}
	if m.DesignStep == "" {
		return ProgressChart
	}
	return ProgressStep(m.DesignStep)
}

// progressSteps lists the Progress steps in order.
var progressSteps = []struct {
	step  ProgressStep
	label string
}{
	{ProgressChart, "Chart"},
	{ProgressResolveTickets, "Resolve tickets"},
	{ProgressSpec, "Spec"},
	{ProgressReview, "Review"},
	{ProgressGrillFindings, "Grill findings"},
	{ProgressCreateTickets, "Create tickets"},
	{ProgressBranch, "Branch"},
	{ProgressDone, "Done"},
}

// mapSteps lists all Progress steps with their state. The steps before the
// current one are done, and all of them are once the map is Done.
func mapSteps(m tickets.Epic, byName map[string]tickets.Epic) []MapStep {
	current := mapProgress(m)
	steps := make([]MapStep, 0, len(progressSteps))
	state := MapStepDone
	for _, s := range progressSteps {
		label := s.label
		switch {
		case s.step == current && current == ProgressDone:
		case s.step == current:
			state = MapStepCurrent
			label += currentStepNote(m, byName, current)
		case state == MapStepCurrent:
			state = MapStepTodo
		}
		steps = append(steps, MapStep{Step: s.step, Label: label, State: state})
	}
	return steps
}

// currentStepNote is the suffix of the current step's label.
func currentStepNote(m tickets.Epic, byName map[string]tickets.Epic, step ProgressStep) string {
	switch step {
	case ProgressChart:
		return " · confirm destination"
	case ProgressResolveTickets:
		if !hasOpenTicket(m) && schema.Section(m.Body, "Not yet specified") != "" {
			return " · fog left"
		}
	case ProgressGrillFindings:
		checked, total := findingCounts(schema.Section(m.Body, "Review findings"))
		return fmt.Sprintf(" · %d of %d", checked, total)
	case ProgressCreateTickets:
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

// hasOpenTicket reports whether any of m's tickets is open or claimed.
func hasOpenTicket(m tickets.Epic) bool {
	for _, t := range m.Tickets {
		if t.Status == string(schema.StatusOpen) || t.Status == string(schema.StatusClaimed) {
			return true
		}
	}
	return false
}

// findingsChecked reports whether every "- [ ]" or "- [x]" line of a findings
// section is checked.
func findingsChecked(section string) bool {
	checked, total := findingCounts(section)
	return checked == total
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

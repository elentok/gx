package triage

import (
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// OldEpicNote is the Notes text of an area epic an earlier Triage made.
const OldEpicNote = "Triaged from follow-ups"

// IsOldEpic: a map, still charting, whose Notes say it was triaged from
// follow-ups.
func IsOldEpic(e tickets.Epic) bool {
	step := schema.DesignStep(e.DesignStep)
	return e.IsMap && (step == "" || step == schema.StepChart) &&
		strings.Contains(schema.Section(e.Body, "Notes"), OldEpicNote)
}

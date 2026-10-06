package tickets

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/tickets/schema"
)

// ValidateProject loads every epic under projectDir and reports each ticket's
// problems at once: its own frontmatter (every ticket, so archived history is
// still well-formed) plus the cross-ticket checks — parent edge, blocked_by
// refs and wait cycles. Done and cancelled tickets skip the cross-ticket
// checks only: their blockers may legitimately be gone.
func ValidateProject(projectDir string) error {
	epics, err := Load(projectDir)
	if err != nil {
		return err
	}
	graph := newProjectGraph(ProjectName(projectDir), epics)
	var errs []error
	for _, epic := range epics {
		for _, t := range epic.Tickets {
			if _, err := schema.ParseTicket(t.Path); err != nil {
				errs = append(errs, err)
				continue
			}
			if t.IsTerminal() {
				continue
			}
			if t.GraphErr != "" {
				errs = append(errs, fmt.Errorf("%s: %s", t.Path, t.GraphErr))
			}
			for _, err := range append(graph.checkBlockedBy(epic.Name, t), graph.cycleErrors(epic.Name, t)...) {
				errs = append(errs, fmt.Errorf("%s: %w", t.Path, err))
			}
		}
	}
	return errors.Join(errs...)
}

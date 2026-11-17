package ticketmigrate

import (
	"os"
	"regexp"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// legacyPaneBusy matches the raw `pause_reason: agent_pane_busy` line. It
// matches text because the reader already maps the value to iteration-error.
var legacyPaneBusy = regexp.MustCompile(`(?m)^pause_reason:[ \t]*["']?agent_pane_busy["']?[ \t]*$`)

// MigratePaneBusyToIterationError rewrites `pause_reason: agent_pane_busy` to
// iteration-error on every ticket of the project at root, live and archived. A
// rerun changes nothing.
func MigratePaneBusyToIterationError(root string) ([]tickets.MigrationChange, error) {
	return backfillEpics(root, func(epic *tickets.Epic) ([]tickets.MigrationChange, error) {
		var changes []tickets.MigrationChange
		for _, t := range epic.Tickets {
			path := t.FilePath()
			data, err := os.ReadFile(path)
			if err != nil || !legacyPaneBusy.Match(frontMatter(data)) {
				continue
			}
			err = schema.UpdateTicketInner(path, func(t *schema.Ticket) {
				t.PauseReason = schema.PauseIterationError
			})
			if err != nil {
				return changes, err
			}
			changes = append(changes, tickets.MigrationChange{Path: path, Notes: []string{"pause_reason agent_pane_busy renamed to iteration-error"}})
		}
		return changes, nil
	})
}

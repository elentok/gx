package notify

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/ralphloop/schedule"
)

// pausedIdentifierCap bounds how many paused identifiers a counts line lists
// inline before falling back to an overflow marker — enough to be useful
// without a pause-heavy epic turning one chat message into a wall of ids.
const pausedIdentifierCap = 5

// RenderCountsLine renders c in the fixed bucket order done, in progress,
// paused, blocked, ready, total. Every bucket but done/total is suppressed
// at zero, so a line only names what's materially true rather than
// repeating "0 blocked" on every message. This is a pure function of c —
// the formatting decisions live here, not in any sink — which is what makes
// it table-testable independent of chat transport or epic state.
func RenderCountsLine(c schedule.EpicCounts) string {
	clauses := []string{fmt.Sprintf("%d done", c.Done)}
	if c.InProgress > 0 {
		clauses = append(clauses, fmt.Sprintf("%d in progress", c.InProgress))
	}
	if len(c.PausedIdentifiers) > 0 {
		clauses = append(clauses, fmt.Sprintf("%d paused: %s", len(c.PausedIdentifiers), joinPausedIdentifiers(c.PausedIdentifiers)))
	}
	if c.Blocked > 0 {
		clauses = append(clauses, fmt.Sprintf("%d blocked", c.Blocked))
	}
	if c.Ready > 0 {
		clauses = append(clauses, fmt.Sprintf("%d ready", c.Ready))
	}
	clauses = append(clauses, fmt.Sprintf("%d total", c.Total))
	return strings.Join(clauses, " · ")
}

// joinPausedIdentifiers lists identifiers inline, capped at
// pausedIdentifierCap: beyond that, the rest collapse into a single "+N
// more" marker instead of growing the line without bound.
func joinPausedIdentifiers(identifiers []string) string {
	if len(identifiers) <= pausedIdentifierCap {
		return strings.Join(identifiers, ", ")
	}
	shown := strings.Join(identifiers[:pausedIdentifierCap], ", ")
	return fmt.Sprintf("%s, +%d more", shown, len(identifiers)-pausedIdentifierCap)
}

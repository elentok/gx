package tickets

import "strings"

// RenderedStatus is the tickets tab's collapse of the tracker's raw Status:/
// triage-label vocabulary into a small set of user-facing states, plus a
// sixth "error" state for any value none of the others recognize.
type RenderedStatus int

const (
	StatusOpen RenderedStatus = iota
	StatusClaimed
	StatusBlocked
	StatusNeedsInfo
	StatusDone
	StatusError
)

// openStatuses covers raw Status: values meaning "unclaimed, nothing external
// blocks picking it up": a missing Status:, needs-triage (nobody has
// evaluated it yet), and the ready-for-agent/ready-for-human triage labels
// (see .ai's triage-labels skill), which don't distinguish who picks it up.
var openStatuses = map[string]bool{
	"":                true,
	"open":            true,
	"needs-triage":    true,
	"ready-for-agent": true,
	"ready-for-human": true,
}

var claimedStatuses = map[string]bool{"claimed": true}

// needsInfoStatuses covers raw values meaning work is stalled on someone
// providing more information before it can proceed.
var needsInfoStatuses = map[string]bool{
	"needs-info": true,
}

// baseStatus classifies t's raw Status: value alone, before the Blocked by:
// overlay (see Epic.RenderedStatus) is applied.
func (t Ticket) baseStatus() RenderedStatus {
	status := strings.ToLower(strings.TrimSpace(t.Status))
	switch {
	case doneStatuses[status]:
		return StatusDone
	case claimedStatuses[status]:
		return StatusClaimed
	case needsInfoStatuses[status]:
		return StatusNeedsInfo
	case openStatuses[status]:
		return StatusOpen
	default:
		return StatusError
	}
}

// RenderedStatus computes t's rendered status within e: t's base status,
// overlaid with "blocked" when t has an unresolved Blocked by: and its base
// status is open or claimed (needs-info and done tickets keep their own
// state regardless of Blocked by:).
func (e Epic) RenderedStatus(t Ticket) RenderedStatus {
	base := t.baseStatus()
	if (base == StatusOpen || base == StatusClaimed) && len(e.UnresolvedBlockers(t)) > 0 {
		return StatusBlocked
	}
	return base
}

// UnresolvedBlockers returns t's Blocked by: numbers that are not yet done
// within e, in Blocked by: order. A blocker number with no matching ticket
// in e counts as unresolved (it can't be verified done).
func (e Epic) UnresolvedBlockers(t Ticket) []int {
	if len(t.BlockedBy) == 0 {
		return nil
	}
	done := make(map[int]bool, len(e.Tickets))
	for _, other := range e.Tickets {
		if other.IsDone() {
			done[other.Number] = true
		}
	}
	var unresolved []int
	for _, n := range t.BlockedBy {
		if !done[n] {
			unresolved = append(unresolved, n)
		}
	}
	return unresolved
}

// Word renders s as the status word shown in the ticket preview panel's
// metadata line.
func (s RenderedStatus) Word() string {
	switch s {
	case StatusOpen:
		return "open"
	case StatusClaimed:
		return "claimed"
	case StatusBlocked:
		return "blocked"
	case StatusNeedsInfo:
		return "needs-info"
	case StatusDone:
		return "done"
	default: // StatusError
		return "error"
	}
}

// GroupOrder returns s's sort rank for grouping tickets within an epic:
// unblocked (open/claimed) → blocked → needs-info → done → error.
func GroupOrder(s RenderedStatus) int {
	switch s {
	case StatusOpen, StatusClaimed:
		return 0
	case StatusBlocked:
		return 1
	case StatusNeedsInfo:
		return 2
	case StatusDone:
		return 3
	default: // StatusError
		return 4
	}
}

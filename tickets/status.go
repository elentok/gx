package tickets

import "strings"

// RenderedStatus is the tickets tab's collapse of the tracker's raw Status:/
// triage-label vocabulary into a small set of user-facing states, plus a
// seventh "error" state for any value none of the others recognize.
type RenderedStatus int

const (
	StatusOpen RenderedStatus = iota
	StatusClaimed
	StatusBlocked
	StatusNeedsInfo
	StatusNeedsAttention
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

var needsAttentionStatuses = map[string]bool{
	"needs-attention": true,
}

// baseStatus classifies t's raw Status: value alone, before the Blocked by:
// overlay (see Epic.RenderedStatus) is applied.
func (t Ticket) baseStatus() RenderedStatus {
	if t.ReadErr != "" {
		return StatusError
	}
	status := strings.ToLower(strings.TrimSpace(t.Status))
	switch {
	case doneStatuses[status]:
		return StatusDone
	case claimedStatuses[status]:
		return StatusClaimed
	case needsInfoStatuses[status]:
		return StatusNeedsInfo
	case needsAttentionStatuses[status]:
		return StatusNeedsAttention
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
//
// "Blocked by:" text only ever carries a bare number (parseBlockedBy strips
// letter suffixes), so a "Blocked by: 04" reference can't distinguish a
// split ticket's original from its lettered replacements (04, 04a, 04b all
// share Number 4) — it means the whole family, and there's no way to name
// just the replacements. A number counts as resolved only once every ticket
// sharing it is done, not as soon as any one of them is — otherwise the
// superseded original (closed immediately at split time, per to-tickets'
// mid-flight-split convention) would resolve the blocker before its
// replacements ever land. This relies on that convention holding (the
// original does get marked done promptly); it isn't separately verified
// here.
func (e Epic) UnresolvedBlockers(t Ticket) []int {
	if len(t.BlockedBy) == 0 {
		return nil
	}
	total := make(map[int]int, len(e.Tickets))
	done := make(map[int]int, len(e.Tickets))
	for _, other := range e.Tickets {
		total[other.Number]++
		if other.IsDone() {
			done[other.Number]++
		}
	}
	var unresolved []int
	for _, n := range t.BlockedBy {
		if total[n] == 0 || done[n] != total[n] {
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
	case StatusNeedsAttention:
		return "needs-attention"
	case StatusDone:
		return "done"
	default: // StatusError
		return "error"
	}
}

// GroupOrder returns s's sort rank for grouping tickets within an epic:
// unblocked (open/claimed) → blocked → needs-info/needs-attention → done → error.
func GroupOrder(s RenderedStatus) int {
	switch s {
	case StatusOpen, StatusClaimed:
		return 0
	case StatusBlocked:
		return 1
	case StatusNeedsInfo, StatusNeedsAttention:
		return 2
	case StatusDone:
		return 3
	default: // StatusError
		return 4
	}
}

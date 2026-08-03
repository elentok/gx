package tickets

import (
	"strconv"
	"strings"
)

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
	StatusSuperseded
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

// supersededStatuses covers a ticket closed by a mid-flight split (see
// UnresolvedBlockers) rather than by landing work — IsDone/doneStatuses
// still treats it as done for scheduling and blocker resolution, but the
// tickets list renders it distinctly from a ticket that actually shipped.
var supersededStatuses = map[string]bool{
	"superseded": true,
}

// baseStatus classifies t's raw Status: value alone, before the Blocked by:
// overlay (see Epic.RenderedStatus) is applied.
func (t Ticket) baseStatus() RenderedStatus {
	if t.ReadErr != "" {
		return StatusError
	}
	status := strings.ToLower(strings.TrimSpace(t.Status))
	switch {
	case supersededStatuses[status]:
		return StatusSuperseded
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

// UnresolvedBlockers returns t's Blocked by: tokens that are not yet done
// within e, in Blocked by: order. A bare-number token (e.g. "04") means the
// whole number family — it can't distinguish a split ticket's original from
// its lettered replacements (04, 04a, 04b all share Number 4) — and counts
// as resolved only once every ticket sharing that number is done, not as
// soon as any one of them is: otherwise the superseded original (closed
// immediately at split time, per to-tickets' mid-flight-split convention)
// would resolve the blocker before its replacements ever land. A lettered
// token (e.g. "04a") names one specific sibling and resolves as soon as
// that ticket alone is done, independent of its siblings. Either way, a
// blocker with no matching ticket in e counts as unresolved (it can't be
// verified done). t itself is excluded from the family it's checking: a
// ticket that merely shares its family's leading number (e.g. 06b, a
// follow-up ticket to 06, not a lettered replacement created by splitting
// 06) must not be required to finish itself before its own "Blocked by: 06"
// can resolve — that can never happen, since t is still open for the
// duration of this very check.
func (e Epic) UnresolvedBlockers(t Ticket) []string {
	if len(t.BlockedBy) == 0 {
		return nil
	}
	total := make(map[int]int, len(e.Tickets))
	done := make(map[int]int, len(e.Tickets))
	// Keyed by number+lowercased letter suffix rather than the raw
	// Identifier string, so a token's zero-padding doesn't have to match
	// the ticket file's exactly (e.g. "Blocked by: 4a" still finds "04a").
	byNumberAndSuffix := make(map[string]Ticket, len(e.Tickets))
	for _, other := range e.Tickets {
		if other.Number == t.Number && other.Identifier == t.Identifier {
			continue
		}
		total[other.Number]++
		if other.IsDone() {
			done[other.Number]++
		}
		if other.Identifier != "" {
			_, suffix := splitBlockedByToken(other.Identifier)
			byNumberAndSuffix[siblingKey(other.Number, suffix)] = other
		}
	}
	var unresolved []string
	for _, token := range t.BlockedBy {
		num, letters := splitBlockedByToken(token)
		if letters != "" {
			other, ok := byNumberAndSuffix[siblingKey(num, letters)]
			if !ok || !other.IsDone() {
				unresolved = append(unresolved, token)
			}
			continue
		}
		if total[num] == 0 || done[num] != total[num] {
			unresolved = append(unresolved, token)
		}
	}
	return unresolved
}

// BlockingTickets resolves t's UnresolvedBlockers tokens to the concrete
// Ticket entries within e they refer to, for surfacing identifier+title (the
// tickets tab's blocked-selection confirmation modal, ticket 04). A lettered
// token (e.g. "03a") resolves to that one sibling; a bare-number token (e.g.
// "3") can resolve to several not-yet-done siblings sharing that number, same
// family semantics as UnresolvedBlockers. Nil once every blocker has
// resolved.
func (e Epic) BlockingTickets(t Ticket) []Ticket {
	unresolved := e.UnresolvedBlockers(t)
	if len(unresolved) == 0 {
		return nil
	}
	byNumber := make(map[int][]Ticket, len(e.Tickets))
	byNumberAndSuffix := make(map[string]Ticket, len(e.Tickets))
	for _, other := range e.Tickets {
		if other.Number == t.Number && other.Identifier == t.Identifier {
			continue
		}
		byNumber[other.Number] = append(byNumber[other.Number], other)
		if other.Identifier != "" {
			_, suffix := splitBlockedByToken(other.Identifier)
			byNumberAndSuffix[siblingKey(other.Number, suffix)] = other
		}
	}
	var result []Ticket
	seen := map[string]bool{}
	for _, token := range unresolved {
		num, letters := splitBlockedByToken(token)
		if letters != "" {
			if other, ok := byNumberAndSuffix[siblingKey(num, letters)]; ok && !seen[siblingKey(other.Number, letters)] {
				seen[siblingKey(other.Number, letters)] = true
				result = append(result, other)
			}
			continue
		}
		for _, other := range byNumber[num] {
			_, otherLetters := splitBlockedByToken(other.Identifier)
			key := siblingKey(other.Number, otherLetters)
			if other.IsDone() || seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, other)
		}
	}
	return result
}

// splitBlockedByToken splits a parseBlockedBy token (e.g. "04a") into its
// leading number and trailing letter suffix ("", for a bare number like
// "04"). token is always digits-then-letters, per blockedByTokenRe.
func splitBlockedByToken(token string) (number int, letters string) {
	i := 0
	for i < len(token) && token[i] >= '0' && token[i] <= '9' {
		i++
	}
	number, _ = strconv.Atoi(token[:i])
	return number, token[i:]
}

// siblingKey builds a lookup key from a ticket number and letter suffix,
// case-insensitive, so a lettered "Blocked by:" token matches its ticket
// regardless of zero-padding differences (see UnresolvedBlockers).
func siblingKey(number int, letters string) string {
	return strconv.Itoa(number) + strings.ToLower(letters)
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
	case StatusSuperseded:
		return "superseded"
	default: // StatusError
		return "error"
	}
}

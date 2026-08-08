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

// typeCodeReview mirrors schema.TypeCodeReview's on-disk value. tickets.Ticket
// carries Type as a plain string (it predates the schema package), so the
// comparison is against the literal rather than the typed constant.
const typeCodeReview = "code-review"

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
// state regardless of Blocked by:). A type: code-review ticket uses a
// different overlay in place of Blocked by: (see hasOtherOpenTicket) — it
// deliberately carries no explicit Blocked by: list, so there's nothing for
// UnresolvedBlockers to check.
func (e Epic) RenderedStatus(t Ticket) RenderedStatus {
	base := t.baseStatus()
	if base != StatusOpen && base != StatusClaimed {
		return base
	}
	if t.Type == typeCodeReview {
		if e.hasOtherOpenTicket(t) {
			return StatusBlocked
		}
		return base
	}
	if len(e.UnresolvedBlockers(t)) > 0 {
		return StatusBlocked
	}
	return base
}

// hasOtherOpenTicket reports whether e has any ticket, other than t itself,
// that isn't done — the eligibility gate for a type: code-review ticket
// (RenderedStatus), which reviews the whole epic and so must wait for every
// other ticket in it, independent of its own (empty) Blocked by:.
func (e Epic) hasOtherOpenTicket(t Ticket) bool {
	for _, other := range e.Tickets {
		if other.Number == t.Number && other.Identifier == t.Identifier {
			continue
		}
		if !other.IsDone() {
			return true
		}
	}
	return false
}

// UnresolvedBlockers returns t's Blocked by: tokens that are not yet done
// within e, in Blocked by: order. Each token, bare-number ("04") or lettered
// ("04a"), now names exactly one ticket in e: Children/Parent (03) make a
// split's children a direct, walkable edge, so a bare-number token no longer
// has to stand in for a whole number family the way it did when 04, 04a, 04b
// were only ever linked by sharing Number 4. The resolution check is
// Epic.FullyDone of that one named ticket, which recurses into its own
// Children — so a downstream ticket blocked on a since-split ticket still
// waits for every one of its (recursively split) children, without that
// ticket's Blocked by: list ever needing hand-editing when a blocker splits
// mid-flight. A blocker with no matching ticket in e counts as unresolved
// (it can't be verified done).
func (e Epic) UnresolvedBlockers(t Ticket) []string {
	if len(t.BlockedBy) == 0 {
		return nil
	}
	byNumberAndSuffix := e.byNumberAndSuffix()
	// A split's children inherit the original's Blocked by: token (e.g. 05b
	// and 05c both carry "Blocked by: 05" after 05 splits), so t's own split
	// siblings, reached while walking a checked ticket's own Split list,
	// must not be required to finish as part of resolving t's own inherited
	// blocker — that can never happen, since t is still open for the
	// duration of this very check. The same is true of t's own further
	// splits (e.g. 01a's inherited "Blocked by: 01" recursing into 01's
	// children must not require 01a's own child 01b to be done first — 01b
	// can't finish before 01a even starts). That exclusion must not fire on
	// the token's direct resolution target though: a token naming one
	// specific split sibling (e.g. 02c's "Blocked by: 02b") needs that
	// sibling's real status checked, not a free pass for sharing t's
	// Parent. See isSelfOrSplitSiblingOrDescendant.
	exclude := func(other Ticket) bool { return isSelfOrSplitSiblingOrDescendant(t, other, byNumberAndSuffix) }
	var unresolved []string
	for _, token := range t.BlockedBy {
		num, letters := splitBlockedByToken(token)
		other, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if ok && other.Number == t.Number && other.Identifier == t.Identifier {
			// A ticket pathologically Blocked by: its own id would
			// otherwise deadlock: it can never be done while still open for
			// this very check.
			continue
		}
		if !ok || !e.fullyDone(other, byNumberAndSuffix, exclude, map[string]bool{}) {
			unresolved = append(unresolved, token)
		}
	}
	return unresolved
}

// isSelfOrSplitSiblingOrDescendant reports whether other must be excluded
// from t's own inherited-blocker resolution: other is t itself, a split
// sibling (Parent pointing at the same original ticket, e.g. 05b and 05c
// both split from 05), or a descendant of t reached by walking Parent hops
// upward (t's own forward-split chain, e.g. 01a's child 01b) — the
// exclusion fullyDone applies while walking a ticket's own Split list. See
// UnresolvedBlockers.
//
// A descendant must be excluded for the same reason a sibling is: it's part
// of t's own family, reached by construction only after t itself runs, so
// requiring it done as a precondition for t's own blocker to resolve would
// deadlock t against its own not-yet-created follow-on work.
func isSelfOrSplitSiblingOrDescendant(t, other Ticket, byNumberAndSuffix map[string]Ticket) bool {
	if t.Parent != nil && other.Parent != nil && *t.Parent == *other.Parent {
		return true
	}
	return isDescendantOf(t, other, byNumberAndSuffix)
}

// isDescendantOf reports whether t is reached by walking other's own Parent
// chain upward — i.e. other descends from t, directly or via further
// splits. Guarded against a malformed Parent cycle by capping the walk at
// one hop per ticket in the epic.
func isDescendantOf(t, other Ticket, byNumberAndSuffix map[string]Ticket) bool {
	current := other
	for i := 0; i < len(byNumberAndSuffix)+1 && current.Parent != nil; i++ {
		num, letters := splitBlockedByToken(*current.Parent)
		parent, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if !ok {
			return false
		}
		if parent.Number == t.Number && parent.Identifier == t.Identifier {
			return true
		}
		current = parent
	}
	return false
}

// byNumberAndSuffix indexes e.Tickets by number+lowercased letter suffix
// (see siblingKey), so a Blocked by: token's zero-padding doesn't have to
// match the ticket file's exactly (e.g. "Blocked by: 4a" still finds "04a"),
// and a childless-suffix ticket (Identifier "04", or unset in test fixtures)
// is still found by a bare-number token.
func (e Epic) byNumberAndSuffix() map[string]Ticket {
	index := make(map[string]Ticket, len(e.Tickets))
	for _, t := range e.Tickets {
		_, suffix := splitBlockedByToken(t.Identifier)
		index[siblingKey(t.Number, suffix)] = t
	}
	return index
}

// FullyDone reports whether t's own status is done and every one of t's
// Split (children) tickets is, recursively, fully done too — the check
// UnresolvedBlockers uses in place of the plain Ticket.IsDone, so a
// downstream ticket blocked on t doesn't unblock until t's whole subtree has
// landed, not just t itself. Every other "is this ticket done" check
// (claiming, frontier eligibility, re-run guards) keeps using Ticket.IsDone
// unchanged — this is additive, specific to blocked-by resolution. Cycle-
// guarded: a ticket already on the current recursion path is treated as
// fully done rather than walked again, so a malformed Children/Parent loop
// terminates instead of recursing forever.
func (e Epic) FullyDone(t Ticket) bool {
	return e.fullyDone(t, e.byNumberAndSuffix(), nil, map[string]bool{})
}

// fullyDone is FullyDone's recursive core, plus an optional exclude hook
// (used by UnresolvedBlockers to skip t's own split family, see
// isSelfOrSplitSiblingOrDescendant): a child exclude reports true for is treated as
// fully done without recursing into its own children. exclude only applies
// to children reached by walking t's Split list, never to t itself — t's
// own resolution as a blocked_by token's direct target is UnresolvedBlockers'
// job, not this function's.
func (e Epic) fullyDone(t Ticket, byNumberAndSuffix map[string]Ticket, exclude func(Ticket) bool, visiting map[string]bool) bool {
	_, ownSuffix := splitBlockedByToken(t.Identifier)
	key := siblingKey(t.Number, ownSuffix)
	if visiting[key] {
		return true
	}
	visiting[key] = true
	if !t.IsDone() {
		return false
	}
	for _, childID := range t.Split {
		num, letters := splitBlockedByToken(childID)
		child, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if !ok {
			return false
		}
		if exclude != nil && exclude(child) {
			continue
		}
		if !e.fullyDone(child, byNumberAndSuffix, exclude, visiting) {
			return false
		}
	}
	return true
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
	default: // StatusError
		return "error"
	}
}

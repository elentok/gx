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
	// StatusDraft is a ticket its author has explicitly parked: written down,
	// but not yet offered to anyone. It is neither open (never enters an
	// epic's frontier, so no agent ever claims it) nor done (it still counts
	// as outstanding work).
	StatusDraft
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

// draftStatuses covers raw values meaning the ticket is parked by its author
// and must not be scheduled (see StatusDraft).
var draftStatuses = map[string]bool{
	"draft": true,
}

// typeCodeReview mirrors schema.TypeCodeReview's on-disk value. tickets.Ticket
// carries Type as a plain string (it predates the schema package), so the
// comparison is against the literal rather than the typed constant.
const typeCodeReview = "code-review"

// baseStatus classifies t's raw Status: value alone, before the Blocked by:
// overlay (see Epic.RenderedStatus) is applied.
func (t Ticket) baseStatus() RenderedStatus {
	if t.ReadErr != "" || t.GraphErr != "" {
		return StatusError
	}
	status := strings.ToLower(strings.TrimSpace(t.Status))
	switch {
	case draftStatuses[status]:
		return StatusDraft
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
// fork's children a direct, walkable edge, so a bare-number token no longer
// has to stand in for a whole number family the way it did when 04, 04a, 04b
// were only ever linked by sharing Number 4. The resolution check is
// Epic.FullyDone of that one named ticket, which recurses into its whole
// descendant subtree — so a downstream ticket blocked on a since-forked
// ticket still waits for every one of its (recursively forked) descendants,
// without that ticket's Blocked by: list ever needing hand-editing when a
// blocker forks mid-flight. A blocker with no matching ticket in e counts as
// unresolved (it can't be verified done).
func (e Epic) UnresolvedBlockers(t Ticket) []string {
	return e.unresolvedBlockers(t, e.byNumberAndSuffix(), e.childrenIndex(), map[string]bool{})
}

// unresolvedBlockers is UnresolvedBlockers' recursive core: it takes shared
// byNumberAndSuffix/childrenIndex indexes and a visiting set so fullyDone can
// call back into it (see below) to check a candidate blocker's own Blocked
// by: list, using the same cycle guard across the whole walk instead of
// restarting one.
func (e Epic) unresolvedBlockers(t Ticket, byNumberAndSuffix map[string]Ticket, childrenIndex map[string][]Ticket, visiting map[string]bool) []string {
	if len(t.BlockedBy) == 0 {
		return nil
	}
	var unresolved []string
	for _, token := range t.BlockedBy {
		num, letters := splitBlockedByToken(token)
		other, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if !ok {
			unresolved = append(unresolved, token)
			continue
		}
		if other.Number == t.Number && other.Identifier == t.Identifier {
			// A ticket pathologically Blocked by: its own id would
			// otherwise deadlock: it can never be done while still open for
			// this very check.
			continue
		}
		// A fork's children inherit the original's Blocked by: token (e.g.
		// 05b and 05c both carry "Blocked by: 05" after 05 forks) only when
		// the token names an ancestor of t (other is reached by walking t's
		// own Parent chain upward) — in that case t's own fork siblings and
		// further forks, reached while walking other's descendant subtree,
		// must not be required to finish first: that can never happen,
		// since t is still open for the duration of this very check. This
		// exclusion must NOT extend to a token naming a direct, non-ancestor
		// blocker (e.g. 02c's "Blocked by: 02b") — that blocker's own real
		// descendants need their status checked, not a free pass for
		// incidentally sharing t's Parent (mis-parented data, see
		// gx-investigate/gotchas.md, can otherwise make an unrelated
		// blocker's own still-open child look like t's family). t itself,
		// and t's own descendants, are excluded unconditionally either way
		// — requiring t's own not-yet-created follow-on work to be done as
		// a precondition for t's own blocker to resolve would deadlock t
		// against itself regardless of which kind of token this is.
		inherited := isDescendantOf(other, t, byNumberAndSuffix)
		exclude := func(candidate Ticket) bool {
			if isSelfOrDescendant(t, candidate, byNumberAndSuffix) {
				return true
			}
			return inherited && isForkSibling(t, candidate)
		}
		if !e.fullyDone(other, byNumberAndSuffix, childrenIndex, exclude, visiting) {
			unresolved = append(unresolved, token)
		}
	}
	return unresolved
}

// isSelfOrDescendant reports whether other is t itself, or a descendant of t
// reached by walking Parent hops upward from other (t's own forward-fork
// chain, e.g. 01a's child 01b) — excluded from t's own blocker resolution
// unconditionally, regardless of whether the Blocked by: token being
// resolved is inherited or names other directly. See unresolvedBlockers.
func isSelfOrDescendant(t, other Ticket, byNumberAndSuffix map[string]Ticket) bool {
	if other.Number == t.Number && other.Identifier == t.Identifier {
		return true
	}
	return isDescendantOf(t, other, byNumberAndSuffix)
}

// isForkSibling reports whether t and other share a Parent (both forked
// from the same original ticket, e.g. 05b and 05c both forked from 05).
// Only excluded from a blocker's resolution when the Blocked by: token being
// resolved is inherited from an ancestor — see unresolvedBlockers.
func isForkSibling(t, other Ticket) bool {
	return t.Parent != nil && other.Parent != nil && *t.Parent == *other.Parent
}

// isDescendantOf reports whether t is reached by walking other's own Parent
// chain upward — i.e. other descends from t, directly or via further
// forks. Guarded against a malformed Parent cycle by capping the walk at
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

// childrenIndex maps a ticket's siblingKey to every other ticket in e whose
// Parent points at it — the reverse of Ticket.Parent, and what fullyDone
// actually treats as authoritative for a ticket's descendants (see
// FullyDone): Children is an agent-maintained list that a mid-flight fork
// can easily forget or only partially update, while Parent is the field
// `gx tickets add --parent` reliably writes on every fork. A missed
// Children update would otherwise let fullyDone silently skip a real,
// still-open subtree.
func (e Epic) childrenIndex() map[string][]Ticket {
	index := make(map[string][]Ticket)
	for _, t := range e.Tickets {
		if t.Parent == nil {
			continue
		}
		num, letters := splitBlockedByToken(*t.Parent)
		index[siblingKey(num, letters)] = append(index[siblingKey(num, letters)], t)
	}
	return index
}

// FullyDone reports whether t's own status is done, t's own Blocked by: is
// itself fully resolved, and every one of t's descendants (t.Children, plus
// any ticket whose Parent points back at t — see childrenIndex) is,
// recursively, fully done too — the check UnresolvedBlockers uses in place
// of the plain Ticket.IsDone, so a downstream ticket blocked on t doesn't
// unblock until t's whole subtree has landed, not just t itself. Every other
// "is this ticket done" check (claiming, frontier eligibility, re-run
// guards) keeps using Ticket.IsDone unchanged — this is additive, specific
// to blocked-by resolution. Cycle-guarded: a ticket currently on the active
// recursion path is treated as fully done rather than walked again, so a
// malformed Children/Parent loop terminates instead of recursing forever;
// the guard unwinds once that path returns, so it doesn't wrongly memoize
// across separate calls that may resolve the same ticket under a different
// exclude scope (see unresolvedBlockers).
//
// Checking t's own Blocked by: here (not just t.IsDone()) matters for a
// ticket born already status: done without ever passing through the
// scheduler's own claim-time blocker check — a commitless mid-flight-fork
// placeholder (e.g. a "06c" split off "06", marked done at creation) never
// goes through claimNext, so nothing else ever verifies its declared
// Blocked by: was satisfied. Without this, a ticket blocked on that
// placeholder (e.g. "06c1", Blocked by: "06c") would trust "06c" is done at
// face value and start immediately, ignoring that "06c" was itself supposed
// to wait on a still-in-progress fork family (e.g. "06b"→"06b1"→"06b2").
// Found live in tickets-tree: "06c1" started within 200ms of "06b1", before
// "06b"'s fork chain had done any work, because "06c"'s own Blocked by:
// "06b" was never checked. See gx-investigate/gotchas.md.
func (e Epic) FullyDone(t Ticket) bool {
	return e.fullyDone(t, e.byNumberAndSuffix(), e.childrenIndex(), nil, map[string]bool{})
}

// fullyDone is FullyDone's recursive core, plus an optional exclude hook
// (used by unresolvedBlockers to skip t's own fork family, see
// isSelfOrDescendant/isForkSibling): a child exclude reports true for is
// treated as fully done without recursing into it. exclude only applies to
// descendants reached by walking t's own subtree, never to t itself — t's
// own resolution as a blocked_by token's direct target is unresolvedBlockers'
// job, not this function's.
func (e Epic) fullyDone(t Ticket, byNumberAndSuffix map[string]Ticket, childrenIndex map[string][]Ticket, exclude func(Ticket) bool, visiting map[string]bool) bool {
	_, ownSuffix := splitBlockedByToken(t.Identifier)
	key := siblingKey(t.Number, ownSuffix)
	if visiting[key] {
		return true
	}
	visiting[key] = true
	defer delete(visiting, key)
	if !t.IsDone() {
		return false
	}
	if len(e.unresolvedBlockers(t, byNumberAndSuffix, childrenIndex, visiting)) > 0 {
		return false
	}
	seen := map[string]bool{}
	walk := func(child Ticket) bool {
		_, childSuffix := splitBlockedByToken(child.Identifier)
		childKey := siblingKey(child.Number, childSuffix)
		if seen[childKey] {
			return true
		}
		seen[childKey] = true
		if exclude != nil && exclude(child) {
			return true
		}
		return e.fullyDone(child, byNumberAndSuffix, childrenIndex, exclude, visiting)
	}
	for _, childID := range t.Children {
		num, letters := splitBlockedByToken(childID)
		child, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if !ok {
			return false
		}
		if !walk(child) {
			return false
		}
	}
	for _, child := range childrenIndex[key] {
		if !walk(child) {
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
	case StatusDraft:
		return "draft"
	default: // StatusError
		return "error"
	}
}

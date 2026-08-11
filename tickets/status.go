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
	StatusNeedsAnswer
	StatusNeedsRepair
	StatusDone
	StatusError
	// StatusDraft is a ticket its author has explicitly parked: written down,
	// but not yet offered to anyone. It is neither open (never enters an
	// epic's frontier, so no agent ever claims it) nor done (it still counts
	// as outstanding work).
	StatusDraft
	// StatusWaitingForChildren is a computed overlay on top of StatusDone: t's
	// own Status: is done, but its fork subtree (see Epic.Blocking) isn't. It
	// is never written to disk — a ticket's Status: field means only "this
	// ticket's own work is finished" (see ticket 03's commitless: true
	// convention); whether the fork subtree it spawned is also finished is
	// derived fresh from the graph on every render. Being distinct from
	// StatusDone is what keeps an epic containing one from counting as
	// complete, in both Epic.AllDone and ralphloop.allDone.
	StatusWaitingForChildren
)

// openStatuses covers raw Status: values meaning "unclaimed, nothing external
// blocks picking it up". A missing Status: is deliberately absent: status is
// required now, so an empty value falls through to StatusError rather than
// rendering — and scheduling — as open.
var openStatuses = map[string]bool{
	"open": true,
}

var claimedStatuses = map[string]bool{"claimed": true}

// needsAnswerStatuses covers raw values meaning work is stalled on someone
// providing more information before it can proceed.
var needsAnswerStatuses = map[string]bool{
	"needs-answer": true,
}

var needsRepairStatuses = map[string]bool{
	"needs-repair": true,
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
	case needsAnswerStatuses[status]:
		return StatusNeedsAnswer
	case needsRepairStatuses[status]:
		return StatusNeedsRepair
	case openStatuses[status]:
		return StatusOpen
	default:
		return StatusError
	}
}

// RenderedStatus computes t's rendered status within e: t's base status,
// overlaid with "blocked" when t has an unresolved Blocked by: and its base
// status is open or claimed (needs-answer and done tickets keep their own
// state regardless of Blocked by:). A type: code-review ticket's Blocked by:
// is synthesized rather than read from its file (see effectiveBlockedBy),
// but is otherwise resolved the same way as every other ticket's.
func (e Epic) RenderedStatus(t Ticket) RenderedStatus {
	base := t.baseStatus()
	if base == StatusDone && e.Blocking(t) {
		return StatusWaitingForChildren
	}
	if base != StatusOpen && base != StatusClaimed {
		return base
	}
	if len(e.UnresolvedBlockers(t)) > 0 {
		return StatusBlocked
	}
	return base
}

// effectiveBlockedBy returns t's Blocked by: tokens for scheduling: for
// every ticket type but code-review, t's literal, on-file BlockedBy. A
// type: code-review ticket's tokens are synthesized here instead — one
// lettered token per other non-code-review ticket in e, so the review waits
// on the rest of the epic without ever writing a wall of ids into its own
// file. This is a resolver-side view recomputed on every call: it is never
// written back, so the preview panel keeps showing the literal frontmatter
// (see ui/tickets' preview_frontmatter.go), and it is not frozen at claim
// time — a ticket added or forked into the epic after the review was
// claimed is picked up by the next call. Two code-review tickets never
// block each other, since neither is included in the other's expansion.
func (e Epic) effectiveBlockedBy(t Ticket) []string {
	if t.Type != typeCodeReview {
		return t.BlockedBy
	}
	var tokens []string
	for _, other := range e.Tickets {
		if other.Type == typeCodeReview {
			continue
		}
		if other.Number == t.Number && other.Identifier == t.Identifier {
			continue
		}
		_, suffix := splitBlockedByToken(other.Identifier)
		tokens = append(tokens, siblingKey(other.Number, suffix))
	}
	return tokens
}

// Blocking is the refactor's single predicate: t counts as blocking anything
// that names it in a Blocked by: token while t's own status isn't done, or
// while any ticket in t's fork subtree — every ticket reached by following
// Parent reverse-edges down from t, at any depth — is itself Blocking.
// Membership comes from Parent alone; Children is never read (an
// agent-maintained list a mid-flight fork can forget to update, whereas
// Parent is the field `gx tickets add --parent` reliably writes on every
// fork). The recursion carries no cycle guard: Epic construction quarantines
// every cyclic or dangling Parent edge (see quarantineInvalidParents) before
// one can reach here, so every walk down Parent reverse-edges is guaranteed
// to terminate.
func (e Epic) Blocking(t Ticket) bool {
	return e.blocking(t, e.forkChildren())
}

func (e Epic) blocking(t Ticket, forkChildren map[string][]Ticket) bool {
	if !t.IsDone() {
		return true
	}
	for _, child := range forkChildren[ticketKey(t)] {
		if e.blocking(child, forkChildren) {
			return true
		}
	}
	return false
}

// UnresolvedBlockers returns t's Blocked by: tokens that are not yet
// resolved within e, in Blocked by: order. Each token, bare-number ("04") or
// lettered ("04a"), names exactly one ticket in e (see byNumberAndSuffix); a
// token resolves once Epic.Blocking is false for the ticket it names, which
// recurses into that ticket's whole fork subtree — so a downstream ticket
// blocked on a since-forked ticket still waits for every one of its
// (recursively forked) descendants. A blocker with no matching ticket in e
// counts as unresolved (it can't be verified done).
func (e Epic) UnresolvedBlockers(t Ticket) []string {
	blockedBy := e.effectiveBlockedBy(t)
	if len(blockedBy) == 0 {
		return nil
	}
	byNumberAndSuffix := e.byNumberAndSuffix()
	forkChildren := e.forkChildren()
	var unresolved []string
	for _, token := range blockedBy {
		num, letters := splitBlockedByToken(token)
		other, ok := byNumberAndSuffix[siblingKey(num, letters)]
		if !ok || e.blocking(other, forkChildren) {
			unresolved = append(unresolved, token)
		}
	}
	return unresolved
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

// forkChildren maps a ticket's siblingKey to every other ticket in e whose
// Parent points at it — the reverse of Ticket.Parent, and Blocking's only
// source of fork-subtree membership.
func (e Epic) forkChildren() map[string][]Ticket {
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
	case StatusNeedsAnswer:
		return "needs-answer"
	case StatusNeedsRepair:
		return "needs-repair"
	case StatusDone:
		return "done"
	case StatusDraft:
		return "draft"
	case StatusWaitingForChildren:
		return "waiting-for-children"
	default: // StatusError
		return "error"
	}
}

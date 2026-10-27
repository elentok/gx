package tickets

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/elentok/gx/tickets/schema"
)

// RenderedStatus is the tickets tab's collapse of the tracker's raw Status:/
// triage-label vocabulary into a small set of user-facing states, plus a
// seventh "error" state for any value none of the others recognize.
type RenderedStatus int

const (
	StatusOpen RenderedStatus = iota
	StatusClaimed
	StatusBlocked
	StatusPausedAnswer
	StatusPausedRepair
	StatusDone
	StatusError
	// StatusDraft is a ticket its author has explicitly set aside: written down,
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
	// StatusCancelled is a withdrawn ticket: terminal like StatusDone, but the
	// work was never performed.
	StatusCancelled
)

// Terminal reports whether s is a finished state that needs no more work:
// done or cancelled.
func (s RenderedStatus) Terminal() bool {
	return s == StatusDone || s == StatusCancelled
}

// openStatuses covers raw Status: values meaning "unclaimed, nothing external
// blocks picking it up". A missing Status: is deliberately absent: status is
// required now, so an empty value falls through to StatusError rather than
// rendering — and scheduling — as open.
var openStatuses = map[string]bool{
	"open": true,
}

var claimedStatuses = map[string]bool{"claimed": true}

// legacyAnswerStatuses covers raw values meaning work is stalled on someone
// providing more information before it can proceed.
var legacyAnswerStatuses = map[string]bool{
	"needs-answer": true,
}

var legacyRepairStatuses = map[string]bool{
	"needs-repair": true,
}

// draftStatuses covers raw values meaning the ticket is set aside by its author
// and must not be scheduled (see StatusDraft).
var draftStatuses = map[string]bool{
	"draft": true,
}

// typeCodeReview mirrors schema.TypeCodeReview's on-disk value.
// tickets.Ticket carries Type as a plain string (it predates the schema
// package), so the comparison is against the literal rather than the typed
// constant.
const typeCodeReview = "code-review"

// IsCodeReview reports whether t's Type is code-review.
func (t Ticket) IsCodeReview() bool {
	return t.Type == typeCodeReview
}

// IsVerify reports whether t's Type is verify.
func (t Ticket) IsVerify() bool {
	return t.Type == string(schema.TypeVerify)
}

// commitlessByDesignTypes lists the ticket types schema.Ticket.IsCommitless
// treats as commitless regardless of the Commitless flag. It's an allowlist
// (rather than excluding known-noisy types) so a new commitless-by-design
// type added to schema shows up here too instead of silently getting the
// wrong default.
var commitlessByDesignTypes = map[string]bool{
	string(schema.TypeResearch):           true,
	string(schema.TypeGrilling):           true,
	string(schema.TypeCodeReview):         true,
	string(schema.TypeConflictResolution): true,
	string(schema.TypePrompt):             true,
	string(schema.TypeInvestigate):        true,
	string(schema.TypeEvolve):             true,
	string(schema.TypeMerge):              true,
	string(schema.TypeDiagnose):           true,
	string(schema.TypeVerify):             true,
	string(schema.TypeTrunkConflict):      true,
}

// ShowsCommitlessSuffix reports whether t's UI row should append
// " (commitless)". Types that are commitless by design
// (schema.Ticket.IsCommitless) are excluded since the suffix would be noise
// there; it's only informative for types where a missing commit is
// unexpected.
func (t Ticket) ShowsCommitlessSuffix() bool {
	return t.Commitless && !commitlessByDesignTypes[t.Type]
}

// LegacyStatus is t.Status with paused spelled as the needs-answer or
// needs-repair its class used to be stored as. Wire formats that clients still
// read as the old words use it.
func (t Ticket) LegacyStatus() string {
	if t.Status == string(schema.StatusPaused) {
		return string(t.PauseReason.Class().LegacyStatus())
	}
	return t.Status
}

// PausedClass is the pause class of t when it is paused, else "".
func (t Ticket) PausedClass() schema.PauseClass {
	if t.Status == string(schema.StatusPaused) {
		return t.PauseReason.Class()
	}
	return ""
}

// PausedForRepair is true when t is paused with no question in its log: a
// fault, or a pause gx receives the reply to.
func (t Ticket) PausedForRepair() bool {
	return t.PausedClass() != "" && !t.PauseReason.AsksQuestion()
}

// RawStatus is t's status as its file says it, with no Blocked by: or
// waiting-for-children overlay.
func (t Ticket) RawStatus() RenderedStatus { return t.baseStatus() }

// baseStatus classifies t's raw Status: value alone, before the Blocked by:
// overlay (see Epic.RenderedStatus) is applied.
func (t Ticket) baseStatus() RenderedStatus {
	if t.ReadErr != "" || t.GraphErr != "" || t.BlockedByErr != "" {
		return StatusError
	}
	status := strings.ToLower(strings.TrimSpace(t.Status))
	switch {
	case draftStatuses[status]:
		return StatusDraft
	case doneStatuses[status]:
		return StatusDone
	case cancelledStatuses[status]:
		return StatusCancelled
	case claimedStatuses[status]:
		return StatusClaimed
	case legacyAnswerStatuses[status]:
		return StatusPausedAnswer
	case legacyRepairStatuses[status]:
		return StatusPausedRepair
	case status == string(schema.StatusPaused):
		if t.PauseReason.Class() == schema.PauseClassAnswer {
			return StatusPausedAnswer
		}
		return StatusPausedRepair
	case openStatuses[status]:
		return StatusOpen
	default:
		return StatusError
	}
}

// RenderedStatus computes t's rendered status within e: t's base status,
// overlaid with "blocked" when t has an unresolved Blocked by: and its base
// status is open or claimed (paused and done tickets keep their own
// state regardless of Blocked by:). A type: code-review ticket's Blocked by:
// is synthesized rather than read from its file (see effectiveBlockedBy),
// but is otherwise resolved the same way as every other ticket's.
func (e Epic) RenderedStatus(t Ticket) RenderedStatus {
	forkChildren := e.forkChildren()
	base := t.baseStatus()
	if base == StatusDone && e.blocking(t, forkChildren) || pausedOnInvestigation(t, base, forkChildren) {
		return StatusWaitingForChildren
	}
	if base != StatusOpen && base != StatusClaimed {
		return base
	}
	if len(e.UnresolvedBlockers(t)) > 0 || !e.ParentDone(t) {
		return StatusBlocked
	}
	return base
}

// IsPaused reports whether t is stalled on a person rather than on the run:
// paused for an answer or a repair, error, or draft (a stub nobody has filled
// in). None of these is ever schedulable, but a person can clear any of them,
// which is what separates a pause from a deadlock. The one rule the server,
// ralph-loop and the Queue tab share.
func (e Epic) IsPaused(t Ticket) bool {
	return e.RenderedStatus(t).Paused()
}

// Paused is the status half of Epic.IsPaused.
func (s RenderedStatus) Paused() bool {
	switch s {
	case StatusPausedAnswer, StatusPausedRepair, StatusError, StatusDraft:
		return true
	}
	return false
}

// pausedOnInvestigation reports whether t is paused while an investigate child
// of it is still unfinished: it renders as waiting for that child. An
// investigate ticket is the one fork child that works on a paused parent, so
// ParentDone never holds it back.
func pausedOnInvestigation(t Ticket, base RenderedStatus, forkChildren map[string][]Ticket) bool {
	if base != StatusPausedRepair && base != StatusPausedAnswer {
		return false
	}
	for _, child := range forkChildren[ticketKey(t)] {
		if child.Type == string(schema.TypeInvestigate) && !child.IsTerminal() {
			return true
		}
	}
	return false
}

// ParentDone reports whether t's Parent ticket (see Ticket.Parent) has
// finished its own work — Parent's raw Status alone, via IsDone, never its
// fork subtree. This is the implicit edge every fork child carries on its
// parent: recursing into the subtree here (as Blocking does for explicit
// Blocked by: tokens) would deadlock every fork, since a parent isn't done-
// with-subtree-resolved until its children are, and the children can't start
// until it is. A ticket with no Parent is trivially "done" by this measure —
// it has nothing to wait on. A paused parent, like
// any non-done status, holds the child; a commitless-done parent still has
// Status: done (see ticket 08), so it releases the child same as any other.
func (e Epic) ParentDone(t Ticket) bool {
	if t.Parent == nil || t.Type == string(schema.TypeInvestigate) {
		return true
	}
	num, letters := splitBlockedByToken(*t.Parent)
	parent, ok := e.byNumberAndSuffix()[siblingKey(num, letters)]
	if !ok {
		return false
	}
	return parent.IsDone()
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
	if t.IsVerify() {
		return e.verifyBlockers(t)
	}
	if t.Type != typeCodeReview {
		// Cross-epic refs name another epic; ExternalBlockers carries them.
		return slices.DeleteFunc(slices.Clone(t.BlockedBy), crossEpicRef)
	}
	var tokens []string
	for _, other := range e.Tickets {
		if other.Type == typeCodeReview {
			continue
		}
		// Verify runs after the review, so waiting on it (or on a ticket it
		// opened) would deadlock both.
		if e.inVerifySubtree(other) {
			continue
		}
		if other.Number == t.Number && other.Identifier == t.Identifier {
			continue
		}
		// A fix ticket the review forked can't start until the review is
		// done (ParentDone), so waiting on it would block both forever.
		if e.descendsFrom(other, t) {
			continue
		}
		_, suffix := splitBlockedByToken(other.Identifier)
		tokens = append(tokens, siblingKey(other.Number, suffix))
	}
	return tokens
}

// verifyBlockers is a verify ticket's Blocked by: tokens: the epic's
// code-review tickets plus its own literal same-epic Blocked by:. A verify
// ticket with no literal Blocked by: never waits on other verify tickets or on
// tickets it opened.
func (e Epic) verifyBlockers(t Ticket) []string {
	tokens := slices.DeleteFunc(slices.Clone(t.BlockedBy), crossEpicRef)
	for _, other := range e.Tickets {
		if other.Type != typeCodeReview || e.descendsFrom(other, t) {
			continue
		}
		_, suffix := splitBlockedByToken(other.Identifier)
		tokens = append(tokens, siblingKey(other.Number, suffix))
	}
	return tokens
}

// inVerifySubtree reports whether t is a verify ticket or was forked from one.
func (e Epic) inVerifySubtree(t Ticket) bool {
	if t.IsVerify() {
		return true
	}
	for _, other := range e.Tickets {
		if other.IsVerify() && e.descendsFrom(t, other) {
			return true
		}
	}
	return false
}

// descendsFrom reports whether t is in ancestor's fork subtree, following
// Parent links upward. The step bound keeps a malformed cycle from looping.
func (e Epic) descendsFrom(t, ancestor Ticket) bool {
	index := e.byNumberAndSuffix()
	for range len(e.Tickets) {
		if t.Parent == nil {
			return false
		}
		num, letters := splitBlockedByToken(*t.Parent)
		parent, ok := index[siblingKey(num, letters)]
		if !ok {
			return false
		}
		if ticketKey(parent) == ticketKey(ancestor) {
			return true
		}
		t = parent
	}
	return false
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
	if !t.IsTerminal() {
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
	unresolved := slices.Clone(t.ExternalBlockers)
	if len(blockedBy) == 0 {
		return unresolved
	}
	byNumberAndSuffix := e.byNumberAndSuffix()
	forkChildren := e.forkChildren()
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
//
// The map is built once per epic: an Epic from the loader carries a forkCache,
// which every copy shares and rebuilds only when Tickets is a different slice.
// A hand-built Epic has no cache and builds it per call.
func (e Epic) forkChildren() map[string][]Ticket {
	if e.forks == nil {
		return buildForkChildren(e.Tickets)
	}
	return e.forks.get(e.Tickets)
}

// forkCache holds the fork-children map for one Tickets slice. Epic copies
// share it, so it is guarded and keyed by the slice it was built from.
type forkCache struct {
	mu       sync.Mutex
	first    *Ticket
	n        int
	children map[string][]Ticket
}

func (c *forkCache) get(ts []Ticket) map[string][]Ticket {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.children == nil || c.first != unsafe.SliceData(ts) || c.n != len(ts) {
		c.children = buildForkChildren(ts)
		c.first, c.n = unsafe.SliceData(ts), len(ts)
	}
	return c.children
}

// forkChildrenBuilds counts buildForkChildren calls, so a test can pin the
// once-per-epic build.
var forkChildrenBuilds atomic.Int64

func buildForkChildren(ts []Ticket) map[string][]Ticket {
	forkChildrenBuilds.Add(1)
	index := make(map[string][]Ticket)
	for _, t := range ts {
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
			if other.IsTerminal() || seen[key] {
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
	case StatusPausedAnswer:
		return "needs-answer"
	case StatusPausedRepair:
		return "needs-repair"
	case StatusDone:
		return "done"
	case StatusCancelled:
		return "cancelled"
	case StatusDraft:
		return "draft"
	case StatusWaitingForChildren:
		return "waiting-for-children"
	default: // StatusError
		return "error"
	}
}

// ParseRenderedStatus is Word's inverse; ok is false for a word Word never
// returns.
func ParseRenderedStatus(word string) (RenderedStatus, bool) {
	for s := StatusOpen; s <= StatusCancelled; s++ {
		if s.Word() == word {
			return s, true
		}
	}
	return StatusError, false
}

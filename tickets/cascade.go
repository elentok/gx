package tickets

import "strings"

// TokenRefersToTicket reports whether a Blocked by: token names t: a bare
// number token (e.g. "04") names t's whole number family (see
// UnresolvedBlockers), a lettered token (e.g. "04a") names only that specific
// sibling.
func TokenRefersToTicket(token string, t Ticket) bool {
	num, letters := splitBlockedByToken(token)
	if num != t.Number {
		return false
	}
	if letters == "" {
		return true
	}
	_, suffix := splitBlockedByToken(t.Identifier)
	return strings.EqualFold(letters, suffix)
}

// dependsOn reports whether other's Blocked by: contains a token naming
// target, per TokenRefersToTicket.
func dependsOn(other, target Ticket) bool {
	for _, token := range other.BlockedBy {
		if TokenRefersToTicket(token, target) {
			return true
		}
	}
	return false
}

// CascadeDelete computes what deleting target requires elsewhere in e:
// toDelete is target (always first) plus every ticket transitively blocked
// by something already in toDelete — every ticket that can now never be
// worked, since one of its blockers is gone for good. Traversal stops at a
// done ticket: its work already landed, so it survives deletion — it's
// collected into toClear instead, and nothing depending on it is explored
// further (a survivor still resolves whatever depends on it, independent of
// target's deletion).
func (e Epic) CascadeDelete(target Ticket) (toDelete, toClear []Ticket) {
	deleted := map[string]bool{target.Path: true}
	toDelete = []Ticket{target}
	cleared := map[string]bool{}
	queue := []Ticket{target}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, other := range e.Tickets {
			if deleted[other.Path] || cleared[other.Path] {
				continue
			}
			if !dependsOn(other, cur) {
				continue
			}
			if other.IsDone() {
				cleared[other.Path] = true
				toClear = append(toClear, other)
				continue
			}
			deleted[other.Path] = true
			toDelete = append(toDelete, other)
			queue = append(queue, other)
		}
	}
	return toDelete, toClear
}

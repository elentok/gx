package tickets

import (
	"errors"
	"fmt"
)

// ForkParents is an epic's canonical fork-parent lookup: the one way to walk
// from a ticket to the ticket its `parent` names. Building it once and reusing
// it is what keeps a caller that resolves many tickets (the tickets UI's
// auto-queueing of forks) from rescanning the epic per ticket.
//
// Resolution goes through the same sibling-key index every other parent and
// blocked-by resolver uses (see byNumberAndSuffix), so a token's zero-padding
// and letter case never change which ticket it finds — only its spelling.
type ForkParents struct {
	bySibling map[string]Ticket
}

// ForkParents builds e's fork-parent lookup.
func (e Epic) ForkParents() ForkParents {
	return ForkParents{bySibling: e.byNumberAndSuffix()}
}

// Of returns the ticket t's Parent names, or ok=false when t has no parent or
// its token names no ticket in the epic. A loaded epic never carries the
// latter — quarantineInvalidParents drops dangling edges — so it only shows up
// for hand-built Epic values.
func (p ForkParents) Of(t Ticket) (parent Ticket, ok bool) {
	if t.Parent == nil {
		return Ticket{}, false
	}
	num, letters := splitBlockedByToken(*t.Parent)
	parent, ok = p.bySibling[siblingKey(num, letters)]
	return parent, ok
}

// ValidateParentGraph checks every Parent edge in e at once: each Parent must
// name a ticket that exists in the epic, and following Parent hops from any
// ticket must terminate rather than loop back onto a ticket already on the
// walk (a ticket re-parented onto its own fork subtree). This deliberately
// isn't part of schema.Validate — that sees one ticket's frontmatter with no
// epic around it, and neither question can be answered from a single file.
//
// Every offending edge is reported at once via errors.Join, in e.Tickets
// order. Callers that want the epic to keep loading despite a bad edge use
// quarantineInvalidParents instead.
func (e Epic) ValidateParentGraph() error {
	bad := e.invalidParentEdges()
	if len(bad) == 0 {
		return nil
	}
	var errs []error
	for _, t := range e.Tickets {
		if err, ok := bad[ticketKey(t)]; ok {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// quarantineInvalidParents drops every Parent edge ValidateParentGraph
// rejects, recording why on the ticket that carried it (GraphErr, which
// renders as StatusError). The loader calls it so one hand-edited file
// doesn't blank the whole tracker the way a hard load failure would, while
// still guaranteeing no caller ever receives a cyclic or dangling parent
// graph — the invariant the blocked-by resolver's unguarded recursion rests
// on.
func (e *Epic) quarantineInvalidParents() {
	bad := e.invalidParentEdges()
	if len(bad) == 0 {
		return
	}
	for i := range e.Tickets {
		err, ok := bad[ticketKey(e.Tickets[i])]
		if !ok {
			continue
		}
		e.Tickets[i].GraphErr = err.Error()
		e.Tickets[i].Parent = nil
	}
}

// invalidParentEdges maps the ticketKey of each ticket carrying a bad Parent
// edge to that edge's error. Walking from every ticket (rather than only from
// roots) is what makes a cycle report all of its own edges: each member's walk
// closes on a different one, so quarantining the reported set breaks the cycle
// outright rather than leaving a shorter one behind.
func (e Epic) invalidParentEdges() map[string]error {
	parents := e.ForkParents()
	bad := map[string]error{}
	for _, start := range e.Tickets {
		walked := map[string]bool{ticketKey(start): true}
		current := start
		for current.Parent != nil {
			parent, ok := parents.Of(current)
			if !ok {
				bad[ticketKey(current)] = fmt.Errorf("ticket %s: parent %q names no ticket in this epic", current.DisplayNumber(), *current.Parent)
				break
			}
			key := ticketKey(parent)
			if walked[key] {
				bad[ticketKey(current)] = fmt.Errorf("ticket %s: parent %q closes a cycle in the parent graph", current.DisplayNumber(), *current.Parent)
				break
			}
			walked[key] = true
			current = parent
		}
	}
	return bad
}

// ticketKey is t's own siblingKey — the identity every parent/blocked-by
// token resolves against (see byNumberAndSuffix).
func ticketKey(t Ticket) string {
	_, suffix := splitBlockedByToken(t.Identifier)
	return siblingKey(t.Number, suffix)
}

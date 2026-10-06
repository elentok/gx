package tickets

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// CheckBlockedBy is the one verdict on t's bare blocked_by refs, shared by
// `gx tickets validate` and the loader so both report the same message: each
// must name a ticket in e. A qualified ref ("epic/06") needs the whole project
// to resolve, so it is left to ValidateProject. Every bad ref is reported at
// once.
func (e Epic) CheckBlockedBy(t Ticket) error {
	t.BlockedBy = slices.DeleteFunc(slices.Clone(t.BlockedBy), func(ref string) bool {
		return strings.Contains(ref, "/")
	})
	return errors.Join(newProjectGraph("", []Epic{e}).checkBlockedBy(e.Name, t)...)
}

// flagDanglingBlockers records CheckBlockedBy's verdict on each ticket that
// fails it (BlockedByErr, which renders as StatusError). The ticket keeps its
// BlockedBy so the message and `validate` stay in step; StatusError is never
// runnable, so the loop can't claim it.
//
// A single epic can't resolve a qualified ref (its own or its epic's), so it
// fails closed here: only ResolveCrossEpic, which sees the whole project, lifts
// that. This is what an in-process loop (no server) falls back on.
func (e *Epic) flagDanglingBlockers() {
	epicQualified := qualifiedRefs(e.BlockedBy)
	for i := range e.Tickets {
		t := &e.Tickets[i]
		if err := e.CheckBlockedBy(*t); err != nil {
			t.BlockedByErr = err.Error()
			continue
		}
		if refs := append(qualifiedRefs(t.BlockedBy), epicQualified...); len(refs) > 0 {
			t.BlockedByErr = fmt.Sprintf("blocked_by %s crosses epics; only the server resolves that", strings.Join(refs, ", "))
		}
	}
}

func qualifiedRefs(refs []string) []string {
	return slices.DeleteFunc(slices.Clone(refs), func(ref string) bool {
		return !strings.Contains(ref, "/")
	})
}

// ResolveCrossEpic returns epics (all of one project, named project) with each
// ticket's blockers resolved project-wide. A ticket is runnable only if its own
// blocked_by refs and its epic's are all resolved; children never copy an
// ancestor's blocked_by, the epic's refs are applied here instead. Unresolved
// qualified refs land in Ticket.ExternalBlockers (rendering the ticket
// blocked); bad refs set BlockedByErr (rendering it error).
func ResolveCrossEpic(project string, epics []Epic) []Epic {
	g := newProjectGraph(project, epics)
	out := slices.Clone(epics)
	for i := range out {
		e := &out[i]
		e.Tickets = slices.Clone(e.Tickets)
		for j := range e.Tickets {
			t := &e.Tickets[j]
			t.BlockedByErr, t.ExternalBlockers = "", nil
			errs := g.checkBlockedBy(e.Name, *t)
			for _, ref := range qualifiedRefs(e.BlockedBy) {
				if _, err := g.resolve(e.Name, ref); err != nil {
					errs = append(errs, fmt.Errorf("epic %s: blocked_by %w", e.Name, err))
				}
			}
			if len(errs) > 0 {
				t.BlockedByErr = errors.Join(errs...).Error()
				continue
			}
			for _, ref := range append(qualifiedRefs(t.BlockedBy), qualifiedRefs(e.BlockedBy)...) {
				if key, _ := g.resolve(e.Name, ref); g.blocking(key) {
					t.ExternalBlockers = append(t.ExternalBlockers, ref)
				}
			}
		}
	}
	return out
}

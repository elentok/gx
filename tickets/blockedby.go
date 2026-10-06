package tickets

import (
	"errors"
	"fmt"
	"strings"
)

// CheckBlockedBy is the one verdict on t's literal blocked_by refs, shared by
// `gx tickets validate` and the loader so both report the same message. A ref
// qualified with an epic ("epic/06") is malformed until cross-epic refs exist;
// a bare ref must name a ticket in e. Every bad ref is reported at once.
func (e Epic) CheckBlockedBy(t Ticket) error {
	index := e.byNumberAndSuffix()
	var errs []error
	for _, ref := range t.BlockedBy {
		if strings.Contains(ref, "/") {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q is malformed (cross-epic refs are not supported)", t.DisplayNumber(), ref))
			continue
		}
		num, letters := splitBlockedByToken(ref)
		if _, ok := index[siblingKey(num, letters)]; !ok {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q names no ticket in this epic", t.DisplayNumber(), ref))
		}
	}
	return errors.Join(errs...)
}

// CheckBlockedByCycles reports each of t's blocked_by refs that closes a wait
// cycle (see blockedByCycleErrors). It is `gx tickets validate`'s alone: the
// loader doesn't flag cycles, so the queue still lists cyclic tickets and
// reports "no unblocked tickets left" instead of hiding them behind StatusError.
func (e Epic) CheckBlockedByCycles(t Ticket) error {
	return errors.Join(e.blockedByCycleErrors(t)...)
}

// flagDanglingBlockers records CheckBlockedBy's verdict on each ticket that
// fails it (BlockedByErr, which renders as StatusError). The ticket keeps its
// BlockedBy so the message and `validate` stay in step; StatusError is never
// runnable, so the loop can't claim it.
func (e *Epic) flagDanglingBlockers() {
	for i := range e.Tickets {
		if err := e.CheckBlockedBy(e.Tickets[i]); err != nil {
			e.Tickets[i].BlockedByErr = err.Error()
		}
	}
}

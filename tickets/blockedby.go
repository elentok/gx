package tickets

import (
	"errors"
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
func (e *Epic) flagDanglingBlockers() {
	for i := range e.Tickets {
		if err := e.CheckBlockedBy(e.Tickets[i]); err != nil {
			e.Tickets[i].BlockedByErr = err.Error()
		}
	}
}

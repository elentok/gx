package schema

import (
	"os"
	"time"

	"github.com/elentok/gx/fsutil"
)

// UpdateTicket reads path's frontmatter ticket, applies mutate to it,
// validates the result, and — only if valid — writes it back atomically,
// leaving the body untouched. Nothing is written if the mutated ticket fails
// Validate; its error is returned as-is. This is the schema-package
// counterpart to ralphloop's private updateTicket, exported so cmd (the
// `gx tickets set` CLI) doesn't need to import ralphloop just to reuse its
// write path.
func UpdateTicket(path string, mutate func(*Ticket)) error {
	return UpdateTicketWithBody(path, func(t *Ticket, _ *string) { mutate(t) })
}

// UpdateTicketWithBody is UpdateTicket's variant for mutations that also
// rewrite the ticket's markdown body, with the same validate-then-atomic-write
// guarantees.
func UpdateTicketWithBody(path string, mutate func(*Ticket, *string)) error {
	unlock, err := LockTicket(path)
	if err != nil {
		return err
	}
	defer unlock()

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t, err := ParseTicketFromRaw(string(raw), path)
	if err != nil {
		return err
	}
	body := ParseBody(string(raw))

	before := t.Status
	mutate(&t, &body)
	t.StampStatusChange(before, time.Now().UTC())

	if err := Validate(t); err != nil {
		return err
	}

	out, err := MarshalTicket(t, body)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, out)
}

// StampStatusChange stamps ClaimedAt or DoneAt when t.Status just moved from
// before into claimed or done. An unchanged status stamps nothing, so a
// rewrite of a done ticket keeps its original DoneAt. A new claim clears DoneAt.
func (t *Ticket) StampStatusChange(before Status, now time.Time) {
	if t.Status == before {
		return
	}
	switch t.Status {
	case StatusClaimed:
		t.ClaimedAt = now
		t.DoneAt = time.Time{}
	case StatusDone:
		t.DoneAt = now
	}
}

// ClearIterationStatus removes path's iteration_status, leaving Status and
// everything else untouched. This is the primitive ralph-loop's reattach
// wiring calls once a fresh agent session picks a ticket back up.
func ClearIterationStatus(path string) error {
	return UpdateTicket(path, func(t *Ticket) {
		t.IterationStatus = ""
		t.IterationFinishedAt = time.Time{}
	})
}

package tickets

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// ArchivedOneOffsEpic is where finished one-offs are moved; the move records
// the old address in the ticket's Resolution as "- original: <address>".
const ArchivedOneOffsEpic = "one-offs-archived"

// FindOriginal looks up the ticket an archived one-off was moved from addr to:
// the one in one-offs-archived whose Resolution names addr as its original.
// It works from the store dir alone, so a client needs no server. No match is
// CodeUnknownTicket; more than one is CodeAmbiguousOriginal.
func FindOriginal(project, projectDir string, addr Address) (Address, Ticket, error) {
	epics, err := Load(projectDir)
	if err != nil {
		return Address{}, Ticket{}, err
	}
	want := "- original: " + addr.String()
	var found []Ticket
	for _, e := range epics {
		if e.Name != ArchivedOneOffsEpic {
			continue
		}
		for _, t := range e.Tickets {
			for line := range strings.SplitSeq(schema.Section(t.Body, "Resolution"), "\n") {
				if strings.TrimSpace(line) == want {
					found = append(found, t)
					break
				}
			}
		}
	}
	switch len(found) {
	case 0:
		return Address{}, Ticket{}, &AddressError{Code: CodeUnknownTicket, Msg: fmt.Sprintf("no ticket %s", addr)}
	case 1:
		return Address{Project: project, Epic: ArchivedOneOffsEpic, ID: found[0].Identifier}, found[0], nil
	}
	return Address{}, Ticket{}, &AddressError{Code: CodeAmbiguousOriginal, Msg: fmt.Sprintf("%d archived tickets name %s as their original", len(found), addr)}
}

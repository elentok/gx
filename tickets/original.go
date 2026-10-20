package tickets

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets/schema"
)

// ArchivedOneOffsEpic is where finished one-offs are moved; the move records
// the old address in the ticket's Resolution as "- original: <address>".
const ArchivedOneOffsEpic = "one-offs-archived"

// OneOffsEpic is where one-offs are submitted.
const OneOffsEpic = "one-offs"

// OriginalLine is the Resolution line recording the address a ticket was moved
// from, without a trailing newline. NamesOriginal is its matcher.
func OriginalLine(addr Address) string {
	return "- original: " + addr.String()
}

// NamesOriginal reports whether a ticket body's Resolution records addr as its
// original address.
func NamesOriginal(body string, addr Address) bool {
	want := OriginalLine(addr)
	for line := range strings.SplitSeq(schema.Section(body, "Resolution"), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// IsTerminalStatus reports whether a raw ticket status is done or cancelled.
func IsTerminalStatus(status string) bool {
	return status == string(schema.StatusDone) || status == string(schema.StatusCancelled)
}

// LoadOneOffs loads the project's one-offs epic and its archive. A missing epic
// comes back as the zero Epic.
func LoadOneOffs(projectDir string) (live, archived Epic, err error) {
	epics, err := Load(projectDir)
	if err != nil {
		return Epic{}, Epic{}, err
	}
	for _, e := range epics {
		switch e.Name {
		case OneOffsEpic:
			live = e
		case ArchivedOneOffsEpic:
			archived = e
		}
	}
	return live, archived, nil
}

// archivedFrom is the tickets of the archive epic whose Resolution names addr
// as their original.
func archivedFrom(archive Epic, addr Address) []Ticket {
	var found []Ticket
	for _, t := range archive.Tickets {
		if NamesOriginal(t.Body, addr) {
			found = append(found, t)
		}
	}
	return found
}

// FindOriginal looks up the ticket an archived one-off was moved from addr to:
// the one in one-offs-archived whose Resolution names addr as its original.
// It works from the store dir alone, so a client needs no server. No match is
// CodeUnknownTicket; more than one is CodeAmbiguousOriginal.
func FindOriginal(project, projectDir string, addr Address) (Address, Ticket, error) {
	epics, err := Load(projectDir)
	if err != nil {
		return Address{}, Ticket{}, err
	}
	var found []Ticket
	for _, e := range epics {
		if e.Name == ArchivedOneOffsEpic {
			found = append(found, archivedFrom(e, addr)...)
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

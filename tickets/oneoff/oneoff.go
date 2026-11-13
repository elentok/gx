package oneoff

import (
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
)

// OneOffFile is the name of a one-off's single ticket; nothing else writes it.
const OneOffFile = "01-one-off.md"

// IsOneOffsEpicName reports whether name is one of the one-offs epics.
func IsOneOffsEpicName(name string) bool {
	return name == tickets.OneOffsEpic || name == tickets.ArchivedOneOffsEpic
}

// IsOneOffTicket reports whether ticketPath (project/epic/issues/file) is a
// one-off's ticket: in a one-offs epic, or the single ticket of an old
// one-epic-per-one-off submit.
func IsOneOffTicket(ticketPath string) bool {
	epic := filepath.Base(filepath.Dir(filepath.Dir(ticketPath)))
	return IsOneOffsEpicName(epic) || filepath.Base(ticketPath) == OneOffFile
}

// IsOneOffEpic reports whether e is a one-off epic: one of the one-offs epics,
// or an old single-ticket epic whose ticket is the one-off file.
func IsOneOffEpic(e tickets.Epic) bool {
	if IsOneOffsEpicName(filepath.Base(e.Path)) {
		return true
	}
	for _, t := range e.Tickets {
		if IsOneOffTicket(t.FilePath()) {
			return true
		}
	}
	return false
}

// EpicAddress is the "project:epic" part of a ticket address, whatever project
// the session itself runs in.
func EpicAddress(addr string) string {
	epicPart, _, _ := strings.Cut(addr, "/")
	return epicPart
}

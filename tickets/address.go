package tickets

import (
	"fmt"
	"regexp"
)

// Reason codes carried by AddressError. They are the stable, machine-readable
// part of a refusal; the message is for humans and may change.
const (
	CodeMalformedAddress = "malformed-address"
	CodeEpicRequired     = "epic-required"
	CodeUnknownProject   = "unknown-project"
	CodeUnknownEpic      = "unknown-epic"
	CodeUnknownTicket    = "unknown-ticket"
)

// AddressError is a refused address with a stable Code.
type AddressError struct {
	Code string
	Msg  string
}

func (e *AddressError) Error() string { return e.Code + ": " + e.Msg }

// Address is the one canonical name of a ticket: "project:epic/06".
type Address struct {
	Project string
	Epic    string
	ID      string
}

func (a Address) String() string { return a.Project + ":" + a.Epic + "/" + a.ID }

// AddressContext supplies what short forms omit: the cwd project, and the
// current epic (empty when cwd is not inside one).
type AddressContext struct {
	Project string
	Epic    string
}

var addressRe = regexp.MustCompile(`^(?:(?:([^:/\s]+):)?([^:/\s]+)/)?(\d+[[:alpha:]]?\d*)$`)

// ParseAddress accepts "project:epic/06", "epic/06" and "06" and always
// returns the full form. Short forms are an input convenience only: nothing
// stores or prints them.
func ParseAddress(s string, ctx AddressContext) (Address, error) {
	m := addressRe.FindStringSubmatch(s)
	if m == nil {
		return Address{}, &AddressError{CodeMalformedAddress, fmt.Sprintf("%q is not a ticket address (want project:epic/06, epic/06 or 06)", s)}
	}
	project, epic, id := m[1], m[2], m[3]
	if project == "" {
		project = ctx.Project
	}
	if epic == "" {
		epic = ctx.Epic
	}
	if epic == "" {
		return Address{}, &AddressError{CodeEpicRequired, fmt.Sprintf("%q names no epic and the current directory is not inside one", s)}
	}
	return Address{Project: project, Epic: epic, ID: id}, nil
}

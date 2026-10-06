package tickets

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
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

// AddressOfPath names the ticket with the given id stored at path, when path
// sits in the tracker's <project>/<epic>/issues/<file>.md layout. Ad-hoc
// files outside it have no address.
func AddressOfPath(path, id string) (Address, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Address{}, false
	}
	issuesDir := filepath.Dir(abs)
	if filepath.Base(issuesDir) != "issues" {
		return Address{}, false
	}
	epicDir := filepath.Dir(issuesDir)
	return Address{Project: ProjectName(filepath.Dir(epicDir)), Epic: filepath.Base(epicDir), ID: id}, true
}

// SplitTrailerValue is the inverse of Address.String for a landing trailer
// value, tolerating the legacy "epic/id" form that carries no project.
func SplitTrailerValue(v string) (epic, id string, ok bool) {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[i+1:]
	}
	i := strings.LastIndexByte(v, '/')
	if i < 0 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
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

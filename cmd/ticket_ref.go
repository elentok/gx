package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// findTicket resolves addr (full or short form) against the cwd project. An
// address that names nothing is refused with a tickets.AddressError carrying
// a stable code.
func findTicket(cwd, addr string) (tickets.Address, tickets.Ticket, error) {
	root, err := ticketRoot(cwd)
	if err != nil {
		return tickets.Address{}, tickets.Ticket{}, err
	}
	project := tickets.ProjectName(root)
	a, err := tickets.ParseAddress(addr, tickets.AddressContext{Project: project, Epic: currentEpic(cwd)})
	if err != nil {
		return a, tickets.Ticket{}, err
	}
	if a.Project != project {
		return a, tickets.Ticket{}, &tickets.AddressError{Code: tickets.CodeUnknownProject, Msg: fmt.Sprintf("project %q is not the current project (%q)", a.Project, project)}
	}

	epics, err := tickets.Load(root)
	if err != nil {
		return a, tickets.Ticket{}, err
	}
	for _, epic := range epics {
		if epic.Name != a.Epic {
			continue
		}
		for _, t := range epic.Tickets {
			if t.Identifier == a.ID {
				return a, t, nil
			}
		}
		return a, tickets.Ticket{}, &tickets.AddressError{Code: tickets.CodeUnknownTicket, Msg: fmt.Sprintf("no ticket %s in epic %q", a.ID, a.Epic)}
	}
	return a, tickets.Ticket{}, &tickets.AddressError{Code: tickets.CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q in project %q", a.Epic, a.Project)}
}

// resolveTicketRef turns a command's ticket argument — a file path or an
// address — into the ticket file's path. An existing file always wins, so a
// path never gets reinterpreted; anything that is not an address either is
// passed through for the caller's own "no such file" error.
func resolveTicketRef(getwd func() (string, error), arg string) (string, error) {
	if _, err := os.Stat(arg); err == nil || getwd == nil {
		return arg, nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", err
	}
	_, t, err := findTicket(cwd, arg)
	var addrErr *tickets.AddressError
	if errors.As(err, &addrErr) && addrErr.Code == tickets.CodeMalformedAddress {
		return arg, nil
	}
	if err != nil {
		return "", err
	}
	return t.Path, nil
}

// ticketLabel is how output names the ticket at path: its canonical address
// when path sits in the tracker's <project>/<epic>/issues/<file>.md layout,
// the path itself for anything else (ad-hoc files have no address).
func ticketLabel(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	issuesDir := filepath.Dir(abs)
	if filepath.Base(issuesDir) != "issues" {
		return path
	}
	t, err := schema.ParseTicket(abs)
	if err != nil {
		return path
	}
	epicDir := filepath.Dir(issuesDir)
	return tickets.Address{
		Project: tickets.ProjectName(filepath.Dir(epicDir)),
		Epic:    filepath.Base(epicDir),
		ID:      string(t.ID),
	}.String()
}

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// findTicket resolves addr (full or short form) against the cwd project. An
// address that names nothing is refused with a tickets.AddressError carrying
// a stable code.
func findTicket(cwd, addr string) (tickets.Address, tickets.Ticket, error) {
	cwdRoot, rootErr := ticketRoot(cwd)
	ctxProject := ""
	if rootErr == nil {
		ctxProject = tickets.ProjectName(cwdRoot)
	}
	a, err := tickets.ParseAddress(addr, tickets.AddressContext{Project: ctxProject, Epic: currentEpic(cwd)})
	if err != nil {
		return a, tickets.Ticket{}, err
	}
	if a.Project == "" {
		// A short form needs the cwd project, which cwd doesn't have.
		return a, tickets.Ticket{}, rootErr
	}
	root, err := projectRoot(cwd, a.Project)
	if err != nil {
		return a, tickets.Ticket{}, err
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

// projectRoot is the one resolver for a `project:` address prefix: the ticket
// store directory of the registered project called project, which may be the
// cwd project or any other. An unregistered name is refused with
// tickets.CodeUnknownProject. add, show, set and section use it; close and
// move are meant to reuse it.
func projectRoot(cwd, project string) (string, error) {
	if root, err := ticketRoot(cwd); err == nil && tickets.ProjectName(root) == project {
		return root, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return tickets.ProjectDirByName(cfg.TicketStore.Path, project)
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
	if project, epic, ok := tickets.ParseEpicAddress(arg); ok {
		root, err := projectRoot(cwd, project)
		if err != nil {
			return "", err
		}
		return tickets.EpicTicketMDPath(root, project, epic)
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

// isEpicTicketMD reports whether path is an epic's own ticket.md, the file a
// project:epic address resolves to.
func isEpicTicketMD(path string) bool {
	return filepath.Base(path) == "ticket.md"
}

// ticketLabel is how output names the ticket at path: its canonical address
// when path sits in the tracker's <project>/<epic>/issues/<file>.md layout,
// the path itself for anything else (ad-hoc files have no address). An epic's
// ticket.md is labelled by the epic's own address, project:epic.
func ticketLabel(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if isEpicTicketMD(abs) {
		epicDir := filepath.Dir(abs)
		return tickets.ProjectName(filepath.Dir(epicDir)) + ":" + filepath.Base(epicDir)
	}
	t, err := schema.ParseTicket(abs)
	if err != nil {
		return path
	}
	a, ok := tickets.AddressOfPath(abs, string(t.ID))
	if !ok {
		return path
	}
	return a.String()
}

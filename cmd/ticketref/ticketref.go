// Package ticketref resolves the ticket, epic and project a command names:
// addresses, file paths and bare epic names.
package ticketref

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketgit"
)

// FindTicket resolves addr (full or short form) against the cwd project. An
// address that names nothing is refused with a tickets.AddressError carrying
// a stable code.
func FindTicket(dirs config.Dirs, cwd, addr string) (tickets.Address, tickets.Ticket, error) {
	a, t, err := lookupTicket(dirs, cwd, addr)
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || (ae.Code != tickets.CodeUnknownTicket && ae.Code != tickets.CodeUnknownEpic) {
		return a, t, err
	}
	// An archived one-off keeps answering to its old address.
	root, rootErr := ProjectRoot(dirs, cwd, a.Project)
	if rootErr != nil {
		return a, t, err
	}
	if moved, mt, ferr := tickets.FindOriginal(a.Project, root, a); ferr == nil || !isUnknownTicket(ferr) {
		return moved, mt, ferr
	}
	return a, t, err
}

func lookupTicket(dirs config.Dirs, cwd, addr string) (tickets.Address, tickets.Ticket, error) {
	cwdRoot, rootErr := tickets.RootForDirs(dirs, cwd)
	ctxProject := ""
	if rootErr == nil {
		ctxProject = tickets.ProjectName(cwdRoot)
	}
	a, err := tickets.ParseAddress(addr, tickets.AddressContext{Project: ctxProject, Epic: CurrentEpic(cwd)})
	if err != nil {
		return a, tickets.Ticket{}, err
	}
	if a.Project == "" {
		// A short form needs the cwd project, which cwd doesn't have.
		return a, tickets.Ticket{}, rootErr
	}
	root, err := ProjectRoot(dirs, cwd, a.Project)
	if err != nil {
		return a, tickets.Ticket{}, err
	}

	epics, err := tickets.LoadResolved(a.Project, root)
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

func isUnknownTicket(err error) bool {
	var ae *tickets.AddressError
	return errors.As(err, &ae) && ae.Code == tickets.CodeUnknownTicket
}

// ProjectRoot is the one resolver for a `project:` address prefix: the ticket
// store directory of the registered project called project, which may be the
// cwd project or any other. An unregistered name is refused with
// tickets.CodeUnknownProject. add, show, set and section use it; close and
// move are meant to reuse it.
func ProjectRoot(dirs config.Dirs, cwd, project string) (string, error) {
	if root, err := tickets.RootForDirs(dirs, cwd); err == nil && tickets.ProjectName(root) == project {
		return root, nil
	}
	cfg, err := dirs.Load()
	if err != nil {
		return "", err
	}
	return tickets.ProjectDirByName(cfg.TicketStore.Path, project)
}

// ResolveProject is ProjectRoot for a command with an optional project
// argument: an empty one means the cwd's project. It returns the name too.
func ResolveProject(env cmdbase.Env, project string) (root, name string, err error) {
	cwd, err := env.Getwd()
	if err != nil {
		return "", "", err
	}
	if project == "" {
		cwdRoot, err := tickets.RootForDirs(env.Dirs(), cwd)
		if err != nil {
			return "", "", err
		}
		project = tickets.ProjectName(cwdRoot)
	}
	root, err = ProjectRoot(env.Dirs(), cwd, project)
	return root, project, err
}

// ResolveTicketRef turns a command's ticket argument — a file path or an
// address — into the ticket file's path. An existing file always wins, so a
// path never gets reinterpreted; anything that is not an address either is
// passed through for the caller's own "no such file" error.
func ResolveTicketRef(dirs config.Dirs, getwd func() (string, error), arg string) (string, error) {
	if _, err := os.Stat(arg); err == nil || getwd == nil {
		return arg, nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", err
	}
	if project, epic, ok := tickets.ParseEpicAddress(arg); ok {
		root, err := ProjectRoot(dirs, cwd, project)
		if err != nil {
			return "", err
		}
		return tickets.EpicTicketMDPath(root, project, epic)
	}
	_, t, err := FindTicket(dirs, cwd, arg)
	var addrErr *tickets.AddressError
	if errors.As(err, &addrErr) && addrErr.Code == tickets.CodeMalformedAddress {
		return arg, nil
	}
	if err != nil {
		return "", err
	}
	return t.Path, nil
}

// IsEpicTicketMD reports whether path is an epic's own ticket.md, the file a
// project:epic address resolves to.
func IsEpicTicketMD(path string) bool {
	return filepath.Base(path) == "ticket.md"
}

// TicketLabel is how output names the ticket at path: its canonical address
// when path sits in the tracker's <project>/<epic>/issues/<file>.md layout,
// the path itself for anything else (ad-hoc files have no address). An epic's
// ticket.md is labelled by the epic's own address, project:epic.
func TicketLabel(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if IsEpicTicketMD(abs) {
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

// CurrentEpic is the epic cwd's worktree is working on, read from its
// iteration branch name; "" on any other branch.
func CurrentEpic(cwd string) string {
	branch, err := git.CurrentBranch(cwd)
	if err != nil {
		return ""
	}
	epic, _, _ := ticketgit.ParseIterationBranch(branch)
	return epic
}

// ResolveEpicArg resolves a command's single epic argument to an epic path. A
// `project:epic` address resolves against that project, as `gx tickets show`
// does. Otherwise arg is tried as a bare epic name against the current repo's
// ticket-store root (`gx tickets root`); if that directory doesn't exist, arg
// is returned unchanged so a full path still works.
func ResolveEpicArg(dirs config.Dirs, arg, cwd string) string {
	if project, epic, ok := tickets.ParseEpicAddress(arg); ok {
		if root, err := ProjectRoot(dirs, cwd, project); err == nil {
			return filepath.Join(root, epic)
		}
		return arg
	}
	root, err := tickets.RootForDirs(dirs, cwd)
	if err != nil {
		return arg
	}
	candidate := filepath.Join(root, arg)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return arg
}

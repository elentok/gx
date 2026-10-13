package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// ticketShowJSON is the --json shape of `gx tickets show`.
type ticketShowJSON struct {
	Address   string   `json:"address"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	Parent    *string  `json:"parent"`
	BlockedBy []string `json:"blocked_by"`
	Path      string   `json:"path"`
	Body      string   `json:"body"`
}

// currentEpic is the epic cwd's worktree is working on, read from its
// iteration branch name; "" on any other branch.
func currentEpic(cwd string) string {
	branch, err := git.CurrentBranch(cwd)
	if err != nil {
		return ""
	}
	epic, _, _ := parseIterationBranch(branch)
	return epic
}

// runTicketsShow resolves addr (full or short form) against the cwd project
// and prints the ticket. Short forms are expanded before anything is
// printed; an address that names nothing is refused with an
// tickets.AddressError carrying a stable code.
func runTicketsShow(cwd, addr string, jsonOut bool, w io.Writer) error {
	a, t, err := findTicket(cwd, addr)
	var addrErr *tickets.AddressError
	if errors.As(err, &addrErr) && addrErr.Code == tickets.CodeMalformedAddress {
		if project, epic, ok := parseEpicArg(addr); ok {
			if jsonOut {
				return &tickets.AddressError{Code: tickets.CodeEpicJSON, Msg: "--json is not supported for an epic address"}
			}
			return showEpic(cwd, project, epic, w)
		}
	}
	if err != nil {
		return err
	}
	return printTicket(a, t, jsonOut, w)
}

// parseEpicArg accepts "project:epic", or a bare epic slug, which leaves
// project empty for the cwd project to fill in.
func parseEpicArg(s string) (project, epic string, ok bool) {
	if project, epic, ok := tickets.ParseEpicAddress(s); ok {
		return project, epic, true
	}
	if s == "" || strings.ContainsAny(s, ":/ \t\n") {
		return "", "", false
	}
	return "", s, true
}

// showEpic prints the epic's ticket.md verbatim, then one line per ticket.
func showEpic(cwd, project, name string, w io.Writer) error {
	var root string
	var err error
	if project == "" {
		root, err = ticketRoot(cwd)
		project = tickets.ProjectName(root)
	} else {
		root, err = projectRoot(cwd, project)
	}
	if err != nil {
		return err
	}
	path, err := tickets.EpicTicketMDPath(root, project, name)
	if err != nil {
		return err
	}
	entry, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s:%s\npath: %s\n%s", project, name, path, entry)
	if len(entry) > 0 && entry[len(entry)-1] != '\n' {
		out.WriteString("\n")
	}
	out.WriteString("\ntickets:\n")
	for _, epic := range epics {
		if epic.Name != name {
			continue
		}
		for _, t := range epic.Tickets {
			fmt.Fprintf(&out, "%s  %s  %s\n", t.Identifier, t.Status, t.Title)
		}
	}
	_, err = io.WriteString(w, out.String())
	return err
}

func printTicket(a tickets.Address, t tickets.Ticket, jsonOut bool, w io.Writer) error {
	if t.ReadErr != "" {
		return fmt.Errorf("%s: %s", a, t.ReadErr)
	}
	blocked := t.BlockedBy
	if blocked == nil {
		blocked = []string{}
	}
	if jsonOut {
		return encodeProvenance(w, ticketShowJSON{
			Address: a.String(), Title: t.Title, Type: t.Type, Status: t.Status,
			Parent: t.Parent, BlockedBy: blocked, Path: t.Path, Body: t.Body,
		}, viaDirect, callerActor(os.Getwd))
	}
	fmt.Fprintf(w, "%s  %s\n", a, t.Title)
	fmt.Fprintf(w, "status: %s\ntype: %s\n", t.Status, t.Type)
	if t.Parent != nil {
		fmt.Fprintf(w, "parent: %s\n", *t.Parent)
	}
	if len(blocked) > 0 {
		// JSON keeps the file's tokens; only the text form shows the widening.
		shown := make([]string, len(blocked))
		for i, ref := range blocked {
			shown[i] = ref
			if epic, ok := t.WidenedBlockers[ref]; ok {
				shown[i] = fmt.Sprintf("%s (via %s)", epic, ref)
			}
		}
		fmt.Fprintf(w, "blocked_by: %s\n", strings.Join(shown, ", "))
	}
	fmt.Fprintf(w, "path: %s\n%s", t.Path, t.Body)
	if !strings.HasSuffix(t.Body, "\n") {
		fmt.Fprintln(w)
	}
	return nil
}

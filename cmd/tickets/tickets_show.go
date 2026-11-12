package tickets

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// ticketShowJSON is the --json shape of `gx tickets show`.
type ticketShowJSON struct {
	Address   string   `json:"address"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Status    string   `json:"status"`
	Parent    *string  `json:"parent"`
	Agent     string   `json:"agent,omitempty"`
	BlockedBy []string `json:"blocked_by"`
	Path      string   `json:"path"`
	Body      string   `json:"body"`
}

// runTicketsShow resolves addr (full or short form) against the cwd project
// and prints the ticket. Short forms are expanded before anything is
// printed; an address that names nothing is refused with an
// tickets.AddressError carrying a stable code.
//
// With a non-empty section it prints only that "## section" of the ticket, or
// of every ticket of an epic that has it, each under a line naming the ticket.
// A single ticket without the section is an error; --section and --json are
// kept apart by the command's flag definitions.
func runTicketsShow(dirs config.Dirs, cwd, addr string, jsonOut bool, section, only string, w io.Writer) error {
	filter, err := parseTicketNumbers(only)
	if err != nil {
		return err
	}
	if filter != nil && section == "" {
		return errors.New("--tickets needs --section")
	}
	a, t, err := ticketref.FindTicket(dirs, cwd, addr)
	var addrErr *tickets.AddressError
	if errors.As(err, &addrErr) && addrErr.Code == tickets.CodeMalformedAddress {
		if project, epic, ok := parseEpicArg(addr); ok {
			if jsonOut {
				return &tickets.AddressError{Code: tickets.CodeEpicJSON, Msg: "--json is not supported for an epic address"}
			}
			if section != "" {
				return showEpicSection(dirs, cwd, project, epic, section, filter, w)
			}
			return showEpic(dirs, cwd, project, epic, w)
		}
	}
	if err != nil {
		return err
	}
	if filter != nil {
		return errors.New("--tickets only applies to an epic address")
	}
	if section != "" {
		if t.ReadErr == "" && !schema.HasSection(t.Body, section) {
			return fmt.Errorf("no section %s in %s", section, a)
		}
		return printTicketSection(a, t, section, w)
	}
	return printTicket(a, t, jsonOut, w)
}

// printTicketSection prints the ticket's address and title, then the named
// section; a ticket without the section prints nothing.
func printTicketSection(a tickets.Address, t tickets.Ticket, section string, w io.Writer) error {
	if t.ReadErr != "" {
		return fmt.Errorf("%s: %s", a, t.ReadErr)
	}
	if !schema.HasSection(t.Body, section) {
		return nil
	}
	_, err := fmt.Fprintf(w, "%s  %s\n%s\n\n", a, t.Title, schema.Section(t.Body, section))
	return err
}

// resolveEpic finds the ticket root and project for an epic argument whose
// project may be empty (the cwd project), and checks the epic exists.
func resolveEpic(dirs config.Dirs, cwd, project, name string) (root, resolved, path string, err error) {
	if project == "" {
		root, err = tickets.RootForDirs(dirs, cwd)
		project = tickets.ProjectName(root)
	} else {
		root, err = ticketref.ProjectRoot(dirs, cwd, project)
	}
	if err != nil {
		return "", "", "", err
	}
	path, err = tickets.EpicTicketMDPath(root, project, name)
	if err != nil {
		return "", "", "", err
	}
	return root, project, path, nil
}

// parseTicketNumbers parses a --tickets value such as "15-18,20" into a set of
// ticket numbers. An empty value returns nil, meaning every ticket.
func parseTicketNumbers(s string) (map[int]bool, error) {
	if s == "" {
		return nil, nil
	}
	set := map[int]bool{}
	for part := range strings.SplitSeq(s, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(part), "-")
		from, err := strconv.Atoi(lo)
		to := from
		if err == nil && isRange {
			to, err = strconv.Atoi(hi)
		}
		if err != nil || from < 0 || to < from {
			return nil, fmt.Errorf("--tickets: bad entry %q, want numbers or ranges like 15-18,20", part)
		}
		for n := from; n <= to; n++ {
			set[n] = true
		}
	}
	return set, nil
}

// showEpicSection prints the named section of each of the epic's tickets,
// limited to the numbers in filter when it is non-nil.
func showEpicSection(dirs config.Dirs, cwd, project, name, section string, filter map[int]bool, w io.Writer) error {
	root, project, _, err := resolveEpic(dirs, cwd, project, name)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	for _, epic := range epics {
		if epic.Name != name {
			continue
		}
		for _, t := range epic.Tickets {
			if filter != nil && !filter[t.Number] {
				continue
			}
			a := tickets.Address{Project: project, Epic: name, ID: t.Identifier}
			if err := printTicketSection(a, t, section, w); err != nil {
				return err
			}
		}
	}
	return nil
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
func showEpic(dirs config.Dirs, cwd, project, name string, w io.Writer) error {
	root, project, path, err := resolveEpic(dirs, cwd, project, name)
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
		return cmdbase.EncodeProvenance(w, ticketShowJSON{
			Address: a.String(), Title: t.Title, Type: t.Type, Status: t.Status,
			Parent: t.Parent, Agent: t.Agent, BlockedBy: blocked, Path: t.Path, Body: t.Body,
		}, cmdbase.ViaDirect, cmdbase.CallerActor(os.Getwd))
	}
	fmt.Fprintf(w, "%s  %s\n", a, t.Title)
	fmt.Fprintf(w, "status: %s\ntype: %s\n", t.Status, t.Type)
	if t.Parent != nil {
		fmt.Fprintf(w, "parent: %s\n", *t.Parent)
	}
	if t.Agent != "" {
		fmt.Fprintf(w, "agent: %s\n", t.Agent)
	}
	if len(blocked) > 0 {
		// JSON keeps the file's tokens; only the text form shows the widening.
		shown := make([]string, len(blocked))
		for i, ref := range blocked {
			shown[i] = t.BlockerLabel(ref)
		}
		fmt.Fprintf(w, "blocked_by: %s\n", strings.Join(shown, ", "))
	}
	fmt.Fprintf(w, "path: %s\n%s", t.Path, t.Body)
	if !strings.HasSuffix(t.Body, "\n") {
		fmt.Fprintln(w)
	}
	events, err := tickets.ReadLog(t.Path)
	if err != nil {
		return err
	}
	if rounds := ticketlog.RenderRounds(events); rounds != "" {
		fmt.Fprintf(w, "\n%s", rounds)
	}
	return nil
}

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
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
	if err != nil {
		return err
	}
	return printTicket(a, t, jsonOut, w)
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
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(ticketShowJSON{
			Address: a.String(), Title: t.Title, Type: t.Type, Status: t.Status,
			Parent: t.Parent, BlockedBy: blocked, Path: t.Path, Body: t.Body,
		})
	}
	fmt.Fprintf(w, "%s  %s\n", a, t.Title)
	fmt.Fprintf(w, "status: %s\ntype: %s\n", t.Status, t.Type)
	if t.Parent != nil {
		fmt.Fprintf(w, "parent: %s\n", *t.Parent)
	}
	if len(blocked) > 0 {
		fmt.Fprintf(w, "blocked_by: %s\n", strings.Join(blocked, ", "))
	}
	fmt.Fprintf(w, "path: %s\n%s", t.Path, t.Body)
	if !strings.HasSuffix(t.Body, "\n") {
		fmt.Fprintln(w)
	}
	return nil
}

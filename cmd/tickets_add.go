package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runTicketsAdd atomically allocates the next ticket ID for epicPath (a
// flat sibling with no parent, a lettered child of parent, or the next
// number under a lettered parent regardless of whether parent itself
// already carries trailing digits — see tickets.NextTicketID) and writes a
// minimal stub ticket file for the caller to fill in, printing the created
// file's path to w. The stub is written status: draft — parked work that
// never enters an epic's frontier — so a freshly allocated, still-empty
// ticket can never be handed to an agent; `set --status open` is what
// promotes it once the body has real content. slug must be non-empty — it
// becomes the stub's filename suffix (<id>-<slug>.md), so the file lands
// with a real name instead of a placeholder the caller has to remember to
// rename.
func runTicketsAdd(epicPath, parent, slug string, w io.Writer) error {
	path, err := createTicket(epicPath, parent, slug, nil)
	if err != nil {
		return err
	}
	pingServer(path)
	// The address, like everywhere else the CLI names a ticket: `set` and
	// `section` accept it to fill the stub in, and `show` gives the path.
	return printTicketRef(path, false, w)
}

// runTicketsAddBody is runTicketsAdd for agents: the ticket arrives with its
// body and is written open in that one write (no draft stub to promote), and
// the output names it by address — an agent never needs the file path.
func runTicketsAddBody(epicPath, parent, slug, body string, jsonOut bool, w io.Writer) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("empty body; refusing to create an open ticket with no content")
	}
	path, err := createTicket(epicPath, parent, slug, &body)
	if err != nil {
		return err
	}
	pingServer(path)
	return printTicketRef(path, jsonOut, w)
}

// createTicket allocates the next id under the epic lock and writes the new
// ticket file, returning its path. A nil body writes the empty draft stub; a
// non-nil body writes an open ticket carrying it.
func createTicket(epicPath, parent, slug string, body *string) (string, error) {
	if slug == "" {
		return "", errors.New("slug is required")
	}

	epicPath = filepath.Clean(epicPath)

	epic, unlock, err := tickets.LoadLockedEpic(epicPath)
	if err != nil {
		return "", err
	}
	defer unlock()

	id, err := tickets.NextTicketID(*epic, parent)
	if err != nil {
		return "", err
	}

	path := filepath.Join(epicPath, "issues", fmt.Sprintf("%s-%s.md", id, slug))
	t := schema.Ticket{
		ID:     schema.TicketID(id),
		Status: schema.StatusDraft,
		Type:   schema.TypeImplement,
	}
	if parent != "" {
		parentID := schema.TicketID(parent)
		t.Parent = &parentID
	}

	text := fmt.Sprintf(
		"\n# %s\n\n## What to build\n\n\n## Test seams\n\n\n## Acceptance criteria\n\n- [ ] \n",
		id,
	)
	if body != nil {
		t.Status = schema.StatusOpen
		text = "\n" + strings.Trim(*body, "\n") + "\n"
	}

	out, err := schema.MarshalTicket(t, text)
	if err != nil {
		return "", fmt.Errorf("marshaling ticket: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return "", fmt.Errorf("writing ticket %s: %w", path, err)
	}

	if _, err := schema.ParseTicket(path); err != nil {
		return "", fmt.Errorf("ticket %s failed validation: %w", path, err)
	}
	return path, nil
}

// resolveAddTarget turns add's argument into the epic to allocate in and the
// parent ticket id: an epic (name or path) as before, or a ticket address,
// which names the epic and the parent in one go.
func resolveAddTarget(d deps, arg, parentFlag string) (epicPath, parent string, err error) {
	cwd, err := d.getwd()
	if err != nil {
		return "", "", err
	}
	epicPath = resolveEpicArg(arg, cwd)
	if info, statErr := os.Stat(epicPath); statErr == nil && info.IsDir() {
		return epicPath, parentFlag, nil
	}
	_, t, findErr := findTicket(cwd, arg)
	if findErr != nil {
		// Not an address either: let the epic path's own error surface.
		return epicPath, parentFlag, nil
	}
	if parentFlag != "" {
		return "", "", errors.New("--parent conflicts with a ticket address argument")
	}
	return filepath.Dir(filepath.Dir(t.Path)), t.Identifier, nil
}

// readBodyArg returns arg, or all of stdin when arg is "-".
func readBodyArg(d deps, arg string) (string, error) {
	if arg != "-" {
		return arg, nil
	}
	in := d.stdin
	if in == nil {
		in = os.Stdin
	}
	b, err := io.ReadAll(in)
	return string(b), err
}

// printTicketRef prints the ticket at path by address, or as {address, path}
// JSON.
func printTicketRef(path string, jsonOut bool, w io.Writer) error {
	label := ticketLabel(path)
	if !jsonOut {
		_, err := fmt.Fprintln(w, label)
		return err
	}
	return json.NewEncoder(w).Encode(struct {
		Address string `json:"address"`
		Path    string `json:"path"`
	}{label, path})
}

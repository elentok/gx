package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
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
	path, err := createTicket(epicPath, parent, slug, nil, addFrontmatter{})
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
//
// The body may open with its own frontmatter block (the ticket template does):
// its settable fields are merged in rather than written as a second block. See
// bodyFrontmatter.
func runTicketsAddBody(epicPath, parent, slug, body string, jsonOut bool, w, stderr io.Writer) error {
	fm, text, err := bodyFrontmatter(body)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("empty body; refusing to create an open ticket with no content")
	}
	if fm.Parent != "" {
		if parent != "" && parent != fm.Parent {
			return fmt.Errorf("--parent %s conflicts with the body's parent: %s", parent, fm.Parent)
		}
		parent = fm.Parent
	}
	path, err := createTicket(epicPath, parent, slug, &text, fm)
	if err != nil {
		return err
	}
	if fm.ID != "" && stderr != nil {
		if got := ticketLabel(path); !strings.HasSuffix(got, "/"+fm.ID) {
			fmt.Fprintf(stderr, "warning: body says id %q; the allocated id wins: %s\n", fm.ID, got)
		}
	}
	if err != nil {
		return err
	}
	pingServer(path)
	return printTicketRef(path, jsonOut, w)
}

// addFrontmatter is what `add --body` takes from a frontmatter block at the
// top of the body: the settable fields, plus id and status, which are checked
// rather than written. Any other field refuses the add.
type addFrontmatter struct {
	ID                    string   `yaml:"id"`
	Status                string   `yaml:"status"`
	BlockedBy             []string `yaml:"blocked_by"`
	Type                  string   `yaml:"type"`
	ExpectedContextWindow int      `yaml:"expected_context_window"`
	Commitless            bool     `yaml:"commitless"`
	Parent                string   `yaml:"parent"`
}

// bodyFrontmatter splits a leading frontmatter block off body and validates
// it. A body with no block returns a zero addFrontmatter and body unchanged.
func bodyFrontmatter(body string) (addFrontmatter, string, error) {
	var fm addFrontmatter
	yamlPart, text, ok := schema.SplitFrontmatter(strings.TrimLeft(body, "\n"))
	if !ok {
		return fm, body, nil
	}
	dec := yaml.NewDecoder(strings.NewReader(yamlPart))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil && !errors.Is(err, io.EOF) {
		return fm, "", fmt.Errorf("body frontmatter: %w (settable: blocked_by, type, expected_context_window, commitless, parent)", err)
	}
	switch schema.Status(fm.Status) {
	case "", schema.StatusOpen, schema.StatusDraft:
	default:
		return fm, "", fmt.Errorf("body frontmatter: status %q; a new ticket is open or draft", fm.Status)
	}
	if fm.Type != "" && !schema.TicketType(fm.Type).Valid() {
		return fm, "", fmt.Errorf("body frontmatter: unknown type %q", fm.Type)
	}
	if fm.ExpectedContextWindow < 0 {
		return fm, "", errors.New("body frontmatter: expected_context_window must not be negative")
	}
	return fm, text, nil
}

// createTicket allocates the next id under the epic lock and writes the new
// ticket file, returning its path. A nil body writes the empty draft stub; a
// non-nil body writes an open ticket carrying it, with fm's fields applied.
func createTicket(epicPath, parent, slug string, body *string, fm addFrontmatter) (string, error) {
	if slug == "" {
		return "", errors.New("slug is required")
	}

	epicPath = filepath.Clean(epicPath)
	if _, err := os.Stat(epicPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0755); err != nil {
			return "", err
		}
		if err := tickets.WriteEpicTicketMD(epicPath, schema.StatusOpen); err != nil {
			return "", err
		}
	}

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
		if fm.Status != "" {
			t.Status = schema.Status(fm.Status)
		}
		if fm.Type != "" {
			t.Type = schema.TicketType(fm.Type)
		}
		t.BlockedBy = parseCSVIDs(strings.Join(fm.BlockedBy, ","))
		t.ExpectedContextWindow = fm.ExpectedContextWindow
		t.Commitless = fm.Commitless
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
	return writeStamped(w, struct {
		Address string `json:"address"`
		Path    string `json:"path"`
	}{label, path}, viaDirect, callerActor(os.Getwd))
}

package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// moveRefusal is a refused move with a stable code.
func moveRefusal(code, format string, args ...any) error {
	return &tickets.AddressError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// runTicketsMove moves one flat, never-run ticket of the cwd-resolved source
// to another epic of the same project, written directly like `add`. Both epic
// locks are held, in path order, from the checks through the write; the new
// file lands before the old one is deleted so a crash never loses the ticket.
func runTicketsMove(d deps, addr, target, slug string, dryRun, jsonOut bool, w io.Writer) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	a, src, err := findTicket(cwd, addr)
	if err != nil {
		return err
	}
	srcEpicPath := filepath.Dir(filepath.Dir(src.Path))
	root := filepath.Dir(srcEpicPath)

	dstProject, dstEpic := a.Project, target
	if m := projectEpicRe.FindStringSubmatch(target); m != nil {
		dstProject, dstEpic = m[1], m[2]
	}
	if dstProject != a.Project {
		if _, err := projectRoot(cwd, dstProject); err != nil {
			return err
		}
		return moveRefusal(tickets.CodeMoveCrossProject, "target epic %q is in project %q, not %q", dstEpic, dstProject, a.Project)
	}
	if dstEpic == a.Epic {
		return moveRefusal(tickets.CodeMoveSameEpic, "ticket %s is already in epic %q", a.ID, a.Epic)
	}
	dstEpicPath := filepath.Join(root, dstEpic)
	if info, err := os.Stat(dstEpicPath); err != nil || !info.IsDir() {
		return moveRefusal(tickets.CodeUnknownEpic, "no epic %q in project %q", dstEpic, a.Project)
	}
	if tickets.IsMapEpic(srcEpicPath) {
		return moveRefusal(tickets.CodeMoveMapSource, "epic %q is a map; its tickets cannot be moved out", a.Epic)
	}

	unlock, err := lockEpics(srcEpicPath, dstEpicPath)
	if err != nil {
		return err
	}
	defer unlock()

	plan, err := planMove(root, a, src, dstEpic, slug)
	if err != nil {
		return err
	}
	if dryRun {
		return printMove(w, plan, jsonOut, "dry-run")
	}

	if err := applyMove(plan); err != nil {
		return err
	}
	pingServer(plan.newPath)
	return printMove(w, plan, jsonOut, "")
}

// lockEpics locks every epic in path order, so two moves in opposite
// directions can't each hold one lock and wait on the other.
func lockEpics(paths ...string) (unlock func(), err error) {
	sorted := slices.Sorted(slices.Values(paths))
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, p := range sorted {
		u, err := tickets.LockEpic(p)
		if err != nil {
			release()
			return nil, err
		}
		unlocks = append(unlocks, u)
	}
	return release, nil
}

type movePlan struct {
	from, to  tickets.Address
	oldPath   string
	newPath   string
	ticket    schema.Ticket
	body      string
	rewritten []string
}

// planMove runs the refusals and computes the moved ticket, writing nothing.
// Called with both epic locks held, so what it reads is what applyMove writes.
func planMove(root string, a tickets.Address, src tickets.Ticket, dstEpic, slug string) (movePlan, error) {
	raw, err := os.ReadFile(src.Path)
	if err != nil {
		return movePlan{}, err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), src.Path)
	if err != nil {
		return movePlan{}, err
	}
	if t.Status != schema.StatusDraft && t.Status != schema.StatusOpen {
		return movePlan{}, moveRefusal(tickets.CodeMoveStatus, "ticket %s is %s; only draft or open tickets move", a.ID, t.Status)
	}
	if len(t.SessionIDs) > 0 || t.ResolvedBase != "" || t.IterationStatus != "" {
		return movePlan{}, moveRefusal(tickets.CodeMoveRunHistory, "ticket %s has run history", a.ID)
	}

	epics, err := tickets.Load(root)
	if err != nil {
		return movePlan{}, err
	}
	var dst, srcEpic *tickets.Epic
	for i := range epics {
		switch epics[i].Name {
		case dstEpic:
			dst = &epics[i]
		case a.Epic:
			srcEpic = &epics[i]
		}
	}
	if dst == nil || srcEpic == nil {
		return movePlan{}, moveRefusal(tickets.CodeUnknownEpic, "no epic %q in project %q", dstEpic, a.Project)
	}
	if t.Parent != nil {
		return movePlan{}, moveRefusal(tickets.CodeMoveLineage, "ticket %s is a fork of %s", a.ID, *t.Parent)
	}
	for _, o := range srcEpic.Tickets {
		if o.Parent != nil && *o.Parent == a.ID {
			return movePlan{}, moveRefusal(tickets.CodeMoveLineage, "ticket %s has fork child %s", a.ID, o.Identifier)
		}
	}

	newID, err := tickets.NextTicketID(*dst, "")
	if err != nil {
		return movePlan{}, err
	}
	if slug == "" {
		slug = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(src.Path), a.ID+"-"), ".md")
	}

	p := movePlan{
		from:    a,
		to:      tickets.Address{Project: a.Project, Epic: dstEpic, ID: newID},
		oldPath: src.Path,
		newPath: filepath.Join(root, dstEpic, "issues", fmt.Sprintf("%s-%s.md", newID, slug)),
		ticket:  t,
		body:    schema.ParseBody(string(raw)),
	}
	p.ticket.ID = schema.TicketID(newID)
	p.rewritten = []string{"id: " + a.ID + " -> " + newID}

	for i, ref := range p.ticket.BlockedBy {
		if next := rewriteMovedRef(string(ref), a.Epic, dstEpic, a.Project); next != string(ref) {
			p.ticket.BlockedBy[i] = schema.TicketID(next)
			p.rewritten = append(p.rewritten, fmt.Sprintf("blocked_by: %s -> %s", ref, next))
		}
	}
	if p.ticket.Base != "" {
		if next := rewriteMovedRef(p.ticket.Base, a.Epic, dstEpic, a.Project); next != p.ticket.Base {
			p.rewritten = append(p.rewritten, fmt.Sprintf("base: %s -> %s", p.ticket.Base, next))
			p.ticket.Base = next
		}
	}
	if body, ok := retitle(p.body, a.ID, newID); ok {
		p.body = body
		p.rewritten = append(p.rewritten, "title: # "+a.ID+" -> # "+newID)
	}

	if err := schema.Validate(p.ticket); err != nil {
		return movePlan{}, fmt.Errorf("moved ticket would be invalid: %w", err)
	}
	if _, err := os.Stat(p.newPath); err == nil {
		return movePlan{}, fmt.Errorf("%s already exists", p.newPath)
	}
	return p, nil
}

// applyMove writes the new file, then deletes the old one, under the old
// file's ticket lock.
func applyMove(p movePlan) error {
	unlock, err := schema.LockTicket(p.oldPath)
	if err != nil {
		return err
	}
	defer unlock()

	out, err := schema.MarshalTicket(p.ticket, p.body)
	if err != nil {
		return fmt.Errorf("marshaling ticket: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.newPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(p.newPath, out, 0644); err != nil {
		return fmt.Errorf("writing ticket %s: %w", p.newPath, err)
	}
	return os.Remove(p.oldPath)
}

func printMove(w io.Writer, p movePlan, jsonOut bool, mode string) error {
	if jsonOut {
		rewritten := p.rewritten
		if rewritten == nil {
			rewritten = []string{}
		}
		return writeStamped(w, struct {
			From      string   `json:"from"`
			To        string   `json:"to"`
			Path      string   `json:"path"`
			Rewritten []string `json:"rewritten"`
			DryRun    bool     `json:"dry_run,omitempty"`
		}{p.from.String(), p.to.String(), p.newPath, rewritten, mode == "dry-run"}, viaDirect, callerActor(os.Getwd))
	}
	if mode != "dry-run" {
		_, err := fmt.Fprintln(w, p.to.String())
		return err
	}
	if _, err := fmt.Fprintf(w, "%s -> %s (%s)\n", p.from, p.to, p.newPath); err != nil {
		return err
	}
	for _, r := range p.rewritten {
		if _, err := fmt.Fprintln(w, "  "+r); err != nil {
			return err
		}
	}
	return nil
}

// rewriteMovedRef re-spells one of the moved ticket's own ticket refs for its
// new epic: a bare ref named a ticket of the source epic, so it is qualified;
// a ref qualified to the target epic is now bare. Anything else (another
// epic, a branch name for base:) is left alone.
func rewriteMovedRef(ref, srcEpic, dstEpic, project string) string {
	if schema.TicketID(ref).Valid() {
		return srcEpic + "/" + ref
	}
	rest := ref
	if p, r, ok := strings.Cut(ref, ":"); ok {
		if p != project {
			return ref
		}
		rest = r
	}
	if epic, id, ok := strings.Cut(rest, "/"); ok && epic == dstEpic && schema.TicketID(id).Valid() {
		return id
	}
	return ref
}

var titleRe = regexp.MustCompile(`(?m)^# (\S+)`)

// retitle swaps the old id for the new one in the first `# <id> ...` heading,
// when the heading starts with the old id.
func retitle(body, oldID, newID string) (string, bool) {
	loc := titleRe.FindStringSubmatchIndex(body)
	if loc == nil || body[loc[2]:loc[3]] != oldID {
		return body, false
	}
	return body[:loc[2]] + newID + body[loc[3]:], true
}

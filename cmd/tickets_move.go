package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
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

	if err := refuseIfActive(d.dial(), a); err != nil {
		return err
	}

	plan, err := planMove(root, a, src, dstEpic, slug, moveOpts{})
	if err != nil {
		return err
	}
	if dryRun {
		return printMove(w, plan, jsonOut, "dry-run")
	}

	if err := applyMove(plan); err != nil {
		return err
	}
	// The old file is gone, so both sides are pinged by address, not by path.
	pingLabels(d.dial(), plan.from.String(), plan.to.String())
	return printMove(w, plan, jsonOut, "")
}

// refuseIfActive refuses a ticket that is queued or has a live iteration. A
// reachable server answers; otherwise the persisted queue and run files do, and
// if those can't be read either the move is refused rather than guessed.
func refuseIfActive(dial func() (*apiclient.Client, error), a tickets.Address) error {
	active, err := activeAddresses(dial)
	if err != nil {
		return moveRefusal(tickets.CodeMoveActive, "cannot tell whether %s is queued or live: %v", a, err)
	}
	if slices.Contains(active, a.String()) {
		return moveRefusal(tickets.CodeMoveActive, "ticket %s is queued or live on the server", a)
	}
	return nil
}

func activeAddresses(dial func() (*apiclient.Client, error)) ([]string, error) {
	if cl, err := dial(); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		items, qerr := cl.QueueItems(ctx)
		its, ierr := cl.Iterations(ctx)
		if qerr == nil && ierr == nil {
			var out []string
			for _, i := range items {
				out = append(out, i.Address)
			}
			for _, i := range its {
				out = append(out, i.Address)
			}
			return out, nil
		}
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return nil, err
	}
	return server.PersistedActive(stateDir)
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
	inbound   []inboundEdit
	warnings  []string
}

// moveOpts is what `tickets close` changes about a move. The zero value is a
// plain `tickets move`.
type moveOpts struct {
	// archive is the one exception to move's draft/open, never-run rule: a
	// ticket on its way into the archive may be done or have run history.
	archive bool
	// resolution, when set, cancels the ticket and records it as its
	// ## Resolution section.
	resolution string
}

// planMove runs the refusals and computes the moved ticket, writing nothing.
// Called with both epic locks held, so what it reads is what applyMove writes.
func planMove(root string, a tickets.Address, src tickets.Ticket, dstEpic, slug string, opts moveOpts) (movePlan, error) {
	raw, err := os.ReadFile(src.Path)
	if err != nil {
		return movePlan{}, err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), src.Path)
	if err != nil {
		return movePlan{}, err
	}
	if opts.resolution != "" && t.Status != schema.StatusDraft && t.Status != schema.StatusOpen {
		return movePlan{}, moveRefusal(tickets.CodeCloseStatus, "ticket %s is %s; only draft or open follow-ups close", a.ID, t.Status)
	}
	if !opts.archive {
		if t.Status != schema.StatusDraft && t.Status != schema.StatusOpen {
			return movePlan{}, moveRefusal(tickets.CodeMoveStatus, "ticket %s is %s; only draft or open tickets move", a.ID, t.Status)
		}
		if len(t.SessionIDs) > 0 || t.ResolvedBase != "" || t.IterationStatus != "" {
			return movePlan{}, moveRefusal(tickets.CodeMoveRunHistory, "ticket %s has run history", a.ID)
		}
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

	if opts.resolution != "" {
		p.ticket.Status = schema.StatusCancelled
	}

	if err := schema.Validate(p.ticket); err != nil {
		return movePlan{}, fmt.Errorf("moved ticket would be invalid: %w", err)
	}
	if _, err := os.Stat(p.newPath); err == nil {
		return movePlan{}, fmt.Errorf("%s already exists", p.newPath)
	}
	if err := p.planInbound(root, epics); err != nil {
		return movePlan{}, err
	}
	if opts.resolution != "" {
		// After planInbound: the resolution names the old address on purpose,
		// which is no stale prose mention.
		p.body = schema.SetSection(p.body, "Resolution", opts.resolution)
	}
	return p, nil
}

// inboundEdit is one file (a ticket, or an epic's ticket.md) holding refs to
// the moved ticket.
type inboundEdit struct {
	path string
	fn   func(ref string) string
}

// respeller returns the function that re-spells a ref written in epic inEpic
// when it names the moved ticket, matched by sibling key rather than spelling;
// every other ref comes back unchanged.
func respeller(project string, from, to tickets.Address, inEpic string) func(string) string {
	return func(ref string) string {
		rest := ref
		if proj, r, ok := strings.Cut(ref, ":"); ok {
			if proj != project {
				return ref
			}
			rest = r
		}
		epic, id := inEpic, rest
		if e, i, ok := strings.Cut(rest, "/"); ok {
			epic, id = e, i
		}
		if epic != from.Epic || !schema.TicketID(id).Valid() || tickets.SiblingKey(id) != tickets.SiblingKey(from.ID) {
			return ref
		}
		if inEpic == to.Epic {
			return to.ID
		}
		return to.Epic + "/" + to.ID
	}
}

// planInbound finds every blocked_by/base ref to the moved ticket in the
// project (terminal tickets and epic ticket.md files included), validates the
// rewritten project in memory and lists files that still mention the old
// address in prose. It writes nothing; applyMove replays p.inbound.
//
// Only errors the move introduces refuse it: a project that was already
// broken elsewhere must not become unmovable.
func (p *movePlan) planInbound(root string, epics []tickets.Epic) error {
	project := p.from.Project
	rewrittenFiles := map[string]string{} // path -> content after the rewrite
	post := make([]tickets.Epic, len(epics))

	describe := func(path string, fn func(string) string, blockedBy []string, base string) {
		for _, ref := range blockedBy {
			if next := fn(ref); next != ref {
				p.rewritten = append(p.rewritten, fmt.Sprintf("%s: blocked_by: %s -> %s", path, ref, next))
			}
		}
		if next := fn(base); base != "" && next != base {
			p.rewritten = append(p.rewritten, fmt.Sprintf("%s: base: %s -> %s", path, base, next))
		}
	}
	// scan reads path, records what its rewrite yields and queues the edit.
	scan := func(path string, fn func(string) string) (changed bool, err error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		next, changed, err := tickets.RewriteFrontmatterRefs(string(raw), fn)
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
		rewrittenFiles[path] = next
		if changed {
			p.inbound = append(p.inbound, inboundEdit{path: path, fn: fn})
		}
		return changed, nil
	}
	mapRefs := func(refs []string, fn func(string) string) []string {
		out := make([]string, len(refs))
		for i, r := range refs {
			out[i] = fn(r)
		}
		return out
	}

	for i, e := range epics {
		fn := respeller(project, p.from, p.to, e.Name)
		post[i] = e
		post[i].Tickets = nil

		mdPath := filepath.Join(root, e.Name, "ticket.md")
		if e.HasTicketMD {
			changed, err := scan(mdPath, fn)
			if err != nil {
				return err
			}
			if changed {
				describe(mdPath, fn, e.BlockedBy, e.Base)
				post[i].BlockedBy = mapRefs(e.BlockedBy, fn)
				post[i].Base = fn(e.Base)
			}
		}
		for _, t := range e.Tickets {
			if t.Path == p.oldPath {
				continue
			}
			changed, err := scan(t.Path, fn)
			if err != nil {
				return err
			}
			if changed {
				describe(t.Path, fn, t.BlockedBy, t.Base)
				t.BlockedBy = mapRefs(t.BlockedBy, fn)
				t.Base = fn(t.Base)
			}
			post[i].Tickets = append(post[i].Tickets, t)
		}
		if e.Name == p.to.Epic {
			moved := tickets.Ticket{
				Number:     leadingNumber(p.to.ID),
				Identifier: p.to.ID,
				Path:       p.newPath,
				Type:       string(p.ticket.Type),
				BlockedBy:  idsToStrings(p.ticket.BlockedBy),
				Status:     string(p.ticket.Status),
				Base:       p.ticket.Base,
			}
			post[i].Tickets = append(post[i].Tickets, moved)
		}
	}

	before := map[string]bool{}
	for _, err := range tickets.ValidateRefs(project, epics) {
		before[err.Error()] = true
	}
	var broken []string
	for _, err := range tickets.ValidateRefs(project, post) {
		if !before[err.Error()] {
			broken = append(broken, err.Error())
		}
	}
	if len(broken) > 0 {
		return moveRefusal(tickets.CodeMoveBrokenProj, "moving %s would break the project:\n  %s", p.from.ID, strings.Join(broken, "\n  "))
	}

	p.warnings = p.proseMentions(root, rewrittenFiles)
	return nil
}

func idsToStrings(ids []schema.TicketID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// leadingNumber is the numeric prefix of a ticket id ("12a" -> 12).
func leadingNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimRightFunc(id, unicode.IsLetter))
	return n
}

// proseMentions lists the markdown files that still spell the old address
// ("epic/NN") after the ref rewrites; prose is never rewritten, only reported.
// after holds the post-rewrite content of the files planInbound already read.
func (p movePlan) proseMentions(root string, after map[string]string) []string {
	old := regexp.MustCompile(`(?:^|[^A-Za-z0-9_/-])` + regexp.QuoteMeta(p.from.Epic+"/"+p.from.ID) + `(?:[^A-Za-z0-9]|$)`)
	var out []string
	if old.MatchString(p.body) {
		out = append(out, p.newPath)
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || path == p.oldPath {
			return nil
		}
		content, ok := after[path]
		if !ok {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content = string(data)
		}
		if old.MatchString(content) {
			out = append(out, path)
		}
		return nil
	})
	slices.Sort(out)
	return out
}

// applyMove writes the new file, rewrites the inbound refs, then deletes the
// old file, so a crash in between leaves a valid project with the ticket in
// both epics rather than a dangling ref or a lost ticket. The old file's
// ticket lock is held throughout.
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
	for _, e := range p.inbound {
		if _, err := tickets.RewriteRefsInFile(e.path, e.fn); err != nil {
			return err
		}
	}
	return os.Remove(p.oldPath)
}

func printMove(w io.Writer, p movePlan, jsonOut bool, mode string) error {
	if jsonOut {
		rewritten := p.rewritten
		if rewritten == nil {
			rewritten = []string{}
		}
		warnings := p.warnings
		if warnings == nil {
			warnings = []string{}
		}
		return writeStamped(w, struct {
			From      string   `json:"from"`
			To        string   `json:"to"`
			Path      string   `json:"path"`
			Rewritten []string `json:"rewritten"`
			Warnings  []string `json:"warnings"`
			DryRun    bool     `json:"dry_run,omitempty"`
		}{p.from.String(), p.to.String(), p.newPath, rewritten, warnings, mode == "dry-run"}, viaDirect, callerActor(os.Getwd))
	}
	if mode != "dry-run" {
		if _, err := fmt.Fprintln(w, p.to.String()); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%s -> %s (%s)\n", p.from, p.to, p.newPath); err != nil {
			return err
		}
		for _, r := range p.rewritten {
			if _, err := fmt.Fprintln(w, "  "+r); err != nil {
				return err
			}
		}
	}
	for _, path := range p.warnings {
		if _, err := fmt.Fprintf(w, "warning: %s still mentions %s\n", path, p.from); err != nil {
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

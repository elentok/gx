package tickets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/tickets/schema"
)

// MoveRefusal is a refused move with a stable code.
func MoveRefusal(code, format string, args ...any) error {
	return &AddressError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// LockEpics locks every epic in path order, so two moves in opposite
// directions can't each hold one lock and wait on the other.
func LockEpics(paths ...string) (unlock func(), err error) {
	sorted := slices.Sorted(slices.Values(paths))
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, p := range sorted {
		u, err := LockEpic(p)
		if err != nil {
			release()
			return nil, err
		}
		unlocks = append(unlocks, u)
	}
	return release, nil
}

// MovePlan is a computed ticket move. PlanMove builds it without writing;
// ApplyMove writes it.
type MovePlan struct {
	From, To  Address
	OldPath   string
	NewPath   string
	ticket    schema.Ticket
	body      string
	Rewritten []string
	inbound   []inboundEdit
	Warnings  []string
}

// MoveOpts is what `tickets close` changes about a move. The zero value is a
// plain `tickets move`.
type MoveOpts struct {
	// Archive is the one exception to move's draft/open, never-run rule: a
	// ticket on its way into the archive may be done or have run history.
	Archive bool
	// Resolution, when set, cancels the ticket and records it as its
	// ## Resolution section.
	Resolution string
	// RecordOriginal adds a ## Resolution naming the old address when the
	// ticket has none, leaving its status alone.
	RecordOriginal bool
	// AppendOriginal is RecordOriginal that also adds the old address to a
	// ## Resolution the ticket already has.
	AppendOriginal bool
	// CompletedAt, when set, is stamped on a done ticket that has no
	// completed_at yet. A cancelled ticket never gets one.
	CompletedAt time.Time
}

// PlanMove runs the refusals and computes the moved ticket, writing nothing.
// Called with both epic locks held, so what it reads is what ApplyMove writes.
func PlanMove(root string, a Address, src Ticket, dstEpic, slug string, opts MoveOpts) (MovePlan, error) {
	raw, err := os.ReadFile(src.Path)
	if err != nil {
		return MovePlan{}, err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), src.Path)
	if err != nil {
		return MovePlan{}, err
	}
	if opts.Resolution != "" && t.Status != schema.StatusDraft && t.Status != schema.StatusOpen {
		return MovePlan{}, MoveRefusal(CodeCloseStatus, "ticket %s is %s; only draft or open follow-ups close", a.ID, t.Status)
	}
	if !opts.Archive {
		if t.Status != schema.StatusDraft && t.Status != schema.StatusOpen {
			return MovePlan{}, MoveRefusal(CodeMoveStatus, "ticket %s is %s; only draft or open tickets move", a.ID, t.Status)
		}
		if len(t.SessionIDs) > 0 || t.ResolvedBase != "" || t.IterationStatus != "" {
			return MovePlan{}, MoveRefusal(CodeMoveRunHistory, "ticket %s has run history", a.ID)
		}
	}

	epics, err := Load(root)
	if err != nil {
		return MovePlan{}, err
	}
	var dst, srcEpic *Epic
	for i := range epics {
		switch epics[i].Name {
		case dstEpic:
			dst = &epics[i]
		case a.Epic:
			srcEpic = &epics[i]
		}
	}
	if dst == nil || srcEpic == nil {
		return MovePlan{}, MoveRefusal(CodeUnknownEpic, "no epic %q in project %q", dstEpic, a.Project)
	}
	if t.Parent != nil {
		return MovePlan{}, MoveRefusal(CodeMoveLineage, "ticket %s is a fork of %s", a.ID, *t.Parent)
	}
	for _, o := range srcEpic.Tickets {
		if o.Parent != nil && *o.Parent == a.ID {
			return MovePlan{}, MoveRefusal(CodeMoveLineage, "ticket %s has fork child %s", a.ID, o.Identifier)
		}
	}

	newID, err := NextTicketID(*dst, "")
	if err != nil {
		return MovePlan{}, err
	}
	if slug == "" {
		slug = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(src.Path), a.ID+"-"), ".md")
	}

	p := MovePlan{
		From:    a,
		To:      Address{Project: a.Project, Epic: dstEpic, ID: newID},
		OldPath: src.Path,
		NewPath: filepath.Join(root, dstEpic, "issues", fmt.Sprintf("%s-%s.md", newID, slug)),
		ticket:  t,
		body:    schema.ParseBody(string(raw)),
	}
	p.ticket.ID = schema.TicketID(newID)
	p.Rewritten = []string{"id: " + a.ID + " -> " + newID}

	for i, ref := range p.ticket.BlockedBy {
		if next := rewriteMovedRef(string(ref), a.Epic, dstEpic, a.Project); next != string(ref) {
			p.ticket.BlockedBy[i] = schema.TicketID(next)
			p.Rewritten = append(p.Rewritten, fmt.Sprintf("blocked_by: %s -> %s", ref, next))
		}
	}
	if p.ticket.Base != "" {
		if next := rewriteMovedRef(p.ticket.Base, a.Epic, dstEpic, a.Project); next != p.ticket.Base {
			p.Rewritten = append(p.Rewritten, fmt.Sprintf("base: %s -> %s", p.ticket.Base, next))
			p.ticket.Base = next
		}
	}
	if body, ok := retitle(p.body, a.ID, newID); ok {
		p.body = body
		p.Rewritten = append(p.Rewritten, "title: # "+a.ID+" -> # "+newID)
	}

	if opts.Resolution != "" {
		p.ticket.Status = schema.StatusCancelled
	}
	if p.ticket.Status == schema.StatusDone && p.ticket.CompletedAt.IsZero() {
		p.ticket.CompletedAt = opts.CompletedAt
	}

	if err := schema.Validate(p.ticket); err != nil {
		return MovePlan{}, fmt.Errorf("moved ticket would be invalid: %w", err)
	}
	if _, err := os.Stat(p.NewPath); err == nil {
		return MovePlan{}, fmt.Errorf("%s already exists", p.NewPath)
	}
	if err := p.planInbound(root, epics); err != nil {
		return MovePlan{}, err
	}
	if opts.Resolution != "" {
		// After planInbound: the resolution names the old address on purpose,
		// which is no stale prose mention.
		p.body = schema.SetSection(p.body, "Resolution", opts.Resolution)
	} else if opts.RecordOriginal || opts.AppendOriginal {
		original := OriginalLine(a) + "\n"
		if res := schema.Section(p.body, "Resolution"); res == "" {
			p.body = schema.SetSection(p.body, "Resolution", original)
		} else if opts.AppendOriginal && !strings.Contains(res, original) {
			p.body = schema.SetSection(p.body, "Resolution", strings.TrimRight(res, "\n")+"\n"+original)
		}
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
func respeller(project string, from, to Address, inEpic string) func(string) string {
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
		if epic != from.Epic || !schema.TicketID(id).Valid() || SiblingKey(id) != SiblingKey(from.ID) {
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
// address in prose. It writes nothing; ApplyMove replays p.inbound.
//
// Only errors the move introduces refuse it: a project that was already
// broken elsewhere must not become unmovable.
func (p *MovePlan) planInbound(root string, epics []Epic) error {
	project := p.From.Project
	rewrittenFiles := map[string]string{} // path -> content after the rewrite
	post := make([]Epic, len(epics))

	describe := func(path string, fn func(string) string, blockedBy []string, base string) {
		for _, ref := range blockedBy {
			if next := fn(ref); next != ref {
				p.Rewritten = append(p.Rewritten, fmt.Sprintf("%s: blocked_by: %s -> %s", path, ref, next))
			}
		}
		if next := fn(base); base != "" && next != base {
			p.Rewritten = append(p.Rewritten, fmt.Sprintf("%s: base: %s -> %s", path, base, next))
		}
	}
	// scan reads path, records what its rewrite yields and queues the edit.
	scan := func(path string, fn func(string) string) (changed bool, err error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		next, changed, err := RewriteFrontmatterRefs(string(raw), fn)
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
		fn := respeller(project, p.From, p.To, e.Name)
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
			// NewPath is already on disk when a crashed move resumes.
			if t.Path == p.OldPath || t.Path == p.NewPath {
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
		if e.Name == p.To.Epic {
			moved := Ticket{
				Number:     leadingNumber(p.To.ID),
				Identifier: p.To.ID,
				Path:       p.NewPath,
				Type:       string(p.ticket.Type),
				BlockedBy:  idsToStrings(p.ticket.BlockedBy),
				Status:     string(p.ticket.Status),
				Base:       p.ticket.Base,
			}
			post[i].Tickets = append(post[i].Tickets, moved)
		}
	}

	before := map[string]bool{}
	for _, err := range ValidateRefs(project, epics) {
		before[err.Error()] = true
	}
	var broken []string
	for _, err := range ValidateRefs(project, post) {
		if !before[err.Error()] {
			broken = append(broken, err.Error())
		}
	}
	if len(broken) > 0 {
		return MoveRefusal(CodeMoveBrokenProj, "moving %s would break the project:\n  %s", p.From.ID, strings.Join(broken, "\n  "))
	}

	p.Warnings = p.proseMentions(root, rewrittenFiles)
	return nil
}

// leadingNumber is the numeric prefix of a ticket id ("12a" -> 12).
func leadingNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimRightFunc(id, unicode.IsLetter))
	return n
}

// proseMentions lists the markdown files that still spell the old address
// ("epic/NN") after the ref rewrites; prose is never rewritten, only reported.
// after holds the post-rewrite content of the files planInbound already read.
func (p MovePlan) proseMentions(root string, after map[string]string) []string {
	old := regexp.MustCompile(`(?:^|[^A-Za-z0-9_/-])` + regexp.QuoteMeta(p.From.Epic+"/"+p.From.ID) + `(?:[^A-Za-z0-9]|$)`)
	var out []string
	if old.MatchString(p.body) {
		out = append(out, p.NewPath)
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || path == p.OldPath {
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

// ApplyMove writes the new file, rewrites the inbound refs, then deletes the
// old file, so a crash in between leaves a valid project with the ticket in
// both epics rather than a dangling ref or a lost ticket. The old file's
// ticket lock is held throughout.
func ApplyMove(p MovePlan) error {
	unlock, err := schema.LockTicket(p.OldPath)
	if err != nil {
		return err
	}
	defer unlock()

	out, err := schema.MarshalTicket(p.ticket, p.body)
	if err != nil {
		return fmt.Errorf("marshaling ticket: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.NewPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(p.NewPath, out, 0644); err != nil {
		return fmt.Errorf("writing ticket %s: %w", p.NewPath, err)
	}
	return p.finish()
}

// ResumeMove finishes a move whose new file was written but whose old file is
// still there (a crash between the two): it re-plans the inbound refs against
// the existing new file, rewrites them and removes the old file.
func ResumeMove(root string, from, to Address, oldPath, newPath string) error {
	unlock, err := schema.LockTicket(oldPath)
	if err != nil {
		return err
	}
	defer unlock()

	raw, err := os.ReadFile(newPath)
	if err != nil {
		return err
	}
	t, err := schema.ParseTicketFromRaw(string(raw), newPath)
	if err != nil {
		return err
	}
	p := MovePlan{From: from, To: to, OldPath: oldPath, NewPath: newPath, ticket: t, body: schema.ParseBody(string(raw))}
	epics, err := Load(root)
	if err != nil {
		return err
	}
	if err := p.planInbound(root, epics); err != nil {
		return err
	}
	return p.finish()
}

// finish rewrites the inbound refs, then deletes the old file.
func (p MovePlan) finish() error {
	for _, e := range p.inbound {
		if _, err := RewriteRefsInFile(e.path, e.fn); err != nil {
			return err
		}
	}
	return os.Remove(p.OldPath)
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

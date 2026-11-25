package tickets

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets/schema"
)

// CheckBlockedBy is the one verdict on t's bare blocked_by refs, shared by
// `gx tickets validate` and the loader so both report the same message: each
// must name a ticket in e; a cross-epic ref ("epic/06" or a whole "epic") is
// refused. Every bad ref is reported at once.
func (e Epic) CheckBlockedBy(t Ticket) error {
	return e.checkBlockedByIn(newProjectGraph("", []Epic{e}, nil), t)
}

// checkBlockedByIn is CheckBlockedBy against g, the graph of e alone, so a
// caller checking every ticket builds it once.
func (e Epic) checkBlockedByIn(g *projectGraph, t Ticket) error {
	return errors.Join(g.checkBlockedBy(e.Name, t)...)
}

// flagDanglingBlockers records CheckBlockedBy's verdict on each ticket that
// fails it (BlockedByErr, which renders as StatusError). The ticket keeps its
// BlockedBy so the message and `validate` stay in step; StatusError is never
// runnable, so the loop can't claim it.
//
// A ticket's cross-epic ref is refused outright. A single epic can't resolve
// its epic's own whole-epic blocked_by, so it fails closed here: only
// resolveEpicHold, which sees the whole project, lifts that. This is what an
// in-process loop (no server) falls back on.
func (e *Epic) flagDanglingBlockers() {
	epicQualified := qualifiedRefs(e.BlockedBy)
	g := newProjectGraph("", []Epic{*e}, nil)
	for i := range e.Tickets {
		t := &e.Tickets[i]
		if err := e.checkBlockedByIn(g, *t); err != nil {
			t.BlockedByErr = err.Error()
			continue
		}
		// A finished ticket is never claimed, so failing it closed only hides
		// that it is done (and blocks archiving its epic), as resolveEpicHold knows.
		if t.IsTerminal() {
			continue
		}
		if len(epicQualified) > 0 {
			t.BlockedByErr = fmt.Sprintf("blocked_by %s crosses epics; only the server resolves that", strings.Join(epicQualified, ", "))
		}
	}
}

func qualifiedRefs(refs []string) []string {
	return slices.DeleteFunc(slices.Clone(refs), func(ref string) bool {
		return !crossEpicRef(ref)
	})
}

// resolveEpicHold returns epics (all of one project, named project) with each
// ticket held by its epic's own whole-epic blocked_by. A ticket is runnable
// only if its own refs and its epic's are all resolved; children never copy an
// ancestor's blocked_by, the epic's refs are applied here instead. An
// unreleased whole-epic ref goes into Ticket.ExternalBlockers (rendering the
// ticket blocked); bad refs and wait cycles set BlockedByErr (rendering it
// error), the same check ValidateRefs runs; a wait cycle sets CycleErr and
// leaves the ticket blocked. release decides when a whole-epic
// ref is resolved; archived holds the project's archived epic names, so a
// blocker on one counts as satisfied.
func resolveEpicHold(project string, release EpicRelease, epics []Epic, archived map[string]bool) []Epic {
	g := newProjectGraph(project, epics, archived)
	g.release = release
	out := slices.Clone(epics)
	for i := range out {
		e := &out[i]
		e.Tickets = slices.Clone(e.Tickets)
		epicErrs := g.epicBlockedByErrors(*e)
		e.UnreleasedBlockers, e.InvalidBlockers = g.epicBlockers(*e)
		for j := range e.Tickets {
			t := &e.Tickets[j]
			t.BlockedByErr, t.CycleErr, t.ExternalBlockers = "", "", nil
			errs := g.checkBlockedBy(e.Name, *t)
			if !t.IsTerminal() {
				if cycles := g.cycleErrors(e.Name, *t); len(cycles) > 0 {
					t.CycleErr = errors.Join(cycles...).Error()
				}
				errs = append(errs, epicErrs...)
			}
			if len(errs) > 0 {
				t.BlockedByErr = errors.Join(errs...).Error()
				continue
			}
			t.ExternalBlockers = e.UnreleasedBlockers
		}
	}
	return out
}

// epicBlockers splits e's blocked_by into the whole epics that still block
// (bare names, in order) and the stored refs that do not resolve.
func (g *projectGraph) epicBlockers(e Epic) (unreleased, invalid []string) {
	for _, ref := range e.BlockedBy {
		if _, _, isEpic := BlockedByEpicRef(g.project, ref); !isEpic {
			invalid = append(invalid, ref)
			continue
		}
		key, err := g.resolveBlocker(e.Name, ref)
		switch {
		case err != nil:
			invalid = append(invalid, ref)
		case g.blocking(key):
			unreleased = append(unreleased, key)
		}
	}
	return unreleased, invalid
}

// ValidateRefs is ValidateProject's cross-ticket half, run on epics already in
// memory (all of project): every epic's and every live ticket's blocked_by
// ref must resolve, and none may close a wait cycle. Done and
// cancelled tickets are skipped, as in ValidateProject. archived is the
// project's ArchivedEpicNames: a blocker in one of them is not dangling.
func ValidateRefs(project string, epics []Epic, archived map[string]bool) []error {
	g := newProjectGraph(project, epics, archived)
	var errs []error
	for _, e := range epics {
		errs = append(errs, g.epicBlockedByErrors(e)...)
		for _, t := range e.Tickets {
			if t.IsTerminal() {
				continue
			}
			for _, err := range append(g.checkBlockedBy(e.Name, t), g.cycleErrors(e.Name, t)...) {
				errs = append(errs, fmt.Errorf("epic %s: %w", e.Name, err))
			}
		}
	}
	return errs
}

// EpicsWaitedOn is the set of epics (by name) that some non-terminal ticket is
// held on as a whole: the whole-epic blocked_by refs of each epic that still
// has a non-terminal ticket. Whether the blocker still blocks is not asked.
func EpicsWaitedOn(project string, epics []Epic) map[string]bool {
	g := newProjectGraph(project, epics, nil)
	waited := map[string]bool{}
	for _, e := range epics {
		if !slices.ContainsFunc(e.Tickets, func(t Ticket) bool { return !t.IsTerminal() }) {
			continue
		}
		for _, ref := range qualifiedRefs(e.BlockedBy) {
			if key, err := g.resolveBlocker(e.Name, ref); err == nil && isEpicKey(key) {
				waited[key] = true
			}
		}
	}
	return waited
}

// CheckBlockedByWrite is the check `tickets add` and `tickets set --blocked-by`
// run before writing st to path (<project>/<epic>/issues/<file>.md, new or
// existing): st's refs must resolve project-wide and close no wait cycle on
// the wait graph. Only st's own faults refuse, so a project already broken
// elsewhere stays writable. A path outside that layout has no project, so
// nothing is checked.
func CheckBlockedByWrite(path string, st schema.Ticket) error {
	issuesDir := filepath.Dir(path)
	number, identifier, _, ok := ParseTicketFilename(filepath.Base(path))
	if filepath.Base(issuesDir) != "issues" || !ok {
		return nil
	}
	epicDir := filepath.Dir(issuesDir)
	root := filepath.Dir(epicDir)
	epics, err := Load(root)
	if err != nil {
		return err
	}
	t := Ticket{
		Number:     number,
		Identifier: identifier,
		Path:       path,
		BlockedBy:  IDsToStrings(st.BlockedBy),
		Parent:     IDToStringPtr(st.Parent),
		Status:     string(st.Status),
		Commitless: st.IsCommitless(),
	}
	epic := filepath.Base(epicDir)
	post := slices.Clone(epics)
	for i := range post {
		if post[i].Name != epic {
			continue
		}
		post[i].Tickets = slices.DeleteFunc(slices.Clone(post[i].Tickets), func(o Ticket) bool {
			return ticketKey(o) == ticketKey(t)
		})
		post[i].Tickets = append(post[i].Tickets, t)
	}
	archived, err := ArchivedEpicNames(root)
	if err != nil {
		return err
	}
	g := newProjectGraph(ProjectName(root), post, archived)
	errs := g.checkBlockedBy(epic, t)
	if !t.IsTerminal() {
		errs = append(errs, g.cycleErrors(epic, t)...)
	}
	return errors.Join(errs...)
}

// CheckEpicBlockedByWrite is CheckBlockedByWrite for an epic's own
// blocked_by: refs is what `tickets epic set --blocked-by` would write to the
// epic ticket.md at ticketMD (<project>/<epic>/ticket.md).
func CheckEpicBlockedByWrite(ticketMD string, refs []string) error {
	epicDir := filepath.Dir(ticketMD)
	root := filepath.Dir(epicDir)
	epics, err := Load(root)
	if err != nil {
		return err
	}
	name := filepath.Base(epicDir)
	post := slices.Clone(epics)
	i := slices.IndexFunc(post, func(e Epic) bool { return e.Name == name })
	if i < 0 {
		return &AddressError{Code: CodeUnknownEpic, Msg: fmt.Sprintf("no epic %q", name)}
	}
	post[i].BlockedBy = refs
	archived, err := ArchivedEpicNames(root)
	if err != nil {
		return err
	}
	g := newProjectGraph(ProjectName(root), post, archived)
	return errors.Join(g.epicBlockedByErrors(post[i])...)
}

// LoadResolved is Load plus resolveEpicHold: the project's epics (dir holds
// all of project's epics) with each epic's whole-epic blockers resolved. Use
// it wherever status, blockers or counts are read; a plain Load fails closed
// on a whole-epic blocker, so a done ticket would read as error.
func LoadResolved(project, dir string) ([]Epic, error) {
	epics, err := Load(dir)
	if err != nil {
		return nil, err
	}
	// An unreadable project.json falls back to the default policy, the
	// strictest release rule, so a whole-epic blocker fails closed.
	pf, _ := config.ReadProjectFile(dir)
	archived, err := ArchivedEpicNames(dir)
	if err != nil {
		return nil, err
	}
	return resolveEpicHold(project, EpicReleaseOf(pf), epics, archived), nil
}

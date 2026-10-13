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
// must name a ticket in e. A cross-epic ref ("epic/06" or a whole "epic")
// needs the whole project to resolve, so it is left to ValidateProject. Every
// bad ref is reported at once.
func (e Epic) CheckBlockedBy(t Ticket) error {
	t.BlockedBy = slices.DeleteFunc(slices.Clone(t.BlockedBy), crossEpicRef)
	return errors.Join(newProjectGraph("", []Epic{e}).checkBlockedBy(e.Name, t)...)
}

// flagDanglingBlockers records CheckBlockedBy's verdict on each ticket that
// fails it (BlockedByErr, which renders as StatusError). The ticket keeps its
// BlockedBy so the message and `validate` stay in step; StatusError is never
// runnable, so the loop can't claim it.
//
// A single epic can't resolve a qualified ref (its own or its epic's), so it
// fails closed here: only ResolveCrossEpic, which sees the whole project, lifts
// that. This is what an in-process loop (no server) falls back on.
func (e *Epic) flagDanglingBlockers() {
	epicQualified := qualifiedRefs(e.BlockedBy)
	for i := range e.Tickets {
		t := &e.Tickets[i]
		if err := e.CheckBlockedBy(*t); err != nil {
			t.BlockedByErr = err.Error()
			continue
		}
		if refs := append(qualifiedRefs(t.BlockedBy), epicQualified...); len(refs) > 0 {
			t.BlockedByErr = fmt.Sprintf("blocked_by %s crosses epics; only the server resolves that", strings.Join(refs, ", "))
		}
	}
}

func qualifiedRefs(refs []string) []string {
	return slices.DeleteFunc(slices.Clone(refs), func(ref string) bool {
		return !crossEpicRef(ref)
	})
}

// ResolveCrossEpic returns epics (all of one project, named project) with each
// ticket's blockers resolved project-wide. A ticket is runnable only if its own
// blocked_by refs and its epic's are all resolved; children never copy an
// ancestor's blocked_by, the epic's refs are applied here instead. Unresolved
// cross-epic refs land in Ticket.ExternalBlockers (rendering the ticket
// blocked); bad refs and wait cycles set BlockedByErr (rendering it error),
// the same check ValidateRefs runs. release decides
// when a whole-epic ref is resolved.
func ResolveCrossEpic(project string, release EpicRelease, epics []Epic) []Epic {
	g := newProjectGraph(project, epics)
	g.release = release
	out := slices.Clone(epics)
	for i := range out {
		e := &out[i]
		e.Tickets = slices.Clone(e.Tickets)
		for j := range e.Tickets {
			t := &e.Tickets[j]
			t.BlockedByErr, t.ExternalBlockers, t.WidenedBlockers = "", nil, nil
			errs := g.checkBlockedBy(e.Name, *t)
			if !t.IsTerminal() {
				errs = append(errs, g.cycleErrors(e.Name, *t)...)
			}
			for _, ref := range qualifiedRefs(e.BlockedBy) {
				if _, err := g.resolveBlocker(e.Name, ref); err != nil {
					errs = append(errs, fmt.Errorf("epic %s: blocked_by %w", e.Name, err))
				}
			}
			if len(errs) > 0 {
				t.BlockedByErr = errors.Join(errs...).Error()
				continue
			}
			for _, ref := range qualifiedRefs(t.BlockedBy) {
				if key, _ := g.resolveBlocker(e.Name, ref); widened(ref, key) {
					if t.WidenedBlockers == nil {
						t.WidenedBlockers = map[string]string{}
					}
					t.WidenedBlockers[ref] = key
				}
			}
			for _, ref := range append(qualifiedRefs(t.BlockedBy), qualifiedRefs(e.BlockedBy)...) {
				if key, _ := g.resolveBlocker(e.Name, ref); g.blocking(key) {
					t.ExternalBlockers = append(t.ExternalBlockers, ref)
				}
			}
		}
	}
	return out
}

// CheckBlockedByWrite is the check `tickets add` and `tickets set --blocked-by`
// run before writing st to path (<project>/<epic>/issues/<file>.md, new or
// existing): st's refs must resolve project-wide and close no wait cycle on
// the widened graph. Only st's own faults refuse, so a project already broken
// elsewhere stays writable. On success it returns one notice per ref that
// widens to its whole epic. A path outside that layout has no project, so
// nothing is checked.
func CheckBlockedByWrite(path string, st schema.Ticket) ([]string, error) {
	issuesDir := filepath.Dir(path)
	number, identifier, _, ok := parseTicketFilename(filepath.Base(path))
	if filepath.Base(issuesDir) != "issues" || !ok {
		return nil, nil
	}
	epicDir := filepath.Dir(issuesDir)
	root := filepath.Dir(epicDir)
	epics, err := Load(root)
	if err != nil {
		return nil, err
	}
	t := Ticket{
		Number:     number,
		Identifier: identifier,
		Path:       path,
		BlockedBy:  idsToStrings(st.BlockedBy),
		Parent:     idToStringPtr(st.Parent),
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
	g := newProjectGraph(ProjectName(root), post)
	errs := g.checkBlockedBy(epic, t)
	if !t.IsTerminal() {
		errs = append(errs, g.cycleErrors(epic, t)...)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	var notices []string
	for _, ref := range t.BlockedBy {
		if key, _ := g.resolveBlocker(epic, ref); widened(ref, key) {
			notices = append(notices, fmt.Sprintf("`%s` needs commits; waiting for all of `%s`", ref, key))
		}
	}
	return notices, nil
}

// LoadResolved is Load plus ResolveCrossEpic: the project's epics (dir holds
// all of project's epics) with cross-epic blockers resolved. Use it wherever
// status, blockers or counts are read; a plain Load fails closed on a
// qualified blocker, so a done ticket would read as error.
func LoadResolved(project, dir string) ([]Epic, error) {
	epics, err := Load(dir)
	if err != nil {
		return nil, err
	}
	// An unreadable project.json falls back to the default policy, the
	// strictest release rule, so a whole-epic blocker fails closed.
	pf, _ := config.ReadProjectFile(dir)
	return ResolveCrossEpic(project, EpicReleaseOf(pf), epics), nil
}

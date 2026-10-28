package tickets

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/elentok/gx/config"
)

// Reason codes for a whole-epic blocked_by token that can never resolve.
const (
	CodeBlockedByOtherProject = "blocked-by-other-project"
	CodeBlockedByOwnEpic      = "blocked-by-own-epic"
	CodeBlockedByMapEpic      = "blocked-by-map-epic"
)

// ticketIDRe is a bare ticket-id token ("06", "04a"); any other bare token is
// a whole-epic address, so an all-numeric epic name needs "project:epic".
var ticketIDRe = regexp.MustCompile(`^\d+[[:alpha:]]?\d*$`)

// crossEpicRef reports whether ref can only resolve against the whole
// project: a qualified ticket ("epic/06") or a whole epic ("epic").
func crossEpicRef(ref string) bool {
	return strings.Contains(ref, "/") || !ticketIDRe.MatchString(ref)
}

// IsWholeEpicRef reports whether a blocked_by token names a whole epic.
func IsWholeEpicRef(ref string) bool {
	return !strings.Contains(ref, "/") && !ticketIDRe.MatchString(ref)
}

// EpicRelease is when a whole-epic blocker releases its dependents, from the
// project's merge-release policy and auto-ff-merge. The zero value is the
// default on-merge policy.
type EpicRelease struct {
	OnDone      bool
	AutoFFMerge bool
}

// EpicReleaseOf reads the release rule from a project.json.
func EpicReleaseOf(pf config.ProjectFile) EpicRelease {
	return EpicRelease{OnDone: pf.LandingPolicy() == config.LandingOnDone, AutoFFMerge: pf.AutoFFMergeEnabled()}
}

// stamped reports whether e carries the stamp its policy needs. Under on-done
// with auto-ff-merge, completed_at alone is not enough: a root pause stamps it
// without the branch ever merging.
func (r EpicRelease) stamped(e Epic) bool {
	if !r.OnDone {
		return !e.MergedAt.IsZero()
	}
	return !e.CompletedAt.IsZero() && (!r.AutoFFMerge || !e.MergedAt.IsZero())
}

// projectGraph is the project-wide view blocked_by refs resolve against: a ref
// may name a ticket in another epic of the same project ("epic/06"), or a
// whole epic ("epic"). Every ticket is identified by nodeKey (its epic plus
// its ticketKey); a whole epic by its bare name, which never holds a "/".
type projectGraph struct {
	project string
	release EpicRelease
	tickets map[string]Ticket
	epics   map[string]Epic
	// archived holds the names of epics moved to .archive. An archived epic is
	// done and merged, so a ref to it is satisfied; a live epic of the same
	// name wins.
	archived map[string]bool
	// children maps a node to its fork children (the reverse of Ticket.Parent).
	children map[string][]string
	// wait maps each node to the nodes it must wait on (see buildWait).
	wait map[string][]string
}

func nodeKey(epic string, t Ticket) string { return epic + "/" + ticketKey(t) }

func isEpicKey(key string) bool { return !strings.Contains(key, "/") }

// newProjectGraph indexes epics (all of one project, named project; the name
// only decides which qualified refs are "this project's"). archived is the
// project's ArchivedEpicNames, or nil when the caller has no project dir.
func newProjectGraph(project string, epics []Epic, archived map[string]bool) *projectGraph {
	g := &projectGraph{
		project:  project,
		archived: archived,
		tickets:  map[string]Ticket{},
		epics:    map[string]Epic{},
		children: map[string][]string{},
		wait:     map[string][]string{},
	}
	for _, e := range epics {
		g.epics[e.Name] = e
		for _, t := range e.Tickets {
			g.tickets[nodeKey(e.Name, t)] = t
		}
	}
	for _, e := range epics {
		for _, t := range e.Tickets {
			if t.Parent == nil {
				continue
			}
			if parent, err := g.resolve(e.Name, *t.Parent); err == nil {
				g.children[parent] = append(g.children[parent], nodeKey(e.Name, t))
			}
		}
	}
	g.buildWait(epics)
	return g
}

// resolve returns the node ref names, relative to the epic it is written in. A
// bare ref names a ticket in that epic; "epic/06" (optionally "project:epic/06"
// for this project) names one in another.
func (g *projectGraph) resolve(epic, ref string) (string, error) {
	target, token := epic, ref
	if strings.Contains(ref, "/") {
		a, err := ParseAddress(ref, AddressContext{Project: g.project})
		if err != nil {
			return "", fmt.Errorf("%q is malformed (want 06 or epic/06)", ref)
		}
		if a.Project != g.project {
			return "", fmt.Errorf("%q names another project (cross-project blocking not supported)", ref)
		}
		target, token = a.Epic, a.ID
	}
	num, letters := splitBlockedByToken(token)
	key := target + "/" + siblingKey(num, letters)
	if _, ok := g.tickets[key]; !ok {
		if moved, ok := g.archivedOriginal(Address{Project: g.project, Epic: target, ID: token}); ok {
			return moved, nil
		}
		if g.isArchived(target) {
			return target + "/" + siblingKey(num, letters), nil
		}
		where := "this epic"
		if target != epic {
			where = fmt.Sprintf("epic %q", target)
		}
		return "", fmt.Errorf("%q names no ticket in %s", ref, where)
	}
	return key, nil
}

// isArchived reports whether name is an archived epic with no live twin.
func (g *projectGraph) isArchived(name string) bool {
	_, live := g.epics[name]
	return g.archived[name] && !live
}

// archivedOriginal is the node key of the archived one-off that was moved from
// addr, so a blocker naming the old address keeps working. Ambiguous or no
// match is not found.
func (g *projectGraph) archivedOriginal(addr Address) (string, bool) {
	archive, ok := g.epics[ArchivedOneOffsEpic]
	if !ok {
		return "", false
	}
	if found := archivedFrom(archive, addr); len(found) == 1 {
		return nodeKey(ArchivedOneOffsEpic, found[0]), true
	}
	return "", false
}

// BlockedByEpicRef parses a blocked_by token that names a whole epic ("epic"
// or "project:epic"). A bare name belongs to defaultProject. ok is false for a
// ticket reference.
func BlockedByEpicRef(defaultProject, ref string) (project, epic string, ok bool) {
	if strings.Contains(ref, "/") || ticketIDRe.MatchString(ref) {
		return "", "", false
	}
	if p, e, isAddr := ParseEpicAddress(ref); isAddr {
		return p, e, true
	}
	return defaultProject, ref, true
}

// resolveBlocker is resolve widened to blocked_by's whole-epic tokens: a
// token that is not in ticket-id form names an epic ("epic" or
// "project:epic"), keyed by its bare name. A ticket in another epic that has
// commits widens to that whole epic (see widen).
func (g *projectGraph) resolveBlocker(epic, ref string) (string, error) {
	project, name, isEpic := BlockedByEpicRef(g.project, ref)
	if !isEpic {
		key, err := g.resolve(epic, ref)
		if err != nil {
			return "", err
		}
		return g.widen(epic, key), nil
	}
	switch target, ok := g.epics[name]; {
	case project == g.project && g.isArchived(name):
		return name, nil
	case project != g.project:
		return "", &AddressError{CodeBlockedByOtherProject, fmt.Sprintf("%q names another project (cross-project blocking not supported)", ref)}
	case name == epic:
		return "", &AddressError{CodeBlockedByOwnEpic, fmt.Sprintf("%q names this ticket's own epic", ref)}
	case !ok:
		return "", &AddressError{CodeUnknownEpic, fmt.Sprintf("%q names no epic in this project", ref)}
	case target.IsMap:
		return "", &AddressError{CodeBlockedByMapEpic, fmt.Sprintf("%q names a map epic, which never merges", ref)}
	}
	return name, nil
}

// widen turns a ticket key in another epic into that epic's key unless the
// ticket is commitless: its code only reaches this epic once its epic merges.
// The file keeps the ticket token, so it narrows back if the ticket turns
// commitless. A map epic never merges, so its tickets stay tickets.
func (g *projectGraph) widen(epic, key string) string {
	target := key[:strings.Index(key, "/")]
	if target == epic || g.tickets[key].Commitless || g.epics[target].IsMap {
		return key
	}
	return target
}

// widened reports whether ref resolved to blocker only by widening.
func widened(ref, blocker string) bool {
	return strings.Contains(ref, "/") && isEpicKey(blocker)
}

// expand is the ticket nodes a blocker key stands for: a whole epic's every
// ticket, or a ticket's fork subtree.
func (g *projectGraph) expand(key string) []string {
	if !isEpicKey(key) {
		return g.subtree(key)
	}
	var keys []string
	for _, t := range g.epics[key].Tickets {
		keys = append(keys, nodeKey(key, t))
	}
	return keys
}

// waitTargets is what waiting on blocker means in the wait graph: its expanded
// tickets and, for a whole epic, the epic's own node, which waits on that
// epic's blocked_by. The node is what makes an empty epic part of a cycle.
func (g *projectGraph) waitTargets(blocker string) []string {
	targets := g.expand(blocker)
	if isEpicKey(blocker) {
		targets = append(targets, blocker)
	}
	return targets
}

// buildWait fills g.wait. A ref to Y makes t wait on Y and on Y's whole fork
// subtree (see Blocking), and every fork child waits on its parent (see
// ParentDone), so the wait graph is: blocked_by edges expanded over the named
// ticket's subtree (or a whole epic's tickets), plus child→parent edges. That
// is what makes A↔B, a ticket blocked by its own ancestor, and one blocked by
// its own descendant all deadlocks. An epic's own blocked_by hangs off its
// epic node, which every ticket of the epic waits on. Unresolvable refs are
// skipped; checkBlockedBy and epicBlockedByErrors report those.
func (g *projectGraph) buildWait(epics []Epic) {
	for _, e := range epics {
		for _, ref := range e.BlockedBy {
			if blocker, err := g.resolveBlocker(e.Name, ref); err == nil && crossEpicRef(ref) {
				g.wait[e.Name] = append(g.wait[e.Name], g.waitTargets(blocker)...)
			}
		}
		for _, t := range e.Tickets {
			key := nodeKey(e.Name, t)
			if len(g.wait[e.Name]) > 0 {
				g.wait[key] = append(g.wait[key], e.Name)
			}
			if t.Parent != nil {
				if parent, err := g.resolve(e.Name, *t.Parent); err == nil {
					g.wait[key] = append(g.wait[key], parent)
				}
			}
			for _, ref := range t.BlockedBy {
				if blocker, err := g.resolveBlocker(e.Name, ref); err == nil {
					g.wait[key] = append(g.wait[key], g.waitTargets(blocker)...)
				}
			}
		}
	}
}

// checkBlockedBy is the one verdict on t's literal blocked_by refs, reporting
// every bad ref at once.
func (g *projectGraph) checkBlockedBy(epic string, t Ticket) []error {
	var errs []error
	for _, ref := range t.BlockedBy {
		if _, err := g.resolveBlocker(epic, ref); err != nil {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %w", t.DisplayNumber(), err))
		}
	}
	return errs
}

// cycleErrors reports each of t's blocked_by refs that closes a wait cycle,
// naming every ticket on the chain.
func (g *projectGraph) cycleErrors(epic string, t Ticket) []error {
	self := nodeKey(epic, t)
	var errs []error
	for _, ref := range t.BlockedBy {
		blocker, err := g.resolveBlocker(epic, ref)
		if err != nil {
			continue
		}
		if chain := g.cycleThrough(self, blocker); chain != nil {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %s closes a cycle: %s",
				t.DisplayNumber(), cycleToken(ref, blocker), g.renderChain(chain, epic)))
		}
	}
	return errs
}

// epicBlockedByErrors is the one verdict on epic e's own blocked_by: each ref
// must resolve as a ticket's would, name another epic's work, and close no
// wait cycle.
func (g *projectGraph) epicBlockedByErrors(e Epic) []error {
	var errs []error
	for _, ref := range e.BlockedBy {
		blocker, err := g.resolveBlocker(e.Name, ref)
		if err == nil && !crossEpicRef(ref) {
			err = fmt.Errorf("%q names a ticket of this epic", ref)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("epic %s: blocked_by %w", e.Name, err))
			continue
		}
		if chain := g.cycleThrough(e.Name, blocker); chain != nil {
			errs = append(errs, fmt.Errorf("epic %s: blocked_by %s closes a cycle: %s",
				e.Name, cycleToken(ref, blocker), g.renderChain(chain, e.Name)))
		}
	}
	return errs
}

// cycleThrough returns the wait chain from self through blocker back to self,
// or nil when waiting on blocker closes no cycle.
func (g *projectGraph) cycleThrough(self, blocker string) []string {
	for _, target := range g.waitTargets(blocker) {
		back := shortestPath(g.wait, target, self)
		if back == nil {
			continue
		}
		chain := []string{self, blocker}
		if isEpicKey(blocker) && target != blocker {
			chain = append(chain, target)
		}
		return append(chain, back...)
	}
	return nil
}

func cycleToken(ref, blocker string) string {
	token := fmt.Sprintf("%q", ref)
	if widened(ref, blocker) {
		token += fmt.Sprintf(" (widened to epic %q)", blocker)
	}
	return token
}

// subtree is root followed by every fork descendant.
func (g *projectGraph) subtree(root string) []string {
	keys := []string{root}
	seen := map[string]bool{root: true}
	for i := 0; i < len(keys); i++ {
		for _, child := range g.children[keys[i]] {
			if !seen[child] {
				seen[child] = true
				keys = append(keys, child)
			}
		}
	}
	return keys
}

// blocking reports whether the ticket at key, or any of its fork descendants,
// is not yet terminal (the project-wide form of Epic.Blocking). A whole epic
// blocks until every ticket in it is terminal and it carries the stamp
// g.release needs, so an empty epic blocks until stamped and a ticket added
// to a finished epic blocks again.
func (g *projectGraph) blocking(key string) bool {
	if g.isArchived(key) {
		return false
	}
	for _, k := range g.expand(key) {
		if !g.tickets[k].IsTerminal() {
			return true
		}
	}
	return isEpicKey(key) && !g.release.stamped(g.epics[key])
}

// renderChain names tickets of `epic` by number, others as "epic/number" and
// whole epics by name.
func (g *projectGraph) renderChain(chain []string, epic string) string {
	names := make([]string, len(chain))
	for i, key := range chain {
		if isEpicKey(key) {
			names[i] = key
			continue
		}
		names[i] = g.tickets[key].DisplayNumber()
		if !strings.HasPrefix(key, epic+"/") {
			names[i] = key[:strings.Index(key, "/")] + "/" + names[i]
		}
	}
	return strings.Join(names, " → ")
}

// shortestPath returns the keys from `from` to `to` inclusive of `to` but, when
// from == to, just [to]; nil when `to` is unreachable.
func shortestPath(graph map[string][]string, from, to string) []string {
	if from == to {
		return []string{to}
	}
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range graph[cur] {
			if _, seen := prev[next]; seen {
				continue
			}
			prev[next] = cur
			if next == to {
				var path []string
				for k := to; k != ""; k = prev[k] {
					path = append([]string{k}, path...)
				}
				return path[1:]
			}
			queue = append(queue, next)
		}
	}
	return nil
}

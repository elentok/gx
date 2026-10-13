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

// EpicRelease is when a whole-epic blocker releases its dependents, from the
// project's landing policy and auto-ff-merge. The zero value is the default
// on-landed policy.
type EpicRelease struct {
	OnDone      bool
	AutoFFMerge bool
}

// EpicReleaseOf reads the release rule from a project.json.
func EpicReleaseOf(pf config.ProjectFile) EpicRelease {
	return EpicRelease{OnDone: pf.LandingPolicy() == config.LandingOnDone, AutoFFMerge: pf.AutoFFMergeEnabled()}
}

// stamped reports whether e carries the stamp its policy needs. Under on-done
// with auto-ff-merge, completed_at alone is not enough: a root park stamps it
// without the branch ever landing.
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
	// children maps a node to its fork children (the reverse of Ticket.Parent).
	children map[string][]string
	// wait maps each node to the nodes it must wait on (see buildWait).
	wait map[string][]string
}

func nodeKey(epic string, t Ticket) string { return epic + "/" + ticketKey(t) }

func isEpicKey(key string) bool { return !strings.Contains(key, "/") }

// newProjectGraph indexes epics (all of one project, named project; the name
// only decides which qualified refs are "this project's").
func newProjectGraph(project string, epics []Epic) *projectGraph {
	g := &projectGraph{
		project:  project,
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
		where := "this epic"
		if target != epic {
			where = fmt.Sprintf("epic %q", target)
		}
		return "", fmt.Errorf("%q names no ticket in %s", ref, where)
	}
	return key, nil
}

// resolveBlocker is resolve widened to blocked_by's whole-epic tokens: a
// token that is not in ticket-id form names an epic ("epic" or
// "project:epic"), keyed by its bare name. A ticket in another epic that has
// commits widens to that whole epic (see widen).
func (g *projectGraph) resolveBlocker(epic, ref string) (string, error) {
	if strings.Contains(ref, "/") || ticketIDRe.MatchString(ref) {
		key, err := g.resolve(epic, ref)
		if err != nil {
			return "", err
		}
		return g.widen(epic, key), nil
	}
	project, name := g.project, ref
	if p, e, ok := ParseEpicAddress(ref); ok {
		project, name = p, e
	}
	switch target, ok := g.epics[name]; {
	case project != g.project:
		return "", &AddressError{CodeBlockedByOtherProject, fmt.Sprintf("%q names another project (cross-project blocking not supported)", ref)}
	case name == epic:
		return "", &AddressError{CodeBlockedByOwnEpic, fmt.Sprintf("%q names this ticket's own epic", ref)}
	case !ok:
		return "", &AddressError{CodeUnknownEpic, fmt.Sprintf("%q names no epic in this project", ref)}
	case target.IsMap:
		return "", &AddressError{CodeBlockedByMapEpic, fmt.Sprintf("%q names a map epic, which never lands", ref)}
	}
	return name, nil
}

// widen turns a ticket key in another epic into that epic's key unless the
// ticket is commitless: its code only reaches this epic once its epic lands.
// The file keeps the ticket token, so it narrows back if the ticket turns
// commitless. A map epic never lands, so its tickets stay tickets.
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

// buildWait fills g.wait. A ref to Y makes t wait on Y and on Y's whole fork
// subtree (see Blocking), and every fork child waits on its parent (see
// parentDone), so the wait graph is: blocked_by edges expanded over the named
// ticket's subtree (or a whole epic's tickets), plus child→parent edges. That
// is what makes A↔B, a ticket blocked by its own ancestor, and one blocked by
// its own descendant all deadlocks. Unresolvable refs are skipped;
// checkBlockedBy reports those.
func (g *projectGraph) buildWait(epics []Epic) {
	for _, e := range epics {
		for _, t := range e.Tickets {
			key := nodeKey(e.Name, t)
			if t.Parent != nil {
				if parent, err := g.resolve(e.Name, *t.Parent); err == nil {
					g.wait[key] = append(g.wait[key], parent)
				}
			}
			for _, ref := range t.BlockedBy {
				if blocker, err := g.resolveBlocker(e.Name, ref); err == nil {
					g.wait[key] = append(g.wait[key], g.expand(blocker)...)
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
		for _, target := range g.expand(blocker) {
			back := shortestPath(g.wait, target, self)
			if back == nil {
				continue
			}
			chain := []string{self, blocker}
			if isEpicKey(blocker) {
				chain = append(chain, target)
			}
			chain = append(chain, back...)
			token := fmt.Sprintf("%q", ref)
			if widened(ref, blocker) {
				token += fmt.Sprintf(" (widened to epic %q)", blocker)
			}
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %s closes a cycle: %s",
				t.DisplayNumber(), token, g.renderChain(chain, epic)))
			break
		}
	}
	return errs
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

package tickets

import (
	"fmt"
	"strings"
)

// projectGraph is the project-wide view blocked_by refs resolve against: a ref
// may name a ticket in another epic of the same project ("epic/06"). Every
// ticket is identified by nodeKey (its epic plus its ticketKey).
type projectGraph struct {
	project string
	tickets map[string]Ticket
	// children maps a node to its fork children (the reverse of Ticket.Parent).
	children map[string][]string
	// wait maps each node to the nodes it must wait on (see buildWait).
	wait map[string][]string
}

func nodeKey(epic string, t Ticket) string { return epic + "/" + ticketKey(t) }

// newProjectGraph indexes epics (all of one project, named project; the name
// only decides which qualified refs are "this project's").
func newProjectGraph(project string, epics []Epic) *projectGraph {
	g := &projectGraph{
		project:  project,
		tickets:  map[string]Ticket{},
		children: map[string][]string{},
		wait:     map[string][]string{},
	}
	for _, e := range epics {
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

// buildWait fills g.wait. A ref to Y makes t wait on Y and on Y's whole fork
// subtree (see Blocking), and every fork child waits on its parent (see
// parentDone), so the wait graph is: blocked_by edges expanded over the named
// ticket's subtree, plus child→parent edges. That is what makes A↔B, a ticket
// blocked by its own ancestor, and one blocked by its own descendant all
// deadlocks. Unresolvable refs are skipped; checkBlockedBy reports those.
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
				if blocker, err := g.resolve(e.Name, ref); err == nil {
					g.wait[key] = append(g.wait[key], g.subtree(blocker)...)
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
		if _, err := g.resolve(epic, ref); err != nil {
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
		blocker, err := g.resolve(epic, ref)
		if err != nil {
			continue
		}
		for _, target := range g.subtree(blocker) {
			back := shortestPath(g.wait, target, self)
			if back == nil {
				continue
			}
			chain := append([]string{self, blocker}, back...)
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q closes a cycle: %s",
				t.DisplayNumber(), ref, g.renderChain(chain, epic)))
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

// renderChain names tickets of `epic` by number and others as "epic/number".
func (g *projectGraph) renderChain(chain []string, epic string) string {
	names := make([]string, len(chain))
	for i, key := range chain {
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

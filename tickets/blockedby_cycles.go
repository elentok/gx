package tickets

import (
	"fmt"
	"strings"
)

// blockedByCycleErrors reports each of t's blocked_by refs that closes a wait
// cycle. A ref to Y makes t wait on Y and on Y's whole fork subtree (see
// Blocking), and every fork child waits on its parent (see parentDone), so the
// wait graph is: blocked_by edges expanded over the named ticket's subtree,
// plus child→parent edges. That is what makes A↔B, a ticket blocked by its own
// ancestor, and one blocked by its own descendant all deadlocks. Dangling and
// malformed refs are skipped; CheckBlockedBy reports those separately.
func (e Epic) blockedByCycleErrors(t Ticket) []error {
	index := e.byNumberAndSuffix()
	forkChildren := e.forkChildren()
	graph := e.waitGraph(index, forkChildren)
	self := ticketKey(t)

	var errs []error
	for _, ref := range t.BlockedBy {
		if strings.Contains(ref, "/") {
			continue
		}
		num, letters := splitBlockedByToken(ref)
		blocker, ok := index[siblingKey(num, letters)]
		if !ok {
			continue
		}
		for _, target := range subtreeKeys(blocker, forkChildren) {
			back := shortestPath(graph, target, self)
			if back == nil {
				continue
			}
			chain := append([]string{self, ticketKey(blocker)}, back...)
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q closes a cycle: %s",
				t.DisplayNumber(), ref, e.renderChain(chain, index)))
			break
		}
	}
	return errs
}

// waitGraph maps each ticketKey to the ticketKeys it must wait on.
func (e Epic) waitGraph(index map[string]Ticket, forkChildren map[string][]Ticket) map[string][]string {
	graph := map[string][]string{}
	for _, t := range e.Tickets {
		key := ticketKey(t)
		if t.Parent != nil {
			num, letters := splitBlockedByToken(*t.Parent)
			if parent, ok := index[siblingKey(num, letters)]; ok {
				graph[key] = append(graph[key], ticketKey(parent))
			}
		}
		for _, ref := range t.BlockedBy {
			if strings.Contains(ref, "/") {
				continue
			}
			num, letters := splitBlockedByToken(ref)
			if blocker, ok := index[siblingKey(num, letters)]; ok {
				graph[key] = append(graph[key], subtreeKeys(blocker, forkChildren)...)
			}
		}
	}
	return graph
}

// subtreeKeys is root's key followed by every fork descendant's key.
func subtreeKeys(root Ticket, forkChildren map[string][]Ticket) []string {
	keys := []string{ticketKey(root)}
	seen := map[string]bool{keys[0]: true}
	for i := 0; i < len(keys); i++ {
		for _, child := range forkChildren[keys[i]] {
			if k := ticketKey(child); !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys
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

func (e Epic) renderChain(chain []string, index map[string]Ticket) string {
	names := make([]string, len(chain))
	for i, key := range chain {
		names[i] = index[key].DisplayNumber()
	}
	return strings.Join(names, " → ")
}

package server

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/tickets"
)

// rootRef names a queued root: a project's epic. Its string form "project:epic"
// is the registry key and the persisted Root of a run.
type rootRef struct {
	Project string
	Epic    string
}

func parseRootRef(s string) (rootRef, error) {
	project, epic, ok := strings.Cut(s, ":")
	if !ok || project == "" || epic == "" {
		return rootRef{}, fmt.Errorf("invalid root %q, want project:epic", s)
	}
	return rootRef{Project: project, Epic: epic}, nil
}

func (r rootRef) String() string { return r.Project + ":" + r.Epic }

func rootOf(addr tickets.Address) rootRef { return rootRef{Project: addr.Project, Epic: addr.Epic} }

// parseQueuedAddress parses a queue entry: a ticket address, or an epic address
// ("project:epic"), which comes back with an empty ID.
func parseQueuedAddress(s string) (tickets.Address, error) {
	if project, epic, ok := tickets.ParseEpicAddress(s); ok {
		return tickets.Address{Project: project, Epic: epic}, nil
	}
	return tickets.ParseAddress(s, tickets.AddressContext{})
}

// removeRoot drops every queued ticket of root and reports whether any went.
func (q *queueStore) removeRoot(root rootRef) (bool, error) {
	return q.removeRootIf(root, func() bool { return true })
}

// removeRootIf is removeRoot gated by drop, which runs under the queue lock so a
// submit's enqueue cannot slip between the check and the removal.
func (q *queueStore) removeRootIf(root rootRef, drop func() bool) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := make([]QueueItem, 0, len(q.items))
	for _, it := range q.items {
		if a, err := parseQueuedAddress(it.Address); err == nil && rootOf(a) == root {
			continue
		}
		kept = append(kept, it)
	}
	if len(kept) == len(q.items) || !drop() {
		return false, nil
	}
	prev := q.items
	q.items = kept
	if err := q.save(); err != nil {
		q.items = prev
		return false, err
	}
	return true, nil
}

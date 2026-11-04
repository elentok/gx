package queue

import (
	"fmt"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// RootRef names a queued root: a project's epic. Its string form "project:epic"
// is the registry key and the persisted Root of a run.
type RootRef struct {
	Project string
	Epic    string
}

// ParseRootRef reads the "project:epic" form of a RootRef.
func ParseRootRef(s string) (RootRef, error) {
	project, epic, ok := strings.Cut(s, ":")
	if !ok || project == "" || epic == "" {
		return RootRef{}, fmt.Errorf("invalid root %q, want project:epic", s)
	}
	return RootRef{Project: project, Epic: epic}, nil
}

func (r RootRef) String() string { return r.Project + ":" + r.Epic }

// RootOf is the root a ticket address belongs to.
func RootOf(addr tickets.Address) RootRef { return RootRef{Project: addr.Project, Epic: addr.Epic} }

// RemoveRoot drops every queued ticket of root and reports whether any went.
func (q *Queue) RemoveRoot(root RootRef) (bool, error) {
	return q.removeRootIf(root, func() bool { return true })
}

// RootQueued reports whether any ticket of root is queued.
func (q *Queue) RootQueued(root RootRef) bool {
	for _, it := range q.List() {
		if a, err := tickets.ParseQueuedAddress(it.Address); err == nil && RootOf(a) == root {
			return true
		}
	}
	return false
}

// removeRootIf is RemoveRoot gated by drop, which runs under the queue lock so a
// submit's enqueue cannot slip between the check and the removal.
func (q *Queue) removeRootIf(root RootRef, drop func() bool) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := make([]wire.QueueItem, 0, len(q.items))
	for _, it := range q.items {
		if a, err := tickets.ParseQueuedAddress(it.Address); err == nil && RootOf(a) == root {
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

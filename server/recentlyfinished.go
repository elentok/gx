package server

import (
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
)

// FinishedEpic is one entry of the snapshot's Recently finished list.
type FinishedEpic struct {
	Address     string    `json:"address"`
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Done        int       `json:"done"`
	Total       int       `json:"total"`
	CompletedAt time.Time `json:"completed_at"`
}

// recentlyFinished lists the last limit completed loop epics, newest first. It
// reads the completed_at stamps of the last scan, so it needs no storage of its
// own and a restart rebuilds the same list. A map epic never counts.
func (i *index) recentlyFinished(limit int) []FinishedEpic {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := []FinishedEpic{}
	for dir, epics := range i.loaded {
		project := tickets.ProjectName(dir)
		for _, e := range epics {
			if e.IsMap || e.CompletedAt.IsZero() {
				continue
			}
			f := FinishedEpic{
				Address: rootRef{Project: project, Epic: e.Name}.String(),
				Name:    e.Name, Project: project, Done: e.DoneCount(), Total: e.TotalCount(), CompletedAt: e.CompletedAt,
			}
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b FinishedEpic) int {
		if c := b.CompletedAt.Compare(a.CompletedAt); c != 0 {
			return c
		}
		return strings.Compare(a.Address, b.Address)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

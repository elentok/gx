package server

import (
	"sort"
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
				Name:    e.Name, Project: project, Total: len(e.Tickets), CompletedAt: e.CompletedAt,
			}
			for _, t := range e.Tickets {
				if e.RenderedStatus(t) == tickets.StatusDone {
					f.Done++
				}
			}
			out = append(out, f)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if !out[a].CompletedAt.Equal(out[b].CompletedAt) {
			return out[a].CompletedAt.After(out[b].CompletedAt)
		}
		return out[a].Address < out[b].Address
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

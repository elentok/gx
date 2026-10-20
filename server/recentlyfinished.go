package server

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// FinishedEpic is one entry of the snapshot's Recently finished list.
type FinishedEpic struct {
	Address     string    `json:"address"`
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Done        int       `json:"done"`
	Total       int       `json:"total"`
	CompletedAt time.Time `json:"completed_at"`
	// OneOff marks a ticket entry: Address is the ticket's, Done and Total are 0.
	OneOff bool `json:"one_off"`
}

// finishedOneOffs lists the done one-offs of e: archived ones by their
// completed_at stamp, and done ones the sweep has not moved yet by file mod
// time. A cancelled one-off carries no stamp and never counts.
func finishedOneOffs(project string, e tickets.Epic) []FinishedEpic {
	epic := filepath.Base(e.Path)
	if !isOneOffsEpicName(epic) {
		return nil
	}
	var out []FinishedEpic
	for _, t := range e.Tickets {
		at := t.CompletedAt
		if at.IsZero() {
			if epic != OneOffsEpic || t.Status != string(schema.StatusDone) {
				continue
			}
			info, err := os.Stat(t.FilePath())
			if err != nil {
				continue
			}
			at = info.ModTime()
		}
		out = append(out, FinishedEpic{
			Address: tickets.Address{Project: project, Epic: epic, ID: t.Identifier}.String(),
			Name:    t.Title, Project: project, OneOff: true, CompletedAt: at,
		})
	}
	return out
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
			if e.IsMap || e.CompletedAt.IsZero() || isOneOffsEpicName(filepath.Base(e.Path)) {
				continue
			}
			f := FinishedEpic{
				Address: rootRef{Project: project, Epic: e.Name}.String(),
				Name:    e.Name, Project: project, Done: e.DoneCount(), Total: e.TotalCount(), CompletedAt: e.CompletedAt,
			}
			out = append(out, f)
		}
		for _, e := range epics {
			out = append(out, finishedOneOffs(project, e)...)
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

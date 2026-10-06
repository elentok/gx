package server

import (
	"path/filepath"
	"sort"
	"sync"

	"github.com/elentok/gx/tickets"
)

// TicketInfo is one ticket in the snapshot. Markdown stays the truth: this is
// a read-only projection rebuilt from the store scan.
type TicketInfo struct {
	Address   string   `json:"address"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	// Parent is the address of the ticket this one was forked from.
	Parent string `json:"parent,omitempty"`
}

// Snapshot is the /v1/snapshot payload: tickets sorted by address (so each
// epic's fork tree reads top-down via Parent) and the sequence number of the
// index state they were read at.
type Snapshot struct {
	Seq     uint64       `json:"seq"`
	Tickets []TicketInfo `json:"tickets"`
}

// index is the server's in-memory view of the ticket store.
type index struct {
	mu   sync.RWMutex
	seq  uint64
	list []TicketInfo
}

// buildIndex scans every project in the ticket store.
func buildIndex(storePath string) (*index, error) {
	idx := &index{}
	if storePath == "" {
		return idx, nil
	}
	dirs, err := tickets.ProjectDirs(storePath)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := tickets.Load(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range epics {
			for _, t := range e.Tickets {
				idx.list = append(idx.list, ticketInfo(project, filepath.Base(e.Path), t))
			}
		}
	}
	sort.Slice(idx.list, func(i, j int) bool { return idx.list[i].Address < idx.list[j].Address })
	return idx, nil
}

func ticketInfo(project, epic string, t tickets.Ticket) TicketInfo {
	addr := func(id string) string { return tickets.Address{Project: project, Epic: epic, ID: id}.String() }
	info := TicketInfo{
		Address:   addr(t.DisplayNumber()),
		Title:     t.Title,
		Status:    t.Status,
		Type:      t.Type,
		BlockedBy: t.BlockedBy,
	}
	if t.Parent != nil {
		info.Parent = addr(*t.Parent)
	}
	return info
}

func (i *index) snapshot() Snapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return Snapshot{Seq: i.seq, Tickets: append([]TicketInfo{}, i.list...)}
}

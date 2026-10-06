package server

import (
	"path/filepath"
	"reflect"
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
	Seq uint64 `json:"seq"`
	// HerdrUnavailable is set while herdr isn't answering; the server's own
	// view, filled in by the handler rather than the index.
	HerdrUnavailable bool         `json:"herdr_unavailable,omitempty"`
	Tickets          []TicketInfo `json:"tickets"`
}

// index is the server's in-memory view of the ticket store.
type index struct {
	mu     sync.RWMutex
	events *broker
	list   []TicketInfo
}

// scanStore reads every project in the ticket store. Always a full read of the
// files: the index never merges, so the file wins on every rescan.
func scanStore(storePath string) ([]TicketInfo, error) {
	list := []TicketInfo{}
	if storePath == "" {
		return list, nil
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
				list = append(list, ticketInfo(project, filepath.Base(e.Path), t))
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	return list, nil
}

// buildIndex does the initial scan.
func buildIndex(storePath string, events *broker) (*index, error) {
	list, err := scanStore(storePath)
	if err != nil {
		return nil, err
	}
	return &index{list: list, events: events}, nil
}

// refresh rescans the store and replaces the index, publishing one
// ticket-changed event per ticket that differs. Publishing happens under the
// lock so a snapshot's seq always matches the list it carries.
func (i *index) refresh(storePath string) error {
	list, err := scanStore(storePath)
	if err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, addr := range changedAddresses(i.list, list) {
		i.events.publish(EventTicketChanged, addr)
	}
	i.list = list
	return nil
}

// changedAddresses lists, in address order, tickets added, removed or edited
// between two address-sorted lists.
func changedAddresses(old, next []TicketInfo) []string {
	byAddr := make(map[string]TicketInfo, len(old))
	for _, t := range old {
		byAddr[t.Address] = t
	}
	var changed []string
	for _, t := range next {
		prev, ok := byAddr[t.Address]
		if !ok || !reflect.DeepEqual(prev, t) {
			changed = append(changed, t.Address)
		}
		delete(byAddr, t.Address)
	}
	for addr := range byAddr {
		changed = append(changed, addr)
	}
	sort.Strings(changed)
	return changed
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
	return Snapshot{Seq: i.events.currentSeq(), Tickets: append([]TicketInfo{}, i.list...)}
}

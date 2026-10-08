package server

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

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
	// File is the ticket's markdown path, for clients that edit content or
	// set a plain status directly (those are direct writes, not server ones).
	File string `json:"file,omitempty"`
	// ClaimedAt is when the server launched the claimed ticket's iteration;
	// filled in by the snapshot handler, zero while the ticket is not running.
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// The landing-time metrics the Queue header and the preview sum up; zero
	// until the ticket lands.
	ActualContextWindow   int     `json:"actual_context_window,omitempty"`
	ExpectedContextWindow int     `json:"expected_context_window,omitempty"`
	ElapsedTime           int     `json:"elapsed_time,omitempty"`
	ActualCost            float64 `json:"actual_cost,omitempty"`
	Compactions           int     `json:"compactions,omitempty"`

	// sum fingerprints the file the index read, so a claim can tell the file
	// moved on since. Unexported: it is not part of the API.
	sum string
}

// Snapshot is the /v1/snapshot payload: tickets sorted by address (so each
// epic's fork tree reads top-down via Parent) and the sequence number of the
// index state they were read at.
type Snapshot struct {
	Seq uint64 `json:"seq"`
	// HerdrUnavailable is set while herdr isn't answering; the server's own
	// view, filled in by the handler rather than the index.
	HerdrUnavailable bool `json:"herdr_unavailable,omitempty"`
	// Budget is today's spend, filled in by the handler like HerdrUnavailable.
	Budget  BudgetStatus `json:"budget"`
	Tickets []TicketInfo `json:"tickets"`
	// Pending is every queue entry with its explain verdict, so a client never
	// asks per row. Filled in by the handler, like HerdrUnavailable.
	Pending []PendingRow `json:"pending"`
}

// index is the server's in-memory view of the ticket store.
type index struct {
	mu     sync.RWMutex
	events *broker
	list   []TicketInfo
	// loaded is each project's epics as the last scan read them, by project
	// dir. Treated as read-only: readers share it until the next scan.
	loaded map[string][]tickets.Epic
}

// scanStore reads every project in the ticket store. Always a full read of the
// files: the index never merges, so the file wins on every rescan.
func scanStore(storePath string) ([]TicketInfo, map[string][]tickets.Epic, error) {
	list := []TicketInfo{}
	loaded := map[string][]tickets.Epic{}
	if storePath == "" {
		return list, loaded, nil
	}
	dirs, err := tickets.ProjectDirs(storePath)
	if err != nil {
		return nil, nil, err
	}
	for _, dir := range dirs {
		project := tickets.ProjectName(dir)
		epics, err := tickets.Load(dir)
		if err != nil {
			return nil, nil, err
		}
		loaded[dir] = epics
		for _, e := range epics {
			for _, t := range e.Tickets {
				list = append(list, ticketInfo(project, filepath.Base(e.Path), t))
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	return list, loaded, nil
}

// buildIndex does the initial scan.
func buildIndex(storePath string, events *broker) (*index, error) {
	list, loaded, err := scanStore(storePath)
	if err != nil {
		return nil, err
	}
	return &index{list: list, loaded: loaded, events: events}, nil
}

// refresh rescans the store and replaces the index, publishing one
// ticket-changed event per ticket that differs. Publishing happens under the
// lock so a snapshot's seq always matches the list it carries.
func (i *index) refresh(storePath string) error {
	list, loaded, err := scanStore(storePath)
	if err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, addr := range changedAddresses(i.list, list) {
		i.events.publish(EventTicketChanged, addr)
	}
	i.list, i.loaded = list, loaded
	return nil
}

// epicsOf is the project's epics as of the last scan.
func (i *index) epicsOf(dir string) ([]tickets.Epic, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	epics, ok := i.loaded[dir]
	return epics, ok
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
		File:      t.Path,

		ActualContextWindow:   t.ActualContextWindow,
		ExpectedContextWindow: t.ExpectedContextWindow,
		ElapsedTime:           t.ElapsedTime,
		ActualCost:            t.ActualCost,
		Compactions:           t.Compactions,
	}
	if t.Parent != nil {
		info.Parent = addr(*t.Parent)
	}
	info.sum, _ = fileSum(t.Path)
	return info
}

// fileSum fingerprints a ticket file's bytes. An unreadable file has no sum,
// which never equals a readable one's.
func fileSum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// sumOf is the fingerprint the index holds for addr.
func (i *index) sumOf(addr string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	n := sort.Search(len(i.list), func(n int) bool { return i.list[n].Address >= addr })
	if n < len(i.list) && i.list[n].Address == addr {
		return i.list[n].sum, true
	}
	return "", false
}

func (i *index) snapshot() Snapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return Snapshot{Seq: i.events.currentSeq(), Tickets: append([]TicketInfo{}, i.list...)}
}

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// indexedTicket is a wire.TicketInfo plus what the index keeps but the API does
// not carry.
type indexedTicket struct {
	wire.TicketInfo
	// agent is the ticket's frontmatter `agent:`, the resolver's top input.
	agent string
	// sum fingerprints the file the index read, so a claim can tell the file
	// moved on since.
	sum string
}

// index is the server's in-memory view of the ticket store.
type index struct {
	mu     sync.RWMutex
	events *broker
	list   []indexedTicket
	// loaded is each project's epics as the last scan read them, by project
	// dir. Treated as read-only: readers share it until the next scan.
	loaded map[string][]tickets.Epic
}

// scanStore reads every project in the ticket store. Always a full read of the
// files: the index never merges, so the file wins on every rescan.
func scanStore(storePath string) ([]indexedTicket, map[string][]tickets.Epic, error) {
	list := []indexedTicket{}
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
		epics, err := tickets.LoadResolved(project, dir)
		if err != nil {
			return nil, nil, err
		}
		loaded[dir] = epics
		for _, e := range epics {
			for _, t := range e.Tickets {
				info := ticketInfo(project, filepath.Base(e.Path), t)
				info.RenderedStatus = e.RenderedStatus(t).Word()
				info.EpicMergedAt = e.MergedAt
				info.EpicIsMap = e.IsMap
				info.EpicStatus = e.Status
				list = append(list, info)
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	return list, loaded, nil
}

// buildIndex does the initial scan. A store the strict scan cannot read yet
// (old-shape tickets a migration is about to rewrite) gives an empty index and
// no error: the scan is deferred to the first rescan after the migrations end.
func buildIndex(storePath string, events *broker) (*index, error) {
	list, loaded, err := scanStore(storePath)
	if err != nil {
		return &index{list: []indexedTicket{}, loaded: map[string][]tickets.Epic{}, events: events}, nil
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
	for _, addr := range changedEpicMeta(i.loaded, loaded) {
		i.events.publish(EventTicketChanged, addr)
	}
	i.list, i.loaded = list, loaded
	return nil
}

// changedEpicMeta lists, in address order, the epics whose design_step or
// merge block differs between two scans. An epic's ticket.md edit changes no
// TicketInfo, so without this a Progress view never learns the step moved and
// an Epic page never learns a merge paused.
func changedEpicMeta(old, next map[string][]tickets.Epic) []string {
	steps := func(m map[string][]tickets.Epic) map[string]string {
		out := map[string]string{}
		for dir, epics := range m {
			project := tickets.ProjectName(dir)
			for _, e := range epics {
				key := strings.Join(append([]string{e.DesignStep, e.MergeBlockKind, e.MergeBlockReason}, e.MergeBlockFiles...), "\x00")
				if strings.Trim(key, "\x00") != "" {
					out[project+":"+filepath.Base(e.Path)] = key
				}
			}
		}
		return out
	}
	prev, cur := steps(old), steps(next)
	var changed []string
	for addr, step := range cur {
		if prev[addr] != step {
			changed = append(changed, addr)
		}
	}
	for addr := range prev {
		if _, ok := cur[addr]; !ok {
			changed = append(changed, addr)
		}
	}
	sort.Strings(changed)
	return changed
}

// epicsOf is the project's epics as of the last scan.
func (i *index) epicsOf(dir string) ([]tickets.Epic, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	epics, ok := i.loaded[dir]
	return epics, ok
}

// allEpics is every project's epics as of the last scan.
func (i *index) allEpics() [][]tickets.Epic {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([][]tickets.Epic, 0, len(i.loaded))
	for _, es := range i.loaded {
		out = append(out, es)
	}
	return out
}

// projectEpics is the named project's directory and epics as of the last scan,
// with no store read. ok is false for a project the scan has not seen.
func (i *index) projectEpics(project string) (dir string, epics []tickets.Epic, ok bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for d, es := range i.loaded {
		if tickets.ProjectName(d) == project {
			return d, es, true
		}
	}
	return "", nil, false
}

// changedAddresses lists, in address order, tickets added, removed or edited
// between two address-sorted lists.
func changedAddresses(old, next []indexedTicket) []string {
	byAddr := make(map[string]indexedTicket, len(old))
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

func ticketInfo(project, epic string, t tickets.Ticket) indexedTicket {
	addr := func(id string) string { return tickets.Address{Project: project, Epic: epic, ID: id}.String() }
	info := wire.TicketInfo{
		Address:   addr(t.DisplayNumber()),
		Title:     t.Title,
		Status:    t.LegacyStatus(),
		Type:      t.Type,
		BlockedBy: t.BlockedBy,
		File:      t.Path,

		ExternalBlockers: t.ExternalBlockers,
		WidenedBlockers:  t.WidenedBlockers,

		ActualContextWindow:   t.ActualContextWindow,
		ExpectedContextWindow: t.ExpectedContextWindow,
		ElapsedTime:           t.ElapsedTime,
		ActualCost:            t.ActualCost,
		HadCodex:              t.HadCodex,
		Compactions:           t.Compactions,

		Category:   t.Category,
		ExportedAt: t.ExportedAt,
		Mutes:      t.Mutes,
	}
	if t.Parent != nil {
		info.Parent = addr(*t.Parent)
	}
	sum, _ := fileSum(t.Path)
	return indexedTicket{TicketInfo: info, agent: t.Agent, sum: sum}
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

func (i *index) snapshot() wire.Snapshot {
	snap, _ := i.snapshotAgents()
	return snap
}

// snapshotAgents is snapshot plus each ticket's frontmatter agent, parallel to
// Snapshot.Tickets.
func (i *index) snapshotAgents() (wire.Snapshot, []string) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	infos := make([]wire.TicketInfo, len(i.list))
	agents := make([]string, len(i.list))
	for n, t := range i.list {
		infos[n], agents[n] = t.TicketInfo, t.agent
	}
	return wire.Snapshot{Seq: i.events.currentSeq(), Tickets: infos}, agents
}

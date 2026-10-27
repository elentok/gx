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
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TicketInfo is one ticket in the snapshot. Markdown stays the truth: this is
// a read-only projection rebuilt from the store scan.
type TicketInfo struct {
	Address string `json:"address"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	// RenderedStatus is tickets.Epic.RenderedStatus's word (blocked,
	// waiting-for-children, ...): the one status every client shows.
	RenderedStatus string   `json:"rendered_status"`
	Type           string   `json:"type"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	// ExternalBlockers and WidenedBlockers carry the cross-epic resolution
	// (see tickets.Ticket), which a client can't redo from one scope's tickets.
	ExternalBlockers []string          `json:"external_blockers,omitempty"`
	WidenedBlockers  map[string]string `json:"widened_blockers,omitempty"`
	// Parent is the address of the ticket this one was forked from.
	Parent string `json:"parent,omitempty"`
	// File is the ticket's markdown path, for clients that edit content or
	// set a plain status directly (those are direct writes, not server ones).
	File string `json:"file,omitempty"`
	// ClaimedAt is when the server launched the claimed ticket's iteration;
	// filled in by the snapshot handler, zero while the ticket is not running.
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// Recovery is RecoveryPending or RecoveryEscalated for a paused ticket that
	// recovery is working on or gave up on; filled in by the snapshot handler.
	Recovery string `json:"recovery,omitempty"`
	// The merging-time metrics the Queue header and the preview sum up; zero
	// until the ticket merges.
	ActualContextWindow   int     `json:"actual_context_window,omitempty"`
	ExpectedContextWindow int     `json:"expected_context_window,omitempty"`
	ElapsedTime           int     `json:"elapsed_time,omitempty"`
	ActualCost            float64 `json:"actual_cost,omitempty"`
	Compactions           int     `json:"compactions,omitempty"`

	Category   string    `json:"category,omitempty"`
	ExportedAt time.Time `json:"exported_at,omitzero"`
	// Mutes is what the Context menu's "Unmute & Reopen" keys off.
	Mutes []schema.MuteRecord `json:"mutes,omitempty"`

	// EpicMergedAt is the ticket's epic's merged_at. It rides on every ticket
	// rather than in a separate epic list so a stamp reaches clients through
	// the same ticket-changed events as any other edit.
	EpicMergedAt time.Time `json:"epic_merged_at,omitzero"`
	// EpicIsMap is set on a wayfinder map epic's tickets, riding along like
	// EpicMergedAt.
	EpicIsMap bool `json:"epic_is_map,omitempty"`
	// EpicStatus is the epic's ticket.md status, riding along likewise.
	EpicStatus string `json:"epic_status,omitempty"`
	// EpicMergeTarget is the branch an epic merge merges on; filled in by the
	// snapshot handler, empty when the project has no repo or trunk.
	EpicMergeTarget string `json:"epic_merge_target,omitempty"`

	// sum fingerprints the file the index read, so a claim can tell the file
	// moved on since. Unexported: it is not part of the API.
	sum string
}

// Snapshot is the /v1/snapshot payload: tickets sorted by address (so each
// epic's fork tree reads top-down via Parent) and the sequence number of the
// index state they were read at.
type Snapshot struct {
	Seq uint64 `json:"seq"`
	// Build is the server's own version, filled in by the handler. Empty from a
	// server that predates it.
	Build string `json:"build,omitempty"`
	// HerdrUnavailable is set while herdr isn't answering; the server's own
	// view, filled in by the handler rather than the index.
	HerdrUnavailable bool `json:"herdr_unavailable,omitempty"`
	// Budget is today's spend, filled in by the handler like HerdrUnavailable.
	Budget BudgetStatus `json:"budget"`
	// ExtraUsage is set while the account auto-purchases extra usage, so the
	// enqueue confirm can warn; filled in by the handler.
	ExtraUsage bool         `json:"extra_usage,omitempty"`
	Tickets    []TicketInfo `json:"tickets"`
	// Pending is every queue entry with its explain verdict, so a client never
	// asks per row. Filled in by the handler, like HerdrUnavailable.
	Pending []PendingRow `json:"pending"`
	// Mode is QueueModeRunning or QueueModePaused; filled in by the handler.
	// Empty from a server that predates it.
	Mode QueueMode `json:"mode,omitempty"`
	// AFK is set while AFK mode sends session banners to chat too; filled in by
	// the handler.
	AFK bool `json:"afk,omitempty"`
	// SkillsInstallProblem is why the skill install `gx server start` ran
	// last did not finish; empty when it did. Filled in by the handler.
	SkillsInstallProblem string `json:"skills_install_problem,omitempty"`
	// Sessions are the interactive sessions, oldest first; filled in by the
	// handler.
	Sessions []SessionInfo `json:"sessions"`
	// Epics summarizes every epic holding a queue item; filled in by the handler.
	Epics []EpicSummary `json:"epics"`
	// RecentlyFinished is the last completed loop epics, newest first; filled in
	// by the handler.
	RecentlyFinished []FinishedEpic `json:"recently_finished"`
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
		Compactions:           t.Compactions,

		Category:   t.Category,
		ExportedAt: t.ExportedAt,
		Mutes:      t.Mutes,
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

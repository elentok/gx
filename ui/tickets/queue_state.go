package tickets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
)

// queueItemStatus is a checked ticket's queue-run status (ticket 11),
// persisted alongside the checked set so a restart shows the same per-item
// progress an in-flight run had reached. Distinct from both
// tickets.RenderedStatus (the ticket file's own Status: frontmatter) and
// liveTicketState (in-memory-only orchestrator detail from a live event
// stream) — this is the queue's own durable bookkeeping, updated as
// execution wiring (tickets 08/09/12) progresses a ticket through it.
type queueItemStatus string

const (
	queueStatusPending queueItemStatus = "pending"
	queueStatusRunning queueItemStatus = "running"
	queueStatusDone    queueItemStatus = "done"
	queueStatusErrored queueItemStatus = "errored"
)

// queueStateDirFn resolves the same base directory config.FilePath uses,
// overridden in tests (see TestMain) so the package's test suite never
// touches the real machine's config dir.
var queueStateDirFn = config.UserConfigDir

// queueStateFilePath returns the on-disk path for the persisted queue state,
// mirroring config.FilePath's ~/.config/gx/ layout.
func queueStateFilePath() (string, error) {
	base, err := queueStateDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gx", "queue-state.json"), nil
}

// persistedQueueState is the on-disk shape: keyed by Ticket.Path, the same
// key Model.checked/Model.queueStatus use in memory.
//
// Items/CheckOrder is the Queue tab's durable membership+status (today's
// coupled meaning — see ticket 13's findings: presence in Items has always
// meant both "checked" and "queued" at once). It's kept exactly as-is, with
// unchanged read/write semantics on IsChecked/SetChecked/Check/Uncheck/
// SetStatus/Replace/Snapshot, so every pre-existing Tickets/Queue-tab call
// site (ui/tickets/checked.go, queue.go, queue_clear.go, implement.go,
// model.go) keeps behaving exactly as it does today — ticket 15 is the one
// that migrates those call sites onto the independent API below.
//
// Checked/TicketCheckOrder is the new, genuinely independent Tickets-tab
// checked selection (ticket 13's decoupled design): a ticket can be checked
// without being queued, and vice versa. Nothing currently reads or writes
// it; it exists so ticket 15 can wire the Tickets-tab checkbox and the "i"
// queueing flow onto it without another storage-layer change.
type persistedQueueState struct {
	Items      map[string]queueItemStatus `json:"items"`
	CheckOrder map[string]uint64          `json:"check_order"`

	Checked          map[string]bool   `json:"checked"`
	TicketCheckOrder map[string]uint64 `json:"ticket_check_order,omitempty"`
}

type QueueSnapshot struct {
	Checked map[string]bool
	Order   map[string]uint64
	Status  map[string]queueItemStatus

	// TicketChecked/TicketCheckOrder mirror persistedQueueState's
	// independent checked set — see its doc comment.
	TicketChecked    map[string]bool
	TicketCheckOrder map[string]uint64
}

// QueueStore is the application-lifetime owner of durable queue state.
type QueueStore struct {
	mu    sync.RWMutex
	path  string
	state persistedQueueState
}

func LoadQueueStore() *QueueStore {
	path, err := queueStateFilePath()
	if err != nil {
		return newQueueStore("")
	}
	return loadQueueStoreAt(path)
}

func newQueueStore(path string) *QueueStore {
	return &QueueStore{path: path, state: emptyPersistedQueueState()}
}

func emptyPersistedQueueState() persistedQueueState {
	return persistedQueueState{
		Items:            map[string]queueItemStatus{},
		CheckOrder:       map[string]uint64{},
		Checked:          map[string]bool{},
		TicketCheckOrder: map[string]uint64{},
	}
}

func loadQueueStoreAt(path string) *QueueStore {
	store := newQueueStore(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var state persistedQueueState
	if json.Unmarshal(data, &state) != nil || state.Items == nil || state.CheckOrder == nil {
		return store
	}
	items, checkOrder := cloneStatus(state.Items), cloneOrder(state.CheckOrder)
	checked, ticketCheckOrder := state.Checked, state.TicketCheckOrder
	if checked == nil || ticketCheckOrder == nil {
		// Pre-decoupling file (or a partially-shaped one): Items meant
		// "checked AND queued" together, so the only correct migration is
		// to seed the independent checked set from Items' own keys — see
		// ticket 13's migration design. The next write persists the
		// two-concept shape, so this is a read-time-only upgrade.
		checked = make(map[string]bool, len(items))
		for path := range items {
			checked[path] = true
		}
		ticketCheckOrder = cloneOrder(checkOrder)
	} else {
		checked, ticketCheckOrder = cloneBool(checked), cloneOrder(ticketCheckOrder)
	}
	store.state = persistedQueueState{
		Items: items, CheckOrder: checkOrder,
		Checked: checked, TicketCheckOrder: ticketCheckOrder,
	}
	return store
}

func (s *QueueStore) Snapshot() QueueSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	checked := make(map[string]bool, len(s.state.Items))
	for path := range s.state.Items {
		checked[path] = true
	}
	return QueueSnapshot{
		Checked: checked, Order: cloneOrder(s.state.CheckOrder), Status: cloneStatus(s.state.Items),
		TicketChecked: cloneBool(s.state.Checked), TicketCheckOrder: cloneOrder(s.state.TicketCheckOrder),
	}
}

func (s *QueueStore) IsChecked(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.state.Items[path]
	return ok
}

func (s *QueueStore) Check(path string) error {
	return s.SetChecked([]string{path}, true)
}

func (s *QueueStore) SetChecked(paths []string, checked bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cloneStateLocked()
	changed := false
	for _, path := range paths {
		_, exists := next.Items[path]
		if checked {
			if exists {
				continue
			}
			next.Items[path] = queueStatusPending
			next.CheckOrder[path] = nextCheckOrdinal(next.CheckOrder)
			changed = true
			continue
		}
		if !exists {
			continue
		}
		delete(next.Items, path)
		delete(next.CheckOrder, path)
		changed = true
	}
	if !changed {
		return nil
	}
	return s.commitLocked(next)
}

func (s *QueueStore) Uncheck(path string) error {
	return s.SetChecked([]string{path}, false)
}

func (s *QueueStore) SetStatus(path string, status queueItemStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Items[path]; !ok {
		return nil
	}
	next := s.cloneStateLocked()
	next.Items[path] = status
	return s.commitLocked(next)
}

func (s *QueueStore) Replace(items map[string]queueItemStatus, order map[string]uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cloneStateLocked()
	next.Items, next.CheckOrder = cloneStatus(items), cloneOrder(order)
	return s.commitLocked(next)
}

// IsQueued reports whether path is in the Queue tab's durable membership —
// today identical to IsChecked (Items backs both concepts), but named for
// ticket 15's independent-API migration so callers moving off the coupled
// checked/queued behavior have a queue-only read that won't change meaning
// when SetChecked/IsChecked are eventually narrowed to the checked set alone.
func (s *QueueStore) IsQueued(path string) bool {
	return s.IsChecked(path)
}

// SetQueued mutates the Queue tab's durable membership only — see IsQueued.
func (s *QueueStore) SetQueued(paths []string, queued bool) error {
	return s.SetChecked(paths, queued)
}

// IsTicketChecked reports whether path is in the independent Tickets-tab
// checked set (ticket 13's decoupled design) — separate from queue
// membership. Nothing is wired to this yet; ticket 15 adopts it.
func (s *QueueStore) IsTicketChecked(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Checked[path]
}

// SetTicketChecked mutates only the independent Tickets-tab checked set,
// leaving queue membership/status untouched — the real decoupled
// counterpart to SetChecked (see persistedQueueState's doc comment).
func (s *QueueStore) SetTicketChecked(paths []string, checked bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cloneStateLocked()
	changed := false
	for _, path := range paths {
		exists := next.Checked[path]
		if checked {
			if exists {
				continue
			}
			next.Checked[path] = true
			next.TicketCheckOrder[path] = nextCheckOrdinal(next.TicketCheckOrder)
			changed = true
			continue
		}
		if !exists {
			continue
		}
		delete(next.Checked, path)
		delete(next.TicketCheckOrder, path)
		changed = true
	}
	if !changed {
		return nil
	}
	return s.commitLocked(next)
}

// EnqueueAndClearChecked atomically replaces the Queue tab's durable
// membership (Items/CheckOrder) with (queued, queueOrder) and removes
// clearedPaths from the independent Tickets-tab checked set, in a single
// write. Atomicity matters here specifically because this is the one call
// site (ticket 10's replaceQueuedSelection) that touches both concepts
// together — an interrupted write must never leave the queue updated with
// the Tickets-tab checkboxes still showing the just-queued selection, or
// vice versa.
func (s *QueueStore) EnqueueAndClearChecked(queued map[string]queueItemStatus, queueOrder map[string]uint64, clearedPaths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := persistedQueueState{
		Items: cloneStatus(queued), CheckOrder: cloneOrder(queueOrder),
		Checked: cloneBool(s.state.Checked), TicketCheckOrder: cloneOrder(s.state.TicketCheckOrder),
	}
	for _, path := range clearedPaths {
		delete(next.Checked, path)
		delete(next.TicketCheckOrder, path)
	}
	return s.commitLocked(next)
}

func (s *QueueStore) cloneStateLocked() persistedQueueState {
	return persistedQueueState{
		Items: cloneStatus(s.state.Items), CheckOrder: cloneOrder(s.state.CheckOrder),
		Checked: cloneBool(s.state.Checked), TicketCheckOrder: cloneOrder(s.state.TicketCheckOrder),
	}
}

func (s *QueueStore) commitLocked(next persistedQueueState) error {
	if s.path == "" {
		return fmt.Errorf("queue state path is unavailable")
	}
	if err := writeQueueStateAtomically(s.path, next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func cloneStatus(in map[string]queueItemStatus) map[string]queueItemStatus {
	out := make(map[string]queueItemStatus, len(in))
	for path, status := range in {
		out[path] = status
	}
	return out
}

func cloneOrder(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for path, order := range in {
		out[path] = order
	}
	return out
}

func cloneBool(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for path, v := range in {
		out[path] = v
	}
	return out
}

func writeQueueStateAtomically(path string, state persistedQueueState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

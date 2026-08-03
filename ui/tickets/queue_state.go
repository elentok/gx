package tickets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// queueStateDirFn resolves the same base directory config.FilePath's
// os.UserConfigDir()-based precedent uses, overridden in tests (see
// TestMain) so the package's test suite never touches the real machine's
// config dir.
var queueStateDirFn = os.UserConfigDir

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
type persistedQueueState struct {
	Items      map[string]queueItemStatus `json:"items"`
	CheckOrder map[string]uint64          `json:"check_order,omitempty"`
}

type QueueSnapshot struct {
	Checked map[string]bool
	Order   map[string]uint64
	Status  map[string]queueItemStatus
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
	return &QueueStore{path: path, state: persistedQueueState{Items: map[string]queueItemStatus{}, CheckOrder: map[string]uint64{}}}
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
	store.state = persistedQueueState{Items: cloneStatus(state.Items), CheckOrder: cloneOrder(state.CheckOrder)}
	return store
}

func (s *QueueStore) Snapshot() QueueSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	checked := make(map[string]bool, len(s.state.Items))
	for path := range s.state.Items {
		checked[path] = true
	}
	return QueueSnapshot{Checked: checked, Order: cloneOrder(s.state.CheckOrder), Status: cloneStatus(s.state.Items)}
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
	return s.commitLocked(persistedQueueState{Items: cloneStatus(items), CheckOrder: cloneOrder(order)})
}

func (s *QueueStore) cloneStateLocked() persistedQueueState {
	return persistedQueueState{Items: cloneStatus(s.state.Items), CheckOrder: cloneOrder(s.state.CheckOrder)}
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

// loadQueueState reads the persisted queue state. A missing or corrupt file
// falls back to an empty queue rather than surfacing a startup error — the
// queue is recoverable bookkeeping (the user can just re-check tickets), not
// worth blocking startup over.
func loadQueueState() (map[string]queueItemStatus, map[string]uint64) {
	path, err := queueStateFilePath()
	if err != nil {
		return map[string]queueItemStatus{}, map[string]uint64{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]queueItemStatus{}, map[string]uint64{}
	}
	var state persistedQueueState
	if err := json.Unmarshal(data, &state); err != nil || state.Items == nil {
		return map[string]queueItemStatus{}, map[string]uint64{}
	}
	if state.CheckOrder == nil {
		state.CheckOrder = map[string]uint64{}
	}
	return state.Items, state.CheckOrder
}

// persistQueueState publishes the page's working snapshot through its store.
func (m Model) persistQueueState() error {
	if m.queueStore != nil {
		return m.queueStore.Replace(m.queueStatus, m.checkOrder)
	}
	return saveQueueState(m.queueStatus, m.checkOrder)
}

// setQueueItemStatus updates a checked ticket's queue status and persists
// the change — the transition point execution wiring (tickets 08/09/12)
// calls as a run proceeds a ticket from pending through running to
// done/errored. A no-op if path isn't currently checked.
func (m *Model) setQueueItemStatus(path string, status queueItemStatus) error {
	if m.queueStore != nil {
		if !m.queueStore.IsChecked(path) {
			return nil
		}
		if err := m.queueStore.SetStatus(path, status); err != nil {
			return err
		}
		m.refreshQueueSnapshot()
		return nil
	}
	if !m.checked[path] {
		return nil
	}
	if m.queueStatus == nil {
		m.queueStatus = map[string]queueItemStatus{}
	}
	m.queueStatus[path] = status
	return m.persistQueueState()
}

// saveQueueState writes the checked set and its per-item status to disk, so
// the next startup's loadQueueState sees this exact state.
func saveQueueState(items map[string]queueItemStatus, checkOrder map[string]uint64) error {
	path, err := queueStateFilePath()
	if err != nil {
		return err
	}
	return writeQueueStateAtomically(path, persistedQueueState{Items: items, CheckOrder: checkOrder})
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

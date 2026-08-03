package tickets

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	Items map[string]queueItemStatus `json:"items"`
}

// loadQueueState reads the persisted queue state. A missing or corrupt file
// falls back to an empty queue rather than surfacing a startup error — the
// queue is recoverable bookkeeping (the user can just re-check tickets), not
// worth blocking startup over.
func loadQueueState() map[string]queueItemStatus {
	path, err := queueStateFilePath()
	if err != nil {
		return map[string]queueItemStatus{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]queueItemStatus{}
	}
	var state persistedQueueState
	if err := json.Unmarshal(data, &state); err != nil || state.Items == nil {
		return map[string]queueItemStatus{}
	}
	return state.Items
}

// persistQueueState best-effort saves m.queueStatus to disk. A save failure
// isn't surfaced to the user — the queue is recoverable bookkeeping, and
// interrupting a checkbox toggle over a disk-write error would be worse than
// silently retrying on the next mutation.
func (m Model) persistQueueState() {
	_ = saveQueueState(m.queueStatus)
}

// setQueueItemStatus updates a checked ticket's queue status and persists
// the change — the transition point execution wiring (tickets 08/09/12)
// calls as a run proceeds a ticket from pending through running to
// done/errored. A no-op if path isn't currently checked.
func (m *Model) setQueueItemStatus(path string, status queueItemStatus) {
	if !m.checked[path] {
		return
	}
	if m.queueStatus == nil {
		m.queueStatus = map[string]queueItemStatus{}
	}
	m.queueStatus[path] = status
	m.persistQueueState()
}

// saveQueueState writes the checked set and its per-item status to disk, so
// the next startup's loadQueueState sees this exact state.
func saveQueueState(items map[string]queueItemStatus) error {
	path, err := queueStateFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(persistedQueueState{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

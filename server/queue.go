package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

// EventQueueChanged is emitted once per queue write; Address is the ticket it
// touched. Clients re-read the queue on it.
const EventQueueChanged = "queue-changed"

const queueFileName = "queue.json"

// Refusal reasons of the queue writes. Stable: clients switch on them.
const (
	ReasonSchedulerNotSelected = "scheduler-not-selected"
	// ReasonServerNotRunning is produced by the CLI, never the server.
	ReasonServerNotRunning = "server-not-running"
	ReasonUnknownTicket        = "unknown-ticket"
	ReasonInvalidAddress       = "invalid-address"
	ReasonInvalidAgent         = "invalid-agent"
	ReasonAlreadyQueued        = "already-queued"
	ReasonNotQueued            = "not-queued"
	ReasonBadPosition          = "bad-position"
)

// QueueItem is one queued ticket and the agent it will run under.
type QueueItem struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
}

// QueueResult is what every queue write returns: the queue after the write, or
// a refusal (the queue is then untouched).
type QueueResult struct {
	Queue   []QueueItem `json:"queue,omitempty"`
	Refused bool        `json:"refused,omitempty"`
	Reason  string      `json:"reason,omitempty"`
	Message string      `json:"message,omitempty"`
}

func refusal(reason, msg string) QueueResult {
	return QueueResult{Refused: true, Reason: reason, Message: msg}
}

// QueueRequest is the body of the queue writes; each uses the fields it needs.
type QueueRequest struct {
	Address string `json:"address"`
	Agent   string `json:"agent,omitempty"`
	// Position is the 1-based slot a move puts the ticket in.
	Position int `json:"position,omitempty"`
}

// queueStore is the server-wide queue. It lives in a state-dir file, never in a
// ticket's markdown, so order survives a restart without touching the store.
type queueStore struct {
	mu    sync.Mutex
	path  string
	items []QueueItem
}

func openQueue(stateDir string) (*queueStore, error) {
	q := &queueStore{path: filepath.Join(stateDir, queueFileName), items: []QueueItem{}}
	data, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return q, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &q.items); err != nil {
		return nil, err
	}
	return q, nil
}

// save writes through a rename so a crash never leaves a torn queue file.
func (q *queueStore) save() error {
	data, err := json.Marshal(q.items)
	if err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path)
}

func (q *queueStore) indexOf(address string) int {
	for i, it := range q.items {
		if it.Address == address {
			return i
		}
	}
	return -1
}

func (q *queueStore) list() []QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]QueueItem{}, q.items...)
}

// writeQueue applies mutate under the lock, persists, then publishes, so events
// arrive in queue order. A refusal from mutate leaves the queue untouched.
func (s *Server) writeQueue(address string, mutate func(items []QueueItem) ([]QueueItem, *QueueResult)) (QueueResult, error) {
	if s.cfg.Orchestrator != config.OrchestratorServer {
		return refusal(ReasonSchedulerNotSelected, `orchestrator is not "server"`), nil
	}
	q := s.queued
	q.mu.Lock()
	defer q.mu.Unlock()
	next, refused := mutate(append([]QueueItem{}, q.items...))
	if refused != nil {
		return *refused, nil
	}
	prev := q.items
	q.items = next
	if err := q.save(); err != nil {
		q.items = prev
		return QueueResult{}, err
	}
	s.events.publish(EventQueueChanged, address)
	s.kickRunner()
	return QueueResult{Queue: append([]QueueItem{}, next...)}, nil
}

func (s *Server) hasTicket(address string) bool {
	for _, t := range s.idx.snapshot().Tickets {
		if t.Address == address {
			return true
		}
	}
	return false
}

// canonical normalises a request's address, or returns the refusal for it.
func canonical(raw string) (string, *QueueResult) {
	a, err := tickets.ParseAddress(raw, tickets.AddressContext{})
	if err != nil {
		r := refusal(ReasonInvalidAddress, err.Error())
		return "", &r
	}
	return a.String(), nil
}

func (s *Server) queueAdd(req QueueRequest) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	agent := ralphloop.AgentKind(req.Agent)
	if req.Agent == "" {
		agent = ralphloop.AgentClaude
	}
	if err := ralphloop.ValidateAgentKind(agent); err != nil {
		return refusal(ReasonInvalidAgent, err.Error()), nil
	}
	return s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		if !s.hasTicket(addr) {
			r := refusal(ReasonUnknownTicket, "no ticket "+addr)
			return nil, &r
		}
		for _, it := range items {
			if it.Address == addr {
				r := refusal(ReasonAlreadyQueued, addr+" is already queued")
				return nil, &r
			}
		}
		return append(items, QueueItem{Address: addr, Agent: string(agent)}), nil
	})
}

func (s *Server) queueRemove(req QueueRequest) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	return s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		for i, it := range items {
			if it.Address == addr {
				return append(items[:i], items[i+1:]...), nil
			}
		}
		r := refusal(ReasonNotQueued, addr+" is not queued")
		return nil, &r
	})
}

func (s *Server) queueMove(req QueueRequest) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	return s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		from := -1
		for i, it := range items {
			if it.Address == addr {
				from = i
			}
		}
		if from < 0 {
			r := refusal(ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		if req.Position < 1 || req.Position > len(items) {
			r := refusal(ReasonBadPosition, "position must be between 1 and the queue length")
			return nil, &r
		}
		it := items[from]
		items = append(items[:from], items[from+1:]...)
		to := req.Position - 1
		items = append(items[:to], append([]QueueItem{it}, items[to:]...)...)
		return items, nil
	})
}

func (s *Server) queueItems(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.queued.list())
}

// queueWrite adapts a queue write to a POST handler. Refusals are results, so
// they answer 200; only a failure to persist is an HTTP error.
func (s *Server) queueWrite(do func(QueueRequest) (QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req QueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

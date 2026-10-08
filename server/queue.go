package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	ReasonUnknownTicket    = "unknown-ticket"
	ReasonInvalidAddress   = "invalid-address"
	ReasonInvalidAgent     = "invalid-agent"
	ReasonAlreadyQueued    = "already-queued"
	ReasonNotQueued        = "not-queued"
	ReasonBadPosition      = "bad-position"
	ReasonTicketLive       = "ticket-live"
	ReasonMapEpic          = "map-epic"
)

// QueueItem is one queued ticket and the agent it will run under.
type QueueItem struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
}

// QueueResult is what every queue write returns: the queue after the write, or
// a refusal (the queue is then untouched).
type QueueResult struct {
	Queue []QueueItem `json:"queue,omitempty"`
	// Mode is set by pause, resume and drain: running, paused or draining.
	Mode string `json:"mode,omitempty"`
	// ExtraUsage is the warning an enqueue carries while the account
	// auto-purchases extra usage.
	ExtraUsage bool   `json:"extra_usage,omitempty"`
	Refused    bool   `json:"refused,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
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
	// Project and Items are what a replace swaps in.
	Project string      `json:"project,omitempty"`
	Items   []QueueItem `json:"items,omitempty"`
	// ParkReason is the one-line reason a park writes into the ticket.
	ParkReason string `json:"park_reason,omitempty"`
	// Stop lets a cancel stop the live pane of a claimed ticket instead of refusing.
	Stop bool `json:"stop,omitempty"`
	// Front makes an add land at the head of the queue instead of the tail.
	Front bool `json:"front,omitempty"`
	// Text is what a nudge types into the live pane.
	Text string `json:"text,omitempty"`

	// actor is set only by the server's own recovery calls, never decoded from a
	// client: a park made by recovery must not trigger recovery again.
	actor string
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

// mapEpicRefusal refuses to queue a ticket of a map epic: a map's tickets are
// decisions a person resolves by hand, never scheduled work.
func (s *Server) mapEpicRefusal(address string) *QueueResult {
	if !s.inMapEpic(address) {
		return nil
	}
	r := refusal(ReasonMapEpic, address+" is in a map epic; its tickets are resolved by hand, not scheduled")
	return &r
}

// inMapEpic reports whether address names a ticket (or root) of a map epic.
func (s *Server) inMapEpic(address string) bool {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return false
	}
	dir, err := s.projectDir(a.Project)
	if err != nil {
		return false
	}
	return tickets.IsMapEpic(filepath.Join(dir, a.Epic))
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

// withExtraUsage runs the extra-usage check for an enqueue and puts the warning
// on an accepted result.
func (s *Server) withExtraUsage(res QueueResult, err error) (QueueResult, error) {
	if err == nil && !res.Refused {
		res.ExtraUsage = s.checkExtraUsage(time.Now())
	}
	return res, err
}

func (s *Server) queueAdd(req QueueRequest) (QueueResult, error) {
	return s.withExtraUsage(s.queueAddItem(req))
}

// resolveAgent defaults an empty agent to claude and refuses an unknown one.
func resolveAgent(name string) (ralphloop.AgentKind, *QueueResult) {
	agent := ralphloop.AgentKind(name)
	if name == "" {
		agent = ralphloop.AgentClaude
	}
	if err := ralphloop.ValidateAgentKind(agent); err != nil {
		r := refusal(ReasonInvalidAgent, err.Error())
		return "", &r
	}
	return agent, nil
}

func (s *Server) queueAddItem(req QueueRequest) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	agent, badAgent := resolveAgent(req.Agent)
	if badAgent != nil {
		return *badAgent, nil
	}
	return s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		if !s.hasTicket(addr) {
			r := refusal(ReasonUnknownTicket, "no ticket "+addr)
			return nil, &r
		}
		if r := s.mapEpicRefusal(addr); r != nil {
			return nil, r
		}
		for _, it := range items {
			if it.Address == addr {
				r := refusal(ReasonAlreadyQueued, addr+" is already queued")
				return nil, &r
			}
		}
		item := QueueItem{Address: addr, Agent: string(agent)}
		if req.Front {
			return append([]QueueItem{item}, items...), nil
		}
		return append(items, item), nil
	})
}

func (s *Server) queueRemove(req QueueRequest) (QueueResult, error) {
	addr, bad := canonical(req.Address)
	if bad != nil {
		return *bad, nil
	}
	return s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		if indexOfAddr(items, addr) < 0 {
			r := refusal(ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		subtree := s.subtree(addr)
		if live := s.liveIn(subtree); live != "" {
			r := refusal(ReasonTicketLive, live+" has a live iteration")
			return nil, &r
		}
		return dropAddresses(items, subtree), nil
	})
}

// queueReplace swaps one project's pending entries for req.Items, leaving the
// other projects' entries where they are. The new entries go at the end.
func (s *Server) queueReplace(req QueueRequest) (QueueResult, error) {
	return s.withExtraUsage(s.queueReplaceItems(req))
}

func (s *Server) queueReplaceItems(req QueueRequest) (QueueResult, error) {
	items := make([]QueueItem, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		addr, bad := canonical(it.Address)
		if bad != nil {
			return *bad, nil
		}
		agent := ralphloop.AgentKind(it.Agent)
		if it.Agent == "" {
			agent = ralphloop.AgentClaude
		}
		if err := ralphloop.ValidateAgentKind(agent); err != nil {
			return refusal(ReasonInvalidAgent, err.Error()), nil
		}
		if projectOfAddress(addr) != req.Project {
			return refusal(ReasonInvalidAddress, addr+" is not in project "+req.Project), nil
		}
		if seen[addr] {
			return refusal(ReasonAlreadyQueued, addr+" is listed twice"), nil
		}
		seen[addr] = true
		items = append(items, QueueItem{Address: addr, Agent: string(agent)})
	}
	return s.writeQueue("", func(cur []QueueItem) ([]QueueItem, *QueueResult) {
		for _, it := range items {
			if !s.hasTicket(it.Address) {
				r := refusal(ReasonUnknownTicket, "no ticket "+it.Address)
				return nil, &r
			}
			if r := s.mapEpicRefusal(it.Address); r != nil {
				return nil, r
			}
		}
		next := make([]QueueItem, 0, len(cur)+len(items))
		for _, it := range cur {
			if projectOfAddress(it.Address) != req.Project {
				next = append(next, it)
				continue
			}
			// A dropped entry must not have a live run under it.
			if !seen[it.Address] {
				if live := s.liveIn(s.subtree(it.Address)); live != "" {
					r := refusal(ReasonTicketLive, live+" has a live iteration")
					return nil, &r
				}
			}
		}
		return append(next, items...), nil
	})
}

func projectOfAddress(addr string) string {
	project, _, _ := strings.Cut(addr, ":")
	return project
}

func indexOfAddr(items []QueueItem, addr string) int {
	for i, it := range items {
		if it.Address == addr {
			return i
		}
	}
	return -1
}

// subtree is addr plus every ticket forked from it, directly or not.
func (s *Server) subtree(addr string) map[string]bool {
	all := s.idx.snapshot().Tickets
	in := map[string]bool{addr: true}
	for grew := true; grew; {
		grew = false
		for _, t := range all {
			if t.Parent != "" && in[t.Parent] && !in[t.Address] {
				in[t.Address] = true
				grew = true
			}
		}
	}
	return in
}

// liveIn returns the address of a live run inside set, or "".
func (s *Server) liveIn(set map[string]bool) string {
	for _, r := range s.Runs() {
		if set[r.Address] {
			return r.Address
		}
	}
	return ""
}

func dropAddresses(items []QueueItem, drop map[string]bool) []QueueItem {
	out := items[:0]
	for _, it := range items {
		if !drop[it.Address] {
			out = append(out, it)
		}
	}
	return out
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

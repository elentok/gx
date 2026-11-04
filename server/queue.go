package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const queueFileName = "queue.json"

func refusal(reason, msg string) wire.QueueResult {
	return wire.QueueResult{Refused: true, Reason: reason, Message: msg}
}

// queueCtx is what the server knows about a queue write that the body does not
// carry. It travels beside the wire.QueueRequest, never inside it.
type queueCtx struct {
	// actor is set only by the server's own recovery calls, never decoded from a
	// client: a pause made by recovery must not trigger recovery again.
	actor string
	// investigator is the claimed investigate ticket whose agent made the call,
	// from InvestigateHeader. Verbs that honour it verify the ticket.
	investigator string
}

// ignoreCtx adapts a queue write that needs no queueCtx.
func ignoreCtx(do func(wire.QueueRequest) (wire.QueueResult, error)) func(wire.QueueRequest, queueCtx) (wire.QueueResult, error) {
	return func(req wire.QueueRequest, _ queueCtx) (wire.QueueResult, error) { return do(req) }
}

// queueStore is the server-wide queue. It lives in a state-dir file, never in a
// ticket's markdown, so order survives a restart without touching the store.
type queueStore struct {
	mu    sync.Mutex
	path  string
	items []wire.QueueItem
}

func openQueue(stateDir string) (*queueStore, error) {
	q := &queueStore{path: filepath.Join(stateDir, queueFileName), items: []wire.QueueItem{}}
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

func (q *queueStore) list() []wire.QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]wire.QueueItem{}, q.items...)
}

// writeQueue applies mutate under the lock, persists, then publishes, so events
// arrive in queue order. A refusal from mutate leaves the queue untouched.
func (s *Server) writeQueue(address string, mutate func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult)) (wire.QueueResult, error) {
	q := s.queued
	q.mu.Lock()
	defer q.mu.Unlock()
	next, refused := mutate(append([]wire.QueueItem{}, q.items...))
	if refused != nil {
		return *refused, nil
	}
	prev := q.items
	q.items = next
	if err := q.save(); err != nil {
		q.items = prev
		return wire.QueueResult{}, err
	}
	s.events.publish(wire.EventQueueChanged, address)
	s.kickRunner()
	return wire.QueueResult{Queue: append([]wire.QueueItem{}, next...)}, nil
}

// mapEpicRefusal refuses to queue a ticket of a map epic: a map's tickets are
// decisions a person resolves by hand, never scheduled work.
func (s *Server) mapEpicRefusal(address string) *wire.QueueResult {
	if !s.inMapEpic(address) {
		return nil
	}
	r := refusal(wire.ReasonMapEpic, address+" is in a map epic; its tickets are resolved by hand, not scheduled")
	return &r
}

// inMapEpic reports whether address names a ticket (or root) of a map epic.
func (s *Server) inMapEpic(address string) bool {
	a, err := parseQueuedAddress(address)
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
func canonical(raw string) (string, *wire.QueueResult) {
	a, err := tickets.ParseAddress(raw, tickets.AddressContext{})
	if err != nil {
		r := refusal(wire.ReasonInvalidAddress, err.Error())
		return "", &r
	}
	return a.String(), nil
}

// withExtraUsage runs the extra-usage check for an enqueue and puts the warning
// on an accepted result. Extra usage is a Claude account matter, so the check
// runs only when an enqueued agent runs on Claude. An empty agent follows the
// project's Default harness.
func (s *Server) withExtraUsage(project string, agents []string, res wire.QueueResult, err error) (wire.QueueResult, error) {
	if err != nil || res.Refused {
		return res, err
	}
	for _, agent := range agents {
		if agent == "" && s.defaultHarness(project) == ralphloop.AgentClaude || agent == string(ralphloop.AgentClaude) {
			res.ExtraUsage = s.checkExtraUsage(time.Now())
			break
		}
	}
	return res, err
}

func (s *Server) queueAdd(req wire.QueueRequest) (wire.QueueResult, error) {
	res, err := s.queueAddItem(req)
	return s.withExtraUsage(projectOfAddress(req.Address), []string{req.Agent}, res, err)
}

// resolveAgent keeps an empty agent empty (it follows the Default harness) and
// refuses an unknown one.
func resolveAgent(name string) (ralphloop.AgentKind, *wire.QueueResult) {
	agent := ralphloop.AgentKind(name)
	if name == "" {
		return "", nil
	}
	if err := ralphloop.ValidateAgentKind(agent); err != nil {
		r := refusal(wire.ReasonInvalidAgent, err.Error())
		return "", &r
	}
	return agent, nil
}

// queueTarget is what a queue request names: one ticket, or a whole epic.
type queueTarget struct {
	address string // canonical form, the queue key
	project string
	epic    string // set only for an epic target
}

func (t queueTarget) isEpic() bool { return t.epic != "" }

// parseQueueTarget parses a request address once, or returns the refusal.
func parseQueueTarget(raw string) (queueTarget, *wire.QueueResult) {
	if project, epic, ok := tickets.ParseEpicAddress(raw); ok {
		return queueTarget{address: raw, project: project, epic: epic}, nil
	}
	addr, bad := canonical(raw)
	if bad != nil {
		return queueTarget{}, bad
	}
	return queueTarget{address: addr, project: projectOfAddress(addr)}, nil
}

// refusal returns why t cannot be queued, or nil.
func (s *Server) refusal(t queueTarget) *wire.QueueResult {
	if !t.isEpic() {
		if !s.hasTicket(t.address) {
			r := refusal(wire.ReasonUnknownTicket, "no ticket "+t.address)
			return &r
		}
		return s.mapEpicRefusal(t.address)
	}
	epics, err := s.projectEpics(t.project)
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == t.epic })
	if err != nil || i < 0 {
		r := refusal(wire.ReasonUnknownTicket, "no epic "+t.address)
		return &r
	}
	e := epics[i]
	var r wire.QueueResult
	switch {
	case e.IsMap:
		r = refusal(wire.ReasonMapEpic, t.address+" is a map epic; its tickets are resolved by hand, not scheduled")
	case !e.MergedAt.IsZero():
		r = refusal(wire.ReasonAlreadyMerged, t.address+" is already merged")
	case e.QueueRefusal() == tickets.QueueRefusalFollowUps:
		r = refusal(wire.ReasonFollowUpsEpic, t.address+" is a follow-ups epic; a follow-up is not schedulable work")
	case e.Status == string(schema.StatusDraft):
		r = refusal(wire.ReasonEpicDraft, t.address+" is a draft; its tickets are still being created")
	case e.QueueRefusal() == tickets.QueueRefusalNotOpen:
		r = refusal(wire.ReasonEpicNotOpen, t.address+" is "+e.Status+", not open")
	case !epicQueueable(e, false):
		r = refusal(wire.ReasonNoOpenTickets, t.address+" has no open ticket")
	default:
		return nil
	}
	return &r
}

func (s *Server) queueAddItem(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	agent, badAgent := resolveAgent(req.Agent)
	if badAgent != nil {
		return *badAgent, nil
	}
	res, err := s.writeQueue(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		if r := s.refusal(target); r != nil {
			return nil, r
		}
		for _, it := range items {
			if it.Address == addr {
				r := refusal(wire.ReasonAlreadyQueued, addr+" is already queued")
				return nil, &r
			}
		}
		item := newQueueItem(addr, agent)
		if req.Front {
			return append([]wire.QueueItem{item}, items...), nil
		}
		return append(items, item), nil
	})
	if err == nil && !res.Refused {
		s.stampQueuedAt(target)
	}
	return res, err
}

// stampQueuedAt records the epic's first enqueue. StampEpicFields never
// overwrites, so a later remove and add keeps the first time. A failed stamp is
// logged, never fatal: the item is already queued.
func (s *Server) stampQueuedAt(t queueTarget) {
	epic := t.epic
	if !t.isEpic() {
		a, err := parseQueuedAddress(t.address)
		if err != nil {
			return
		}
		epic = a.Epic
	}
	dir, err := s.projectDir(t.project)
	if err != nil {
		return
	}
	s.stampEpic(rootRef{Project: t.project, Epic: epic}, filepath.Join(dir, epic), "queued_at")
}

func (s *Server) queueRemove(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	return s.writeQueue(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		if indexOfAddr(items, addr) < 0 {
			r := refusal(wire.ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		subtree := s.subtree(addr)
		if live := s.liveIn(subtree); live != "" {
			r := refusal(wire.ReasonTicketLive, live+" has a live iteration")
			return nil, &r
		}
		return dropAddresses(items, subtree), nil
	})
}

// queueReplace swaps one project's pending entries for req.Items, leaving the
// other projects' entries where they are. The new entries go at the end.
func (s *Server) queueReplace(req wire.QueueRequest) (wire.QueueResult, error) {
	res, err := s.queueReplaceItems(req)
	agents := make([]string, len(req.Items))
	for i, it := range req.Items {
		agents[i] = it.Agent
	}
	return s.withExtraUsage(req.Project, agents, res, err)
}

func (s *Server) queueReplaceItems(req wire.QueueRequest) (wire.QueueResult, error) {
	items := make([]wire.QueueItem, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		addr, bad := canonical(it.Address)
		if bad != nil {
			return *bad, nil
		}
		agent, badAgent := resolveAgent(it.Agent)
		if badAgent != nil {
			return *badAgent, nil
		}
		if projectOfAddress(addr) != req.Project {
			return refusal(wire.ReasonInvalidAddress, addr+" is not in project "+req.Project), nil
		}
		if seen[addr] {
			return refusal(wire.ReasonAlreadyQueued, addr+" is listed twice"), nil
		}
		seen[addr] = true
		items = append(items, newQueueItem(addr, agent))
	}
	return s.writeQueue("", func(cur []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		for _, it := range items {
			if !s.hasTicket(it.Address) {
				r := refusal(wire.ReasonUnknownTicket, "no ticket "+it.Address)
				return nil, &r
			}
			if r := s.mapEpicRefusal(it.Address); r != nil {
				return nil, r
			}
		}
		next := make([]wire.QueueItem, 0, len(cur)+len(items))
		for _, it := range cur {
			if projectOfAddress(it.Address) != req.Project {
				next = append(next, it)
				continue
			}
			// A dropped entry must not have a live run under it.
			if !seen[it.Address] {
				if live := s.liveIn(s.subtree(it.Address)); live != "" {
					r := refusal(wire.ReasonTicketLive, live+" has a live iteration")
					return nil, &r
				}
			}
		}
		// An address that was already queued keeps its time. items is copied, not
		// mutated, so a rerun of this callback starts from the same stamps.
		for _, it := range items {
			if j := indexOfAddr(cur, it.Address); j >= 0 {
				it.EnqueuedAt = cur[j].EnqueuedAt
			}
			next = append(next, it)
		}
		return next, nil
	})
}

// newQueueItem is the one place an enqueue time is stamped.
func newQueueItem(addr string, agent ralphloop.AgentKind) wire.QueueItem {
	return wire.QueueItem{Address: addr, Agent: string(agent), EnqueuedAt: time.Now()}
}

func projectOfAddress(addr string) string {
	project, _, _ := strings.Cut(addr, ":")
	return project
}

func indexOfAddr(items []wire.QueueItem, addr string) int {
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

func dropAddresses(items []wire.QueueItem, drop map[string]bool) []wire.QueueItem {
	out := items[:0]
	for _, it := range items {
		if !drop[it.Address] {
			out = append(out, it)
		}
	}
	return out
}

func (s *Server) queueMove(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	return s.writeQueue(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		from := -1
		for i, it := range items {
			if it.Address == addr {
				from = i
			}
		}
		if from < 0 {
			r := refusal(wire.ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		if req.Position < 1 || req.Position > len(items) {
			r := refusal(wire.ReasonBadPosition, "position must be between 1 and the queue length")
			return nil, &r
		}
		it := items[from]
		items = append(items[:from], items[from+1:]...)
		to := req.Position - 1
		items = append(items[:to], append([]wire.QueueItem{it}, items[to:]...)...)
		return items, nil
	})
}

func (s *Server) queueItems(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.queued.list())
}

// queueWrite adapts a queue write to a POST handler. Refusals are results, so
// they answer 200; only a failure to persist is an HTTP error.
func (s *Server) queueWrite(do func(wire.QueueRequest) (wire.QueueResult, error)) http.HandlerFunc {
	return queueWriteAs(false, ignoreCtx(do))
}

// queueWriteCtx is queueWrite for a write that reads the queueCtx.
func (s *Server) queueWriteCtx(do func(wire.QueueRequest, queueCtx) (wire.QueueResult, error)) http.HandlerFunc {
	return queueWriteAs(false, do)
}

// queueWriteWeb is queueWrite for the web mux: a request without a state token
// is refused as stale before anything runs.
func (s *Server) queueWriteWeb(do func(wire.QueueRequest) (wire.QueueResult, error)) http.HandlerFunc {
	return queueWriteAs(true, ignoreCtx(do))
}

func queueWriteAs(web bool, do func(wire.QueueRequest, queueCtx) (wire.QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.QueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if web && req.State == "" {
			writeJSON(w, refusal(wire.ReasonStale, "a state token is required; review the current question"))
			return
		}
		req.Source = requestOrigin(r, web)
		var qc queueCtx
		if !web {
			qc.investigator = r.Header.Get(wire.InvestigateHeader)
		}
		res, err := do(req, qc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

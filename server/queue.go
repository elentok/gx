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
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// EventQueueChanged is emitted once per queue write; Address is the ticket it
// touched. Clients re-read the queue on it.
const EventQueueChanged = "queue-changed"

const queueFileName = "queue.json"

// Refusal reasons of the queue writes. Stable: clients switch on them.
const (
	// ReasonServerNotRunning is produced by the CLI, never the server.
	ReasonServerNotRunning = "server-not-running"
	ReasonUnknownTicket    = "unknown-ticket"
	ReasonInvestigateStop  = "investigate-stop"
	ReasonInvalidAddress   = "invalid-address"
	ReasonInvalidAgent     = "invalid-agent"
	ReasonAlreadyQueued    = "already-queued"
	ReasonNotQueued        = "not-queued"
	ReasonBadPosition      = "bad-position"
	ReasonTicketLive       = "ticket-live"
	ReasonMapEpic          = "map-epic"
	ReasonNoOpenTickets    = "no-open-tickets"
	ReasonEpicDraft        = "epic-draft"
	ReasonEpicNotOpen      = "epic-not-open"
	ReasonFollowUpsEpic    = "follow-ups-epic"
	ReasonAlreadyMerged    = "already-merged"
	ReasonMergePending     = "merge-pending"
	ReasonNotComplete      = "not-complete"
	ReasonBranchMissing    = "branch-missing"
	ReasonOneOffsEpic      = "one-offs-epic"
	ReasonNotDirtyTrunk    = "not-dirty-trunk"
	ReasonNeedsRebase      = "needs-rebase"
)

// QueueItem is one queued ticket and the agent it will run under.
type QueueItem struct {
	Address string `json:"address"`
	Agent   string `json:"agent"`
	// EnqueuedAt is set by the server on enqueue, never taken from a client. Items
	// from a queue file written before the field existed have none.
	EnqueuedAt time.Time `json:"enqueued_at,omitzero"`
}

// QueueResult is what every queue write returns: the queue after the write, or
// a refusal (the queue is then untouched).
type QueueResult struct {
	Queue []QueueItem `json:"queue,omitempty"`
	// Mode is set by pause, resume and flush: running or paused.
	Mode QueueMode `json:"mode,omitempty"`
	// ExtraUsage is the warning an enqueue carries while the account
	// auto-purchases extra usage.
	ExtraUsage bool `json:"extra_usage,omitempty"`
	// Merged, MergeTicket and Paused are an epic merge's outcome: a direct
	// merge, the address of the merge ticket it forked, or a pause of the root
	// with PauseKind and Message.
	Merged      bool   `json:"merged,omitempty"`
	MergeTicket string `json:"merge_ticket,omitempty"`
	// TrunkConflict is the address of the trunk-conflict ticket "Stash, merge and
	// restore" added.
	TrunkConflict string `json:"trunk_conflict,omitempty"`
	Paused        bool   `json:"parked,omitempty"`
	PauseKind     string `json:"park_kind,omitempty"`
	// Outcome is an answer's result: delivered or adopted. A stale refusal
	// carries the current Question and its State token.
	Outcome  string            `json:"outcome,omitempty"`
	Question *tickets.LogEvent `json:"question,omitempty"`
	State    string            `json:"state,omitempty"`
	Refused  bool              `json:"refused,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Message  string            `json:"message,omitempty"`
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
	// PauseReason is the one-line reason a pause writes into the ticket.
	PauseReason string `json:"park_reason,omitempty"`
	// Stop lets a cancel stop the live pane of a claimed ticket instead of refusing.
	Stop bool `json:"stop,omitempty"`
	// Front makes an add merge at the head of the queue instead of the tail.
	Front bool `json:"front,omitempty"`
	// Text is what a nudge types into the live pane.
	Text string `json:"text,omitempty"`
	// Verb is the action a recommend stores; Text carries its diagnosis.
	Verb string `json:"verb,omitempty"`
	// Decision is allow or deny, for answering a blocked permission request.
	Decision string `json:"decision,omitempty"`
	// RequestID names the permission request a decision answers; empty answers
	// whatever is open (the TUI and CLI).
	RequestID string `json:"request_id,omitempty"`
	// Choice, Note, State and Source are an answer's choice (an option id, yes or
	// no), its note, the state token of the question it answers and where it
	// came from. The server sets Source from the request's origin and ignores
	// the body. A free-text answer uses Text.
	Choice string `json:"choice,omitempty"`
	Note   string `json:"note,omitempty"`
	State  string `json:"state,omitempty"`
	Source string `json:"source,omitempty"`

	// actor is set only by the server's own recovery calls, never decoded from a
	// client: a pause made by recovery must not trigger recovery again.
	actor string
	// investigator is the claimed investigate ticket whose agent made the call,
	// from InvestigateHeader. Verbs that honour it verify the ticket.
	investigator string
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
func canonical(raw string) (string, *QueueResult) {
	a, err := tickets.ParseAddress(raw, tickets.AddressContext{})
	if err != nil {
		r := refusal(ReasonInvalidAddress, err.Error())
		return "", &r
	}
	return a.String(), nil
}

// withExtraUsage runs the extra-usage check for an enqueue and puts the warning
// on an accepted result. Extra usage is a Claude account matter, so the check
// runs only when an enqueued agent runs on Claude. An empty agent follows the
// project's Default harness.
func (s *Server) withExtraUsage(project string, agents []string, res QueueResult, err error) (QueueResult, error) {
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

func (s *Server) queueAdd(req QueueRequest) (QueueResult, error) {
	res, err := s.queueAddItem(req)
	return s.withExtraUsage(projectOfAddress(req.Address), []string{req.Agent}, res, err)
}

// resolveAgent keeps an empty agent empty (it follows the Default harness) and
// refuses an unknown one.
func resolveAgent(name string) (ralphloop.AgentKind, *QueueResult) {
	agent := ralphloop.AgentKind(name)
	if name == "" {
		return "", nil
	}
	if err := ralphloop.ValidateAgentKind(agent); err != nil {
		r := refusal(ReasonInvalidAgent, err.Error())
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
func parseQueueTarget(raw string) (queueTarget, *QueueResult) {
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
func (s *Server) refusal(t queueTarget) *QueueResult {
	if !t.isEpic() {
		if !s.hasTicket(t.address) {
			r := refusal(ReasonUnknownTicket, "no ticket "+t.address)
			return &r
		}
		return s.mapEpicRefusal(t.address)
	}
	epics, err := s.projectEpics(t.project)
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == t.epic })
	if err != nil || i < 0 {
		r := refusal(ReasonUnknownTicket, "no epic "+t.address)
		return &r
	}
	e := epics[i]
	var r QueueResult
	switch {
	case e.IsMap:
		r = refusal(ReasonMapEpic, t.address+" is a map epic; its tickets are resolved by hand, not scheduled")
	case !e.MergedAt.IsZero():
		r = refusal(ReasonAlreadyMerged, t.address+" is already merged")
	case e.QueueRefusal() == tickets.QueueRefusalFollowUps:
		r = refusal(ReasonFollowUpsEpic, t.address+" is a follow-ups epic; a follow-up is not schedulable work")
	case e.Status == string(schema.StatusDraft):
		r = refusal(ReasonEpicDraft, t.address+" is a draft; its tickets are still being created")
	case e.QueueRefusal() == tickets.QueueRefusalNotOpen:
		r = refusal(ReasonEpicNotOpen, t.address+" is "+e.Status+", not open")
	case !epicQueueable(e, false):
		r = refusal(ReasonNoOpenTickets, t.address+" has no open ticket")
	default:
		return nil
	}
	return &r
}

func (s *Server) queueAddItem(req QueueRequest) (QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	agent, badAgent := resolveAgent(req.Agent)
	if badAgent != nil {
		return *badAgent, nil
	}
	res, err := s.writeQueue(addr, func(items []QueueItem) ([]QueueItem, *QueueResult) {
		if r := s.refusal(target); r != nil {
			return nil, r
		}
		for _, it := range items {
			if it.Address == addr {
				r := refusal(ReasonAlreadyQueued, addr+" is already queued")
				return nil, &r
			}
		}
		item := newQueueItem(addr, agent)
		if req.Front {
			return append([]QueueItem{item}, items...), nil
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

func (s *Server) queueRemove(req QueueRequest) (QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
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
	res, err := s.queueReplaceItems(req)
	agents := make([]string, len(req.Items))
	for i, it := range req.Items {
		agents[i] = it.Agent
	}
	return s.withExtraUsage(req.Project, agents, res, err)
}

func (s *Server) queueReplaceItems(req QueueRequest) (QueueResult, error) {
	items := make([]QueueItem, 0, len(req.Items))
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
			return refusal(ReasonInvalidAddress, addr+" is not in project "+req.Project), nil
		}
		if seen[addr] {
			return refusal(ReasonAlreadyQueued, addr+" is listed twice"), nil
		}
		seen[addr] = true
		items = append(items, newQueueItem(addr, agent))
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
func newQueueItem(addr string, agent ralphloop.AgentKind) QueueItem {
	return QueueItem{Address: addr, Agent: string(agent), EnqueuedAt: time.Now()}
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
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
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
	return queueWriteAs(false, do)
}

// queueWriteWeb is queueWrite for the web mux: a request without a state token
// is refused as stale before anything runs.
func (s *Server) queueWriteWeb(do func(QueueRequest) (QueueResult, error)) http.HandlerFunc {
	return queueWriteAs(true, do)
}

func queueWriteAs(web bool, do func(QueueRequest) (QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req QueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if web && req.State == "" {
			writeJSON(w, refusal(ReasonStale, "a state token is required; review the current question"))
			return
		}
		req.Source = requestOrigin(r, web)
		if !web {
			req.investigator = r.Header.Get(InvestigateHeader)
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	}
}

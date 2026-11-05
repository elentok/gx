package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/index"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const queueFileName = projects.QueueFileName

// Refusal is a queue write the server declines: a result, not an error.
func Refusal(reason, msg string) wire.QueueResult {
	return wire.QueueResult{Refused: true, Reason: reason, Message: msg}
}

// Ctx is what the server knows about a queue write that the body does not
// carry. It travels beside the wire.QueueRequest, never inside it.
type Ctx struct {
	// Actor is set only by the server's own recovery calls, never decoded from a
	// client: a pause made by recovery must not trigger recovery again.
	Actor string
	// Investigator is the claimed investigate ticket whose agent made the call,
	// from InvestigateHeader. Verbs that honour it verify the ticket.
	Investigator string
}

// ignoreCtx adapts a queue write that needs no Ctx.
func ignoreCtx(do func(wire.QueueRequest) (wire.QueueResult, error)) func(wire.QueueRequest, Ctx) (wire.QueueResult, error) {
	return func(req wire.QueueRequest, _ Ctx) (wire.QueueResult, error) { return do(req) }
}

// Host is what the queue, the Queue agent rows and the one-offs need from the
// rest of the server: the back edges to harness, sched, sessions, budget, merge
// and recovery. The root implements it; tests use a fake.
type Host interface {
	DefaultHarness(project string) agent.AgentKind
	RunnerForRun(run runs.Run) agentrunner.Runner
	Runs() []runs.Run
	CheckWaitingLabel(address string) string
	ListSessions() ([]wire.SessionInfo, error)
	CheckExtraUsage(now time.Time) bool
	StampEpic(root RootRef, epicPath string, keys ...string)
	EnsureDraftEpic(epicPath string) error
}

// QueueDeps is what a Queue needs beside its state files, handed over once the
// rest of the server exists.
type QueueDeps struct {
	Events   *broker.Broker
	Guards   *runs.Guards
	Index    *index.Index
	Projects *projects.Registry
	Registry *runs.Registry
	Host     Host
}

// Queue is the server-wide queue and the pause gate in front of it. It lives in
// state-dir files, never in a ticket's markdown, so order survives a restart
// without touching the store.
type Queue struct {
	mu       sync.Mutex
	path     string
	items    []wire.QueueItem
	pause    *pauseState
	stateDir string

	events   *broker.Broker
	guards   *runs.Guards
	idx      *index.Index
	projects *projects.Registry
	registry *runs.Registry
	host     Host
}

// Attach hands the queue the parts of the server that only exist after it is
// loaded.
func (q *Queue) Attach(d QueueDeps) {
	q.events, q.guards, q.idx, q.projects, q.registry, q.host = d.Events, d.Guards, d.Index, d.Projects, d.Registry, d.Host
}

// EpicQueueable reports whether an epic can be queued: an unqueued, unmerged
// loop epic that still has an open ticket. The epic list shares this rule.
func EpicQueueable(e tickets.Epic, queued bool) bool {
	if e.IsMap || queued || !e.MergedAt.IsZero() || e.QueueRefusal() != tickets.QueueRefusalNone {
		return false
	}
	return slices.ContainsFunc(e.Tickets, func(t tickets.Ticket) bool { return t.Status == string(schema.StatusOpen) })
}

// OpenQueue loads the queue and its pause flag from stateDir.
func OpenQueue(stateDir string) (*Queue, error) {
	q, err := openItems(stateDir)
	if err != nil {
		return nil, err
	}
	if q.pause, err = openPause(stateDir); err != nil {
		return nil, fmt.Errorf("load queue pause: %w", err)
	}
	return q, nil
}

func openItems(stateDir string) (*Queue, error) {
	q := &Queue{path: filepath.Join(stateDir, queueFileName), items: []wire.QueueItem{}, stateDir: stateDir}
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
func (q *Queue) save() error {
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

// List is a copy of the queued items, in order.
func (q *Queue) List() []wire.QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]wire.QueueItem{}, q.items...)
}

// Write applies mutate under the lock, persists, then publishes, so events
// arrive in queue order. A refusal from mutate leaves the queue untouched.
func (q *Queue) Write(address string, mutate func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult)) (wire.QueueResult, error) {
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
	q.events.Publish(wire.EventQueueChanged, address)
	q.guards.Kick()
	return wire.QueueResult{Queue: append([]wire.QueueItem{}, next...)}, nil
}

// mapEpicRefusal refuses to queue a ticket of a map epic: a map's tickets are
// decisions a person resolves by hand, never scheduled work.
func (q *Queue) mapEpicRefusal(address string) *wire.QueueResult {
	if !q.inMapEpic(address) {
		return nil
	}
	r := Refusal(wire.ReasonMapEpic, address+" is in a map epic; its tickets are resolved by hand, not scheduled")
	return &r
}

// inMapEpic reports whether address names a ticket (or root) of a map epic.
func (q *Queue) inMapEpic(address string) bool {
	a, err := tickets.ParseQueuedAddress(address)
	if err != nil {
		return false
	}
	dir, err := q.projects.Dir(a.Project)
	if err != nil {
		return false
	}
	return tickets.IsMapEpic(filepath.Join(dir, a.Epic))
}

func (q *Queue) hasTicket(address string) bool {
	for _, t := range q.idx.Snapshot().Tickets {
		if t.Address == address {
			return true
		}
	}
	return false
}

// Canonical normalises a request's address, or returns the refusal for it.
func Canonical(raw string) (string, *wire.QueueResult) {
	a, err := tickets.ParseAddress(raw, tickets.AddressContext{})
	if err != nil {
		r := Refusal(wire.ReasonInvalidAddress, err.Error())
		return "", &r
	}
	return a.String(), nil
}

// WithExtraUsage runs the extra-usage check for an enqueue and puts the warning
// on an accepted result. Extra usage is a Claude account matter, so the check
// runs only when an enqueued agent runs on Claude. An empty agent follows the
// project's Default harness.
func (q *Queue) WithExtraUsage(project string, agents []string, res wire.QueueResult, err error) (wire.QueueResult, error) {
	if err != nil || res.Refused {
		return res, err
	}
	for _, agentKind := range agents {
		if agentKind == "" && q.host.DefaultHarness(project) == agent.AgentClaude || agentKind == string(agent.AgentClaude) {
			res.ExtraUsage = q.host.CheckExtraUsage(time.Now())
			break
		}
	}
	return res, err
}

// Add queues req.Address (a ticket, or a whole epic).
func (q *Queue) Add(req wire.QueueRequest) (wire.QueueResult, error) {
	res, err := q.addItem(req)
	return q.WithExtraUsage(projectOfAddress(req.Address), []string{req.Agent}, res, err)
}

// resolveAgent keeps an empty agent empty (it follows the Default harness) and
// refuses an unknown one.
func resolveAgent(name string) (agent.AgentKind, *wire.QueueResult) {
	agentKind := agent.AgentKind(name)
	if name == "" {
		return "", nil
	}
	if err := agent.ValidateAgentKind(agentKind); err != nil {
		r := Refusal(wire.ReasonInvalidAgent, err.Error())
		return "", &r
	}
	return agentKind, nil
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
	addr, bad := Canonical(raw)
	if bad != nil {
		return queueTarget{}, bad
	}
	return queueTarget{address: addr, project: projectOfAddress(addr)}, nil
}

// refusal returns why t cannot be queued, or nil.
func (q *Queue) refusal(t queueTarget) *wire.QueueResult {
	if !t.isEpic() {
		if !q.hasTicket(t.address) {
			r := Refusal(wire.ReasonUnknownTicket, "no ticket "+t.address)
			return &r
		}
		return q.mapEpicRefusal(t.address)
	}
	epics, err := q.projects.Epics(t.project)
	i := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == t.epic })
	if err != nil || i < 0 {
		r := Refusal(wire.ReasonUnknownTicket, "no epic "+t.address)
		return &r
	}
	e := epics[i]
	var r wire.QueueResult
	switch {
	case e.IsMap:
		r = Refusal(wire.ReasonMapEpic, t.address+" is a map epic; its tickets are resolved by hand, not scheduled")
	case !e.MergedAt.IsZero():
		r = Refusal(wire.ReasonAlreadyMerged, t.address+" is already merged")
	case e.QueueRefusal() == tickets.QueueRefusalFollowUps:
		r = Refusal(wire.ReasonFollowUpsEpic, t.address+" is a follow-ups epic; a follow-up is not schedulable work")
	case e.Status == string(schema.StatusDraft):
		r = Refusal(wire.ReasonEpicDraft, t.address+" is a draft; its tickets are still being created")
	case e.QueueRefusal() == tickets.QueueRefusalNotOpen:
		r = Refusal(wire.ReasonEpicNotOpen, t.address+" is "+e.Status+", not open")
	case !EpicQueueable(e, false):
		r = Refusal(wire.ReasonNoOpenTickets, t.address+" has no open ticket")
	default:
		return nil
	}
	return &r
}

func (q *Queue) addItem(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	agentKind, badAgent := resolveAgent(req.Agent)
	if badAgent != nil {
		return *badAgent, nil
	}
	res, err := q.Write(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		if r := q.refusal(target); r != nil {
			return nil, r
		}
		for _, it := range items {
			if it.Address == addr {
				r := Refusal(wire.ReasonAlreadyQueued, addr+" is already queued")
				return nil, &r
			}
		}
		item := NewItem(addr, agentKind)
		if req.Front {
			return append([]wire.QueueItem{item}, items...), nil
		}
		return append(items, item), nil
	})
	if err == nil && !res.Refused {
		q.stampQueuedAt(target)
	}
	return res, err
}

// stampQueuedAt records the epic's first enqueue. StampEpicFields never
// overwrites, so a later remove and add keeps the first time. A failed stamp is
// logged, never fatal: the item is already queued.
func (q *Queue) stampQueuedAt(t queueTarget) {
	epic := t.epic
	if !t.isEpic() {
		a, err := tickets.ParseQueuedAddress(t.address)
		if err != nil {
			return
		}
		epic = a.Epic
	}
	dir, err := q.projects.Dir(t.project)
	if err != nil {
		return
	}
	q.host.StampEpic(RootRef{Project: t.project, Epic: epic}, filepath.Join(dir, epic), "queued_at")
}

// Remove takes req.Address and everything forked from it off the queue.
func (q *Queue) Remove(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	return q.Write(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		if indexOfAddr(items, addr) < 0 {
			r := Refusal(wire.ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		subtree := q.subtree(addr)
		if live := q.liveIn(subtree); live != "" {
			r := Refusal(wire.ReasonTicketLive, live+" has a live iteration")
			return nil, &r
		}
		return dropAddresses(items, subtree), nil
	})
}

// Replace swaps one project's pending entries for req.Items, leaving the
// other projects' entries where they are. The new entries go at the end.
func (q *Queue) Replace(req wire.QueueRequest) (wire.QueueResult, error) {
	res, err := q.replaceItems(req)
	agents := make([]string, len(req.Items))
	for i, it := range req.Items {
		agents[i] = it.Agent
	}
	return q.WithExtraUsage(req.Project, agents, res, err)
}

func (q *Queue) replaceItems(req wire.QueueRequest) (wire.QueueResult, error) {
	items := make([]wire.QueueItem, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		addr, bad := Canonical(it.Address)
		if bad != nil {
			return *bad, nil
		}
		agentKind, badAgent := resolveAgent(it.Agent)
		if badAgent != nil {
			return *badAgent, nil
		}
		if projectOfAddress(addr) != req.Project {
			return Refusal(wire.ReasonInvalidAddress, addr+" is not in project "+req.Project), nil
		}
		if seen[addr] {
			return Refusal(wire.ReasonAlreadyQueued, addr+" is listed twice"), nil
		}
		seen[addr] = true
		items = append(items, NewItem(addr, agentKind))
	}
	return q.Write("", func(cur []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		for _, it := range items {
			if !q.hasTicket(it.Address) {
				r := Refusal(wire.ReasonUnknownTicket, "no ticket "+it.Address)
				return nil, &r
			}
			if r := q.mapEpicRefusal(it.Address); r != nil {
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
				if live := q.liveIn(q.subtree(it.Address)); live != "" {
					r := Refusal(wire.ReasonTicketLive, live+" has a live iteration")
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

// NewItem is the one place an enqueue time is stamped.
func NewItem(addr string, agentKind agent.AgentKind) wire.QueueItem {
	return wire.QueueItem{Address: addr, Agent: string(agentKind), EnqueuedAt: time.Now()}
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
func (q *Queue) subtree(addr string) map[string]bool {
	all := q.idx.Snapshot().Tickets
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
func (q *Queue) liveIn(set map[string]bool) string {
	for _, r := range q.host.Runs() {
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

// Move puts req.Address at req.Position (1-based).
func (q *Queue) Move(req wire.QueueRequest) (wire.QueueResult, error) {
	target, bad := parseQueueTarget(req.Address)
	if bad != nil {
		return *bad, nil
	}
	addr := target.address
	return q.Write(addr, func(items []wire.QueueItem) ([]wire.QueueItem, *wire.QueueResult) {
		from := -1
		for i, it := range items {
			if it.Address == addr {
				from = i
			}
		}
		if from < 0 {
			r := Refusal(wire.ReasonNotQueued, addr+" is not queued")
			return nil, &r
		}
		if req.Position < 1 || req.Position > len(items) {
			r := Refusal(wire.ReasonBadPosition, "position must be between 1 and the queue length")
			return nil, &r
		}
		it := items[from]
		items = append(items[:from], items[from+1:]...)
		to := req.Position - 1
		items = append(items[:to], append([]wire.QueueItem{it}, items[to:]...)...)
		return items, nil
	})
}

// Handler adapts a queue write to a POST handler. Refusals are results, so
// they answer 200; only a failure to persist is an HTTP error.
func Handler(do func(wire.QueueRequest) (wire.QueueResult, error)) http.HandlerFunc {
	return HandlerAs(false, ignoreCtx(do))
}

// HandlerCtx is Handler for a write that reads the Ctx.
func HandlerCtx(do func(wire.QueueRequest, Ctx) (wire.QueueResult, error)) http.HandlerFunc {
	return HandlerAs(false, do)
}

// HandlerWeb is Handler for the web mux: a request without a state token
// is refused as stale before anything runs.
func HandlerWeb(do func(wire.QueueRequest) (wire.QueueResult, error)) http.HandlerFunc {
	return HandlerAs(true, ignoreCtx(do))
}

// HandlerAs is the shared body of Handler, HandlerCtx and HandlerWeb.
func HandlerAs(web bool, do func(wire.QueueRequest, Ctx) (wire.QueueResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.QueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if web && req.State == "" {
			wire.WriteJSON(w, Refusal(wire.ReasonStale, "a state token is required; review the current question"))
			return
		}
		req.Source = wire.RequestOrigin(r, web)
		var qc Ctx
		if !web {
			qc.Investigator = r.Header.Get(wire.InvestigateHeader)
		}
		res, err := do(req, qc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

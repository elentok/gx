package sched

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/disk"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// ExplainerDeps are the parts the explanation reads. Scheduler is the claim
// pass itself: it reads the same numbers, so a rule changed there explains the
// wait here.
type ExplainerDeps struct {
	Scheduler *Scheduler
	StateDir  string
	Settings  *settings.Holder
	Registry  *runs.Registry
	Guards    *runs.Guards
	Projects  *projects.Registry
	Harness   *harnesschoice.Harness
	Herdr     *HerdrWatch
	Limits    *limits.Limits
	Rec       *merge.Reconciler
	Disks     *disk.Disk
	Queue     *queue.Queue
	Events    *broker.Broker
}

// Explainer says why a stored ticket is or is not claimable: the scheduler's
// own verdict, then the server's queue, slot and hold state. It also builds
// the pending rows, the claim holds and the check-lock label.
type Explainer struct {
	ExplainerDeps
	verdicts verdictLog
}

// NewExplainer builds an Explainer over d.
func NewExplainer(d ExplainerDeps) *Explainer { return &Explainer{ExplainerDeps: d} }

// verdictLog remembers the verdict word last seen per queued ticket, to tell
// when one changed.
type verdictLog struct {
	mu   sync.Mutex
	seen map[string]string
}

// Handle serves GET /v1/tickets/explain.
func (x *Explainer) Handle(w http.ResponseWriter, r *http.Request) {
	addr, err := tickets.ParseAddress(r.URL.Query().Get("address"), tickets.AddressContext{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ex, err := x.explainTicket(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ex)
}

// explainTicket gives the stored ticket's verdict: the scheduler's own, then
// the server's queue and slot state for a ticket nothing else holds back.
func (x *Explainer) explainTicket(addr tickets.Address) (wire.Explanation, error) {
	epics, err := x.Projects.Epics(addr.Project)
	if err != nil {
		return wire.Explanation{}, err
	}
	return x.explainIn(epics, addr)
}

// explainIn is explainTicket's lookup inside one project's epics.
func (x *Explainer) explainIn(epics []tickets.Epic, addr tickets.Address) (wire.Explanation, error) {
	for _, e := range epics {
		if e.Name != addr.Epic {
			continue
		}
		for _, t := range e.Tickets {
			if t.DisplayNumber() == addr.ID {
				return x.verdictOf(e, t, addr), nil
			}
		}
	}
	return wire.Explanation{}, &tickets.AddressError{Code: tickets.CodeUnknownTicket, Msg: "no ticket " + addr.String()}
}

// Hold is why a ticket is held from claiming: the explain verdict and
// reason that name the wait.
type Hold struct {
	Verdict string
	Reason  string
}

// ClaimHold asks whether a ticket of agent in project is held from claiming.
// The claim pass and the schedule verdict both ask it, so a rule added here
// holds the ticket and explains the wait. The usage gate comes last, so a
// rate-limit hold's reason wins; a handoff restart (an answered ticket) is
// never gated.
func (x *Explainer) ClaimHold(project string, t tickets.Ticket, rh harnesschoice.ResolvedHarness) (Hold, bool) {
	agent := rh.Harness
	if reason := x.Harness.QueueMissingReason(project, rh); reason != "" {
		return Hold{Verdict: wire.VerdictHarnessMissing, Reason: reason}, true
	}
	if reason, held := x.Herdr.Holds(x.Harness.LaunchRunner(project, agent)); held {
		return Hold{Verdict: wire.VerdictHerdrDown, Reason: reason}, true
	}
	if limited, held := x.Registry.RateLimitHeld(string(agent)); held {
		return Hold{Verdict: RateLimitVerdict(limited.RateLimitReset)}, true
	}
	if AnsweredSession(t) == "" {
		if v, held := x.Limits.UsageGate(agent, time.Now()); held {
			return Hold{Verdict: v}, true
		}
	}
	return Hold{}, false
}

// CheckWaitingLabel is the status label of a claimed ticket whose `gx check
// --full` waits for the check lock, or "". It is a label only: no pause kind,
// no status change.
//
// Mechanism: the waiting file the check writes names the worktree it checks
// (check.Waiting.Root); the ticket's tracked run knows its worktree (Cwd), so
// the two meet on that path. check.ReadWaiting drops a waiter whose process is
// gone, so a killed waiter never leaves a label, and a run with no tracked
// ticket matches nothing.
func (x *Explainer) CheckWaitingLabel(address string) string {
	t, ok := x.Registry.Tracked(address)
	if !ok || t.Cwd == "" {
		return ""
	}
	cwd := projects.CanonicalDir(t.Cwd)
	for _, w := range check.ReadWaiting(x.StateDir) {
		if w.Root == "" || projects.CanonicalDir(w.Root) != cwd {
			continue
		}
		if w.HeldBy == "" {
			return "waiting for the check lock"
		}
		return "waiting for the check lock, held by " + w.HeldBy
	}
	return ""
}

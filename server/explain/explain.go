// Package explain says why a stored ticket is or is not claimable: the
// scheduler's own verdict, then the server's queue, slot and hold state. It
// also builds the pending rows, the claim holds and the check-lock label.
package explain

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/disk"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/limits"
	"github.com/elentok/gx/server/merge"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Host is the scheduling policy the root server owns: the claim pass reads the
// same numbers, so a rule changed there explains the wait here.
type Host interface {
	PerRootLimit(root queue.RootRef) int
	ProjectAtCap(project string) (running, limit int, atCap bool)
	ConcurrencyLimit() int
	// NextUntracked is the first frontier ticket of e with no registry entry.
	NextUntracked(project string, e tickets.Epic) (tickets.Ticket, bool)
}

// Deps are the server's parts the explanation reads.
type Deps struct {
	Host     Host
	StateDir string
	Settings *settings.Holder
	Registry *runs.Registry
	Guards   *runs.Guards
	Projects *projects.Registry
	Harness  *harnesschoice.Harness
	Herdr    *sched.HerdrWatch
	Limits   *limits.Limits
	Rec      *merge.Reconciler
	Disks    *disk.Disk
	Queue    *queue.Queue
	Events   *broker.Broker
}

// Explainer answers the explain questions.
type Explainer struct {
	Deps
	verdicts verdictLog
}

// New builds an Explainer over d.
func New(d Deps) *Explainer { return &Explainer{Deps: d} }

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

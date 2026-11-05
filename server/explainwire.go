package server

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/explain"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/tickets"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// explainHost is what the explain package asks the server for. It is its own
// type so these methods stay off Server's explain surface.
type explainHost struct{ s *Server }

func (h explainHost) PerRootLimit(root queue.RootRef) int { return h.s.scheduler.PerRootLimit(root) }
func (h explainHost) ProjectAtCap(project string) (int, int, bool) {
	return h.s.scheduler.ProjectAtCap(project)
}
func (h explainHost) ConcurrencyLimit() int { return h.s.scheduler.ConcurrencyLimit() }
func (h explainHost) NextUntracked(project string, e tickets.Epic) (tickets.Ticket, bool) {
	return h.s.scheduler.NextUntracked(project, e)
}

// claimExplainer is sched.ClaimExplainer: sched cannot import explain, which
// imports it.
type claimExplainer struct{ x *explain.Explainer }

func (c claimExplainer) ClaimHeld(project string, t tickets.Ticket, rh harnesschoice.ResolvedHarness) bool {
	_, held := c.x.ClaimHold(project, t, rh)
	return held
}

func (c claimExplainer) PublishChanges() { c.x.PublishChanges() }

func newExplainer(s *Server) *explain.Explainer {
	return explain.New(explain.Deps{
		Host: explainHost{s}, StateDir: s.cfg.StateDir, Settings: &s.settings,
		Registry: s.registry, Guards: s.guards, Projects: s.projects, Harness: s.harness,
		Herdr: s.herdr, Limits: s.limits, Rec: s.rec, Disks: s.disks, Queue: s.queue, Events: s.events,
	})
}

// CheckWaitingLabel is queue.Host's.
func (h queueHost) CheckWaitingLabel(address string) string {
	return h.explain.CheckWaitingLabel(address)
}

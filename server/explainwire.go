package server

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/explain"
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

func (h explainHost) PerRootLimit(root queue.RootRef) int { return h.s.perRootLimit(root) }
func (h explainHost) ProjectAtCap(project string) (int, int, bool) {
	return h.s.projectAtCap(project)
}
func (h explainHost) ConcurrencyLimit() int { return h.s.concurrencyLimit() }
func (h explainHost) NextUntracked(project string, e tickets.Epic) (tickets.Ticket, bool) {
	return h.s.nextUntracked(project, e)
}

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

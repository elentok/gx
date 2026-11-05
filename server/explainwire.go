package server

import (
	"encoding/json"
	"net/http"

	"github.com/elentok/gx/server/sched"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newExplainer(s *Server) *sched.Explainer {
	return sched.NewExplainer(sched.ExplainerDeps{
		Scheduler: s.scheduler, StateDir: s.cfg.StateDir, Settings: &s.settings,
		Registry: s.registry, Guards: s.guards, Projects: s.projects, Harness: s.harness,
		Herdr: s.herdr, Limits: s.limits, Rec: s.rec, Disks: s.disks, Queue: s.queue, Events: s.events,
	})
}

// CheckWaitingLabel is queue.Host's.
func (h queueHost) CheckWaitingLabel(address string) string {
	return h.explain.CheckWaitingLabel(address)
}

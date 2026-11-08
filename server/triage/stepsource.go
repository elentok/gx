package triage

import (
	"log/slog"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	triagerec "github.com/elentok/gx/triage"
)

// StopFinished ends the project's idle Triage sessions once its record is at a
// final step: the agent has nothing left to do, and the card reads "no
// session". A working session is left alone, as Plan is still printing its
// run-end line after the last save. Nothing pauses, so no pause kind is added.
func (t *Triage) StopFinished(project string) {
	if t.sessions == nil || t.manager == nil {
		return
	}
	dir, err := t.projects.Dir(project)
	if err != nil {
		return
	}
	rec, err := triagerec.NewStore(dir).LoadLive()
	if err != nil || !triagerec.Final(rec) {
		return
	}
	infos, err := t.sessions()
	if err != nil {
		return
	}
	for _, in := range infos {
		if in.Flow != string(tickets.FlowTriage) || in.FlowKey != project ||
			in.Status.State == wire.SessionExited || !in.Status.Promptable() {
			continue
		}
		if res, err := t.manager.StopSession(wire.SessionRequest{ID: in.ID}); err != nil || res.Refused {
			slog.Warn("could not stop the finished triage session", "project", project, "session", in.ID, "err", err, "refused", res.Message)
		}
	}
}

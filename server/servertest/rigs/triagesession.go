package rigs

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	triagerec "github.com/elentok/gx/triage"
)

// StartTriageSession starts an idle fake session linked to proj's Triage, whose
// live record is at step. A write step with done set has every row written.
// The auto-advance countdown runs with CountdownDelay.
func StartTriageSession(t *testing.T, step string, done bool) (*GrillingFixture, *triagerec.Store) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "follow-ups", "02", "second", servertest.TicketOpts{})
	// A second project, for a test of a session that belongs to another Triage.
	servertest.WriteTicketWith(t, store, "other", "follow-ups", "01", "first", servertest.TicketOpts{})
	rs := triagerec.NewStore(filepath.Join(store, "proj"))
	rec, err := rs.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec.Step = step
	if done {
		for i := range rec.Rows {
			rec.Rows[i].Write = triagerec.WriteDone
		}
	}
	if err := rs.Save(rec); err != nil {
		t.Fatal(err)
	}
	h, ctx, evs, id, conv, path := StartGrillingSessionIn(t, "", store, servertest.WithAutoAdvanceDelay(CountdownDelay), servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	f := &GrillingFixture{H: h, Ctx: ctx, Evs: evs, ID: id, Conv: conv, Path: path}
	servertest.WaitForHolder(t, filepath.Join(h.SessionsDir, id))
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	servertest.WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: id, Flow: string(tickets.FlowTriage), FlowKey: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("link = %+v, %v", res, err)
	}
	return f, rs
}

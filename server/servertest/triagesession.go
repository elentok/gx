package servertest

import (
	"path/filepath"
	"testing"

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
	WriteTicketWith(t, store, "proj", "follow-ups", "01", "first", TicketOpts{})
	WriteTicketWith(t, store, "proj", "follow-ups", "02", "second", TicketOpts{})
	// A second project, for a test of a session that belongs to another Triage.
	WriteTicketWith(t, store, "other", "follow-ups", "01", "first", TicketOpts{})
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
	h, ctx, evs, id, conv, path := StartGrillingSessionIn(t, "", store, WithAutoAdvanceDelay(CountdownDelay), WithEnv(agentfake.ProgramInputEnv+"=1"))
	f := &GrillingFixture{H: h, Ctx: ctx, Evs: evs, ID: id, Conv: conv, Path: path}
	WaitForHolder(t, filepath.Join(h.SessionsDir, id))
	Report(t, conv, agentfake.Control{State: "idle"})
	WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	res, err := h.Client.Sessions.Link(ctx, wire.LinkSessionRequest{ID: id, Flow: string(tickets.FlowTriage), FlowKey: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("link = %+v, %v", res, err)
	}
	return f, rs
}

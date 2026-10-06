package viewmodel_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/viewmodel"
)

func snapshot() viewmodel.State {
	return viewmodel.State{}.ApplySnapshot(server.Snapshot{
		Seq: 10,
		Tickets: []server.TicketInfo{
			{Address: "gx:e/01", Status: "open"},
			{Address: "gx:e/02", Status: "open"},
		},
	}).SetQueue([]server.QueueItem{{Address: "gx:e/01"}, {Address: "gx:e/02"}})
}

func reduce(t *testing.T, s viewmodel.State, evs ...server.Event) viewmodel.State {
	t.Helper()
	for _, ev := range evs {
		var eff viewmodel.Effect
		s, eff = s.Reduce(ev)
		if eff != viewmodel.EffectNone {
			t.Fatalf("event %+v: unexpected effect %v", ev, eff)
		}
	}
	return s
}

func TestReduce_ClaimIterationDone(t *testing.T) {
	start := snapshot()
	s := reduce(t, start,
		server.Event{Seq: 11, Type: server.EventTicketClaimed, Address: "gx:e/01"},
		server.Event{Seq: 12, Type: server.EventIterationStarted, Address: "gx:e/01"},
	)
	if s.Tickets[0].Status != "claimed" || s.Iterations["gx:e/01"] != viewmodel.IterationRunning {
		t.Fatalf("after start: %+v", s)
	}

	s = reduce(t, s, server.Event{Seq: 13, Type: server.EventTicketDone, Address: "gx:e/01"})
	if s.Tickets[0].Status != "done" || len(s.Iterations) != 0 {
		t.Fatalf("after done: %+v", s)
	}
	if len(s.Queue) != 1 || s.Queue[0] != "gx:e/02" {
		t.Fatalf("queue = %v", s.Queue)
	}
	if s.Seq != 13 {
		t.Fatalf("seq = %d", s.Seq)
	}

	if start.Tickets[0].Status != "open" || len(start.Queue) != 2 {
		t.Fatalf("reduce mutated its input: %+v", start)
	}
}

func TestReduce_SeqGapAsksForResnapshot(t *testing.T) {
	s := snapshot()
	got, eff := s.Reduce(server.Event{Seq: 12, Type: server.EventTicketClaimed, Address: "gx:e/01"})
	if eff != viewmodel.EffectResnapshot {
		t.Fatalf("effect = %v, want resnapshot", eff)
	}
	if got.Seq != 10 || got.Tickets[0].Status != "open" {
		t.Fatalf("gap event was applied: %+v", got)
	}
}

func TestReduce_DuplicateEventIgnored(t *testing.T) {
	s := snapshot()
	got, eff := s.Reduce(server.Event{Seq: 10, Type: server.EventTicketDone, Address: "gx:e/01"})
	if eff != viewmodel.EffectNone || got.Tickets[0].Status != "open" {
		t.Fatalf("duplicate applied: %+v eff=%v", got, eff)
	}
}

func TestReduce_QueueChangedRefetchesQueue(t *testing.T) {
	_, eff := snapshot().Reduce(server.Event{Seq: 11, Type: server.EventQueueChanged})
	if eff != viewmodel.EffectRefetchQueue {
		t.Fatalf("effect = %v", eff)
	}
}

func TestApplySnapshot_KeepsIterationsOnlyForClaimedTickets(t *testing.T) {
	s := reduce(t, snapshot(),
		server.Event{Seq: 11, Type: server.EventTicketClaimed, Address: "gx:e/01"},
		server.Event{Seq: 12, Type: server.EventTicketClaimed, Address: "gx:e/02"},
	)
	s = s.ApplySnapshot(server.Snapshot{Seq: 20, Tickets: []server.TicketInfo{
		{Address: "gx:e/01", Status: "claimed"},
		{Address: "gx:e/02", Status: "done"},
	}})
	if _, ok := s.Iterations["gx:e/01"]; !ok {
		t.Fatal("claimed ticket lost its iteration")
	}
	if _, ok := s.Iterations["gx:e/02"]; ok {
		t.Fatal("done ticket kept an iteration")
	}
	if s.Seq != 20 {
		t.Fatalf("seq = %d", s.Seq)
	}
}

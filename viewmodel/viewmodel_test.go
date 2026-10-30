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

// reduce applies events that must each ask for a re-snapshot, as every
// non-herdr, non-queue event does.
func reduce(t *testing.T, s viewmodel.State, evs ...server.Event) viewmodel.State {
	t.Helper()
	for _, ev := range evs {
		var eff viewmodel.Effect
		s, eff = s.Reduce(ev)
		if eff != viewmodel.EffectResnapshot {
			t.Fatalf("event %+v: effect %v, want resnapshot", ev, eff)
		}
	}
	return s
}

func TestOlderServer(t *testing.T) {
	withRendered := viewmodel.State{}.ApplySnapshot(server.Snapshot{
		Seq:     1,
		Tickets: []server.TicketInfo{{Address: "gx:e/01", Status: "open", RenderedStatus: "open"}},
	})
	cases := []struct {
		name string
		st   viewmodel.State
		want bool
	}{
		{"no snapshot yet", viewmodel.State{}, true},
		{"fresh server snapshot (seq 0)", viewmodel.State{}.ApplySnapshot(server.Snapshot{}), false},
		{"tickets without rendered status", snapshot(), true},
		{"tickets with rendered status", withRendered, false},
	}
	for _, c := range cases {
		if got := c.st.OlderServer(); got != c.want {
			t.Errorf("%s: OlderServer() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReduce_EffectPerEventType(t *testing.T) {
	const resnap = viewmodel.EffectResnapshot
	cases := []struct {
		typ  string
		want viewmodel.Effect
	}{
		{server.EventTicketClaimed, resnap},
		{server.EventIterationStarted, resnap},
		{server.EventIterationPaused, resnap},
		{server.EventIterationFailed, resnap},
		{server.EventIterationLaunchFailed, resnap},
		{server.EventTicketDone, resnap},
		{server.EventTicketCancelled, resnap},
		{server.EventTicketPaused, resnap},
		{server.EventTicketChanged, resnap},
		{server.EventReclaimed, resnap},
		{server.EventExplainVerdictChange, resnap},
		{server.EventRootCompleted, resnap},
		{server.EventRootPaused, resnap},
		{server.EventMergeRolledBack, resnap},
		{server.EventTicketNudged, resnap},
		{server.EventQueueChanged, resnap | viewmodel.EffectRefetchQueue},
		{server.EventSessionChanged, resnap},
		{server.EventHerdrUnavailable, viewmodel.EffectNone},
		{server.EventHerdrAvailable, viewmodel.EffectNone},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			got, eff := snapshot().Reduce(server.Event{Seq: 11, Type: c.typ, Address: "gx:e/01"})
			if eff != c.want {
				t.Fatalf("effect = %v, want %v", eff, c.want)
			}
			if got.Seq != 11 {
				t.Fatalf("seq = %d, event not applied", got.Seq)
			}
		})
	}
}

func TestApplySnapshot_CarriesSessions(t *testing.T) {
	sessions := []server.SessionInfo{{ID: "s1"}, {ID: "s2"}}
	s := snapshot().ApplySnapshot(server.Snapshot{Seq: 12, Sessions: sessions})
	if len(s.Sessions) != 2 || s.Sessions[0].ID != "s1" || s.Sessions[1].ID != "s2" {
		t.Fatalf("sessions = %+v, want s1, s2", s.Sessions)
	}
	sessions[0].ID = "mutated"
	if s.Sessions[0].ID != "s1" {
		t.Fatal("state shares the snapshot's sessions slice")
	}
	if s = s.ApplySnapshot(server.Snapshot{Seq: 13}); len(s.Sessions) != 0 {
		t.Fatalf("sessions after empty snapshot = %+v, want none", s.Sessions)
	}
}

func TestActionableCount(t *testing.T) {
	s := viewmodel.State{}.ApplySnapshot(server.Snapshot{Sessions: []server.SessionInfo{
		{ID: "a", Actionable: server.SessionActionable{Reason: "blocked"}},
		{ID: "b"},
		{ID: "c", Actionable: server.SessionActionable{Reason: "review"}},
		{ID: "fix-gx:m/01", Actionable: server.SessionActionable{Reason: "blocked"}},
		{ID: "d", Design: true, Actionable: server.SessionActionable{Reason: "review"}},
		{ID: "q", Queue: true, Actionable: server.SessionActionable{Reason: "blocked"}},
	}})
	if got := s.ActionableCount(); got != 3 {
		t.Fatalf("ActionableCount = %d, want 3 (Design and Queue agents excluded)", got)
	}
	if got := (viewmodel.State{}).ActionableCount(); got != 0 {
		t.Fatalf("empty ActionableCount = %d, want 0", got)
	}
}

func TestReduce_ConfigReloadedResnapshotsUnlessError(t *testing.T) {
	cases := []struct {
		level string
		want  viewmodel.Effect
	}{
		{server.EventLevelSuccess, viewmodel.EffectResnapshot},
		{server.EventLevelWarning, viewmodel.EffectResnapshot},
		{server.EventLevelError, viewmodel.EffectNone},
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			got, eff := snapshot().Reduce(server.Event{Seq: 11, Type: server.EventConfigReloaded, Level: c.level, Message: "m"})
			if eff != c.want {
				t.Fatalf("effect = %v, want %v", eff, c.want)
			}
			if got.Seq != 11 {
				t.Fatalf("seq = %d, event not applied", got.Seq)
			}
		})
	}
}

func TestToastFor(t *testing.T) {
	cases := []struct {
		name string
		ev   server.Event
		want viewmodel.Toast
	}{
		{"iteration paused", server.Event{Type: server.EventIterationPaused, Address: "gx:e/01"},
			viewmodel.Toast{Level: server.EventLevelWarning, Text: "gx:e/01 paused"}},
		{"ticket paused", server.Event{Type: server.EventTicketPaused, Address: "gx:e/01"},
			viewmodel.Toast{Level: server.EventLevelWarning, Text: "gx:e/01 paused"}},
		{"reload success", server.Event{Type: server.EventConfigReloaded, Level: server.EventLevelSuccess, Message: "ok"},
			viewmodel.Toast{Level: server.EventLevelSuccess, Text: "ok"}},
		{"reload warning", server.Event{Type: server.EventConfigReloaded, Level: server.EventLevelWarning, Message: "warn"},
			viewmodel.Toast{Level: server.EventLevelWarning, Text: "warn"}},
		{"reload error", server.Event{Type: server.EventConfigReloaded, Level: server.EventLevelError, Message: "bad"},
			viewmodel.Toast{Level: server.EventLevelError, Text: "bad"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := viewmodel.ToastFor(c.ev)
			if !ok || got != c.want {
				t.Fatalf("ToastFor = %+v, %v; want %+v, true", got, ok, c.want)
			}
		})
	}
	if _, ok := viewmodel.ToastFor(server.Event{Type: server.EventTicketDone, Address: "gx:e/01"}); ok {
		t.Fatal("ticket-done should not toast")
	}
}

func TestReduce_HerdrAppliedInPlace(t *testing.T) {
	s, _ := snapshot().Reduce(server.Event{Seq: 11, Type: server.EventHerdrUnavailable})
	if !s.HerdrUnavailable {
		t.Fatal("herdr-unavailable not applied")
	}
	s, _ = s.Reduce(server.Event{Seq: 12, Type: server.EventHerdrAvailable})
	if s.HerdrUnavailable {
		t.Fatal("herdr-available not applied")
	}
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
	if len(s.Queue) != 2 || s.Queue[0] != "gx:e/01" || s.Queue[1] != "gx:e/02" {
		t.Fatalf("queue = %v", s.Queue)
	}
	if s.Seq != 13 {
		t.Fatalf("seq = %d", s.Seq)
	}

	if start.Tickets[0].Status != "open" || len(start.Queue) != 2 {
		t.Fatalf("reduce mutated its input: %+v", start)
	}
}

// The server's cancel verb only writes the ticket file; it never touches the
// queue item, so the reducer keeps the address queued too.
func TestReduce_TicketCancelledKeepsQueueItemLikeServerCancel(t *testing.T) {
	s := reduce(t, snapshot(), server.Event{Seq: 11, Type: server.EventTicketCancelled, Address: "gx:e/01"})
	if s.Tickets[0].Status != "cancelled" {
		t.Fatalf("status = %q", s.Tickets[0].Status)
	}
	if len(s.Queue) != 2 || s.Queue[0] != "gx:e/01" {
		t.Fatalf("queue = %v", s.Queue)
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
	if eff != viewmodel.EffectResnapshot|viewmodel.EffectRefetchQueue {
		t.Fatalf("effect = %v", eff)
	}
}

func TestApplySnapshot_CarriesEpicSummaries(t *testing.T) {
	st := viewmodel.State{}.ApplySnapshot(server.Snapshot{
		Epics: []server.EpicSummary{{Address: "gx:e", State: server.EpicStatePaused, Done: 1, Total: 3}},
	})

	if len(st.Epics) != 1 || st.Epics[0].State != server.EpicStatePaused || st.Epics[0].Total != 3 {
		t.Errorf("Epics = %+v, want the snapshot's paused summary", st.Epics)
	}
}

func TestApplySnapshot_CarriesQueueMode(t *testing.T) {
	s := snapshot().ApplySnapshot(server.Snapshot{Seq: 20, Mode: server.QueueModePaused})
	if s.Mode != server.QueueModePaused {
		t.Fatalf("mode = %q", s.Mode)
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

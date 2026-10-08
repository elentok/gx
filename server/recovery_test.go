package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// startRecovery starts a server whose catalog has one test-only low-authority
// rule that relaunches a manually parked ticket, and reports each remedy result.
func startRecovery(t *testing.T, optOut bool) (*servertest.Harness, <-chan recovery.Result) {
	t.Helper()
	return startRecoveryWith(t, optOut, recovery.AuthorityLow)
}

func startRecoveryWith(t *testing.T, optOut bool, authority recovery.Authority) (*servertest.Harness, <-chan recovery.Result) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	if optOut {
		path := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
		data, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.Replace(string(data), "status: open\n", "status: open\nrecover: false\n", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan recovery.Result, 4)
	cat := recovery.Catalog{Enabled: true, Entries: []recovery.Entry{{
		ID: "TEST", Type: events.NeedsRepair, Kind: events.ManualPark,
		Executor: recovery.ExecutorRule, Authority: authority, Enabled: true,
		Remedy: func(f recovery.Failure, v recovery.Verbs) error {
			res, err := v.Relaunch(f.Address)
			results <- res
			return err
		},
	}}}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = cat
	})
	registerLaunch(h)
	return h, results
}

func TestRecovery_MatchingParkRunsItsRemedyThroughAServerVerb(t *testing.T) {
	h, results := startRecovery(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case res := <-results:
		if res.Actor != "recovery" || res.Via != "server" || res.Refused {
			t.Errorf("remedy result = %+v, want a served verb by actor recovery", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remedy never ran")
	}
}

func TestRecovery_RecoveredParkLeavesMatchedAndAppliedWithKind(t *testing.T) {
	h, results := startRecovery(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("remedy never ran")
	}

	var matched, applied *ralphloop.Event
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (matched == nil || applied == nil) {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		matched, applied = nil, nil
		for i, ev := range log {
			switch events.Type(ev.Type) {
			case events.RecoveryMatched:
				matched = &log[i]
			case events.RecoveryApplied:
				applied = &log[i]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if matched == nil || applied == nil {
		t.Fatalf("matched = %v, applied = %v, want both recorded", matched, applied)
	}
	for _, ev := range []*ralphloop.Event{matched, applied} {
		if ev.Ticket != "01" || ev.Kind != string(events.ManualPark) || ev.Reason != "TEST" {
			t.Errorf("%s = %+v, want ticket 01, kind manual-park, entry TEST", ev.Type, *ev)
		}
	}
	if applied.Outcome != "ok" {
		t.Errorf("applied outcome = %q, want ok", applied.Outcome)
	}
}

// parkAfterSeeding appends seed events to ticket 01's run log (as a previous
// server run would have left them), parks it, and returns the escalation or nil.
func parkAfterSeeding(t *testing.T, seed ...ralphloop.Event) (*ralphloop.Event, <-chan recovery.Result) {
	t.Helper()
	h, results := startRecovery(t, false)
	dir := filepath.Join(h.TicketStore, "proj")
	for _, ev := range seed {
		ev.Ticket = "01"
		if err := ralphloop.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for i, ev := range log {
			if events.Type(ev.Type) == events.RecoveryEscalated {
				return &log[i], results
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, results
}

func applied(kind events.Kind) ralphloop.Event {
	return ralphloop.Event{Type: string(events.RecoveryApplied), Kind: string(kind), Reason: "TEST", Outcome: "ok"}
}

func TestRecovery_SecondFailureOfTheSameKindEscalatesInsteadOfRecovering(t *testing.T) {
	// A landing between the two clears the failed-recovery rule, leaving the cap.
	esc, results := parkAfterSeeding(t, applied(events.ManualPark), ralphloop.Event{Type: string(events.CherryPicked)})
	if esc == nil || esc.Kind != string(events.ManualPark) {
		t.Fatalf("escalation = %+v, want recovery-escalated for manual-park", esc)
	}
	select {
	case res := <-results:
		t.Fatalf("capped kind still ran a remedy: %+v", res)
	default:
	}
}

func TestRecovery_PerTicketCapEscalatesDifferentKinds(t *testing.T) {
	landed := ralphloop.Event{Type: string(events.CherryPicked)}
	esc, _ := parkAfterSeeding(t,
		applied(events.ZeroCommit), landed, applied(events.Spinning), landed, applied(events.RetryExhausted), landed)
	if esc == nil {
		t.Fatal("fourth recovery was not escalated")
	}
}

func TestRecovery_ReFailRightAfterARecoveryNamesBothFailures(t *testing.T) {
	esc, _ := parkAfterSeeding(t, applied(events.ZeroCommit))
	if esc == nil {
		t.Fatal("re-fail after a recovery was not escalated")
	}
	if !strings.Contains(esc.Reason, string(events.ZeroCommit)) || !strings.Contains(esc.Reason, string(events.ManualPark)) {
		t.Errorf("reason = %q, want both zero-commit and manual-park", esc.Reason)
	}
}

func TestRecovery_TicketOptOutNeverTriggers(t *testing.T) {
	h, results := startRecovery(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	select {
	case res := <-results:
		t.Fatalf("recover: false ticket ran a remedy: %+v", res)
	case <-time.After(500 * time.Millisecond):
	}
}

// proposeByParking parks ticket 01 under a high-authority rule and waits for
// its proposal.
func proposeByParking(t *testing.T) (h *servertest.Harness, results <-chan recovery.Result, ticketPath string) {
	t.Helper()
	h, results = startRecoveryWith(t, false, recovery.AuthorityHigh)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryProposed {
				if ev.Text != "relaunch proj:epic-a/01" {
					t.Fatalf("proposed text = %q, want the exact relaunch verb", ev.Text)
				}
				return h, results, filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no recovery-proposed event")
	return
}

func TestRecovery_HighAuthorityMatchProposesWithoutActing(t *testing.T) {
	_, results, path := proposeByParking(t)
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "## Proposed Remedy") || !strings.Contains(string(data), "relaunch proj:epic-a/01") {
		t.Errorf("ticket lacks the Proposed Remedy section:\n%s", data)
	}
	select {
	case res := <-results:
		// The proposal itself runs the remedy against a recorder, which has no actor.
		if res.Actor != "" {
			t.Fatalf("high-authority remedy ran: %+v", res)
		}
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRecovery_ApproveRunsTheLatestProposal(t *testing.T) {
	h, results, _ := proposeByParking(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Approve only runs what the proposal recorded.
	res, err := h.Client.TicketApprove(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	_ = results
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	var got *ralphloop.Event
	for i, ev := range log {
		if events.Type(ev.Type) == events.RecoveryApplied {
			got = &log[i]
		}
	}
	if got == nil || got.Outcome != "ok" {
		t.Fatalf("applied = %+v, want ok", got)
	}
}

func TestRecovery_ApproveAfterHandEditRefusesProposalStale(t *testing.T) {
	h, _, path := proposeByParking(t)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, []byte("\nhand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketApprove(ctx, "proj:epic-a/01"); err != nil || res.Reason != server.ReasonProposalStale {
		t.Errorf("approve = %+v, %v, want proposal-stale", res, err)
	}
}

// startUnmatched starts a server whose catalog matches nothing, so every park
// needs judgment, and queues ticket 02 behind the one that will be parked.
func startUnmatched(t *testing.T) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Catalog{Enabled: true}
	})
}

func epicTickets(t *testing.T, h *servertest.Harness) tickets.Epic {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(h.TicketStore, "proj"))
	if err != nil || len(epics) != 1 {
		t.Fatalf("load = %v, %v", epics, err)
	}
	return epics[0]
}

func TestRecovery_UnmatchedParkForksAQueuedInvestigateChildAndNeverRecurses(t *testing.T) {
	h := startUnmatched(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("park = %+v, %v", res, err)
	}
	waitFor(t, func() bool { return len(queueAddresses(t, h)) > 0 && queueAddresses(t, h)[0] == "proj:epic-a/01a" })

	epic := epicTickets(t, h)
	var parent, child tickets.Ticket
	for _, tk := range epic.Tickets {
		switch tk.Identifier {
		case "01":
			parent = tk
		case "01a":
			child = tk
		}
	}
	if child.Type != "investigate" || child.Parent == nil || *child.Parent != "01" {
		t.Errorf("child = %+v, want an investigate fork of 01", child)
	}
	if got := epic.RenderedStatus(parent); got != tickets.StatusWaitingForChildren {
		t.Errorf("parent renders %v, want waiting-for-children", got)
	}

	if res, err := h.Client.TicketPark(ctx, "proj:epic-a/01a", "stuck"); err != nil || res.Refused {
		t.Fatalf("park child = %+v, %v", res, err)
	}
	time.Sleep(500 * time.Millisecond)
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Identifier == "01b" || tk.Identifier == "01a1" {
			t.Errorf("investigate ticket failure forked %s, want no recursion", tk.Identifier)
		}
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition never held")
}

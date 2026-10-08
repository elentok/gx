package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
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

func TestRecovery_SpinningParkIsRecordedByR1WithoutANudge(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
	})
	registerLaunch(h)

	if err := h.Server.ParkAs("proj:epic-a/01", events.Spinning, "parked and re-claimed 3 times within 5m0s"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.Spinning) && ev.Reason == "R1" && ev.Outcome == "ok" {
				return true
			}
		}
		return false
	})
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	for _, ev := range log {
		switch events.Type(ev.Type) {
		case events.RecoveryEscalated, events.RecoveryProposed, events.Reclaimed:
			t.Errorf("unexpected %s after a spinning park", ev.Type)
		}
	}
}

// parkStalledLaunch parks ticket 01 the way a launch whose prompt stalled ends,
// with the pane answering prompts with promptErr, and returns R5's applied
// event plus what was typed and which tabs were closed.
func parkStalledLaunch(t *testing.T, promptErr error) (applied ralphloop.Event, typed, closed []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
	})
	var mu sync.Mutex
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		typed = append(typed, argv[3])
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1"}}, herdrfake.Identities{}, promptErr
	})
	h.Herdr.Register("tab", "close", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		closed = append(closed, argv[2])
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	dir := filepath.Join(store, "proj")
	stalled := ralphloop.Event{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(events.AgentPromptStalled), Attempt: 1}
	if err := ralphloop.AppendEvent(dir, "epic-a", stalled); err != nil {
		t.Fatal(err)
	}
	if err := h.Server.ParkAs("proj:epic-a/01", events.AgentPromptStalled, "agent_prompt_stalled"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied {
				applied = ev
				return true
			}
		}
		return false
	})
	mu.Lock()
	defer mu.Unlock()
	return applied, typed, closed
}

func TestRecovery_StalledPromptIsRetypedInFullByR5(t *testing.T) {
	applied, typed, closed := parkStalledLaunch(t, nil)
	if applied.Reason != "R5" || applied.Kind != string(events.AgentPromptStalled) || applied.Outcome != "ok" {
		t.Errorf("applied = %+v, want R5 ok", applied)
	}
	if len(typed) != 1 || typed[0] != "/gx-implement proj:epic-a/01" {
		t.Errorf("typed = %q, want the full launch prompt once", typed)
	}
	if len(closed) != 0 {
		t.Errorf("closed tabs %q after a delivered prompt", closed)
	}
}

func TestRecovery_UndeliverablePromptClosesThePane(t *testing.T) {
	applied, typed, closed := parkStalledLaunch(t, errors.New("agent_prompt_stalled"))
	if applied.Reason != "R5" || applied.Outcome == "ok" {
		t.Errorf("applied = %+v, want a failed R5", applied)
	}
	if len(typed) != 1 {
		t.Errorf("typed = %q, want one retype", typed)
	}
	if len(closed) != 1 || closed[0] != "tab-1" {
		t.Errorf("closed = %q, want the iteration's tab", closed)
	}
}

// parkTimedOutCompaction parks ticket 01 the way an iteration ends after its
// smart-zone compaction wait timed out, with the pane answering the extended
// wait with waitErr, and returns R7's applied event plus what was typed.
func parkTimedOutCompaction(t *testing.T, waitErr error) (applied ralphloop.Event, waits int, typed []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
	})
	var mu sync.Mutex
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		waits++
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "agent_status": "idle"}}, herdrfake.Identities{}, waitErr
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		typed = append(typed, argv[3])
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1"}}, herdrfake.Identities{}, nil
	})
	dir := filepath.Join(store, "proj")
	for _, ev := range []ralphloop.Event{
		{Type: string(events.IterationStarted), Ticket: "01"},
		{Type: string(events.SmartZoneRecoveryFailed), Ticket: "01", Reason: `compacting epic-a-01 after smart-zone breach: {"code":"timeout"}`},
	} {
		if err := ralphloop.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Server.ParkAs("proj:epic-a/01", events.IterationError, "compact recovery exhausted"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied {
				applied = ev
				return true
			}
		}
		return false
	})
	mu.Lock()
	defer mu.Unlock()
	return applied, waits, typed
}

func TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7(t *testing.T) {
	applied, waits, typed := parkTimedOutCompaction(t, nil)
	if applied.Reason != "R7" || applied.Kind != string(events.IterationError) || applied.Outcome != "ok" {
		t.Errorf("applied = %+v, want R7 ok", applied)
	}
	if waits != 1 {
		t.Errorf("waits = %d, want one extended wait", waits)
	}
	if len(typed) != 1 || typed[0] != recovery.FinishUpPrompt {
		t.Errorf("typed = %q, want only the finish-up prompt, never /compact", typed)
	}
}

func TestRecovery_SecondCompactionTimeoutLeavesTheParkToAPerson(t *testing.T) {
	applied, waits, typed := parkTimedOutCompaction(t, errors.New("timed out waiting for agent status"))
	if applied.Reason != "R7" || applied.Outcome == "ok" {
		t.Errorf("applied = %+v, want a failed R7", applied)
	}
	if waits != 1 || len(typed) != 0 {
		t.Errorf("waits = %d, typed = %q; want one wait and nothing typed", waits, typed)
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

// R2 launches disabled, so a zero-commit park falls through to an investigate
// fork with no R2 event: nothing is applied or proposed on R2's behalf.
func TestRecovery_ZeroCommitParkWithR2DisabledInvestigatesWithoutR2Events(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
	})
	registerLaunch(h)

	if err := h.Server.ParkAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				return true
			}
		}
		return false
	})
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	for _, ev := range log {
		if ev.Reason == "R2" {
			t.Errorf("unexpected %s event for disabled R2", ev.Type)
		}
	}
}

// R3 is an agent entry: an enabled match hands the failure to an investigate
// fork named for R3 and records the recovery as applied. The agent, not the
// server, then proves presence and lands the recoverable branch.
func TestRecovery_EnabledR3MatchForksAnInvestigateTicketNamingR3(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerLaunch(h)

	if err := h.Server.ParkAs("proj:epic-a/01", events.AmbiguousLand, "done but commits missing from epic-a"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R3")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AmbiguousLand) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// startR3 starts a server with the default catalog fully enabled. The server
// does not read the transcript yet, so the commitless-done entry's claim
// predicate is dropped to let a bare zero-commit park reach it.
func startR3(t *testing.T) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
		for i, e := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
			if e.Executor == recovery.ExecutorRule && e.Authority == recovery.AuthorityHigh {
				c.Recovery.Entries[i].Predicate = nil
			}
		}
	})
	registerLaunch(h)
	return h
}

func TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone(t *testing.T) {
	h := startR3(t)
	if err := h.Server.ParkAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryProposed && ev.Reason == "R3" && ev.Text == "commitless-done proj:epic-a/01" {
				return true
			}
		}
		return false
	})
	tk := epicTickets(t, h).Tickets[0]
	if tk.IsDone() || tk.Commitless {
		t.Fatalf("proposal acted: status %s, commitless %v", tk.Status, tk.Commitless)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketApprove(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	if tk := epicTickets(t, h).Tickets[0]; !tk.IsDone() || !tk.Commitless {
		t.Errorf("approved ticket: status %s, commitless %v, want done and commitless", tk.Status, tk.Commitless)
	}
}

func TestRecovery_R3LostCommitsEscalateWithoutAnInvestigation(t *testing.T) {
	h := startR3(t)
	reason := "done but commits missing from epic-a and iteration branch ralph-loop/epic-a-item-01 no longer exists to recover them"
	if err := h.Server.ParkAs("proj:epic-a/01", events.AmbiguousLand, reason); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryEscalated && strings.Contains(ev.Reason, "R3") {
				return true
			}
		}
		return false
	})
	log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
	for _, ev := range log {
		if typ := events.Type(ev.Type); typ == events.RecoveryApplied || typ == events.RecoveryProposed {
			t.Errorf("unexpected %s for lost commits", ev.Type)
		}
	}
	if n := len(epicTickets(t, h).Tickets); n != 1 {
		t.Errorf("tickets = %d, want no investigate fork", n)
	}
}

// R4 is an agent entry: an enabled match hands the blocked pane to an
// investigate fork named for R4, which reads the pane and answers only an
// allow-listed dialog.
func TestRecovery_EnabledR4MatchForksAnInvestigateTicketNamingR4(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerLaunch(h)

	if err := h.Server.ParkAs("proj:epic-a/01", events.BlockedPane, "epic-a-01 is blocked on a prompt gx did not send; answer it in the pane"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R4")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.BlockedPane) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// parkLaunchCollision starts a server with the default catalog fully enabled,
// seeds ticket 01's earlier events and its launch-failed event of kind as the
// loop would, and parks it with the same kind.
func parkLaunchCollision(t *testing.T, kind events.Kind, earlier ...events.Type) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	seeds := []ralphloop.Event{}
	for _, typ := range earlier {
		seeds = append(seeds, ralphloop.Event{Type: string(typ), Ticket: "01"})
	}
	seeds = append(seeds, ralphloop.Event{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(kind), Attempt: 1, Reason: string(kind)})
	for _, seed := range seeds {
		if err := ralphloop.AppendEvent(filepath.Join(store, "proj"), "epic-a", seed); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerLaunch(h)
	if err := h.Server.ParkAs("proj:epic-a/01", kind, "launch failed: "+string(kind)); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRecovery_R6BusyPaneIsRelaunchedByARule(t *testing.T) {
	h := parkLaunchCollision(t, events.AgentPaneBusy)
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentPaneBusy) && ev.Reason == "R6" && ev.Outcome == "ok" {
				return true
			}
		}
		return false
	})
}

// A taken name needs herdr's candidate block read, so R6 hands it to an
// investigate fork rather than acting.
func TestRecovery_R6TakenNameForksAnInvestigateTicketNamingR6(t *testing.T) {
	h := parkLaunchCollision(t, events.AgentNameTaken)
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R6")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentNameTaken) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// A taken name while the ticket's own iteration is still live is a clobbered
// claim, so R11 (not R6) forks the investigation that restores it.
func TestRecovery_R11ClobberedClaimForksAnInvestigateTicketNamingR11(t *testing.T) {
	h := parkLaunchCollision(t, events.AgentNameTaken, events.IterationStarted)
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R11")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AgentNameTaken) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// A reclaim that parks zero-commit at once after an earlier prompt stall is a
// stalled pane read as finished, so R12 forks the investigation that closes it
// and relaunches.
func TestRecovery_R12StalledReclaimForksAnInvestigateTicketNamingR12(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	for _, seed := range []ralphloop.Event{
		{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(events.AgentPromptStalled), Attempt: 1, Reason: "prompt stalled"},
		{Type: string(events.Reclaimed), Ticket: "01"},
	} {
		if err := ralphloop.AppendEvent(filepath.Join(store, "proj"), "epic-a", seed); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerLaunch(h)
	if err := h.Server.ParkAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		for _, tk := range epicTickets(t, h).Tickets {
			if tk.Identifier == "01a" && tk.Type == "investigate" {
				body, _ := os.ReadFile(tk.Path)
				return strings.Contains(string(body), "R12")
			}
		}
		return false
	})
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.ZeroCommit) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// raiseGateHeld raises a held background-task gate on ticket 01 against a
// catalog with R10 enabled. The iteration has a real worktree, shaped by
// prepare, and a pane reporting status. It stands in for the loop's finish by
// dropping the run once the gate is released, and returns R10's applied event
// plus the escalation, if any.
func raiseGateHeld(t *testing.T, status string, prepare func(worktree string)) (applied ralphloop.Event, escalated *ralphloop.Event) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	cat := recovery.Default()
	for i := range cat.Entries {
		if cat.Entries[i].ID == "R10" {
			cat.Entries[i].Enabled = true
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Orchestrator = config.OrchestratorServer
		c.Recovery = cat
	})
	h.Herdr.Register("agent", "get", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1", "agent_status": status}}, herdrfake.Identities{}, nil
	})
	_, branch, worktree := ralphloop.IterationIdentity("epic-a", "01", h.Server.WorktreeDir("proj"))
	testutil.MustGitExported(t, repo, "worktree", "add", "-b", branch, worktree)
	prepare(worktree)

	const addr = "proj:epic-a/01"
	h.Server.PutRunFrom("proj:epic-a", server.Run{Address: addr}, "main")
	dir := filepath.Join(store, "proj")
	held := ralphloop.Event{Type: string(events.BackgroundTaskGateHeld), Ticket: "01", Reason: "background task task-1"}
	if err := ralphloop.AppendEvent(dir, "epic-a", held); err != nil {
		t.Fatal(err)
	}
	h.Server.RecoverAsync(recovery.Failure{Address: addr, Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: held.Reason})
	waitFor(t, func() bool {
		if h.Server.GateReleased(addr) {
			h.Server.DropRun(addr)
		}
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		var done bool
		for i, ev := range log {
			switch events.Type(ev.Type) {
			case events.RecoveryApplied:
				applied, done = ev, ev.Outcome == "ok"
			case events.RecoveryEscalated:
				escalated, done = &log[i], true
			}
		}
		return done
	})
	return applied, escalated
}

func commitInWorktree(t *testing.T) func(string) {
	return func(worktree string) {
		testutil.WriteFile(t, worktree, "work.txt", "done")
		testutil.CommitAll(t, worktree, "work")
	}
}

func TestRecovery_HeldGateWithCommittedIdleWorkIsReleasedAndFinishedByR10(t *testing.T) {
	applied, escalated := raiseGateHeld(t, "idle", commitInWorktree(t))
	if applied.Reason != "R10" || applied.Outcome != "ok" {
		t.Errorf("applied = %+v, want R10 ok", applied)
	}
	if escalated != nil {
		t.Errorf("escalated = %+v after a finished release", *escalated)
	}
}

func TestRecovery_HeldGateThatCannotBeReleasedEscalatesWithoutAPark(t *testing.T) {
	for _, tc := range []struct {
		name, status, reason string
		prepare              func(*testing.T) func(string)
	}{
		{"busy pane", "working", server.ReasonAgentBusy, commitInWorktree},
		{"dirty worktree", "idle", server.ReasonWorktreeDirty, func(t *testing.T) func(string) {
			return func(worktree string) {
				commitInWorktree(t)(worktree)
				testutil.WriteFile(t, worktree, "stray.txt", "uncommitted")
			}
		}},
		{"no commits ahead", "idle", server.ReasonNoCommitsAhead, func(*testing.T) func(string) { return func(string) {} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			applied, escalated := raiseGateHeld(t, tc.status, tc.prepare(t))
			if applied.Reason != "R10" || !strings.Contains(applied.Outcome, tc.reason) {
				t.Errorf("applied = %+v, want a failed R10 naming %s", applied, tc.reason)
			}
			if escalated == nil || escalated.Reason != "R10" || escalated.Outcome != applied.Outcome {
				t.Errorf("escalated = %v, want R10 with the applied outcome %q", escalated, applied.Outcome)
			}
		})
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

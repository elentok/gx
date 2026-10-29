package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/transcript"
)

// startRecovery starts a server whose catalog has one test-only low-authority
// rule that relaunches a ticket paused on an iteration error, and reports each remedy result.
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
		ID: "TEST", Type: events.Paused, Kind: events.IterationError,
		Executor: recovery.ExecutorRule, Authority: authority, Enabled: true, Verbs: []string{recovery.VerbRelaunch},
		Remedy: func(f recovery.Failure, v recovery.Verbs) error {
			res, err := v.Do(recovery.Call{Verb: recovery.VerbRelaunch, Address: f.Address})
			results <- res
			return err
		},
	}}}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = cat
	})
	registerTurn(h)
	return h, results
}

func TestRecovery_MatchingPauseRunsItsRemedyThroughAServerVerb(t *testing.T) {
	t.Parallel()
	h, results := startRecovery(t, false)

	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
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

func TestRecovery_RecoveredPauseLeavesMatchedAndAppliedWithKind(t *testing.T) {
	t.Parallel()
	h, results := startRecovery(t, false)

	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("remedy never ran")
	}

	var matched, applied *ralphloop.Event
	servertest.Poll(5*time.Second, func() bool {
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
		return matched != nil && applied != nil
	})
	if matched == nil || applied == nil {
		t.Fatalf("matched = %v, applied = %v, want both recorded", matched, applied)
	}
	for _, ev := range []*ralphloop.Event{matched, applied} {
		if ev.Ticket != "01" || ev.Kind != string(events.IterationError) || ev.Reason != "TEST" {
			t.Errorf("%s = %+v, want ticket 01, kind iteration-error, entry TEST", ev.Type, *ev)
		}
	}
	if applied.Outcome != "ok" {
		t.Errorf("applied outcome = %q, want ok", applied.Outcome)
	}
}

func TestRecovery_SpinningPauseIsRecordedByR1WithoutANudge(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	registerTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.Spinning, "paused and re-claimed 3 times within 5m0s"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryMatched && ev.Kind == string(events.Spinning) && ev.Reason == "R1" {
				return true
			}
		}
		return false
	})
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	for _, ev := range log {
		switch events.Type(ev.Type) {
		case events.RecoveryApplied, events.RecoveryEscalated, events.RecoveryProposed, events.Reclaimed:
			t.Errorf("unexpected %s after a spinning pause", ev.Type)
		}
	}
}

// pauseStalledLaunch pauses ticket 01 the way a launch whose prompt stalled ends,
// with the pane answering prompts with promptErr, and returns R5's applied
// event plus what the pane was typed and whether its session was stopped.
func pauseStalledLaunch(t *testing.T, promptErr error) (applied ralphloop.Event, typed []string, stopped bool) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake, sess := iterationAgent(t, "headless", agentrunner.StateIdle)
	fake.SetPromptErr(holderLabel, promptErr)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	const addr = "proj:epic-a/01"
	h.Server.PutRunFrom("proj:epic-a", server.Run{Address: addr, Runner: "headless", Session: sess}, "main")
	dir := filepath.Join(store, "proj")
	stalled := ralphloop.Event{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(events.AgentPromptStalled), Attempt: 1}
	if err := ralphloop.AppendEvent(dir, "epic-a", stalled); err != nil {
		t.Fatal(err)
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.AgentPromptStalled, "agent_prompt_stalled"); err != nil {
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
	_, live, _ := fake.Find(holderLabel)
	return applied, fake.Prompts(holderLabel), !live
}

func TestRecovery_StalledPromptIsRetypedInFullByR5(t *testing.T) {
	t.Parallel()
	applied, typed, stopped := pauseStalledLaunch(t, nil)
	if applied.Reason != "R5" || applied.Kind != string(events.AgentPromptStalled) || applied.Outcome != "ok" {
		t.Errorf("applied = %+v, want R5 ok", applied)
	}
	if len(typed) != 1 || typed[0] != "/gx-implement proj:epic-a/01" {
		t.Errorf("typed = %q, want the full launch prompt once", typed)
	}
	if stopped {
		t.Error("session stopped after a delivered prompt")
	}
}

func TestRecovery_UndeliverablePromptClosesThePane(t *testing.T) {
	t.Parallel()
	// Not a stall: herdrrunner nudge-retries a stalled prompt.
	applied, _, stopped := pauseStalledLaunch(t, errors.New("pane is gone"))
	if applied.Reason != "R5" || applied.Outcome == "ok" {
		t.Errorf("applied = %+v, want a failed R5", applied)
	}
	if !stopped {
		t.Error("session still live, want it stopped")
	}
}

// pauseTimedOutCompaction pauses ticket 01 the way an iteration ends after its
// smart-zone compaction wait timed out, with the pane answering the extended
// wait with waitErr, and returns R7's applied event plus what was typed.
func pauseTimedOutCompaction(t *testing.T, waitErr error) (applied ralphloop.Event, waits int, typed []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		// herdrrunner re-polls a timed-out wait until its deadline; one poll
		// has to outlast this.
		c.CompactRewaitTimeout = time.Nanosecond
	})
	var mu sync.Mutex
	// After the test the pane is gone, so the watch a nudge starts on it ends.
	var gone atomic.Bool
	t.Cleanup(func() { gone.Store(true) })
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if gone.Load() {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "tab_id": "tab-1", "agent_status": "idle", "state_change_seq": 1}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if gone.Load() {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		}
		mu.Lock()
		defer mu.Unlock()
		if len(typed) == 0 { // R7's own wait; later ones watch the nudged pane
			waits++
		}
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "agent_status": "idle"}}, herdrfake.Identities{}, waitErr
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		typed = append(typed, argv[3])
		return map[string]any{"agent": map[string]any{"pane_id": "pane-1", "agent_status": "working", "state_change_seq": 2}}, herdrfake.Identities{}, nil
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
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "compact recovery exhausted"); err != nil {
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
	t.Parallel()
	applied, waits, typed := pauseTimedOutCompaction(t, nil)
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

func TestRecovery_SecondCompactionTimeoutLeavesThePauseToAPerson(t *testing.T) {
	t.Parallel()
	applied, waits, typed := pauseTimedOutCompaction(t, errors.New("timed out waiting for agent status"))
	if applied.Reason != "R7" || applied.Outcome == "ok" {
		t.Errorf("applied = %+v, want a failed R7", applied)
	}
	if waits != 1 || len(typed) != 0 {
		t.Errorf("waits = %d, typed = %q; want one wait and nothing typed", waits, typed)
	}
}

// pauseAfterSeeding appends seed events to ticket 01's run log (as a previous
// server run would have left them), pauses it, and returns the escalation, or
// nil once the pause's own recovery is applied instead.
func pauseAfterSeeding(t *testing.T, seed ...ralphloop.Event) (*ralphloop.Event, <-chan recovery.Result) {
	t.Helper()
	h, results := startRecovery(t, false)
	dir := filepath.Join(h.TicketStore, "proj")
	for _, ev := range seed {
		ev.Ticket = "01"
		if err := ralphloop.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	var escalated *ralphloop.Event
	servertest.Poll(2*time.Second, func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for i, ev := range log {
			switch events.Type(ev.Type) {
			case events.RecoveryEscalated:
				escalated = &log[i]
				return true
			case events.RecoveryApplied:
				if i >= len(seed) {
					return true
				}
			}
		}
		return false
	})
	return escalated, results
}

func applied(kind events.Kind) ralphloop.Event {
	return ralphloop.Event{Type: string(events.RecoveryApplied), Kind: string(kind), Reason: "TEST", Outcome: "ok"}
}

func TestRecovery_SecondFailureOfTheSameKindEscalatesInsteadOfRecovering(t *testing.T) {
	t.Parallel()
	// A merging between the two clears the failed-recovery rule, leaving the cap.
	esc, results := pauseAfterSeeding(t, applied(events.IterationError), ralphloop.Event{Type: string(events.CherryPicked)})
	if esc == nil || esc.Kind != string(events.IterationError) {
		t.Fatalf("escalation = %+v, want recovery-escalated for iteration-error", esc)
	}
	select {
	case res := <-results:
		t.Fatalf("capped kind still ran a remedy: %+v", res)
	default:
	}
}

func TestRecovery_PerTicketCapEscalatesDifferentKinds(t *testing.T) {
	t.Parallel()
	merged := ralphloop.Event{Type: string(events.CherryPicked)}
	esc, _ := pauseAfterSeeding(t,
		applied(events.ZeroCommit), merged, applied(events.Spinning), merged, applied(events.RetryExhausted), merged)
	if esc == nil {
		t.Fatal("fourth recovery was not escalated")
	}
}

func TestRecovery_ReFailRightAfterARecoveryOfTheSameKindIsAFailedRecovery(t *testing.T) {
	t.Parallel()
	esc, _ := pauseAfterSeeding(t, applied(events.IterationError))
	if esc == nil {
		t.Fatal("re-fail after a recovery was not escalated")
	}
	if !strings.Contains(esc.Reason, "TEST for "+string(events.IterationError)+" failed") {
		t.Errorf("reason = %q, want a failed TEST recovery for iteration-error", esc.Reason)
	}
}

// A recovery of another kind, an investigation fork or a parent backfill is
// not a remedy for the pause, so the pause recovers as if it never happened.
func TestRecovery_UnrelatedPriorRecoveryDoesNotTripTheGuardRail(t *testing.T) {
	t.Parallel()
	investigated := applied(events.IterationError)
	investigated.Reason = "investigate"
	backfilled := applied(events.ParentDefect)
	backfilled.Reason = "R14"
	for name, seed := range map[string]ralphloop.Event{
		"other kind":  applied(events.ZeroCommit),
		"set-parent":  backfilled,
		"investigate": investigated,
	} {
		t.Run(name, func(t *testing.T) {
			esc, results := pauseAfterSeeding(t, seed)
			if esc != nil && strings.Contains(esc.Reason, "failed") {
				t.Fatalf("escalated as a failed recovery: %+v", esc)
			}
			if name == "investigate" {
				return // the per-kind cap still counts the investigation
			}
			if esc != nil {
				t.Fatalf("escalated: %+v", esc)
			}
			select {
			case <-results:
			case <-time.After(10 * time.Second):
				t.Fatal("remedy did not run")
			}
		})
	}
}

func TestRecovery_TicketOptOutNeverTriggers(t *testing.T) {
	t.Parallel()
	h, results := startRecovery(t, true)

	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	select {
	case res := <-results:
		t.Fatalf("recover: false ticket ran a remedy: %+v", res)
	case <-time.After(500 * time.Millisecond):
	}
}

// proposeByPausing pauses ticket 01 under a high-authority rule and waits for
// its proposal.
func proposeByPausing(t *testing.T) (h *servertest.Harness, results <-chan recovery.Result, ticketPath string) {
	t.Helper()
	h, results = startRecoveryWith(t, false, recovery.AuthorityHigh)
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	dir := filepath.Join(h.TicketStore, "proj")
	servertest.WaitFor(t, 5*time.Second, "a recovery-proposed event", func() bool {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryProposed {
				if ev.Text != "relaunch proj:epic-a/01" {
					t.Fatalf("proposed text = %q, want the exact relaunch verb", ev.Text)
				}
				return true
			}
		}
		return false
	})
	return h, results, filepath.Join(h.TicketStore, "proj", "epic-a", "issues", "01-first.md")
}

func TestRecovery_HighAuthorityMatchProposesWithoutActing(t *testing.T) {
	t.Parallel()
	_, results, path := proposeByPausing(t)
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
	t.Parallel()
	h, results, _ := proposeByPausing(t)
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

func proposedRemedy(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema.Section(schema.ParseBody(string(data)), "Proposed Remedy")
}

func TestRecovery_ApproveRetiresTheProposalSection(t *testing.T) {
	t.Parallel()
	h, _, path := proposeByPausing(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketApprove(ctx, "proj:epic-a/01"); err != nil || res.Refused {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	if got := proposedRemedy(t, path); got != "" {
		t.Errorf("Proposed Remedy still pending after approve:\n%s", got)
	}
}

func TestRecovery_ApproveAfterHandEditRefusesProposalStale(t *testing.T) {
	t.Parallel()
	h, _, path := proposeByPausing(t)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, []byte("\nhand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.TicketApprove(ctx, "proj:epic-a/01"); err != nil || res.Reason != server.ReasonProposalStale {
		t.Errorf("approve = %+v, %v, want proposal-stale", res, err)
	}
	if proposedRemedy(t, path) == "" {
		t.Error("a stale approve retired the proposal")
	}
}

// startUnmatched starts a server whose catalog matches nothing, so every pause
// needs judgment, and queues ticket 02 behind the one that will be paused.
func startUnmatched(t *testing.T) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store, func(c *server.Config) {
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

func TestRecovery_UnmatchedPauseForksAQueuedInvestigateChildAndNeverRecurses(t *testing.T) {
	t.Parallel()
	h := startUnmatched(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-a/02", ""); err != nil {
		t.Fatal(err)
	}

	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
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

	if err := h.Server.PauseAs("proj:epic-a/01a", events.IterationError, "stuck"); err != nil {
		t.Fatalf("pause child: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // negative wait: a forked ticket would show by now
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Identifier == "01b" || tk.Identifier == "01a1" {
			t.Errorf("investigate ticket failure forked %s, want no recursion", tk.Identifier)
		}
	}
}

// R2 launches disabled, so a zero-commit pause falls through to an investigate
// fork with no R2 event: nothing is applied or proposed on R2's behalf.
func TestRecovery_ZeroCommitPauseWithR2DisabledInvestigatesWithoutR2Events(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	registerTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
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
// server, then proves presence and merges the recoverable branch.
func TestRecovery_EnabledR3MatchForksAnInvestigateTicketNamingR3(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.AmbiguousMerge, "done but commits missing from epic-a"); err != nil {
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
			if events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.AmbiguousMerge) && ev.Outcome == "proj:epic-a/01a" {
				return true
			}
		}
		return false
	})
}

// startR3 starts a server with the default catalog fully enabled.
func startR3(t *testing.T) *servertest.Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerTurn(h)
	return h
}

// runToZeroCommitPause queues 01 under an agent whose every turn ends with
// last as its final assistant text and no commit, so the loop pauses it
// zero-commit the way a real iteration does.
func runToZeroCommitPause(t *testing.T, h *servertest.Harness, last string) {
	t.Helper()
	home := h.Env("HOME")
	_, _, cwd := registerTurn(h)
	var started atomic.Bool
	var turns atomic.Int32
	agentAt := func(turn int32) map[string]any {
		return map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": "idle", "state_change_seq": turn, "agent_session": map[string]any{"value": "s1"}}}
	}
	agent := agentAt(0)
	turn := func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		agent := agentAt(turns.Add(1))
		line, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"content": []map[string]any{{"type": "text", "text": last}}}})
		path := transcript.PathIn(home, *cwd, "s1")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, herdrfake.Identities{}, err
		}
		return agent, herdrfake.Identities{}, os.WriteFile(path, append(line, '\n'), 0o644)
	}
	reply := func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return agentAt(turns.Load()), herdrfake.Identities{}, nil
	}
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		started.Store(true)
		return agent, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		if !started.Load() {
			return nil, herdrfake.Identities{}, servertest.AgentNotFound(argv[2])
		}
		return agentAt(turns.Load()), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", reply)
	h.Herdr.Register("agent", "prompt", turn)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	// The loop's own finish debounce outlasts waitFor.
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationPaused, "proj:epic-a/01")
}

// R2 and R3 read the transcript the pause's session left: the matcher sees the
// agent's last words with no test injecting them.
func TestRecovery_ZeroCommitPauseMatchesOnItsTranscriptText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, last, entry string }{
		{"R2", `Bash({"command": "go test ./..."})`, "R2"},
		{"R3", "The feature is already implemented on the feature branch.", "R3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startR3(t)
			runToZeroCommitPause(t, h, tc.last)
			waitFor(t, func() bool {
				for _, tk := range epicTickets(t, h).Tickets {
					if tk.Identifier == "01a" && tk.Type == "investigate" {
						body, _ := os.ReadFile(tk.Path)
						return strings.Contains(string(body), tc.entry)
					}
				}
				return false
			})
		})
	}
}

func TestRecovery_R3CommitlessDoneIsProposedAndOnlyApprovalMarksItDone(t *testing.T) {
	t.Parallel()
	h := startR3(t)
	runToZeroCommitPause(t, h, "Nothing to do: this is already implemented in a sibling ticket.")
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

	// The paused iteration may still be unwinding, and approve refuses while it runs.
	waitFor(t, func() bool { return len(h.Server.Runs()) == 0 })
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
	t.Parallel()
	h := startR3(t)
	reason := "done but commits missing from epic-a and iteration branch ralph-loop/epic-a-item-01 no longer exists to recover them"
	if err := h.Server.PauseAs("proj:epic-a/01", events.AmbiguousMerge, reason); err != nil {
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
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.BlockedPane, "epic-a-01 is blocked on a prompt gx did not send; answer it in the pane"); err != nil {
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

// pauseLaunchCollision starts a server with the default catalog fully enabled,
// seeds ticket 01's earlier events and its launch-failed event of kind as the
// loop would, and pauses it with the same kind.
func pauseLaunchCollision(t *testing.T, kind events.Kind, earlier ...events.Type) *servertest.Harness {
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
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerTurn(h)
	if err := h.Server.PauseAs("proj:epic-a/01", kind, "launch failed: "+string(kind)); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRecovery_R6BusyPaneIsRelaunchedByARule(t *testing.T) {
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentPaneBusy)
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
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentNameTaken)
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
	t.Parallel()
	h := pauseLaunchCollision(t, events.AgentNameTaken, events.IterationStarted)
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

// A reclaim that pauses zero-commit at once after an earlier prompt stall is a
// stalled pane read as finished, so R12 forks the investigation that closes it
// and relaunches.
func TestRecovery_R12StalledReclaimForksAnInvestigateTicketNamingR12(t *testing.T) {
	t.Parallel()
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
		c.Recovery = recovery.Default()
		for i := range c.Recovery.Entries {
			c.Recovery.Entries[i].Enabled = true
		}
	})
	registerTurn(h)
	if err := h.Server.PauseAs("proj:epic-a/01", events.ZeroCommit, "no commits landed"); err != nil {
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

// iterationAgent is a runner answering to name that hosts ticket 01's
// iteration agent in state.
func iterationAgent(t *testing.T, name string, state agentrunner.State) (renamedFake, agentrunner.Session) {
	t.Helper()
	fake := newRenamedFake(name, nativerunner.VerdictLive)
	sess, err := fake.Start(agentrunner.StartOptions{Label: holderLabel})
	if err != nil {
		t.Fatal(err)
	}
	fake.SetState(holderLabel, state, "")
	return fake, sess
}

// gateRaise is what raiseGateHeld saw once R10 was done.
type gateRaise struct {
	applied   ralphloop.Event
	escalated *ralphloop.Event
	log       []ralphloop.Event
	ticket    string
	question  string // text of the ticket log's events
	worktree  string
}

// raiseGateHeld raises a held background-task gate on ticket 01 with the
// default catalog. The iteration has a real worktree, shaped by prepare, and an
// agent in state on a runner answering to runner. It stands in for the loop's
// finish by settling the run once the gate is released.
func raiseGateHeld(t *testing.T, runner string, state agentrunner.State, prepare func(worktree string)) gateRaise {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake, sess := iterationAgent(t, runner, state)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	_, branch, worktree := ralphloop.IterationIdentity("epic-a", "01", h.Server.WorktreeDir("proj"))
	testutil.MustGitExported(t, repo, "worktree", "add", "-b", branch, worktree)
	prepare(worktree)

	const addr = "proj:epic-a/01"
	h.Server.PutRunFrom("proj:epic-a", server.Run{Address: addr, Runner: runner, Session: sess}, "main")
	dir := filepath.Join(store, "proj")
	held := ralphloop.Event{Type: string(events.BackgroundTaskGateHeld), Ticket: "01", Reason: "background task task-1"}
	if err := ralphloop.AppendEvent(dir, "epic-a", held); err != nil {
		t.Fatal(err)
	}
	h.Server.RecoverAsync(recovery.Failure{Address: addr, Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: held.Reason})
	var got gateRaise
	waitFor(t, func() bool {
		if h.Server.GateReleased(addr) {
			h.Server.SettleReleasedRun(addr)
		}
		got.log, _, _ = ralphloop.ReadEvents(dir, "epic-a")
		var done bool
		for i, ev := range got.log {
			switch events.Type(ev.Type) {
			case events.RecoveryApplied:
				got.applied, done = ev, ev.Outcome == "ok"
			case events.RecoveryEscalated:
				got.escalated, done = &got.log[i], true
			}
		}
		return done
	})
	ticketPath := filepath.Join(dir, "epic-a", "issues", "01-first.md")
	data, _ := os.ReadFile(ticketPath)
	got.ticket, got.worktree = string(data), worktree
	// The pause reason lives in the log's question, not in the markdown.
	if evs, err := tickets.ReadLog(ticketPath); err == nil {
		for _, ev := range evs {
			got.question += ev.Text + "\n"
		}
	}
	return got
}

func commitInWorktree(t *testing.T) func(string) {
	return func(worktree string) {
		testutil.WriteFile(t, worktree, "work.txt", "done")
		testutil.CommitAll(t, worktree, "work")
	}
}

func dirtyAfter(t *testing.T, prepare func(string)) func(string) {
	return func(worktree string) {
		prepare(worktree)
		testutil.WriteFile(t, worktree, "stray.txt", "uncommitted")
	}
}

// R10 releases the gate on an idle agent whether or not the worktree has
// commits: the finish then merges them or pauses zero-commit on its own.
func TestRecovery_R10ReleasesAnIdleAgentsGateAndLetsTheFinishRun(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "headless"} {
		for _, tc := range []struct {
			name    string
			prepare func(string)
		}{
			{"commits on a clean worktree", commitInWorktree(t)},
			{"no commits on a clean worktree", func(string) {}},
			{"no commits on a dirty worktree", dirtyAfter(t, func(string) {})},
		} {
			t.Run(runner+"/"+tc.name, func(t *testing.T) {
				got := raiseGateHeld(t, runner, agentrunner.StateIdle, tc.prepare)
				if got.applied.Reason != "R10" || got.applied.Outcome != "ok" || got.escalated != nil {
					t.Errorf("applied = %+v, escalated = %v; want R10 ok", got.applied, got.escalated)
				}
				if strings.Contains(got.ticket, "background-task-gate") {
					t.Errorf("R10 paused a finish it should have let run:\n%s", got.ticket)
				}
			})
		}
	}
}

func TestRecovery_R10PausesADirtyWorktreeWithCommitsAndKeepsIt(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "headless"} {
		t.Run(runner, func(t *testing.T) {
			got := raiseGateHeld(t, runner, agentrunner.StateIdle, dirtyAfter(t, commitInWorktree(t)))
			if got.applied.Reason != "R10" || got.applied.Outcome != "ok" {
				t.Errorf("applied = %+v, want R10 ok", got.applied)
			}
			if !strings.Contains(got.ticket, "status: paused") || !strings.Contains(got.ticket, "pause_reason: background-task-gate") || !strings.Contains(got.question, "stray.txt") {
				t.Errorf("ticket not paused background-task-gate naming the dirty file:\n%s", got.ticket)
			}
			if _, err := os.Stat(filepath.Join(got.worktree, "stray.txt")); err != nil {
				t.Errorf("worktree not left: %v", err)
			}
			if n := len(ofType(got.log, events.RecoveryApplied)); n != 1 {
				t.Errorf("recoveries = %d, want R10's one: its own pause must not re-match", n)
			}
		})
	}
}

func TestRecovery_R10LeavesABusyAgentsGateAndEscalates(t *testing.T) {
	t.Parallel()
	for _, runner := range []string{"herdr", "headless"} {
		t.Run(runner, func(t *testing.T) {
			got := raiseGateHeld(t, runner, agentrunner.StateWorking, commitInWorktree(t))
			if got.applied.Reason != "R10" || !strings.Contains(got.applied.Outcome, server.ReasonAgentBusy) {
				t.Errorf("applied = %+v, want a failed R10 naming %s", got.applied, server.ReasonAgentBusy)
			}
			if got.escalated == nil || got.escalated.Reason != "R10" {
				t.Errorf("escalated = %v, want R10", got.escalated)
			}
		})
	}
}

// startParentDefect writes 01, 01a, 01a1 and a 01a2 missing its parent, then
// starts a server with the given catalog.
func startParentDefect(t *testing.T, store string, cat recovery.Catalog) *servertest.Harness {
	t.Helper()
	repo := testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01a", "fork", servertest.TicketOpts{Parent: "01"})
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01a1", "fork", servertest.TicketOpts{Parent: "01a"})
	servertest.WriteTicket(t, store, "proj", "epic-a", "01a2", "fork", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	return servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = cat
	})
}

// parentDefectEvents is the run log's events of kind parent-defect, by type.
func parentDefectEvents(h *servertest.Harness) map[events.Type][]ralphloop.Event {
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	byType := map[events.Type][]ralphloop.Event{}
	for _, ev := range log {
		if ev.Kind == string(events.ParentDefect) {
			byType[events.Type(ev.Type)] = append(byType[events.Type(ev.Type)], ev)
		}
	}
	return byType
}

func ticketParent(t *testing.T, h *servertest.Harness, id string) string {
	t.Helper()
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Identifier == id && tk.Parent != nil {
			return *tk.Parent
		}
	}
	return ""
}

// A lettered ticket missing its parent is found by the scan and backfilled by
// R14, and the scan the write triggers raises nothing again.
func TestRecovery_R14BackfillsTheParentAScanFoundMissing(t *testing.T) {
	t.Parallel()
	cat := recovery.Default()
	for i := range cat.Entries {
		cat.Entries[i].Enabled = cat.Entries[i].ID == "R14"
	}
	h := startParentDefect(t, t.TempDir(), cat)
	waitFor(t, func() bool { return len(parentDefectEvents(h)[events.RecoveryApplied]) > 0 })
	if got := ticketParent(t, h, "01a2"); got != "01a" {
		t.Errorf("01a2 parent = %q, want 01a backfilled", got)
	}
	h.Server.Rescan()
	byType := parentDefectEvents(h)
	applied := byType[events.RecoveryApplied]
	if len(applied) != 1 || applied[0].Ticket != "01a2" || applied[0].Reason != "R14" || applied[0].Outcome != "ok" {
		t.Errorf("recovery-applied = %+v, want one R14 ok for 01a2", applied)
	}
	if raised := byType[events.TicketGraphDefect]; len(raised) != 1 || raised[0].Reason != "01a" {
		t.Errorf("ticket-graph-defect = %+v, want one naming 01a", raised)
	}
}

// With R14 disabled the scan raises nothing, so no investigation is forked for
// a defect no rule can fix.
func TestRecovery_ParentDefectScanRaisesNothingWhileR14IsDisabled(t *testing.T) {
	t.Parallel()
	h := startParentDefect(t, t.TempDir(), recovery.Default())
	h.Server.Rescan()
	time.Sleep(200 * time.Millisecond) // negative wait: a raised event would show by now
	if byType := parentDefectEvents(h); len(byType) != 0 {
		t.Errorf("events = %+v, want none", byType)
	}
	assertNoInvestigation(t, h)
}

// Drafts are unfinished: neither a draft ticket nor a ticket of a draft epic
// is raised.
func TestRecovery_ParentDefectScanSkipsDrafts(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01a", "fork", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "epic-b", "ticket.md"), []byte("---\nstatus: draft\n---\n\n# epic-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "01a", "fork", "")
	draft := filepath.Join(store, "proj", "epic-a", "issues", "01a-fork.md")
	data, _ := os.ReadFile(draft)
	if err := os.WriteFile(draft, []byte(strings.Replace(string(data), "status: open\n", "status: draft\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	cat := recovery.Default()
	for i := range cat.Entries {
		cat.Entries[i].Enabled = cat.Entries[i].ID == "R14"
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = cat
	})
	h.Server.Rescan()
	time.Sleep(200 * time.Millisecond) // negative wait: a recovery event would show by now
	for _, epic := range []string{"epic-a", "epic-b"} {
		log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), epic)
		for _, ev := range log {
			if ev.Kind == string(events.ParentDefect) {
				t.Errorf("%s: %+v, want no parent-defect event for a draft", epic, ev)
			}
		}
	}
}

// A person's own pause and a pane blocked on a dialog no rule allows are a
// person's to handle: neither forks an investigation.
func TestRecovery_ManualAndUnmatchedBlockedPanePausesForkNothing(t *testing.T) {
	t.Parallel()
	pause := map[string]func(*servertest.Harness) error{
		"manual pause": func(h *servertest.Harness) error {
			res, err := h.Client.TicketPause(context.Background(), "proj:epic-a/01", "broken")
			if err == nil && res.Refused {
				err = errors.New(res.Reason)
			}
			return err
		},
		"unmatched blocked pane": func(h *servertest.Harness) error {
			return h.Server.PauseAs("proj:epic-a/01", events.BlockedPane, "blocked on a prompt gx did not send")
		},
	}
	for name, do := range pause {
		t.Run(name, func(t *testing.T) {
			h := startUnmatched(t)
			if err := do(h); err != nil {
				t.Fatalf("pause: %v", err)
			}
			time.Sleep(200 * time.Millisecond) // negative wait: an investigation would show by now
			assertNoInvestigation(t, h)
		})
	}
}

func assertNoInvestigation(t *testing.T, h *servertest.Harness) {
	t.Helper()
	log, _, _ := ralphloop.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	for _, ev := range log {
		if strings.HasPrefix(ev.Type, "recovery-") {
			t.Errorf("recovery event %+v, want none", ev)
		}
	}
	for _, tk := range epicTickets(t, h).Tickets {
		if tk.Type == "investigate" {
			t.Errorf("forked investigate ticket %s, want none", tk.Identifier)
		}
	}
}

// A parent that changed between the scan and the remedy is refused, not
// overwritten.
func TestRecovery_SetParentRefusesAParentChangedSinceTheScan(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(store, "proj", "epic-a", "issues", "01a2-fork.md")
	cat := recovery.Catalog{Enabled: true, Entries: []recovery.Entry{{
		ID: "R14", Type: events.TicketGraphDefect, Kind: events.ParentDefect,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true, Verbs: []string{recovery.VerbSetParent},
		Remedy: func(f recovery.Failure, v recovery.Verbs) error {
			// A person re-parents onto another ancestor the ID allows first.
			err := schema.UpdateTicket(path, func(t *schema.Ticket) { id := schema.TicketID("01a1"); t.Parent = &id })
			if err != nil {
				return err
			}
			return recovery.Run(v, recovery.Call{Verb: recovery.VerbSetParent, Address: f.Address, Parent: f.Parent})
		},
	}}}
	h := startParentDefect(t, store, cat)
	waitFor(t, func() bool { return len(parentDefectEvents(h)[events.RecoveryApplied]) > 0 })
	h.Server.Rescan()
	applied := parentDefectEvents(h)[events.RecoveryApplied]
	if len(applied) != 1 || !strings.Contains(applied[0].Outcome, server.ReasonParentChanged) {
		t.Errorf("recovery-applied = %+v, want one parent-changed refusal", applied)
	}
	if got := ticketParent(t, h, "01a2"); got != "01a1" {
		t.Errorf("01a2 parent = %q, want the person's 01a1 kept", got)
	}
}

// A defect the remedy failed to fix stays on disk and the failure is escalated
// once, as no pause reports it, yet later rescans neither raise it again nor
// send it to the guard rails.
func TestRecovery_UnfixedParentDefectIsRaisedOnce(t *testing.T) {
	t.Parallel()
	cat := recovery.Catalog{Enabled: true, Entries: []recovery.Entry{{
		ID: "R14", Type: events.TicketGraphDefect, Kind: events.ParentDefect,
		Executor: recovery.ExecutorRule, Authority: recovery.AuthorityLow, Enabled: true,
		Remedy: func(recovery.Failure, recovery.Verbs) error { return errors.New("refused") },
	}}}
	h := startParentDefect(t, t.TempDir(), cat)
	waitFor(t, func() bool { return len(parentDefectEvents(h)[events.RecoveryApplied]) > 0 })
	h.Server.Rescan()
	h.Server.Rescan()
	time.Sleep(100 * time.Millisecond) // negative wait: a repeat event would show by now
	byType := parentDefectEvents(h)
	escalated := byType[events.RecoveryEscalated]
	if len(byType[events.TicketGraphDefect]) != 1 || len(byType[events.RecoveryApplied]) != 1 || len(escalated) != 1 || escalated[0].Reason != "R14" {
		t.Errorf("events = %+v, want one defect, one applied and one R14 escalation", byType)
	}
}

// watchHeldGate seeds ticket 01's run log, puts its run in the registry with an
// agent in state, and runs one watchdog pass an hour after the start. It
// returns a counter of the gate's recovery-applied events on 01 and a rerun.
func watchHeldGate(t *testing.T, state agentrunner.State, seed ...ralphloop.Event) (applied func() int, pass func()) {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", testutil.TempRepo(t))
	fake, sess := iterationAgent(t, "headless", state)
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.Recovery = recovery.Default()
	})
	dir := filepath.Join(store, "proj")
	for _, ev := range seed {
		ev.Ticket = "01"
		if err := ralphloop.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	h.Server.PutRun("proj:epic-a", server.Run{Address: "proj:epic-a/01", Runner: "headless", Session: sess})
	pass = func() { h.Server.WatchGates(gateWatchStart.Add(time.Hour)) }
	applied = func() int {
		log, _, _ := ralphloop.ReadEvents(dir, "epic-a")
		n := 0
		for _, ev := range log {
			if ev.Ticket == "01" && events.Type(ev.Type) == events.RecoveryApplied && ev.Kind == string(events.BackgroundTaskGate) {
				n++
			}
		}
		return n
	}
	pass()
	return applied, pass
}

var gateWatchStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func gateEvent(typ events.Type, after time.Duration) ralphloop.Event {
	return ralphloop.Event{Type: string(typ), TaskID: "task-1", Reason: "background task task-1", Time: gateWatchStart.Add(after)}
}

func TestGateWatch_TracksTheGateByTaskIDNotReasonText(t *testing.T) {
	t.Parallel()
	otherReleased := gateEvent(events.BackgroundTaskGateReleased, 2*time.Minute)
	otherReleased.TaskID = "task-2"
	applied, _ := watchHeldGate(t, "idle", gateEvent(events.BackgroundTaskGateHeld, time.Minute), otherReleased)
	waitFor(t, func() bool { return applied() == 1 })
}

func TestGateWatch_HeldGateOnAnIdlePaneIsRaisedOnce(t *testing.T) {
	t.Parallel()
	applied, pass := watchHeldGate(t, "idle", gateEvent(events.IterationStarted, 0), gateEvent(events.BackgroundTaskGateHeld, time.Minute))
	waitFor(t, func() bool { return applied() == 1 })
	pass()
	time.Sleep(200 * time.Millisecond) // negative wait: a second recovery would show by now
	if n := applied(); n != 1 {
		t.Errorf("recoveries after a second pass = %d, want 1", n)
	}
}

func TestGateWatch_LeavesAGateThatIsNotStuck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status agentrunner.State
		seed   []ralphloop.Event
	}{
		{"busy pane", "working", []ralphloop.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute)}},
		{"released gate", "idle", []ralphloop.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), gateEvent(events.BackgroundTaskGateReleased, 2*time.Minute)}},
		{"gate recovery forced open", "idle", []ralphloop.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), {
			Type: string(events.BackgroundTaskGateReleased), TaskID: "task-1", Reason: "background task task-1: recovery forced the release", Time: gateWatchStart.Add(2 * time.Minute),
		}}},
		{"held within the quiet period", "idle", []ralphloop.Event{gateEvent(events.BackgroundTaskGateHeld, 59*time.Minute)}},
		{"held in an earlier iteration", "idle", []ralphloop.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), gateEvent(events.IterationStarted, 2*time.Minute)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			applied, _ := watchHeldGate(t, tc.status, tc.seed...)
			time.Sleep(200 * time.Millisecond) // negative wait: a recovery would show by now
			if n := applied(); n != 0 {
				t.Errorf("recoveries = %d, want 0", n)
			}
		})
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	servertest.WaitFor(t, 15*time.Second, "the condition", ok)
}

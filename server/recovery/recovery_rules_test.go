package recovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
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
	servertest.RegisterTurn(h)
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

	var matched, applied *runlog.Event
	servertest.Poll(5*time.Second, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
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
	for _, ev := range []*runlog.Event{matched, applied} {
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
	servertest.RegisterTurn(h)

	if err := h.Server.PauseAs("proj:epic-a/01", events.Spinning, "paused and re-claimed 3 times within 5m0s"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryMatched && ev.Kind == string(events.Spinning) && ev.Reason == "R1" {
				return true
			}
		}
		return false
	})
	log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
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
func pauseStalledLaunch(t *testing.T, promptErr error) (applied runlog.Event, typed []string, stopped bool) {
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
	h.Server.Registry().PutRunFrom("proj:epic-a", runs.Run{Address: addr, Runner: "headless", Session: sess}, "main")
	dir := filepath.Join(store, "proj")
	stalled := runlog.Event{Type: string(events.LaunchFailed), Ticket: "01", Kind: string(events.AgentPromptStalled), Attempt: 1}
	if err := runlog.AppendEvent(dir, "epic-a", stalled); err != nil {
		t.Fatal(err)
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.AgentPromptStalled, "agent_prompt_stalled"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
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
// smart-zone compaction wait timed out, with the agent's launch turn held open
// until finish (if finish) and R7 waiting at most rewait for it. It returns
// R7's applied event plus every prompt the agent got.
func pauseTimedOutCompaction(t *testing.T, rewait time.Duration, finish bool) (applied runlog.Event, prompts []string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Recovery = recovery.Default()
		c.CompactRewaitTimeout = rewait
	})
	var mu sync.Mutex
	var texts []string
	var release func()
	h.OnTurn(func(turn *servertest.Turn) {
		mu.Lock()
		defer mu.Unlock()
		texts = append(texts, turn.Text)
		if len(texts) == 1 { // the launch turn stays open: the agent is mid-compaction
			release = turn.Hold()
		}
	})
	sent := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(texts)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	// A launched run has seen its agent start the held turn; before that the agent is idle.
	// A launch builds a worktree, which a loaded full run can stretch past waitFor's default.
	// The hook records the turn before the agent shows busy, and a wait that
	// saw that idle instant would type the finish-up prompt, so wait for busy.
	servertest.WaitFor(t, time.Minute, "the launch turn is held", func() bool {
		return len(sent()) == 1 && len(h.Server.Runs()) == 1 && h.Server.Budget().LaunchingRuns() == 0 &&
			!h.Server.Scheduler().AgentWaiting("proj", "epic-a", "01")
	})
	dir := filepath.Join(store, "proj")
	if err := runlog.AppendEvent(dir, "epic-a", runlog.Event{Type: string(events.SmartZoneRecoveryFailed), Ticket: "01", Reason: `compacting epic-a-01 after smart-zone breach: {"code":"timeout"}`}); err != nil {
		t.Fatal(err)
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "compact recovery exhausted"); err != nil {
		t.Fatal(err)
	}
	if finish {
		mu.Lock()
		release()
		mu.Unlock()
	}
	servertest.WaitFor(t, time.Minute, "R7 is applied", func() bool {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
		for _, ev := range log {
			if events.Type(ev.Type) == events.RecoveryApplied {
				applied = ev
				return true
			}
		}
		return false
	})
	return applied, sent()
}

func TestRecovery_TimedOutCompactionIsReWaitedOnceThenFinishedUpByR7(t *testing.T) {
	t.Parallel()
	applied, prompts := pauseTimedOutCompaction(t, 30*time.Second, true)
	if applied.Reason != "R7" || applied.Kind != string(events.IterationError) || applied.Outcome != "ok" {
		t.Errorf("applied = %+v, want R7 ok", applied)
	}
	if len(prompts) != 2 || prompts[1] != recovery.FinishUpPrompt {
		t.Errorf("prompts = %q, want the launch prompt then only the finish-up prompt, never /compact", prompts)
	}
}

func TestRecovery_SecondCompactionTimeoutLeavesThePauseToAPerson(t *testing.T) {
	t.Parallel()
	applied, prompts := pauseTimedOutCompaction(t, 200*time.Millisecond, false)
	if applied.Reason != "R7" || applied.Outcome == "ok" {
		t.Errorf("applied = %+v, want a failed R7", applied)
	}
	if len(prompts) != 1 {
		t.Errorf("prompts = %q, want only the launch prompt", prompts)
	}
}

// pauseAfterSeeding appends seed events to ticket 01's run log (as a previous
// server run would have left them), pauses it, and returns the escalation, or
// nil once the pause's own recovery is applied instead.
func pauseAfterSeeding(t *testing.T, seed ...runlog.Event) (*runlog.Event, <-chan recovery.Result) {
	t.Helper()
	h, results := startRecovery(t, false)
	dir := filepath.Join(h.TicketStore, "proj")
	for _, ev := range seed {
		ev.Ticket = "01"
		if err := runlog.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	var escalated *runlog.Event
	servertest.Poll(2*time.Second, func() bool {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
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

func applied(kind events.Kind) runlog.Event {
	return runlog.Event{Type: string(events.RecoveryApplied), Kind: string(kind), Reason: "TEST", Outcome: "ok"}
}

func TestRecovery_SecondFailureOfTheSameKindEscalatesInsteadOfRecovering(t *testing.T) {
	t.Parallel()
	// A merging between the two clears the failed-recovery rule, leaving the cap.
	esc, results := pauseAfterSeeding(t, applied(events.IterationError), runlog.Event{Type: string(events.CherryPicked)})
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
	merged := runlog.Event{Type: string(events.CherryPicked)}
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
	for name, seed := range map[string]runlog.Event{
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
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
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
	res, err := h.Client.Tickets.Approve(ctx, "proj:epic-a/01")
	if err != nil || res.Refused {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	_ = results
	log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	var got *runlog.Event
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
	if res, err := h.Client.Tickets.Approve(ctx, "proj:epic-a/01"); err != nil || res.Refused {
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
	if res, err := h.Client.Tickets.Approve(ctx, "proj:epic-a/01"); err != nil || res.Reason != wire.ReasonProposalStale {
		t.Errorf("approve = %+v, %v, want proposal-stale", res, err)
	}
	if proposedRemedy(t, path) == "" {
		t.Error("a stale approve retired the proposal")
	}
}

// startUnmatched starts a server whose catalog matches nothing, so every pause
// needs judgment, and queues ticket 02 behind the one that will be paused.

package recovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

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
	applied   runlog.Event
	escalated *runlog.Event
	log       []runlog.Event
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
	h.Server.Registry().PutRunFrom("proj:epic-a", runs.Run{Address: addr, Runner: runner, Session: sess}, "main")
	dir := filepath.Join(store, "proj")
	held := runlog.Event{Type: string(events.BackgroundTaskGateHeld), Ticket: "01", Reason: "background task task-1"}
	if err := runlog.AppendEvent(dir, "epic-a", held); err != nil {
		t.Fatal(err)
	}
	h.Server.Recovery().RecoverAsync(recovery.Failure{Address: addr, Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: held.Reason})
	var got gateRaise
	waitFor(t, func() bool {
		if h.Server.Registry().IsGateReleased(addr) {
			h.Server.SettleReleasedRun(addr)
		}
		got.log, _, _ = runlog.ReadEvents(dir, "epic-a")
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
			if got.applied.Reason != "R10" || !strings.Contains(got.applied.Outcome, wire.ReasonAgentBusy) {
				t.Errorf("applied = %+v, want a failed R10 naming %s", got.applied, wire.ReasonAgentBusy)
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
func parentDefectEvents(h *servertest.Harness) map[events.Type][]runlog.Event {
	log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
	byType := map[events.Type][]runlog.Event{}
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
		log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), epic)
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
	log, _, _ := runlog.ReadEvents(filepath.Join(h.TicketStore, "proj"), "epic-a")
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
	if len(applied) != 1 || !strings.Contains(applied[0].Outcome, wire.ReasonParentChanged) {
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
func watchHeldGate(t *testing.T, state agentrunner.State, seed ...runlog.Event) (applied func() int, pass func()) {
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
		if err := runlog.AppendEvent(dir, "epic-a", ev); err != nil {
			t.Fatal(err)
		}
	}
	h.Server.Registry().PutRun("proj:epic-a", runs.Run{Address: "proj:epic-a/01", Runner: "headless", Session: sess})
	pass = func() { h.Server.Scheduler().WatchGates(gateWatchStart.Add(time.Hour)) }
	applied = func() int {
		log, _, _ := runlog.ReadEvents(dir, "epic-a")
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

func gateEvent(typ events.Type, after time.Duration) runlog.Event {
	return runlog.Event{Type: string(typ), TaskID: "task-1", Reason: "background task task-1", Time: gateWatchStart.Add(after)}
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
		seed   []runlog.Event
	}{
		{"busy pane", "working", []runlog.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute)}},
		{"released gate", "idle", []runlog.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), gateEvent(events.BackgroundTaskGateReleased, 2*time.Minute)}},
		{"gate recovery forced open", "idle", []runlog.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), {
			Type: string(events.BackgroundTaskGateReleased), TaskID: "task-1", Reason: "background task task-1: recovery forced the release", Time: gateWatchStart.Add(2 * time.Minute),
		}}},
		{"held within the quiet period", "idle", []runlog.Event{gateEvent(events.BackgroundTaskGateHeld, 59*time.Minute)}},
		{"held in an earlier iteration", "idle", []runlog.Event{gateEvent(events.BackgroundTaskGateHeld, time.Minute), gateEvent(events.IterationStarted, 2*time.Minute)}},
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

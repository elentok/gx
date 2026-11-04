package ralphloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	eventsc "github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// runUntilPaused runs Run in the background, returns once it has paused —
// the honest end of a test whose epic finishes on a human-clearable ticket
// nobody clears — then cancels it and waits for Run to actually return
// before handing control back to the caller, so no goroutine of Run's
// (including the merge-queue worker) survives the test. The run stays
// blocked in its pause select (the timer it is handed never fires) until
// that cancellation, so it neither spins nor reports a terminal event
// while paused, and the close of the pause signal orders everything Run
// wrote before pausing ahead of the caller's assertions.
func runUntilPaused(t *testing.T, opts RunOptions, d Deps, sink EventSink) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts.Ctx = ctx
	paused := make(chan struct{})
	never := make(chan time.Time)
	var once sync.Once
	d.PauseTimer = func(time.Duration) <-chan time.Time {
		once.Do(func() { close(paused) })
		return never
	}
	done := make(chan error, 1)
	go func() {
		done <- Run(opts, d, sink)
	}()
	select {
	case <-paused:
	case err := <-done:
		t.Fatalf("Run() returned %v, want it to pause on a ticket only a human can clear", err)
	case <-time.After(30 * time.Second):
		t.Fatal("Run() never paused")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() returned %v after cancel, want nil or context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run() never returned after cancel")
	}
}

func TestRun_LinearChain_RunsTicketsInOrderAndMergesAll(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\"]\n---\n# Second\n",
	})
	d, prompts, removed := fakeDeps()

	sink := newRecordingEventSink()
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := []string{
		"/implement " + ticketAddr(scratchDir, "my-epic", "01"),
		"/implement " + ticketAddr(scratchDir, "my-epic", "02"),
	}
	if len(*prompts) != 2 || (*prompts)[0] != wantPrompts[0] || (*prompts)[1] != wantPrompts[1] {
		t.Fatalf("prompts = %v, want %v", *prompts, wantPrompts)
	}

	wantBranches := []string{"ralph-loop/my-epic-item-01", "ralph-loop/my-epic-item-02"}
	if len(*removed) != 2 || (*removed)[0] != wantBranches[0] || (*removed)[1] != wantBranches[1] {
		t.Fatalf("removed worktree branches = %v, want %v", *removed, wantBranches)
	}

	for _, name := range []string{"01-first.md", "02-second.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	if !hasEvent(sink, LiveEventEpicComplete, func(ev LiveEvent) bool { return ev.Completed == 2 }) {
		t.Errorf("events = %+v, want an EpicComplete event reporting 2 completed tickets", sink.Events())
	}
}

// TestRun_IterationCompletion_DeletesIterationBranch exercises ticket 04's
// AC that the normal same-run success path deletes a merged iteration's
// now-redundant branch — something it never did before this ticket.
func TestRun_IterationCompletion_DeletesIterationBranch(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	var deletedBranches []string
	d.DeleteBranch = func(repoDir, branch string) error {
		deletedBranches = append(deletedBranches, branch)
		return nil
	}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(deletedBranches) != 1 || deletedBranches[0] != "ralph-loop/my-epic-item-01" {
		t.Errorf("deletedBranches = %v, want [ralph-loop/my-epic-item-01]", deletedBranches)
	}
}

func TestRun_LogsLifecycleEvents_LinearChain(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
	})
	d, _, _ := fakeDeps()

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	rawEvents, ok, err := ReadEvents(scratchDir, "my-epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	// scheduler-scan events are logged on every claimNext pass, interleaved
	// with (and racing) the async iteration lifecycle below; this test is
	// about that lifecycle's exact order, not the scheduler's own scanning,
	// so exclude them here rather than pin down their nondeterministic count
	// and position.
	var events []Event
	for _, ev := range rawEvents {
		if ev.Type != string(eventsc.SchedulerScan) {
			events = append(events, ev)
		}
	}

	wantTypes := []string{string(eventsc.DepsInstalled), string(eventsc.IterationStarted), string(eventsc.IterationFinished), string(eventsc.CherryPicked)}
	if len(events) != len(wantTypes) {
		t.Fatalf("events = %+v, want %d events (%v)", events, len(wantTypes), wantTypes)
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Errorf("events[%d].Type = %q, want %q", i, events[i].Type, want)
		}
		if events[i].Ticket != "01" {
			t.Errorf("events[%d].Ticket = %q, want %q", i, events[i].Ticket, "01")
		}
	}
	wantSession := "sess-pane-" + iterLabel("my-epic", "01")
	if events[1].AgentSession != wantSession {
		t.Errorf("iteration-started AgentSession = %q, want the agent's session id", events[1].AgentSession)
	}
	// cherry-picked also carries the iteration agent's session/cwd, not just
	// the start/finish pair, since the spec requires it on every event type.
	if events[3].AgentSession != wantSession || events[3].Cwd == "" {
		t.Errorf("cherry-picked event = %+v, want AgentSession=%q and a non-empty Cwd", events[3], wantSession)
	}
	if events[1].Pane == "" {
		t.Errorf("iteration-started event = %+v, want non-empty Pane/Tab", events[1])
	}
	if events[0].Cwd == "" {
		t.Errorf("deps-installed event = %+v, want non-empty Cwd", events[0])
	}
}

// TestRun_SchedulerScan_LogsOutOfScopeTicket covers the case that motivated
// string(eventsc.SchedulerScan): a ticket present in the epic but outside the run's
// RunScope (here, ticket 02 has no parent: pointing back into the requested
// "01", so RunScope.Contains never picks it up) looks, from the Queue tab,
// like it's just sitting there unclaimed — the scheduler-scan log line is
// what lets that be told apart from "still blocked" or "already claimed
// elsewhere".
func TestRun_SchedulerScan_LogsOutOfScopeTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()

	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01"},
	}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := ReadEvents(scratchDir, "my-epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}

	var found bool
	for _, ev := range events {
		if ev.Type != string(eventsc.SchedulerScan) {
			continue
		}
		for _, d := range ev.Scan {
			if d.Ticket == "02" {
				found = true
				if d.Decision != "out-of-scope" {
					t.Errorf("ticket 02 scan decision = %q, want %q", d.Decision, "out-of-scope")
				}
			}
		}
	}
	if !found {
		t.Fatalf("no scheduler-scan event scanned ticket 02; events = %+v", events)
	}
}

// TestRun_SchedulerScan_SplitsBigEpicAcrossLines covers a 77-ticket epic
// whose one-line scan was over events.MaxLineBytes: the log error failed the
// run on its first claim. The scan must instead split across lines that each
// fit, and one pass's lines (same Time) must still cover every ticket once.
func TestRun_SchedulerScan_SplitsBigEpicAcrossLines(t *testing.T) {
	t.Parallel()
	files := map[string]string{}
	for i := 1; i <= 120; i++ {
		id := fmt.Sprintf("%02d", i)
		files[id+"-ticket.md"] = "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\n---\n# T\n"
	}
	scratchDir := writeEpic(t, "my-epic", files)
	d, _, _ := fakeDeps()

	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01"},
	}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(runLogPath(scratchDir, "my-epic"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if len(line)+1 > eventsc.MaxLineBytes {
			t.Errorf("line %d is %d bytes, over the %d cap", i, len(line)+1, eventsc.MaxLineBytes)
		}
	}

	evs, _, err := ReadEvents(scratchDir, "my-epic")
	if err != nil {
		t.Fatal(err)
	}
	var first time.Time
	lines := 0
	seen := map[string]int{}
	for _, ev := range evs {
		if ev.Type != string(eventsc.SchedulerScan) {
			continue
		}
		if first.IsZero() {
			first = ev.Time
		}
		if !ev.Time.Equal(first) {
			continue
		}
		lines++
		for _, d := range ev.Scan {
			seen[d.Ticket]++
		}
	}
	if lines < 2 {
		t.Errorf("first pass logged %d scan lines, want it split across several", lines)
	}
	if len(seen) != 120 {
		t.Errorf("first pass scanned %d distinct tickets, want 120", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("ticket %s appears %d times in one pass, want 1", id, n)
		}
	}
}

// TestRun_FreshIteration_StampsCompactionsOnDone verifies ticket 04: a
// ticket marked done from a fresh iteration whose transcript recorded
// compaction boundaries gets that count written into its frontmatter's
// compactions field alongside status: done.
func TestRun_FreshIteration_StampsCompactionsOnDone(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 12345, true, nil
	}
	d.ReadCompactions = func(cwd, sessionID string) (int, bool, error) {
		return 3, true, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.Compactions != 3 {
		t.Errorf("Compactions = %d, want 3", got.Compactions)
	}
}

// TestRun_FreshIteration_OmitsCompactionsWhenUnavailable verifies that a
// fresh iteration whose compaction count can't be read (ReadCompactions'
// default fake behavior in fakeDeps, which leaves it nil) still marks the
// ticket done, without writing a wrong/placeholder compactions count.
func TestRun_FreshIteration_OmitsCompactionsWhenUnavailable(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.ReadOccupancy = func(cwd, sessionID string) (int, bool, error) {
		return 12345, true, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := mustParse(t, filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", got.Status)
	}
	if got.Compactions != 0 {
		t.Errorf("Compactions = %d, want 0 (unavailable)", got.Compactions)
	}
}

func TestRun_LogsAskingEvent_OnZeroCommitIteration(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	// The paused (answer) ticket is the epic's only one, so the run pauses on it.
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var asking *Event
	for i, ev := range events {
		if ev.Type == string(eventsc.Paused) && ev.Ticket == "01" {
			asking = &events[i]
		}
		if ev.Type == string(eventsc.CherryPicked) {
			t.Errorf("events = %+v, want no cherry-picked event for a zero-commit iteration", events)
		}
	}
	if asking == nil {
		t.Fatalf("events = %+v, want a paused (answer) event for ticket 1", events)
	}
	if asking.AgentSession == "" {
		t.Errorf("paused (answer) event = %+v, want a non-empty AgentSession (the agent_session that produced zero commits)", asking)
	}
}

func TestRun_EventSink_TicketAsking_OnZeroCommitIteration(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	sink := &recordingSink{}
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	if len(sink.ticketNeedsHumanCalls) != 1 {
		t.Fatalf("TicketNeedsHuman calls = %v, want exactly 1", sink.ticketNeedsHumanCalls)
	}
	if got := sink.ticketNeedsHumanCalls[0]; got[0] != "01" || got[1] != "epic" || got[2] != string(schema.StatusLegacyAnswer) {
		t.Errorf("TicketNeedsHuman call = %v, want [01 epic %s ...]", got, schema.StatusLegacyAnswer)
	}
}

// TestRun_HonorsCommitlessFlag_SkipsAsking exercises the escape hatch from
// the zero-commit path above: if the agent reports iteration_status: finished
// with commitless: true before finishing, that's an intentional zero-commit
// finish (e.g. exploration concluded no code change was warranted), not a
// stalled agent — the ticket must not be forced back to paused (answer), gx
// itself writes status: done, and its worktree/tab get cleaned up like a
// normal completion.
func TestRun_HonorsCommitlessFlag_SkipsAsking(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, removedBranches := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	onRunnerPrompt(d, func(agentrunner.Session, string) error {
		// Simulate the agent calling `gx tickets set --iteration-status
		// finished --commitless true` on itself before going idle with no
		// commit.
		return updateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatusFinished
			tk.Commitless = true
		})
	})

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := ReadEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("ReadEvents: ok=%v err=%v", ok, err)
	}
	var commitless *Event
	for i, ev := range events {
		if ev.Type == string(eventsc.Commitless) && ev.Ticket == "01" {
			commitless = &events[i]
		}
		if ev.Type == string(eventsc.Paused) {
			t.Errorf("events = %+v, want no paused event for a declared-commitless iteration", events)
		}
	}
	if commitless == nil {
		t.Fatalf("events = %+v, want a commitless event for ticket 01", events)
	}

	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket file = %q, want status: done (agent-set status preserved)", raw)
	}

	if len(*removedBranches) == 0 {
		t.Errorf("removedBranches = %v, want the iteration worktree cleaned up like a normal completion", *removedBranches)
	}
}

func TestRun_InstallDepsFailure_MarksRepairWithoutLaunchingAgentOrAbortingRun(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
	})
	d, prompts, _ := fakeDeps()
	d.InstallDeps = func(path, logDir string) ([]string, error) {
		return nil, errors.New("npm ci: exit status 1")
	}

	// A failed iteration marks paused (repair) rather than aborting the run,
	// which leaves the epic paused on its only ticket.
	runUntilPaused(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no agent launched after a failed dependency install", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	assertRawPaused(t, string(raw), schema.PauseClassRepair, schema.PauseIterationError)
}

func TestRun_ZeroOpenTickets_NoOpSummary(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# First\n",
	})
	d, prompts, removed := fakeDeps()

	sink := newRecordingEventSink()
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 0 || len(*removed) != 0 {
		t.Fatalf("expected no iterations to run, got prompts=%v removed=%v", *prompts, *removed)
	}
	if !hasEvent(sink, LiveEventEpicStarted, func(ev LiveEvent) bool { return ev.Total > 0 && ev.Done == ev.Total }) {
		t.Errorf("events = %+v, want an EpicStarted event reporting the epic already complete", sink.Events())
	}
}

func TestRun_NoEpicFound_NoOpSummary(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	d, _, _ := fakeDeps()

	sink := newRecordingEventSink()
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !hasEvent(sink, LiveEventEpicStarted, func(ev LiveEvent) bool { return ev.Total == 0 }) {
		t.Errorf("events = %+v, want an EpicStarted event reporting nothing to do", sink.Events())
	}
}

// TestAllDone_RepairIsNotComplete covers ticket 08's central
// inversion: a paused (repair) ticket used to count as terminal and end the
// run, and now must not — the run pauses on it instead.
func TestAllDone_RepairIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "paused", PauseReason: schema.PauseIterationError},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 paused for repair")
	}
}

func TestAllDone_OpenTicketIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 is still open")
	}
}

func TestAllDone_EveryTicketDone(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "done"},
	}}
	if !allDone(epic) {
		t.Errorf("allDone() = false, want true when every ticket is done")
	}
}

// TestAllDone_WaitingForChildrenNotDone covers ticket 05: a done ticket whose
// fork subtree (Parent, ticket 03) is still open renders as
// waiting-for-children, which must not count as terminal — otherwise the loop
// (and StampEpicCompleted) would report an epic complete with unfinished
// forked work still inside it.
func TestAllDone_WaitingForChildrenNotDone(t *testing.T) {
	t.Parallel()
	parent := "1"
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 1, Identifier: "01a", Status: "open", Parent: &parent},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false: ticket 01 is waiting-for-children")
	}
}

// TestRun_TicketSubset_CompletesWithoutTouchingTicketsOutsideSubset covers
// ticket 02's requirement: a caller-supplied RunOptions.TicketIDs subset of a
// larger epic runs and merges only those tickets, and Run exits once they're
// done even though a third, unblocked epic ticket is left open.
func TestRun_TicketSubset_CompletesWithoutTouchingTicketsOutsideSubset(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: implement\n---\n# Third\n",
	})
	d, prompts, _ := fakeDeps()

	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01", "02"},
	}, d, noopEventSink{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := []string{
		"/implement " + ticketAddr(scratchDir, "my-epic", "01"),
		"/implement " + ticketAddr(scratchDir, "my-epic", "02"),
	}
	// 01 and 02 have no blocked_by relation, so they run concurrently
	// (defaultMaxParallel) and can finish in either order.
	gotPrompts := append([]string(nil), *prompts...)
	sort.Strings(gotPrompts)
	sort.Strings(wantPrompts)
	if len(gotPrompts) != 2 || gotPrompts[0] != wantPrompts[0] || gotPrompts[1] != wantPrompts[1] {
		t.Fatalf("prompts = %v, want %v (ticket 03 must never be launched)", *prompts, wantPrompts)
	}

	for _, name := range []string{"01-first.md", "02-second.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", "03-third.md"))
	if err != nil {
		t.Fatalf("ReadFile 03-third.md: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("03-third.md = %q, want it left open (outside the subset)", raw)
	}
}

// TestRun_ScopeWidenedMidRun_TotalGrowsWithIt is a regression test for the
// notification total-count bug: total was captured once from the pre-run
// scope snapshot, so a ticket added to a running epic's scope mid-run (via
// the TUI's 'a' key widening RunScope) was counted in Completed once it
// merged but never grew Total to match, producing nonsensical stats like
// Completed > Total. Ticket 01 is the only ticket originally in scope;
// while its iteration is still running, its Runner wait hook widens the
// live scope to also include ticket 02 (simulating the TUI action), so by
// the time ticket 01's IterationFinished fires, Total must already reflect
// both tickets even though only one has merged.
func TestRun_ScopeWidenedMidRun_TotalGrowsWithIt(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()

	var scope RunScope
	var widenOnce sync.Once
	onRunnerWait(d, func(s agentrunner.Session) {
		if s.Label == iterLabel("my-epic", "01") {
			widenOnce.Do(func() { scope.Add("02") })
		}
	})

	sink := &recordingSink{}

	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01"},
		OnScopeResolved: func(s RunScope) {
			scope = s
		},
	}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	firstStats, ok := sink.iterationStatsByTicket["01"]
	if !ok {
		t.Fatalf("no IterationFinished recorded for ticket 01")
	}
	if firstStats.Total != 2 {
		t.Errorf("ticket 01 stats.Total = %d, want 2 (mid-run Add must grow Total before it merges)", firstStats.Total)
	}
	if firstStats.Completed > firstStats.Total {
		t.Errorf("ticket 01 stats: Completed %d > Total %d", firstStats.Completed, firstStats.Total)
	}

	secondStats, ok := sink.iterationStatsByTicket["02"]
	if !ok {
		t.Fatalf("no IterationFinished recorded for ticket 02")
	}
	if secondStats.Total != 2 {
		t.Errorf("ticket 02 stats.Total = %d, want 2", secondStats.Total)
	}
	if secondStats.Completed > secondStats.Total {
		t.Errorf("ticket 02 stats: Completed %d > Total %d", secondStats.Completed, secondStats.Total)
	}

	// The scheduler never promised which of the two concurrently-eligible
	// tickets' IterationFinished fires first once 02 is widened into scope
	// mid-run (both become runnable near-simultaneously) — only that,
	// together, they merge Completed={1,2} exactly once each. Asserting a
	// fixed ticket-to-Completed mapping made this test an intermittent CI
	// flake whenever 02 happened to finish first.
	gotCompleted := []int{firstStats.Completed, secondStats.Completed}
	sort.Ints(gotCompleted)
	if !(gotCompleted[0] == 1 && gotCompleted[1] == 2) {
		t.Errorf("Completed counts across tickets 01/02 = %v, want {1,2} in some order", gotCompleted)
	}
}

// TestRun_ResumedRun_ReportsEpicWideDoneNotRunLocalCount is a regression
// test for ticket 26: a resumed run only merges one ticket itself, but
// IterationStats.Completed must count every done ticket on disk (including
// ones done before this Run call started), not just the ones this run
// merged — otherwise a resumed run understates progress ("1/10 done" when
// six of ten are already done).
func TestRun_ResumedRun_ReportsEpicWideDoneNotRunLocalCount(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: done\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()
	sink := &recordingSink{}

	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
	}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	stats, ok := sink.iterationStatsByTicket["02"]
	if !ok {
		t.Fatalf("no IterationFinished recorded for ticket 02")
	}
	if stats.Completed != 2 {
		t.Errorf("ticket 02 stats.Completed = %d, want 2 (ticket 01 was already done on disk before this run started)", stats.Completed)
	}
	if stats.Total != 2 {
		t.Errorf("ticket 02 stats.Total = %d, want 2", stats.Total)
	}
}

// TestRun_RepairOutsideSubset_DoesNotPauseRun covers ticket 23's
// requirement: a paused (repair) ticket left outside the requested subset
// must not gate-pause scheduling of the tickets the caller actually asked
// for.
func TestRun_RepairOutsideSubset_DoesNotPauseRun(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: paused\npause_reason: iteration-error\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
	})
	d, prompts, _ := fakeDeps()

	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"02"},
	}, d, noopEventSink{})
	if err != nil {
		t.Fatalf("Run() error = %v, want the unselected paused (repair) ticket 01 to not block scheduling ticket 02", err)
	}

	wantPrompt := "/implement " + ticketAddr(scratchDir, "my-epic", "02")
	if len(*prompts) != 1 || (*prompts)[0] != wantPrompt {
		t.Fatalf("prompts = %v, want [%q]", *prompts, wantPrompt)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", "02-second.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: done") {
		t.Errorf("ticket 02 not marked done:\n%s", raw)
	}
}

// TestRun_RepairInsideSubset_RunsTheRestThenPauses covers the flip
// side of the above: a paused (repair) ticket the caller did select no longer
// stops the run outright. It's human-clearable, so the rest of the subset is
// scheduled first and the run only pauses once nothing else is runnable.
func TestRun_RepairInsideSubset_RunsTheRestThenPauses(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: paused\npause_reason: iteration-error\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# Second\n",
	})
	d, prompts, _ := fakeDeps()

	sink := &recordingSink{}
	runUntilPaused(t, RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01", "02"},
	}, d, sink)

	if len(*prompts) == 0 {
		t.Fatalf("prompts = %v, want ticket 02 launched rather than held behind ticket 01", *prompts)
	}
}

// TestRun_ClaimNext_IgnoresExternalRevertOfAlreadyLaunchedTicket is a
// regression test for the iter-06b agent_name_taken race: ticket files are
// shared, unlocked plain files, and something outside this Run call (e.g. a
// parent split ticket's agent still writing to its child's file after
// handoff) can revert an already-claimed ticket's status back to "open"
// mid-iteration. Without launched-set tracking in claimNext, the scheduler
// scan triggered when a sibling ticket frees a slot would see that revert
// and reclaim + relaunch the same ticket, calling AgentStart a second time
// under its deterministic herdr agent name. Ticket 01 is held mid-iteration
// (blocked in AgentPrompt) while ticket 02's AgentPrompt call simulates the
// external clobber of 01's file, then ticket 02 runs to completion — freeing
// a scheduling slot and triggering exactly the scan that must not reclaim
// 01.
func TestRun_ClaimNext_IgnoresExternalRevertOfAlreadyLaunchedTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\n# B\n",
	})
	ticket01Path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, _ := fakeDeps()

	label01 := iterLabel("epic", "01")
	label02 := iterLabel("epic", "02")

	var mu sync.Mutex
	agentStartCalls := map[string]int{}
	unblock01 := make(chan struct{})
	var clobberOnce sync.Once

	onRunnerStart(d, func(opts agentrunner.StartOptions) error {
		mu.Lock()
		agentStartCalls[opts.Label]++
		mu.Unlock()
		return nil
	})
	onRunnerPrompt(d, func(s agentrunner.Session, _ string) error {
		if s.Label == label02 {
			clobberOnce.Do(func() {
				if err := os.WriteFile(ticket01Path, []byte("---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# A\n"), 0644); err != nil {
					t.Errorf("simulating external clobber of ticket 01: %v", err)
				}
			})
		}
		if s.Label == label01 {
			<-unblock01
		}
		return nil
	})
	tabClosed02 := make(chan struct{})
	var tabClosed02Once sync.Once
	onRunnerStop(d, func(s agentrunner.Session) {
		if s.Label == iterLabel("epic", "02") {
			tabClosed02Once.Do(func() { close(tabClosed02) })
		}
	})

	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})
	}()

	select {
	case <-tabClosed02:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ticket 02 to finish")
	}
	// Give the scheduler scan that 02's finish triggers (the one that must
	// not reclaim 01) time to run before releasing 01.
	time.Sleep(50 * time.Millisecond)
	close(unblock01)

	if err := <-runErr; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if agentStartCalls[label01] != 1 {
		t.Errorf("Start calls for ticket 01 = %d, want exactly 1 (claimNext must not re-claim a ticket it already launched, even after an external write reverted its on-disk status)", agentStartCalls[label01])
	}
}

// TestRun_SelectingBlockedTicketThenEditingBlockersRunsCorrectMultiWave is
// ticket 25's end-to-end regression: a user selects a blocked ticket (the
// Tickets tab's checked.go cascades in its blocker automatically, so the
// requested subset here starts as [01 02]), then edits the ticket's Blocked
// by: list before ever running it. PlanWaves — the same canonical planner
// Queue previews — and Run must agree at every step: while 02 still has an
// unresolved blocker outside the requested subset (03, added by the edit and
// never selected), neither may treat it as runnable; once the edit is
// reverted, both must agree on the resulting two-wave shape, and 03 — never
// selected — must stay untouched throughout.
func TestRun_SelectingBlockedTicketThenEditingBlockersRunsCorrectMultiWave(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: implement\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\"]\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: implement\n---\n# Third\n",
	})
	requested := []string{"01", "02"} // mirrors checked.go's blocker cascade at selection time, before 03 ever enters the picture

	ticketPath := filepath.Join(scratchDir, "my-epic", "issues", "02-second.md")
	planFor := func() ([][]tickets.Ticket, error) {
		epic, err := loadNamedEpic(scratchDir, "my-epic")
		if err != nil || epic == nil {
			t.Fatalf("loadNamedEpic: err=%v epic=%v", err, epic)
		}
		scope, err := ResolveRunScope(*epic, requested)
		if err != nil {
			t.Fatalf("ResolveRunScope: %v", err)
		}
		return PlanWaves(*epic, scope, 2)
	}

	// Edit 02's blockers to require 03 too — a dependency this run's
	// selection never picked up. The plan must surface this as stuck, not a
	// misleading runnable wave, once 01 merges.
	if err := os.WriteFile(ticketPath, []byte("---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\", \"03\"]\n---\n# Second\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := planFor(); err == nil {
		t.Fatalf("PlanWaves() error = nil, want a stuck-plan error while 02 needs unselected blocker 03")
	}

	// Edit again, dropping the unselected blocker — the plan should now
	// resolve into the two waves 01's-then-02's chain always implied.
	if err := os.WriteFile(ticketPath, []byte("---\nid: \"02\"\nstatus: open\ntype: implement\nblocked_by: [\"01\"]\n---\n# Second\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	waves, err := planFor()
	if err != nil {
		t.Fatalf("PlanWaves() error = %v", err)
	}
	if len(waves) != 2 || len(waves[0]) != 1 || waves[0][0].DisplayNumber() != "01" || len(waves[1]) != 1 || waves[1][0].DisplayNumber() != "02" {
		t.Fatalf("PlanWaves() = %v, want [[01] [02]]", waves)
	}

	d, prompts, _ := fakeDeps()
	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  requested,
	}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := []string{
		"/implement " + ticketAddr(scratchDir, "my-epic", "01"),
		"/implement " + ticketAddr(scratchDir, "my-epic", "02"),
	}
	if len(*prompts) != 2 || (*prompts)[0] != wantPrompts[0] || (*prompts)[1] != wantPrompts[1] {
		t.Fatalf("prompts = %v, want %v — the exact membership and order PlanWaves showed", *prompts, wantPrompts)
	}

	for _, name := range []string{"01-first.md", "02-second.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "status: done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", "03-third.md"))
	if err != nil {
		t.Fatalf("ReadFile 03-third.md: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("03-third.md = %q, want it left untouched (never selected)", raw)
	}
}

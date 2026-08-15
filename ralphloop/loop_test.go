package ralphloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// writeEpic builds a fixture epic directory under a fresh t.TempDir()'s
// .scratch/{name}/issues/, with one ticket file per entry in tickets (each a
// "NN-slug.md" -> content pair), and returns the scratch dir.
func writeEpic(t *testing.T, epicName string, tickets map[string]string) string {
	t.Helper()
	scratchDir := t.TempDir()
	issuesDir := filepath.Join(scratchDir, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, content := range tickets {
		if err := os.WriteFile(filepath.Join(issuesDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return scratchDir
}

// testDeps is DefaultDeps for a test: real behavior everywhere except that a
// park polls at full speed rather than on the wall clock, so a test that parks
// unexpectedly surfaces as its own failure instead of a multi-minute wait.
func testDeps() Deps {
	d := DefaultDeps()
	d.ParkTimer = readyTimer
	return d
}

// testDepsWithOverrides is testDeps with DepsOverrides applied, for tests
// that need to isolate HOME/CODEX_HOME/PATH without mutating process env via
// t.Setenv.
func testDepsWithOverrides(overrides DepsOverrides) Deps {
	d := DefaultDepsWithOverrides(overrides)
	d.ParkTimer = readyTimer
	return d
}

// setProcessEnv points the real process env var key at value for the rest of
// the test, restored on cleanup via a manual os.Setenv/os.Unsetenv pair
// rather than t.Setenv (which panics once a test has called t.Parallel()).
// Only for tests driving herdrfake helpers (RegisterCodexRollout,
// NewClaudeCompact) that read real process env directly and have no
// DepsOverrides-style seam of their own — everything else should use
// testDepsWithOverrides instead. A test using this must stay non-parallel
// with any other test that also touches the same env var.
func setProcessEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// runUntilParked runs Run in the background, returns once it has parked —
// the honest end of a test whose epic finishes on a human-clearable ticket
// nobody clears — then cancels it and waits for Run to actually return
// before handing control back to the caller, so no goroutine of Run's
// (including the land-queue worker) survives the test. The run stays
// blocked in its park select (the timer it is handed never fires) until
// that cancellation, so it neither spins nor reports a terminal event
// while parked, and the close of the park signal orders everything Run
// wrote before parking ahead of the caller's assertions.
func runUntilParked(t *testing.T, opts RunOptions, d Deps, sink EventSink) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opts.Ctx = ctx
	parked := make(chan struct{})
	never := make(chan time.Time)
	var once sync.Once
	d.ParkTimer = func(time.Duration) <-chan time.Time {
		once.Do(func() { close(parked) })
		return never
	}
	done := make(chan error, 1)
	go func() {
		done <- Run(opts, d, sink)
	}()
	select {
	case <-parked:
	case err := <-done:
		t.Fatalf("Run() returned %v, want it to park on a ticket only a human can clear", err)
	case <-time.After(30 * time.Second):
		t.Fatal("Run() never parked")
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

// fakeDeps returns a Deps wired to in-memory fakes plus a record of prompt
// texts sent (in call order) and worktree branches removed, for assertions.
// All fake operations are safe to call concurrently, since Run may run
// multiple iterations in parallel.
func fakeDeps() (d Deps, prompts *[]string, removedBranches *[]string) {
	var mu sync.Mutex
	promptsSlice := []string{}
	removedSlice := []string{}
	branchByPath := map[string]string{}

	d = Deps{
		AgentGet: func(target string) (herdr.Agent, error) {
			return herdr.Agent{PaneID: "pane-" + target, WorkspaceID: "ws1", TabID: "tab-" + target, AgentStatus: "working", AgentSession: "session-" + target}, nil
		},
		VerifyCodexSession: func(cwd, sessionID string) (bool, error) {
			return true, nil
		},
		FindOrCreateWorkspace: func(label, cwd string) (string, error) {
			return "ws1", nil
		},
		WorktreeDir: func(repoDir string) (string, error) {
			return "/fake/worktrees", nil
		},
		AddWorktree: func(repoDir, path, branch, base string) error {
			mu.Lock()
			branchByPath[path] = branch
			mu.Unlock()
			return nil
		},
		RemoveWorktree: func(repoDir, path string, force bool) error {
			mu.Lock()
			removedSlice = append(removedSlice, branchByPath[path])
			mu.Unlock()
			return nil
		},
		DeleteBranch: func(repoDir, branch string) error {
			return nil
		},
		TabCreate: func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
			return herdr.CreatedTab{
				Tab:        herdr.Tab{TabID: "tab-" + opts.Label, Label: opts.Label, WorkspaceID: opts.WorkspaceID},
				RootPaneID: "pane-" + opts.Label,
			}, nil
		},
		TabClose: func(tabID string) error {
			return nil
		},
		TabList: func(workspaceID string) ([]herdr.Tab, error) {
			return nil, nil
		},
		AgentStart: func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
		},
		AgentPrompt: func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
			mu.Lock()
			promptsSlice = append(promptsSlice, opts.Text)
			mu.Unlock()
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
		},
		AgentWait: func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
			return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
		},
		AgentRead: func(target string, opts herdr.AgentReadOptions) (string, error) {
			return "compaction complete", nil
		},
		RevParse: func(dir, ref string) (string, error) {
			return "deadbeef", nil
		},
		MergeBase: func(dir, refA, refB string) (string, error) {
			return "deadbeef", nil
		},
		CommitsAhead: func(dir, fromExclusive, toRef string) (int, error) {
			return 1, nil
		},
		CherryPickRange: func(dir, fromExclusive, toInclusive string) error {
			return nil
		},
		CherryPickInProgress: func(dir string) (bool, error) {
			return false, nil
		},
		AbortCherryPick: func(dir string) error {
			return nil
		},
		IsAncestor: func(dir, ancestor, descendant string) (bool, error) {
			return true, nil
		},
		PatchesApplied: func(dir, upstream, base, branch string) (bool, error) {
			return false, nil
		},
		AppendTrailers: func(dir string, trailers ...git.Trailer) error {
			return nil
		},
		WorktreeExists: func(path string) (bool, error) {
			return true, nil
		},
		InstallDeps: func(path string) (string, error) {
			return "", nil
		},
		AgentSendKeys: func(target string, keys ...string) error {
			return nil
		},
		ReadOccupancy: func(cwd, sessionID string) (int, bool, error) {
			return 0, false, nil
		},
		Sleep: func(time.Duration) {},
		Now:   time.Now,
		// A park polls at full speed; a test about the park itself replaces
		// this with a timer that scripts the human clearing the block.
		ParkTimer: readyTimer,
	}
	return d, &promptsSlice, &removedSlice
}

func TestRun_LinearChain_RunsTicketsInOrderAndLandsAll(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n",
	})
	d, prompts, removed := fakeDeps()

	sink := newRecordingEventSink()
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantPrompts := []string{
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md"),
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "02-second.md"),
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
// AC that the normal same-run success path deletes a landed iteration's
// now-redundant branch — something it never did before this ticket.
func TestRun_IterationCompletion_DeletesIterationBranch(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
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
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	rawEvents, ok, err := readEvents(scratchDir, "my-epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	// scheduler-scan events are logged on every claimNext pass, interleaved
	// with (and racing) the async iteration lifecycle below; this test is
	// about that lifecycle's exact order, not the scheduler's own scanning,
	// so exclude them here rather than pin down their nondeterministic count
	// and position.
	var events []Event
	for _, ev := range rawEvents {
		if ev.Type != eventSchedulerScan {
			events = append(events, ev)
		}
	}

	wantTypes := []string{eventDepsInstalled, eventIterationStarted, eventIterationFinished, eventCherryPicked}
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
	if events[1].Pane == "" || events[1].Tab == "" {
		t.Errorf("iteration-started event = %+v, want non-empty Pane/Tab", events[1])
	}
	if events[0].Cwd == "" {
		t.Errorf("deps-installed event = %+v, want non-empty Cwd", events[0])
	}
}

// TestRun_SchedulerScan_LogsOutOfScopeTicket covers the case that motivated
// eventSchedulerScan: a ticket present in the epic but outside the run's
// RunScope (here, ticket 02 has no parent: pointing back into the requested
// "01", so RunScope.Contains never picks it up) looks, from the Queue tab,
// like it's just sitting there unclaimed — the scheduler-scan log line is
// what lets that be told apart from "still blocked" or "already claimed
// elsewhere".
func TestRun_SchedulerScan_LogsOutOfScopeTicket(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
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

	events, ok, err := readEvents(scratchDir, "my-epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}

	var found bool
	for _, ev := range events {
		if ev.Type != eventSchedulerScan {
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

// TestRun_FreshIteration_StampsCompactionsOnDone verifies ticket 04: a
// ticket marked done from a fresh iteration whose transcript recorded
// compaction boundaries gets that count written into its frontmatter's
// compactions field alongside status: done.
func TestRun_FreshIteration_StampsCompactionsOnDone(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-fresh-01"}, nil
	}
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
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-fresh-01"}, nil
	}
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

func TestRun_LogsNeedsAnswerEvent_OnZeroCommitIteration(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	// The needs-answer ticket is the epic's only one, so the run parks on it.
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var needsAnswer *Event
	for i, ev := range events {
		if ev.Type == eventNeedsAnswer && ev.Ticket == "01" {
			needsAnswer = &events[i]
		}
		if ev.Type == eventCherryPicked {
			t.Errorf("events = %+v, want no cherry-picked event for a zero-commit iteration", events)
		}
	}
	if needsAnswer == nil {
		t.Fatalf("events = %+v, want a needs-answer event for ticket 1", events)
	}
	if needsAnswer.AgentSession == "" {
		t.Errorf("needs-answer event = %+v, want a non-empty AgentSession (the agent_session that produced zero commits)", needsAnswer)
	}
}

func TestRun_EventSink_TicketNeedsAnswer_OnZeroCommitIteration(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	sink := &recordingSink{}
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink)

	if len(sink.ticketNeedsHumanCalls) != 1 {
		t.Fatalf("TicketNeedsHuman calls = %v, want exactly 1", sink.ticketNeedsHumanCalls)
	}
	if got := sink.ticketNeedsHumanCalls[0]; got[0] != "01" || got[1] != "epic" || got[2] != "needs-answer" {
		t.Errorf("TicketNeedsHuman call = %v, want [01 epic needs-answer ...]", got)
	}
}

// TestRun_HonorsCommitlessFlag_SkipsNeedsAnswer exercises the escape hatch from
// the zero-commit path above: if the agent reports iteration_status: finished
// with commitless: true before finishing, that's an intentional zero-commit
// finish (e.g. exploration concluded no code change was warranted), not a
// stalled agent — the ticket must not be forced back to needs-answer, gx
// itself writes status: done, and its worktree/tab get cleaned up like a
// normal completion.
func TestRun_HonorsCommitlessFlag_SkipsNeedsAnswer(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, removedBranches := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		// Simulate the agent calling `gx tickets set --iteration-status
		// finished --commitless true` on itself before going idle with no
		// commit.
		if err := updateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.IterationStatus = schema.IterationStatusFinished
			tk.Commitless = true
		}); err != nil {
			t.Fatalf("simulating agent self-report: %v", err)
		}
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var commitless *Event
	for i, ev := range events {
		if ev.Type == eventCommitless && ev.Ticket == "01" {
			commitless = &events[i]
		}
		if ev.Type == eventNeedsAnswer {
			t.Errorf("events = %+v, want no needs-answer event for a declared-commitless iteration", events)
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

func TestRun_InstallDepsFailure_MarksNeedsRepairWithoutLaunchingAgentOrAbortingRun(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, prompts, _ := fakeDeps()
	d.InstallDeps = func(path string) (string, error) {
		return "npm ci", errors.New("npm ci: exit status 1")
	}

	// A failed iteration marks needs-repair rather than aborting the run,
	// which leaves the epic parked on its only ticket.
	runUntilParked(t, RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, noopEventSink{})

	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no agent launched after a failed dependency install", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-repair") {
		t.Errorf("ticket file = %q, want Status: needs-repair after the install failure", raw)
	}
}

func TestRun_ZeroOpenTickets_NoOpSummary(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# First\n",
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

// TestAllDone_NeedsRepairIsNotComplete covers ticket 08's central
// inversion: a needs-repair ticket used to count as terminal and end the
// run, and now must not — the run parks on it instead.
func TestAllDone_NeedsRepairIsNotComplete(t *testing.T) {
	t.Parallel()
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "needs-repair"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 needs repair")
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
// larger epic runs and lands only those tickets, and Run exits once they're
// done even though a third, unblocked epic ticket is left open.
func TestRun_TicketSubset_CompletesWithoutTouchingTicketsOutsideSubset(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Third\n",
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
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md"),
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "02-second.md"),
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

// TestRun_Drain_WithInFlightTickets_FinishesInFlightThenEndsWithoutNewClaims
// covers ticket 01a's core seam: draining a running epic must let every
// already-in-flight ticket finish its full normal lifecycle, must never
// claim a still-open ticket once draining starts, and must end the run
// (Run returns nil, EpicComplete fires) once the last in-flight ticket
// lands — even though ticket 03 is left open, so the ordinary
// scope.AllSettled exit would never fire on its own.
func TestRun_Drain_WithInFlightTickets_FinishesInFlightThenEndsWithoutNewClaims(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Third\n",
	})
	d, prompts, _ := fakeDeps()

	gate := NewGate()
	var drainOnce sync.Once
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if strings.Contains(opts.Target, iterLabel("my-epic", "01")) {
			drainOnce.Do(gate.Drain)
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	sink := &recordingSink{}
	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		Gate:       gate,
	}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, p := range *prompts {
		if strings.Contains(p, "03-third.md") {
			t.Fatalf("prompts = %v, ticket 03 must never be claimed once draining starts", *prompts)
		}
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
		t.Errorf("03-third.md = %q, want it left open (never claimed once draining)", raw)
	}

	calls := sink.snapshot()
	if calls[len(calls)-1] != "EpicComplete" {
		t.Fatalf("last sink call = %q, want EpicComplete (the same code point natural completion reaches)", calls[len(calls)-1])
	}
	if n := countCalls(calls, "DrainComplete"); n != 1 {
		t.Errorf("DrainComplete calls = %d, want exactly 1 (ticket 01b)", n)
	}
}

// countCalls counts how many times name appears in calls, for asserting a
// sink event fired an exact number of times rather than just "at least
// once" or "last".
func countCalls(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

// TestRun_Drain_ZeroInFlight_EndsImmediately covers ticket 01a's other seam:
// draining an epic with nothing in flight ends the run immediately, without
// ever claiming the ticket that's sitting open and unblocked.
func TestRun_Drain_ZeroInFlight_EndsImmediately(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, prompts, _ := fakeDeps()

	gate := NewGate()
	gate.Drain()

	sink := &recordingSink{}
	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		Gate:       gate,
	}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 0 {
		t.Fatalf("prompts = %v, want none: draining with nothing in flight must never claim", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", "01-first.md"))
	if err != nil {
		t.Fatalf("ReadFile 01-first.md: %v", err)
	}
	if !strings.Contains(string(raw), "status: open") {
		t.Errorf("01-first.md = %q, want it left open (never claimed)", raw)
	}

	calls := sink.snapshot()
	if len(calls) == 0 || calls[len(calls)-1] != "EpicComplete" {
		t.Fatalf("sink calls = %v, want the run to end via EpicComplete immediately", calls)
	}
	if n := countCalls(calls, "DrainComplete"); n != 1 {
		t.Errorf("DrainComplete calls = %d, want exactly 1 (ticket 01b, immediate-drain case)", n)
	}
}

// TestRun_Drain_WakesRunParkedInWaitForResume is a regression test for
// ticket 01: draining a run that's paused-and-idle (an iteration paused,
// nothing in flight, blocked in Gate.waitForResume) must end the run on its
// own rather than hang until a ForceResume that Drain never expects to
// come — the exact operator-walks-away scenario drain exists to serve.
func TestRun_Drain_WakesRunParkedInWaitForResume(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, prompts, _ := fakeDeps()

	gate := NewGate()
	gate.pause("some-other-iteration", "context occupancy breach")

	sink := &recordingSink{}
	done := make(chan error, 1)
	go func() {
		done <- Run(RunOptions{
			EpicName:   "my-epic",
			Skill:      "implement",
			ScratchDir: scratchDir,
			RepoDir:    "/fake/repo",
			Gate:       gate,
		}, d, sink)
	}()

	// Give Run a moment to reach waitForResume before draining, so this
	// actually exercises Drain waking a parked waiter rather than racing
	// ahead of it.
	time.Sleep(50 * time.Millisecond)
	gate.Drain()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() never returned after Drain(): the run stayed parked in waitForResume")
	}

	if len(*prompts) != 0 {
		t.Fatalf("prompts = %v, want none: the paused label was never resumed so nothing should ever claim", *prompts)
	}

	calls := sink.snapshot()
	if n := countCalls(calls, "DrainComplete"); n != 1 {
		t.Errorf("DrainComplete calls = %d, want exactly 1", n)
	}
}

// TestRun_ScopeWidenedMidRun_TotalGrowsWithIt is a regression test for the
// notification total-count bug: total was captured once from the pre-run
// scope snapshot, so a ticket added to a running epic's scope mid-run (via
// the TUI's 'a' key widening RunScope) was counted in Completed once it
// landed but never grew Total to match, producing nonsensical stats like
// Completed > Total. Ticket 01 is the only ticket originally in scope;
// while its iteration is still running, its fake AgentWait call widens the
// live scope to also include ticket 02 (simulating the TUI action), so by
// the time ticket 01's IterationFinished fires, Total must already reflect
// both tickets even though only one has landed.
func TestRun_ScopeWidenedMidRun_TotalGrowsWithIt(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()

	var scope RunScope
	var widenOnce sync.Once
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if strings.Contains(opts.Target, iterLabel("my-epic", "01")) {
			widenOnce.Do(func() { scope.Add("02") })
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

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
		t.Errorf("ticket 01 stats.Total = %d, want 2 (mid-run Add must grow Total before it lands)", firstStats.Total)
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
	// together, they land Completed={1,2} exactly once each. Asserting a
	// fixed ticket-to-Completed mapping made this test an intermittent CI
	// flake whenever 02 happened to finish first.
	gotCompleted := []int{firstStats.Completed, secondStats.Completed}
	sort.Ints(gotCompleted)
	if !(gotCompleted[0] == 1 && gotCompleted[1] == 2) {
		t.Errorf("Completed counts across tickets 01/02 = %v, want {1,2} in some order", gotCompleted)
	}
}

// TestRun_ResumedRun_ReportsEpicWideDoneNotRunLocalCount is a regression
// test for ticket 26: a resumed run only lands one ticket itself, but
// IterationStats.Completed must count every done ticket on disk (including
// ones done before this Run call started), not just the ones this run
// landed — otherwise a resumed run understates progress ("1/10 done" when
// six of ten are already done).
func TestRun_ResumedRun_ReportsEpicWideDoneNotRunLocalCount(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
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

// TestRun_NeedsRepairOutsideSubset_DoesNotPauseRun covers ticket 23's
// requirement: a needs-repair ticket left outside the requested subset
// must not gate-pause scheduling of the tickets the caller actually asked
// for.
func TestRun_NeedsRepairOutsideSubset_DoesNotPauseRun(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
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
		t.Fatalf("Run() error = %v, want the unselected needs-repair ticket 01 to not block scheduling ticket 02", err)
	}

	wantPrompt := "/implement " + filepath.Join(scratchDir, "my-epic", "issues", "02-second.md")
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

// TestRun_NeedsRepairInsideSubset_RunsTheRestThenParks covers the flip
// side of the above: a needs-repair ticket the caller did select no longer
// stops the run outright. It's human-clearable, so the rest of the subset is
// scheduled first and the run only parks once nothing else is runnable.
func TestRun_NeedsRepairInsideSubset_RunsTheRestThenParks(t *testing.T) {
	t.Parallel()
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, prompts, _ := fakeDeps()

	sink := &recordingSink{}
	runUntilParked(t, RunOptions{
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
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# B\n",
	})
	ticket01Path := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, _ := fakeDeps()

	label01 := "pane-" + iterLabel("epic", "01")
	label02 := "pane-" + iterLabel("epic", "02")

	var mu sync.Mutex
	agentStartCalls := map[string]int{}
	unblock01 := make(chan struct{})
	var clobberOnce sync.Once

	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		mu.Lock()
		agentStartCalls[opts.Pane]++
		mu.Unlock()
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
	}
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Target == label02 {
			clobberOnce.Do(func() {
				if err := os.WriteFile(ticket01Path, []byte("---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n"), 0644); err != nil {
					t.Errorf("simulating external clobber of ticket 01: %v", err)
				}
			})
		}
		if opts.Target == label01 {
			<-unblock01
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "working"}, nil
	}
	tabClosed02 := make(chan struct{})
	var tabClosed02Once sync.Once
	d.TabClose = func(tabID string) error {
		if tabID == "tab-"+iterLabel("epic", "02") {
			tabClosed02Once.Do(func() { close(tabClosed02) })
		}
		return nil
	}

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
		t.Errorf("AgentStart calls for ticket 01's pane = %d, want exactly 1 (claimNext must not re-claim a ticket it already launched, even after an external write reverted its on-disk status)", agentStartCalls[label01])
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
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Third\n",
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
	// misleading runnable wave, once 01 lands.
	if err := os.WriteFile(ticketPath, []byte("---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\", \"03\"]\n---\n# Second\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := planFor(); err == nil {
		t.Fatalf("PlanWaves() error = nil, want a stuck-plan error while 02 needs unselected blocker 03")
	}

	// Edit again, dropping the unselected blocker — the plan should now
	// resolve into the two waves 01's-then-02's chain always implied.
	if err := os.WriteFile(ticketPath, []byte("---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n"), 0644); err != nil {
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
		"/implement " + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md"),
		"/implement " + ticketPath,
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

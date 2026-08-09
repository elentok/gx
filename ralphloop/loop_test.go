package ralphloop

import (
	"bytes"
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
// run which parks gives up after one poll instead of waiting for a person who
// will never arrive. Tests about parking itself set maxParkPolls back to 0.
func testDeps() Deps {
	d := DefaultDeps()
	d.maxParkPolls = 1
	return d
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
		ResumeSignaled: func(path string) (bool, error) {
			return true, nil
		},
		Sleep: func(time.Duration) {},
		Now:   time.Now,
		// One poll, so a run that parks still returns for the assertions;
		// a test about the park itself raises or clears this itself.
		maxParkPolls: 1,
	}
	return d, &promptsSlice, &removedSlice
}

func TestRun_LinearChain_RunsTicketsInOrderAndLandsAll(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\nblocked_by: [\"01\"]\n---\n# Second\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out))
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

	if !strings.Contains(out.String(), "complete: 2 ticket(s)") {
		t.Errorf("summary output = %q, want a completion summary mentioning 2 tickets", out.String())
	}
}

// TestRun_IterationCompletion_DeletesIterationBranch exercises ticket 04's
// AC that the normal same-run success path deletes a landed iteration's
// now-redundant branch — something it never did before this ticket.
func TestRun_IterationCompletion_DeletesIterationBranch(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	var deletedBranches []string
	d.DeleteBranch = func(repoDir, branch string) error {
		deletedBranches = append(deletedBranches, branch)
		return nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(deletedBranches) != 1 || deletedBranches[0] != "ralph-loop/my-epic-item-01" {
		t.Errorf("deletedBranches = %v, want [ralph-loop/my-epic-item-01]", deletedBranches)
	}
}

func TestRun_LogsLifecycleEvents_LinearChain(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, _, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01"},
	}, d, NewTextEventSink(&out)); err != nil {
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

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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

func TestRun_LogsNeedsInfoEvent_OnZeroCommitIteration(t *testing.T) {
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

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var needsInfo *Event
	for i, ev := range events {
		if ev.Type == eventNeedsInfo && ev.Ticket == "01" {
			needsInfo = &events[i]
		}
		if ev.Type == eventCherryPicked {
			t.Errorf("events = %+v, want no cherry-picked event for a zero-commit iteration", events)
		}
	}
	if needsInfo == nil {
		t.Fatalf("events = %+v, want a needs-info event for ticket 1", events)
	}
	if needsInfo.AgentSession == "" {
		t.Errorf("needs-info event = %+v, want a non-empty AgentSession (the agent_session that produced zero commits)", needsInfo)
	}
}

func TestRun_EventSink_TicketNeedsInfo_OnZeroCommitIteration(t *testing.T) {
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
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, sink); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(sink.ticketNeedsInfoCalls) != 1 {
		t.Fatalf("TicketNeedsInfo calls = %v, want exactly 1", sink.ticketNeedsInfoCalls)
	}
	if got := sink.ticketNeedsInfoCalls[0]; got[0] != "01" || got[1] != "epic" {
		t.Errorf("TicketNeedsInfo call = %v, want [01 epic]", got)
	}
}

// TestRun_HonorsCommitlessFlag_SkipsNeedsInfo exercises the escape hatch from
// the zero-commit path above: if the agent itself sets commitless: true
// (alongside moving status off "claimed") before finishing, that's an
// intentional zero-commit finish (e.g. exploration concluded no code change
// was warranted), not a stalled agent — the ticket must not be forced back to
// needs-info, and its worktree/tab get cleaned up like a normal completion.
func TestRun_HonorsCommitlessFlag_SkipsNeedsInfo(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	ticketPath := filepath.Join(scratchDir, "epic", "issues", "01-a.md")

	d, _, removedBranches := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		// Simulate the agent calling `gx tickets set --status done
		// --commitless true` on itself before going idle with no commit.
		if err := updateTicket(ticketPath, func(tk *schema.Ticket) {
			tk.Status = schema.StatusDone
			tk.Commitless = true
		}); err != nil {
			t.Fatalf("simulating agent self-report: %v", err)
		}
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
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
		if ev.Type == eventNeedsInfo {
			t.Errorf("events = %+v, want no needs-info event for a declared-commitless iteration", events)
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

func TestRun_InstallDepsFailure_MarksNeedsAttentionWithoutLaunchingAgentOrAbortingRun(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	d, prompts, _ := fakeDeps()
	d.InstallDeps = func(path string) (string, error) {
		return "npm ci", errors.New("npm ci: exit status 1")
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v, want a failed iteration to mark needs-attention rather than abort the run", err)
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no agent launched after a failed dependency install", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "status: needs-attention") {
		t.Errorf("ticket file = %q, want Status: needs-attention after the install failure", raw)
	}
}

func TestRun_ZeroOpenTickets_NoOpSummary(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "---\nid: \"01\"\nstatus: done\ntype: task\n---\n# First\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 0 || len(*removed) != 0 {
		t.Fatalf("expected no iterations to run, got prompts=%v removed=%v", *prompts, *removed)
	}
	if !strings.Contains(out.String(), "already complete") {
		t.Errorf("summary output = %q, want a no-op/already-complete message", out.String())
	}
}

func TestRun_NoEpicFound_NoOpSummary(t *testing.T) {
	scratchDir := t.TempDir()
	d, _, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("summary output = %q, want a nothing-to-do message", out.String())
	}
}

// TestAllDone_NeedsAttentionIsNotComplete covers ticket 08's central
// inversion: a needs-attention ticket used to count as terminal and end the
// run, and now must not — the run parks on it instead.
func TestAllDone_NeedsAttentionIsNotComplete(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "needs-attention"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 needs attention")
	}
}

func TestAllDone_OpenTicketIsNotComplete(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if allDone(epic) {
		t.Errorf("allDone() = true, want false while ticket 2 is still open")
	}
}

func TestAllDone_EveryTicketDone(t *testing.T) {
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
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
		"03-third.md":  "---\nid: \"03\"\nstatus: open\ntype: task\n---\n# Third\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01", "02"},
	}, d, NewTextEventSink(&out))
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
	if firstStats.Completed != 1 {
		t.Errorf("ticket 01 stats.Completed = %d, want 1", firstStats.Completed)
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
	if secondStats.Completed != 2 {
		t.Errorf("ticket 02 stats.Completed = %d, want 2", secondStats.Completed)
	}
	if secondStats.Total != 2 {
		t.Errorf("ticket 02 stats.Total = %d, want 2", secondStats.Total)
	}
}

// TestRun_NeedsAttentionOutsideSubset_DoesNotPauseRun covers ticket 23's
// requirement: a needs-attention ticket left outside the requested subset
// must not gate-pause scheduling of the tickets the caller actually asked
// for.
func TestRun_NeedsAttentionOutsideSubset_DoesNotPauseRun(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"02"},
	}, d, NewTextEventSink(&out))
	if err != nil {
		t.Fatalf("Run() error = %v, want the unselected needs-attention ticket 01 to not block scheduling ticket 02", err)
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

// TestRun_NeedsAttentionInsideSubset_RunsTheRestThenParks covers the flip
// side of the above: a needs-attention ticket the caller did select no longer
// stops the run outright. It's human-clearable, so the rest of the subset is
// scheduled first and the run only parks once nothing else is runnable.
func TestRun_NeedsAttentionInsideSubset_RunsTheRestThenParks(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "---\nid: \"01\"\nstatus: needs-attention\ntype: task\n---\n# First\n",
		"02-second.md": "---\nid: \"02\"\nstatus: open\ntype: task\n---\n# Second\n",
	})
	d, prompts, _ := fakeDeps()

	sink := &recordingSink{}
	err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  []string{"01", "02"},
	}, d, sink)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a stalled ticket parks, it doesn't fail the run)", err)
	}
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

	var out bytes.Buffer
	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, NewTextEventSink(&out))
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
	var out bytes.Buffer
	if err := Run(RunOptions{
		EpicName:   "my-epic",
		Skill:      "implement",
		ScratchDir: scratchDir,
		RepoDir:    "/fake/repo",
		TicketIDs:  requested,
	}, d, NewTextEventSink(&out)); err != nil {
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

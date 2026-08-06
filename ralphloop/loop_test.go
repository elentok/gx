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

	events, ok, err := readEvents(scratchDir, "my-epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
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
	wantSession := "sess-pane-iter-01"
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

// TestAllSettled_NeedsAttentionCountsAsTerminal exercises ticket 03's exit
// requirement: a needs-attention ticket (whether Codex's own operator-
// intervention path, or startup reconciliation's unrecoverable-done flag)
// must not keep the loop spinning forever the way an open/claimed/blocked
// ticket would.
func TestAllSettled_NeedsAttentionCountsAsTerminal(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "needs-attention"},
	}}
	if !allSettled(epic) {
		t.Errorf("allSettled() = false, want true when every ticket is done or needs-attention")
	}
}

func TestAllSettled_OpenTicketNotSettled(t *testing.T) {
	epic := tickets.Epic{Tickets: []tickets.Ticket{
		{Number: 1, Status: "done"},
		{Number: 2, Status: "open"},
	}}
	if allSettled(epic) {
		t.Errorf("allSettled() = true, want false while ticket 2 is still open")
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

// TestRun_NeedsAttentionInsideSubset_StillPausesRun covers the flip side of
// the above: a needs-attention ticket the caller did select keeps its
// existing safety behavior — it still gate-pauses the run rather than
// letting scheduling of the rest of the subset proceed silently.
func TestRun_NeedsAttentionInsideSubset_StillPausesRun(t *testing.T) {
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
		TicketIDs:  []string{"01", "02"},
	}, d, NewTextEventSink(&out))
	if err == nil {
		t.Fatalf("Run() error = nil, want the selected needs-attention ticket 01 to still pause scheduling of ticket 02")
	}
	if len(*prompts) != 0 {
		t.Fatalf("prompts = %v, want none (ticket 02 must never launch while the gate is paused)", *prompts)
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

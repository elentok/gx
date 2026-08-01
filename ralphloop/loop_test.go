package ralphloop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
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
		IsAncestor: func(dir, ancestor, descendant string) (bool, error) {
			return true, nil
		},
		WorktreeExists: func(path string) (bool, error) {
			return false, nil
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
	}
	return d, &promptsSlice, &removedSlice
}

func TestRun_LinearChain_RunsTicketsInOrderAndLandsAll(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md":  "# First\n\n**Status:** open\n",
		"02-second.md": "# Second\n\n**Blocked by:** 01\n\n**Status:** open\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out)
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

	wantBranches := []string{"ralph-loop/iter-01", "ralph-loop/iter-02"}
	if len(*removed) != 2 || (*removed)[0] != wantBranches[0] || (*removed)[1] != wantBranches[1] {
		t.Fatalf("removed worktree branches = %v, want %v", *removed, wantBranches)
	}

	for _, name := range []string{"01-first.md", "02-second.md"} {
		raw, err := os.ReadFile(filepath.Join(scratchDir, "my-epic", "issues", name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if !strings.Contains(string(raw), "Status:** done") {
			t.Errorf("%s not marked done:\n%s", name, raw)
		}
	}

	if !strings.Contains(out.String(), "complete: 2 ticket(s)") {
		t.Errorf("summary output = %q, want a completion summary mentioning 2 tickets", out.String())
	}
}

func TestRun_LogsLifecycleEvents_LinearChain(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "# First\n\n**Status:** open\n",
	})
	d, _, _ := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
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
		if events[i].Ticket != 1 {
			t.Errorf("events[%d].Ticket = %d, want 1", i, events[i].Ticket)
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

func TestRun_LogsNeedsInfoEvent_OnZeroCommitIteration(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, _, _ := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var needsInfo *Event
	for i, ev := range events {
		if ev.Type == eventNeedsInfo && ev.Ticket == 1 {
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

func TestRun_InstallDepsFailure_SurfacesAsRunErrorWithoutLaunchingAgent(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()
	d.InstallDeps = func(path string) (string, error) {
		return "npm ci", errors.New("npm ci: exit status 1")
	}

	var out bytes.Buffer
	err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out)
	if err == nil {
		t.Fatal("Run() error = nil, want the install failure surfaced")
	}
	if !strings.Contains(err.Error(), "npm ci") {
		t.Errorf("Run() error = %v, want it to mention the failed install command", err)
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no agent launched after a failed dependency install", *prompts)
	}
}

func TestRun_LogsDepsInstalledEventWithCommand(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, _, _ := fakeDeps()
	d.InstallDeps = func(path string) (string, error) {
		return "npm ci", nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var found *Event
	for i, ev := range events {
		if ev.Type == eventDepsInstalled {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("events = %+v, want a deps-installed event", events)
	}
	if found.Reason != "npm ci" {
		t.Errorf("deps-installed event Reason = %q, want the command run", found.Reason)
	}
}

func TestRun_SkillFlag_OverridesPromptSkill(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "# First\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "tdd", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasPrefix((*prompts)[0], "/tdd ") {
		t.Fatalf("prompts = %v, want a single /tdd-prefixed prompt", *prompts)
	}
}

func TestRun_AgentSelection_ConfiguresLaunchAndPrompt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		agent      AgentKind
		wantKind   string
		wantArgs   []string
		wantPrefix string
	}{
		{
			name:       "claude default",
			wantKind:   "claude",
			wantArgs:   []string{"--permission-mode", "auto"},
			wantPrefix: "/implement ",
		},
		{
			name:       "codex",
			agent:      AgentCodex,
			wantKind:   "codex",
			wantArgs:   []string{"--sandbox", "workspace-write", "--ask-for-approval", "on-request"},
			wantPrefix: "$implement ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scratchDir := writeEpic(t, "my-epic", map[string]string{
				"01-first.md": "# First\n\n**Status:** open\n",
			})
			d, prompts, _ := fakeDeps()
			var start herdr.AgentStartOptions
			d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
				start = opts
				return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle"}, nil
			}

			var out bytes.Buffer
			err := Run(RunOptions{EpicName: "my-epic", Agent: tc.agent, Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if start.Kind != tc.wantKind {
				t.Errorf("AgentStart Kind = %q, want %q", start.Kind, tc.wantKind)
			}
			if len(start.AgentArgs) < len(tc.wantArgs) || !slices.Equal(start.AgentArgs[:len(tc.wantArgs)], tc.wantArgs) {
				t.Errorf("AgentStart AgentArgs = %v, want prefix %v", start.AgentArgs, tc.wantArgs)
			}
			if tc.agent == AgentCodex {
				wantScratch := filepath.Join(scratchDir, "my-epic")
				if !slices.Contains(start.AgentArgs, wantScratch) {
					t.Errorf("AgentStart AgentArgs = %v, want epic scratch directory %q", start.AgentArgs, wantScratch)
				}
			}
			wantPrompt := tc.wantPrefix + filepath.Join(scratchDir, "my-epic", "issues", "01-first.md")
			if len(*prompts) != 1 || (*prompts)[0] != wantPrompt {
				t.Errorf("prompts = %v, want %q", *prompts, wantPrompt)
			}
			events, ok, readErr := readEvents(scratchDir, "my-epic")
			if readErr != nil || !ok || len(events) == 0 {
				t.Fatalf("readEvents: events=%+v ok=%v err=%v", events, ok, readErr)
			}
			for _, event := range events {
				if event.Agent != AgentKind(tc.wantKind) {
					t.Errorf("event %q agent = %q, want %q", event.Type, event.Agent, tc.wantKind)
				}
			}
		})
	}
}

func TestRun_InvalidAgent_ReturnsError(t *testing.T) {
	var out bytes.Buffer
	err := Run(RunOptions{Agent: "other"}, Deps{}, &out)
	if err == nil || !strings.Contains(err.Error(), "must be claude or codex") {
		t.Fatalf("Run() error = %v, want invalid-agent error", err)
	}
}

func TestRun_ZeroOpenTickets_NoOpSummary(t *testing.T) {
	scratchDir := writeEpic(t, "my-epic", map[string]string{
		"01-first.md": "# First\n\n**Status:** done\n",
	})
	d, prompts, removed := fakeDeps()

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
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
	if err := Run(RunOptions{EpicName: "missing-epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("summary output = %q, want a nothing-to-do message", out.String())
	}
}

func TestRun_MaxParallelOne_RunsSerially(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
		"03-c.md": "# C\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()

	var out bytes.Buffer
	err := Run(RunOptions{
		EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
		MaxParallel: 1,
	}, d, &out)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantOrder := []string{"01-a.md", "02-b.md", "03-c.md"}
	if len(*prompts) != len(wantOrder) {
		t.Fatalf("prompts = %v, want %d prompts", *prompts, len(wantOrder))
	}
	for i, name := range wantOrder {
		if !strings.HasSuffix((*prompts)[i], name) {
			t.Errorf("prompts[%d] = %q, want suffix %q (serial, ticket-number order)", i, (*prompts)[i], name)
		}
	}
}

// gatedAgentWait wraps a fakeDeps' AgentWait so that only the "wait for the
// agent to finish" call (the one whose Until includes "done") blocks until
// released, letting a test control exactly when each iteration completes and
// observe how many run concurrently in between.
func gatedAgentWait(next func(herdr.AgentWaitOptions) (herdr.Agent, error)) (
	wait func(herdr.AgentWaitOptions) (herdr.Agent, error),
	started <-chan string,
	release func(pane string),
) {
	var mu sync.Mutex
	gates := map[string]chan struct{}{}
	startedCh := make(chan string, 16)

	wait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		isFinish := false
		for _, u := range opts.Until {
			if u == "done" {
				isFinish = true
			}
		}
		if !isFinish {
			return next(opts)
		}

		gate := make(chan struct{})
		mu.Lock()
		gates[opts.Target] = gate
		mu.Unlock()

		startedCh <- opts.Target
		<-gate
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	release = func(pane string) {
		mu.Lock()
		gate := gates[pane]
		mu.Unlock()
		close(gate)
	}

	return wait, startedCh, release
}

func TestRun_CherryPickConflict_ResolvesInFeatureWorktreeThenCompletes(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, prompts, removed := fakeDeps()
	d.AgentStart = func(opts herdr.AgentStartOptions) (herdr.Agent, error) {
		return herdr.Agent{PaneID: opts.Pane, AgentStatus: "idle", AgentSession: "sess-" + opts.Pane}, nil
	}

	var mu sync.Mutex
	var picks int
	var conflictPane, iterPane string
	var conflictPaneRemovedBefore bool

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		mu.Lock()
		defer mu.Unlock()
		picks++
		if picks == 1 {
			return &fakeConflictErr{}
		}
		return nil
	}

	inProgress := true
	d.CherryPickInProgress = func(dir string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return inProgress, nil
	}

	origTabCreate := d.TabCreate
	d.TabCreate = func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
		mu.Lock()
		conflictPane = "pane-" + opts.Label
		mu.Unlock()
		return origTabCreate(opts)
	}

	origAgentPrompt := d.AgentPrompt
	d.AgentPrompt = func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.Text == "/resolving-merge-conflicts" {
			mu.Lock()
			inProgress = false // resolution "commits", ending the cherry-pick sequence
			mu.Unlock()
		} else {
			mu.Lock()
			iterPane = opts.Target
			mu.Unlock()
		}
		return origAgentPrompt(opts)
	}

	origRemoveWorktree := d.RemoveWorktree
	d.RemoveWorktree = func(repoDir, path string, force bool) error {
		mu.Lock()
		if conflictPane == "" {
			conflictPaneRemovedBefore = true
		}
		mu.Unlock()
		return origRemoveWorktree(repoDir, path, force)
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if conflictPane == "" {
		t.Fatal("expected a conflict-resolution pane to be created")
	}
	if conflictPane == iterPane {
		t.Errorf("conflict-resolution pane %q must differ from the iteration pane %q (must run in the feature worktree)", conflictPane, iterPane)
	}
	if conflictPaneRemovedBefore {
		t.Error("iteration worktree was removed before the conflict-resolution pane was created")
	}

	if !slices.Contains(*prompts, "/resolving-merge-conflicts") {
		t.Errorf("prompts = %v, want a /resolving-merge-conflicts prompt", *prompts)
	}

	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the iteration worktree removed after resolution", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket not marked done after conflict resolution:\n%s", raw)
	}

	events, ok, err := readEvents(scratchDir, "epic")
	if err != nil || !ok {
		t.Fatalf("readEvents: ok=%v err=%v", ok, err)
	}
	var gotTypes []string
	var conflictHit, conflictResolved *Event
	for i, ev := range events {
		gotTypes = append(gotTypes, ev.Type)
		switch ev.Type {
		case eventConflictHit:
			conflictHit = &events[i]
		case eventConflictResolved:
			conflictResolved = &events[i]
		}
	}
	if conflictHit == nil || conflictResolved == nil {
		t.Fatalf("event types = %v, want both %q and %q", gotTypes, eventConflictHit, eventConflictResolved)
	}
	if conflictHit.AgentSession == "" {
		t.Errorf("conflict-hit event = %+v, want a non-empty AgentSession (the iteration agent's own session)", conflictHit)
	}
	if conflictResolved.AgentSession == "" || conflictResolved.AgentSession == conflictHit.AgentSession {
		t.Errorf("conflict-resolved event = %+v, want a non-empty AgentSession distinct from conflict-hit's %q (the resolution agent's own session)", conflictResolved, conflictHit.AgentSession)
	}
}

func TestRun_CherryPickConflict_ResolutionNeverFinishes_SurfacesDistinctError(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, _, _ := fakeDeps()

	d.CherryPickRange = func(dir, fromExclusive, toInclusive string) error {
		return &fakeConflictErr{}
	}
	d.CherryPickInProgress = func(dir string) (bool, error) {
		return true, nil // conflict never resolves
	}
	d.AgentWait = func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		isFinish := slices.Contains(opts.Until, "done")
		isConflictPane := strings.HasPrefix(opts.Target, "pane-conflict-")
		if isFinish && isConflictPane {
			return herdr.Agent{}, errors.New("timeout waiting for agent")
		}
		return herdr.Agent{PaneID: opts.Target, AgentStatus: "idle"}, nil
	}

	var out bytes.Buffer
	err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out)
	if err == nil {
		t.Fatal("Run() error = nil, want an error surfacing the stuck conflict-resolution agent")
	}
	if !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("Run() error = %v, want it to call out the conflict-resolution agent not finishing", err)
	}
}

// fakeConflictErr stands in for the *git.RunError CherryPickRange returns on
// a real conflict; only its presence (not its type) matters to the loop,
// which distinguishes conflicts from other errors via CherryPickInProgress.
type fakeConflictErr struct{}

func (e *fakeConflictErr) Error() string { return "cherry-pick conflict" }

func TestRun_ZeroCommitIteration_MarksNeedsInfoAndLeavesWorktree(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		return 0, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 0 {
		t.Errorf("removed worktree branches = %v, want the zero-commit iteration's worktree left in place", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** needs-info") {
		t.Errorf("ticket not marked needs-info after zero-commit iteration:\n%s", raw)
	}
	if strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket must not be marked done after a zero-commit iteration:\n%s", raw)
	}
}

func TestRun_ZeroCommitIteration_OtherTicketsStillLand(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
	})
	d, _, removed := fakeDeps()
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if strings.Contains(dir, "iter-01") {
			return 0, nil
		}
		return 1, nil
	}

	var out bytes.Buffer
	if err := Run(RunOptions{
		EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
		MaxParallel: 1,
	}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*removed) != 1 || (*removed)[0] != "ralph-loop/iter-02" {
		t.Errorf("removed worktree branches = %v, want only iter-02 removed", *removed)
	}

	raw01, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw01), "Status:** needs-info") {
		t.Errorf("ticket 01 not marked needs-info:\n%s", raw01)
	}

	raw02, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "02-b.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw02), "Status:** done") {
		t.Errorf("ticket 02 not marked done:\n%s", raw02)
	}
}

func TestRun_MaxParallelTwo_RunsExactlyTwoConcurrentlyAndBackfills(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** open\n",
		"03-c.md": "# C\n\n**Status:** open\n",
	})
	d, _, removed := fakeDeps()
	wait, started, release := gatedAgentWait(d.AgentWait)
	d.AgentWait = wait

	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(RunOptions{
			EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo",
			MaxParallel: 2,
		}, d, &out)
	}()

	pane1 := <-started
	pane2 := <-started

	select {
	case pane3 := <-started:
		t.Fatalf("a third iteration started with only 2 slots and both full: %s", pane3)
	case <-time.After(100 * time.Millisecond):
	}

	release(pane1)
	pane3 := <-started // backfilled without waiting for pane2

	release(pane2)
	release(pane3)

	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(*removed) != 3 {
		t.Errorf("removed worktree branches = %v, want 3 entries", *removed)
	}
}

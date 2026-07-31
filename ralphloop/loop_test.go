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
	branchByWorkspace := map[string]string{}

	d = Deps{
		FindOrCreateWorkspace: func(label, cwd string) (string, error) {
			return "ws1", nil
		},
		WorktreeCreate: func(opts herdr.WorktreeCreateOptions) (herdr.Worktree, error) {
			wsID := "ws-" + opts.Branch
			mu.Lock()
			branchByWorkspace[wsID] = opts.Branch
			mu.Unlock()
			return herdr.Worktree{
				WorkspaceID: wsID,
				PaneID:      "pane-" + opts.Branch,
				Path:        "/fake/" + opts.Branch,
				Branch:      opts.Branch,
			}, nil
		},
		WorktreeRemove: func(workspaceID string, force bool) error {
			mu.Lock()
			removedSlice = append(removedSlice, branchByWorkspace[workspaceID])
			mu.Unlock()
			return nil
		},
		TabCreate: func(opts herdr.TabCreateOptions) (herdr.CreatedTab, error) {
			return herdr.CreatedTab{
				Tab:        herdr.Tab{Label: opts.Label, WorkspaceID: opts.WorkspaceID},
				RootPaneID: "pane-" + opts.Label,
			}, nil
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
		CherryPickRange: func(dir, fromExclusive, toInclusive string) error {
			return nil
		},
		CherryPickInProgress: func(dir string) (bool, error) {
			return false, nil
		},
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

	origWorktreeRemove := d.WorktreeRemove
	d.WorktreeRemove = func(workspaceID string, force bool) error {
		mu.Lock()
		if conflictPane == "" {
			conflictPaneRemovedBefore = true
		}
		mu.Unlock()
		return origWorktreeRemove(workspaceID, force)
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

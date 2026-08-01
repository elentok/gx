package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

func TestReconcile_ClaimedWithNoLiveTab_RevertsToOpen(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil // no live tabs at all
	}

	reattached, err := reconcile(d, "ws1", epics[0], func(string, ...any) {})
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none", reattached)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** open") {
		t.Errorf("ticket not reverted to open:\n%s", raw)
	}
}

func TestReconcile_ClaimedWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	reattached, err := reconcile(d, "ws1", epics[0], func(string, ...any) {})
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 1 || reattached[0].Number != 1 {
		t.Fatalf("reattached = %v, want ticket 01", reattached)
	}

	// Status: claimed must be left untouched — the caller resumes it, it's not
	// reverted to open.
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** claimed") {
		t.Errorf("ticket status changed unexpectedly:\n%s", raw)
	}
}

func TestReconcile_NeedsAttentionWithLiveTab_ReturnsReattached(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** needs-attention\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}
	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	reattached, err := reconcile(d, "ws1", epics[0], func(string, ...any) {})
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 1 || reattached[0].Number != 1 {
		t.Fatalf("reattached = %v, want needs-attention ticket 01", reattached)
	}
}

func TestRun_NeedsAttentionWithoutLiveTab_DoesNotScheduleOtherTickets(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "# Attention\n\n**Status:** needs-attention\n",
		"02-open.md":      "# Open\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(string) ([]herdr.Tab, error) { return nil, nil }

	err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("Run() error = %v, want actionable paused error", err)
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no new iteration while ticket needs attention", *prompts)
	}
}

func TestRun_RestartedNeedsAttentionRecoversThenResumesScheduling(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-attention.md": "# Attention\n\n**Status:** needs-attention\n",
		"02-open.md":      "# Open\n\n**Status:** open\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}
	var mu sync.Mutex
	sawClaimed := false
	d.CommitsAhead = func(dir, fromExclusive, toRef string) (int, error) {
		if strings.Contains(dir, "iter-01") {
			raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-attention.md"))
			mu.Lock()
			sawClaimed = err == nil && strings.Contains(string(raw), "claimed")
			mu.Unlock()
		}
		return 1, nil
	}

	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &strings.Builder{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(*prompts) != 1 || !strings.HasSuffix((*prompts)[0], "02-open.md") {
		t.Errorf("prompts = %v, want only the newly scheduled second ticket", *prompts)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawClaimed {
		t.Error("recovered ticket was not restored to claimed before completion")
	}
}

func TestReconcile_OpenAndDoneTicketsIgnored(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** open\n",
		"02-b.md": "# B\n\n**Status:** done\n",
	})
	epics, err := tickets.Load(scratchDir)
	if err != nil {
		t.Fatalf("tickets.Load: %v", err)
	}

	d, _, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	}

	reattached, err := reconcile(d, "ws1", epics[0], func(string, ...any) {})
	if err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if len(reattached) != 0 {
		t.Fatalf("reattached = %v, want none", reattached)
	}
}

// TestRun_RestartWithClaimedTicketButNoLiveTab_RevertsAndRerunsFromScratch
// exercises the full Run() path (not just reconcile in isolation): a ticket
// left claimed by a prior crashed invocation, with nothing live in the herdr
// workspace, is reverted to open and then picked up and run fresh by normal
// scheduling.
func TestRun_RestartWithClaimedTicketButNoLiveTab_RerunsFromScratch(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, prompts, _ := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return nil, nil
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(*prompts) != 1 || !strings.HasSuffix((*prompts)[0], "01-a.md") {
		t.Fatalf("prompts = %v, want ticket 01 run fresh", *prompts)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("ticket not landed after restart:\n%s", raw)
	}
}

// TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt
// verifies the other half of ticket 08: a claimed ticket whose iter-NN tab is
// still alive is reattached (no fresh worktree create, no initial prompt
// replayed) and driven through to completion.
func TestRun_RestartWithClaimedTicketAndLiveTab_ReattachesWithoutReplayingPrompt(t *testing.T) {
	scratchDir := writeEpic(t, "epic", map[string]string{
		"01-a.md": "# A\n\n**Status:** claimed\n",
	})
	d, prompts, removed := fakeDeps()
	d.TabList = func(workspaceID string) ([]herdr.Tab, error) {
		return []herdr.Tab{{TabID: "tab-iter-01", Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var worktreeCreateCalledForIter bool
	origAddWorktree := d.AddWorktree
	d.AddWorktree = func(repoDir, path, branch, base string) error {
		if strings.Contains(path, "iter-01") {
			worktreeCreateCalledForIter = true
		}
		return origAddWorktree(repoDir, path, branch, base)
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if worktreeCreateCalledForIter {
		t.Error("a reattached iteration must reuse its existing worktree, not create a new one")
	}
	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want no initial prompt replayed for a reattached iteration", *prompts)
	}
	if len(*removed) != 1 {
		t.Errorf("removed worktree branches = %v, want the reattached iteration's worktree removed on completion", *removed)
	}

	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), "Status:** done") {
		t.Errorf("reattached ticket not marked done:\n%s", raw)
	}
}

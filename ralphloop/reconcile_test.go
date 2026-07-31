package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
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
		return []herdr.Tab{{Label: "iter-01", WorkspaceID: workspaceID}}, nil
	}

	var worktreeCreateCalledForIter bool
	origCreate := d.WorktreeCreate
	d.WorktreeCreate = func(opts herdr.WorktreeCreateOptions) (herdr.Worktree, error) {
		if opts.Label == "iter-01" {
			worktreeCreateCalledForIter = true
		}
		return origCreate(opts)
	}

	var worktreeOpenedForIter bool
	origOpen := d.WorktreeOpen
	d.WorktreeOpen = func(opts herdr.WorktreeOpenOptions) (herdr.Worktree, error) {
		if opts.Label == "iter-01" {
			worktreeOpenedForIter = true
		}
		return origOpen(opts)
	}

	var out strings.Builder
	if err := Run(RunOptions{EpicName: "epic", Skill: "implement", ScratchDir: scratchDir, RepoDir: "/fake/repo"}, d, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if worktreeCreateCalledForIter {
		t.Error("a reattached iteration must reopen its worktree, not create a new one")
	}
	if !worktreeOpenedForIter {
		t.Error("expected the reattached iteration's worktree to be reopened via WorktreeOpen")
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

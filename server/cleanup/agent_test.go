package cleanup_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// idleCommit commits a file with a date outside the scan's active window.
func idleCommit(t *testing.T, repo, name string) {
	t.Helper()
	old := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	t.Setenv("GIT_AUTHOR_DATE", old)
	t.Setenv("GIT_COMMITTER_DATE", old)
	commitFile(t, repo, name)
}

func startAgentFixture(t *testing.T, withUnresolved bool) *servertest.Harness {
	t.Helper()
	repo, store := testutil.TempRepo(t), t.TempDir()
	trunk := strings.TrimSpace(gitIn(t, repo, "branch", "--show-current"))
	if withUnresolved {
		gitIn(t, repo, "switch", "-c", "unresolved")
		idleCommit(t, repo, "u.txt")
		gitIn(t, repo, "switch", trunk)
		servertest.WriteCompleteEpic(t, store, "pending")
	} else {
		servertest.WriteTicket(t, store, "proj", "open-epic", "01", "first", "")
	}
	gitIn(t, repo, "switch", "-c", "safe")
	idleCommit(t, repo, "s.txt")
	gitIn(t, repo, "switch", trunk)
	gitIn(t, repo, "merge", "--ff-only", "safe")
	servertest.SetProjectRepo(t, store, "proj", repo)
	return servertest.StartWithStore(t, store)
}

func TestCleanupAgent_PromptNamesOnlyTheAgentGroup(t *testing.T) {
	h := startAgentFixture(t, true)

	res, err := h.Client.Reads.StartCleanupAgent(context.Background(), wire.CleanupWriteRequest{Project: "proj"})
	if err != nil || res.Refused || res.Address == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	files, _ := filepath.Glob(filepath.Join(h.TicketStore, "proj", "one-offs", "issues", "*.md"))
	if len(files) != 1 {
		t.Fatalf("one-off tickets = %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	got := string(raw)
	for _, want := range []string{"/gx-cleanup", "- unresolved", "- pending"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "- safe") {
		t.Errorf("prompt names the safe-case branch:\n%s", got)
	}
}

func TestCleanupAgent_RefusesWhenNothingIsForTheAgent(t *testing.T) {
	h := startAgentFixture(t, false)

	res, err := h.Client.Reads.StartCleanupAgent(context.Background(), wire.CleanupWriteRequest{Project: "proj"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonCleanupNothingForAgent {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestCleanupAgent_ARepeatPressDoesNotStartASecond(t *testing.T) {
	h := startAgentFixture(t, true)
	ctx := context.Background()

	if res, err := h.Client.Reads.StartCleanupAgent(ctx, wire.CleanupWriteRequest{Project: "proj"}); err != nil || res.Refused {
		t.Fatalf("first: %+v, %v", res, err)
	}
	res, err := h.Client.Reads.StartCleanupAgent(ctx, wire.CleanupWriteRequest{Project: "proj"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonDuplicateLive {
		t.Fatalf("second: %+v, %v", res, err)
	}
}

func TestCleanupAgent_ReportMarksTheAgentGroup(t *testing.T) {
	h := startAgentFixture(t, true)

	rep, err := h.Client.Reads.Cleanup(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	for _, wt := range rep.Worktrees {
		if wt.AgentGroup {
			marked[wt.Branch] = true
		}
	}
	for _, e := range rep.Epics {
		if e.AgentGroup {
			marked[e.Name] = true
		}
	}
	if len(marked) != 2 || !marked["unresolved"] || !marked["pending"] {
		t.Errorf("agent group = %v", marked)
	}
}

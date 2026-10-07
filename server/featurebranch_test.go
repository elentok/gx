package server_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// claimFirstChild queues the epic's only ticket and returns its file once the
// iteration has started.
func claimFirstChild(t *testing.T, repo string) string {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Orchestrator = config.OrchestratorServer })
	registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == server.EventIterationStarted {
			break
		}
	}
	data, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFeatureBranch_FirstClaimCreatesItAtTheResolvedBase(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := gitOut(t, repo, "rev-parse", "main")

	ticket := claimFirstChild(t, repo)

	if got := gitOut(t, repo, "rev-parse", "refs/heads/epic-a"); got != base {
		t.Errorf("feature branch at %s, want %s", got, base)
	}
	if !strings.Contains(ticket, "resolved_base: main@"+base) {
		t.Errorf("ticket lacks resolved_base main@%s:\n%s", base, ticket)
	}
}

func TestFeatureBranch_AnExistingBranchIsAdoptedAndItsMergeBaseRecorded(t *testing.T) {
	repo := testutil.TempRepo(t)
	base := gitOut(t, repo, "rev-parse", "main")
	gitOut(t, repo, "branch", "epic-a")
	gitOut(t, repo, "commit", "--allow-empty", "-m", "main moves on")

	ticket := claimFirstChild(t, repo)

	if !strings.Contains(ticket, "resolved_base: main@"+base) {
		t.Errorf("ticket lacks the merge-base main@%s:\n%s", base, ticket)
	}
	if got := gitOut(t, repo, "rev-parse", "refs/heads/epic-a"); got != base {
		t.Errorf("adopted branch moved to %s, want %s", got, base)
	}
}

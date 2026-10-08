package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/recovery"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// parkAndCatchInvestigation parks ticket 01 so recovery forks an investigate
// ticket, and returns the prompt that ticket's agent received.
func parkAndCatchInvestigation(t *testing.T, store string) servertest.Prompt {
	t.Helper()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.Recovery = recovery.Catalog{Enabled: true}
	})
	got := make(chan servertest.Prompt, 1)
	h.RegisterLaunch(func(p servertest.Prompt) {
		select {
		case got <- p:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := h.Server.ParkAs("proj:epic-a/01", events.IterationError, "broken"); err != nil {
		t.Fatalf("park: %v", err)
	}
	select {
	case p := <-got:
		return p
	case <-ctx.Done():
		t.Fatal("investigate ticket never launched")
		return servertest.Prompt{}
	}
}

func investigateStore(t *testing.T, projectJSON string) string {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(projectJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestInvestigateLaunch_RunsDetachedAtTheFeatureBranchTipAndNamesTheMatchedEntry(t *testing.T) {
	repo := testutil.TempRepo(t)
	gitOut(t, repo, "switch", "-c", "epic-a")
	gitOut(t, repo, "commit", "--allow-empty", "-m", "feature work")
	gitOut(t, repo, "switch", "main")
	store := investigateStore(t, `{"name":"proj","repo":"`+repo+`"}`)

	p := parkAndCatchInvestigation(t, store)

	if got, want := gitOut(t, p.Cwd, "rev-parse", "HEAD"), gitOut(t, repo, "rev-parse", "epic-a"); got != want {
		t.Errorf("investigate HEAD = %s, want the feature-branch tip %s", got, want)
	}
	if branch := gitOut(t, p.Cwd, "branch", "--show-current"); branch != "" {
		t.Errorf("investigate worktree is on branch %q, want detached", branch)
	}
	if !strings.Contains(p.Text, "Matched catalog entry: No catalog entry matched") {
		t.Errorf("prompt = %q, want it to name the matched entry", p.Text)
	}
}

func TestInvestigateLaunch_WithoutAFeatureBranchRunsAtTheResolvedBase(t *testing.T) {
	repo := testutil.TempRepo(t)
	store := investigateStore(t, `{"name":"proj","repo":"`+repo+`"}`)

	p := parkAndCatchInvestigation(t, store)

	if got, want := gitOut(t, p.Cwd, "rev-parse", "HEAD"), gitOut(t, repo, "rev-parse", "main"); got != want {
		t.Errorf("investigate HEAD = %s, want the base %s", got, want)
	}
}

func TestInvestigateLaunch_InAVCSNoneProjectRunsInAScratchSubdir(t *testing.T) {
	dir := t.TempDir()
	store := investigateStore(t, `{"name":"proj","repo":"`+dir+`","vcs":"none"}`)

	p := parkAndCatchInvestigation(t, store)

	if want := filepath.Join(server.ScratchWorkspace(store), "epic-a"); p.Cwd != want {
		t.Errorf("investigate cwd = %s, want the scratch subdir %s", p.Cwd, want)
	}
}

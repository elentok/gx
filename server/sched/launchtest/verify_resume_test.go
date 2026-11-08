package launchtest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// A relaunched verify agent is told which recorded stories stand at HEAD (skip)
// and which do not (re-check).
func TestLaunch_VerifyRelaunchListsSkippedAndRecheckedStories(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	head, err := git.RevParse(repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "verify", servertest.TicketOpts{Type: "verify"})
	servertest.SetProjectRepo(t, store, "proj", repo)
	path := filepath.Join(store, "proj", "epic-a", "issues", "01-verify.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	body = schema.AppendVerified(body, schema.VerifiedEntry{Story: "43", Verdict: "pass", Commit: head})
	body = schema.AppendVerified(body, schema.VerifiedEntry{Story: "44", Verdict: "pass", Commit: "deadbeef"})
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rigs.NewNamedFake(fake, "pty")))
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
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	for ev := range evs {
		if ev.Type == wire.EventIterationStarted {
			break
		}
	}
	t.Cleanup(func() { fake.SetState(rigs.ResumeLabel, agentrunner.StateDone, "") })

	prompts := fake.Prompts(rigs.ResumeLabel)
	if len(prompts) == 0 {
		t.Fatal("no prompt delivered")
	}
	got := prompts[0]
	skipAt, recheckAt := strings.Index(got, "Skip these stories"), strings.Index(got, "Re-check these stories")
	if skipAt < 0 || recheckAt < skipAt {
		t.Fatalf("prompt lacks skip then re-check groups:\n%s", got)
	}
	if skip, recheck := got[skipAt:recheckAt], got[recheckAt:]; !strings.Contains(skip, "- 43 ") || strings.Contains(skip, "- 44 ") || !strings.Contains(recheck, "- 44 ") {
		t.Errorf("story 43 should be skipped and 44 re-checked:\n%s", got)
	}
}

package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// Seam: once a restarted server sees the agent waited past the grace period it
// frees the only slot, so a queued root claims; the flag is in runs.json.
func TestSlotRelease_QueuedRootClaimsAfterGrace(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-b", "01", "second", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	fake := &headlessFake{Runner: runnerfake.NewRunner(), verdict: nativerunner.VerdictLive}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(fake), func(c *server.Config) {
		c.BudgetPollInterval = time.Hour
		c.MaxAgents = 1
		c.WaitingSlotGrace = time.Minute
		c.MaxReleasedAgents = 5
	})
	wtPath, _ := leaveIteration(t, h, store, repo, "herdr")
	if _, err := fake.Start(agentrunner.StartOptions{Label: "epic-a-iter-01", Cwd: wtPath}); err != nil {
		t.Fatal(err)
	}
	asHeadlessRun(t, h)
	markWaiting(t, h, time.Now().Add(-10*time.Minute))
	h.Restart(t)
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.QueueAdd(ctx, "proj:epic-b/01", "claude"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(h.Server.Runs()) == 2 })
	data, _ := os.ReadFile(filepath.Join(h.StateDir, "runs.json"))
	if !strings.Contains(string(data), `"slot_released":true`) {
		t.Errorf("runs.json lost slot_released:\n%s", data)
	}
}

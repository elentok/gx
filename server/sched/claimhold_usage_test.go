package sched_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/sched"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

func TestClaimHold_RateLimitHoldReasonWinsOverUsageGate(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Its own claude: the host may have none, as on CI.
	lookPath := func(file string) (string, error) { return "/fake/bin/" + file, nil }
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.LookPath = lookPath })
	x := sched.NewExplainer(sched.ExplainerDeps{Harness: h.Server.Harness(), Limits: h.Server.Limits(), Registry: h.Server.Registry()})

	now := time.Now()
	dir := filepath.Join(nativerunner.AgentsRoot(h.StateDir, "proj"), "a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(nativerunner.UsageRecord{
		SeenAt:   now,
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 99, ResetsAt: now.Add(time.Hour).Unix()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.UsageFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	h.Server.Limits().ScanUsage()

	rh := harnesschoice.ResolvedHarness{Harness: ralphloop.AgentClaude}
	hold, held := x.ClaimHold("proj", tickets.Ticket{}, rh)
	if !held || !strings.HasPrefix(hold.Verdict, "Claude usage: 99% of the 5-hour limit") {
		t.Fatalf("gate alone = %+v, %v", hold, held)
	}

	reset := now.Add(2 * time.Hour)
	h.Server.Registry().Put(runs.TrackedRun{Run: runs.Run{Address: "proj:e/01", Agent: string(ralphloop.AgentClaude)}})
	h.Server.Registry().MarkRateLimit("proj:e/01", true, reset)
	hold, held = x.ClaimHold("proj", tickets.Ticket{}, rh)
	if !held || hold.Verdict != sched.RateLimitVerdict(reset) {
		t.Errorf("hold and gate = %+v, %v; want the hold's verdict", hold, held)
	}
}

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

func TestClaimHold_RateLimitHoldReasonWinsOverUsageGate(t *testing.T) {
	t.Parallel()
	// A short path: the server's unix socket path has a length limit.
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Its own claude: the host may have none, as on CI.
	lookPath := func(file string) (string, error) { return "/fake/bin/" + file, nil }
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: store, LookPath: lookPath})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	dir := filepath.Join(nativerunner.AgentsRoot(stateDir, "proj"), "a")
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
	s.limits.ScanUsage()

	h, held := s.claimHold("proj", tickets.Ticket{}, harnesschoice.ResolvedHarness{Harness: ralphloop.AgentClaude})
	if !held || !strings.HasPrefix(h.Verdict, "Claude usage: 99% of the 5-hour limit") {
		t.Fatalf("gate alone = %+v, %v", h, held)
	}

	reset := now.Add(2 * time.Hour)
	s.registry.Put(runs.TrackedRun{Run: runs.Run{Address: "proj:e/01", Agent: string(ralphloop.AgentClaude)}})
	s.registry.MarkRateLimit("proj:e/01", true, reset)
	h, held = s.claimHold("proj", tickets.Ticket{}, harnesschoice.ResolvedHarness{Harness: ralphloop.AgentClaude})
	if !held || h.Verdict != rateLimitVerdict(reset) {
		t.Errorf("hold and gate = %+v, %v; want the hold's verdict", h, held)
	}
}

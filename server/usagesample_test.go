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
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
)

type usageRig struct {
	t        *testing.T
	store    string
	stateDir string
}

func newUsageRig(t *testing.T) *usageRig {
	t.Helper()
	// A short path: the server's unix socket path has a length limit.
	stateDir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	r := &usageRig{t: t, store: t.TempDir(), stateDir: stateDir}
	if err := os.MkdirAll(filepath.Join(r.store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.store, "proj", "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *usageRig) server() *Server {
	r.t.Helper()
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: r.stateDir, TicketStore: r.store})
	if err != nil {
		r.t.Fatal(err)
	}
	return s
}

func (r *usageRig) write(agent string, rec nativerunner.UsageRecord) {
	r.t.Helper()
	dir := filepath.Join(nativerunner.AgentsRoot(r.stateDir, "proj"), agent)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.UsageFile), data, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func usageRecord(seen time.Time, fiveHourPct float64, resets time.Time) nativerunner.UsageRecord {
	return nativerunner.UsageRecord{
		SeenAt:   seen,
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: fiveHourPct, ResetsAt: resets.Unix()},
	}
}

func TestUsageSample_NoRecordMeansNoLimit(t *testing.T) {
	t.Parallel()
	s := newUsageRig(t).server()
	s.scanUsage()
	if _, ok := s.usageWindowAtLimit(time.Now()); ok {
		t.Fatal("no sample, but a window is at limit")
	}
}

func TestUsageSample_NewestRecordAcrossAgentsWins(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	resets := now.Add(time.Hour)

	r.write("a", usageRecord(now, 50, resets))
	s.scanUsage()
	if _, ok := s.usageWindowAtLimit(now); ok {
		t.Fatal("50% is at limit")
	}

	r.write("b", usageRecord(now.Add(time.Minute), 99, resets))
	s.scanUsage()
	if _, ok := s.usageWindowAtLimit(now); !ok {
		t.Fatal("newer 99% record did not replace the sample")
	}

	r.write("c", usageRecord(now.Add(-time.Minute), 10, resets))
	s.scanUsage()
	if _, ok := s.usageWindowAtLimit(now); !ok {
		t.Fatal("older record replaced the newer sample")
	}
}

func TestUsageSample_LimitIs98PercentUntilResetsAt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		pct  float64
		at   time.Duration // now, relative to resets_at
		want bool
	}{
		{"97 is under", 97, -time.Hour, false},
		{"98 is at limit", 98, -time.Hour, true},
		{"100 is at limit", 100, -time.Hour, true},
		{"reset passed", 99, time.Second, false},
		{"reset exactly now", 99, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newUsageRig(t)
			s := r.server()
			resets := time.Unix(2_000_000, 0)
			r.write("a", usageRecord(resets.Add(-2*time.Hour), tc.pct, resets))
			s.scanUsage()
			if _, got := s.usageWindowAtLimit(resets.Add(tc.at)); got != tc.want {
				t.Fatalf("at limit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUsageSample_SevenDayWindowCounts(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Unix(1_000_000, 0)
	r.write("a", nativerunner.UsageRecord{
		SeenAt:   now,
		FiveHour: &nativerunner.UsageWindow{UsedPercentage: 10, ResetsAt: now.Add(time.Hour).Unix()},
		SevenDay: &nativerunner.UsageWindow{UsedPercentage: 98, ResetsAt: now.Add(24 * time.Hour).Unix()},
	})
	s.scanUsage()
	w, ok := s.usageWindowAtLimit(now)
	if !ok || w.UsedPercentage != 98 {
		t.Fatalf("window = %+v, %v; want the 98%% seven-day window", w, ok)
	}
}

func TestClaimHold_RateLimitHoldReasonWinsOverUsageGate(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	s := r.server()
	now := time.Now()
	r.write("a", usageRecord(now, 99, now.Add(time.Hour)))
	s.scanUsage()

	h, held := s.claimHold("proj", tickets.Ticket{}, ralphloop.AgentClaude)
	if !held || !strings.HasPrefix(h.Verdict, "usage at 99% of the 5-hour limit") {
		t.Fatalf("gate alone = %+v, %v", h, held)
	}

	reset := now.Add(2 * time.Hour)
	s.registry.runs["proj:e/01"] = trackedRun{Run: Run{Address: "proj:e/01", Agent: string(ralphloop.AgentClaude)}}
	s.registry.markRateLimit("proj:e/01", true, reset)
	h, held = s.claimHold("proj", tickets.Ticket{}, ralphloop.AgentClaude)
	if !held || h.Verdict != rateLimitVerdict(reset) {
		t.Errorf("hold and gate = %+v, %v; want the hold's verdict", h, held)
	}
}

func TestUsageSample_SurvivesRestart(t *testing.T) {
	t.Parallel()
	r := newUsageRig(t)
	now := time.Unix(1_000_000, 0)
	r.write("a", usageRecord(now, 99, now.Add(time.Hour)))
	s := r.server()
	s.scanUsage()

	// The agent dir is gone (pruned); only the persisted sample is left.
	if err := os.RemoveAll(nativerunner.AgentsRoot(r.stateDir, "proj")); err != nil {
		t.Fatal(err)
	}
	// A second Server on the same state dir would fail on the lock, so reopen
	// the store as New does.
	if _, ok := openUsageSamples(r.stateDir).atLimit(now); !ok {
		t.Fatal("sample lost across restart")
	}
}

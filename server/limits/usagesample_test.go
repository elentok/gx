package limits

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
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

// limits builds the owner over the rig's dirs. env is the server env (Codex's
// home comes from it); eventInterval spaces limits-changed events.
func (r *usageRig) limits(env []string, eventInterval time.Duration) *Limits {
	r.t.Helper()
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{Env: env} }, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, set, nil, nil)
	l := New(Deps{
		Config:   Config{StateDir: r.stateDir, TicketStore: r.store, LimitsEventInterval: eventInterval},
		Settings: set, Projects: reg, Events: broker.New(64), Guards: runs.NewGuards(),
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		SpendDays: func() map[string]float64 { return nil },
	})
	r.t.Cleanup(l.Stop)
	return l
}

func (r *usageRig) server() *Limits { return r.limits(nil, 0) }

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
	if _, _, ok := s.usage.limitWindow(time.Now()); ok {
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
	if _, _, ok := s.usage.limitWindow(now); ok {
		t.Fatal("50% is at limit")
	}

	r.write("b", usageRecord(now.Add(time.Minute), 99, resets))
	s.scanUsage()
	if _, _, ok := s.usage.limitWindow(now); !ok {
		t.Fatal("newer 99% record did not replace the sample")
	}

	r.write("c", usageRecord(now.Add(-time.Minute), 10, resets))
	s.scanUsage()
	if _, _, ok := s.usage.limitWindow(now); !ok {
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
			if _, _, got := s.usage.limitWindow(resets.Add(tc.at)); got != tc.want {
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
	_, w, ok := s.usage.limitWindow(now)
	if !ok || w.UsedPercentage != 98 {
		t.Fatalf("window = %+v, %v; want the 98%% seven-day window", w, ok)
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
	// Reopen the store as New does.
	if _, _, ok := openUsageSamples(r.stateDir).limitWindow(now); !ok {
		t.Fatal("sample lost across restart")
	}
}

package server

import (
	"testing"
	"time"

	"github.com/elentok/gx/testutil/runnerfake"
)

const codexEventInterval = 20 * time.Millisecond

func (r *codexUsageRig) serverWithEvents() *Server {
	r.t.Helper()
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: r.stateDir, TicketStore: r.store,
		Env:                 []string{"HOME=" + r.t.TempDir(), "CODEX_HOME=" + r.codexHome},
		LimitsEventInterval: codexEventInterval})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { s.limitsEvents.stop() })
	return s
}

// limitsEventsSince counts limits-changed events published after seq.
func limitsEventsSince(t *testing.T, s *Server, seq uint64) int {
	t.Helper()
	time.Sleep(10 * codexEventInterval)
	ch, cancel, err := s.events.subscribe(seq)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	n := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == EventLimitsChanged {
				n++
			}
		default:
			return n
		}
	}
}

func codexSamples(s *Server, id string) []LimitSample { return s.limitHistory.Samples(id) }

func TestLimitHistoryCodex_LaterResetWinsThenHighestPercentage(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Now()
	oldReset, newReset := now.Add(time.Hour), now.Add(5*time.Hour)
	r.writeCodexAgent("old", now, codexWindowJSON(95, 300, oldReset), "")
	r.writeCodexAgent("new", now, codexWindowJSON(10, 300, newReset), "")
	r.writeCodexAgent("new2", now, codexWindowJSON(30, 300, newReset), "")
	s.scanUsage()
	got := codexSamples(s, LimitIDCodex5h)
	if len(got) != 1 || got[0].UsedPercentage != 30 || got[0].ResetsAt != newReset.Unix() {
		t.Fatalf("samples = %+v, want one at 30%% resetting %d", got, newReset.Unix())
	}
}

func TestLimitHistoryCodex_TellsWindowsApartByLength(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Now()
	r.writeCodexAgent("a", now, codexWindowJSON(20, 300, now.Add(time.Hour)), codexWindowJSON(60, 10080, now.Add(48*time.Hour)))
	s.scanUsage()
	if got := codexSamples(s, LimitIDCodex5h); len(got) != 1 || got[0].UsedPercentage != 20 {
		t.Fatalf("5h samples = %+v", got)
	}
	if got := codexSamples(s, LimitIDCodexWeekly); len(got) != 1 || got[0].UsedPercentage != 60 {
		t.Fatalf("weekly samples = %+v", got)
	}
}

func TestLimitHistoryCodex_OtherWindowLengthSavesNothing(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Now()
	r.writeCodexAgent("a", now, codexWindowJSON(20, 2880, now.Add(time.Hour)), codexWindowJSON(60, 600, now.Add(time.Hour)))
	s.scanUsage()
	for _, id := range []string{LimitIDCodex5h, LimitIDCodexWeekly} {
		if got := codexSamples(s, id); len(got) != 0 {
			t.Fatalf("%s samples = %+v, want none", id, got)
		}
	}
}

func TestLimitHistoryCodex_SavesOnChangeNotOnRepeat(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Now()
	resets := now.Add(time.Hour)
	r.writeCodexAgent("a", now.Add(-time.Minute), codexWindowJSON(20, 300, resets), "")
	s.scanUsage()
	s.scanUsage()
	if got := codexSamples(s, LimitIDCodex5h); len(got) != 1 {
		t.Fatalf("after repeat: %d samples, want 1", len(got))
	}
	r.writeCodexAgent("a", now, codexWindowJSON(25, 300, resets), "")
	s.scanUsage()
	if got := codexSamples(s, LimitIDCodex5h); len(got) != 2 || got[1].UsedPercentage != 25 {
		t.Fatalf("after change: %+v, want a second sample at 25%%", got)
	}
}

func TestLimitHistoryCodex_UnchangedNewerReadingRefreshesLatestOnly(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	base := time.Now().Add(-time.Hour)
	resets := base.Add(5 * time.Hour)
	r.writeCodexAgent("a", base, codexWindowJSON(20, 300, resets), "")
	s.scanUsage()
	first, _ := s.limitHistory.Latest(LimitIDCodex5h)

	// An idle Codex, rescanned: latest does not move.
	s.scanUsage()
	if again, _ := s.limitHistory.Latest(LimitIDCodex5h); again != first {
		t.Fatalf("idle rescan moved latest: %+v -> %+v", first, again)
	}

	time.Sleep(1100 * time.Millisecond) // received-at has second granularity
	r.writeCodexAgent("a", base.Add(time.Minute), codexWindowJSON(20, 300, resets), "")
	s.scanUsage()
	latest, _ := s.limitHistory.Latest(LimitIDCodex5h)
	if latest.ReceivedAt <= first.ReceivedAt {
		t.Fatalf("newer unchanged reading did not refresh latest: %d -> %d", first.ReceivedAt, latest.ReceivedAt)
	}
	if got := codexSamples(s, LimitIDCodex5h); len(got) != 1 {
		t.Fatalf("unchanged reading saved a sample: %+v", got)
	}
}

func TestLimitHistoryCodex_ScanThatSavesEmitsOneEvent(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.serverWithEvents()
	now := time.Now()
	r.writeCodexAgent("a", now.Add(-time.Minute), codexWindowJSON(20, 300, now.Add(time.Hour)), codexWindowJSON(60, 10080, now.Add(48*time.Hour)))
	s.scanUsage()
	if n := limitsEventsSince(t, s, 0); n != 1 {
		t.Fatalf("limits-changed events = %d, want 1 for a scan saving two limits", n)
	}
	s.scanUsage()
	if n := limitsEventsSince(t, s, 0); n != 1 {
		t.Fatalf("repeat scan emitted: %d events total, want 1", n)
	}
}

func TestLimitHistoryCodex_RestartKeepsSamples(t *testing.T) {
	t.Parallel()
	r := newCodexUsageRig(t)
	s := r.server()
	now := time.Now()
	r.writeCodexAgent("a", now, codexWindowJSON(20, 300, now.Add(time.Hour)), "")
	s.scanUsage()
	s.lock.release() // New was never Run, so nothing else frees the state-dir lock
	again := r.server()
	if got := codexSamples(again, LimitIDCodex5h); len(got) != 1 || got[0].UsedPercentage != 20 {
		t.Fatalf("after restart: %+v", got)
	}
}

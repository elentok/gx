package server

import (
	"log/slog"
	"testing"
	"time"
)

func TestRunRegistry_RateLimitMarkSetsAndClears(t *testing.T) {
	t.Parallel()
	r, _, err := openRuns(t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	r.runs["p:e/01"] = trackedRun{Run: Run{Address: "p:e/01"}}
	r.runs["p:e/02"] = trackedRun{Run: Run{Address: "p:e/02"}}
	reset := time.Now().Add(time.Hour)

	r.markRateLimit("p:e/01", true, reset)
	held := r.rateLimitedRuns()
	if len(held) != 1 || held[0].Address != "p:e/01" || !held[0].RateLimitReset.Equal(reset) {
		t.Fatalf("held = %+v, want only 01 with its reset", held)
	}

	r.markRateLimit("p:e/01", false, time.Time{})
	if held := r.rateLimitedRuns(); len(held) != 0 {
		t.Errorf("held after release = %+v, want none", held)
	}
}

package lifecycle_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// The recorder is process-wide, so this test must not run in parallel.
func TestServe_ShutdownOrder(t *testing.T) {
	h := servertest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case <-h.Server.StartupDone():
	case <-ctx.Done():
		t.Fatal("startup did not finish")
	}

	var mu sync.Mutex
	var got []string
	server.SetShutdownRecorder(func(step string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, step)
	})
	t.Cleanup(func() { server.SetShutdownRecorder(nil) })

	if err := h.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	want := []string{
		"stop-fresh", "limits-events-stop", "budget-events-stop", "session-watches-close",
		"wait-fresh", "wait-herdr", "wait-claim", "wait-prune", "wait-budget", "wait-tokens",
		"wait-gates", "wait-cpu", "wait-disks", "wait-migration-failures", "wait-health",
		"wait-usage", "wait-sweep",
		"config-watch-wait", "stop-hard-grace", "merges-close-and-wait", "stop-commits",
		"chat-close", "lock-release",
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("shutdown order = %v\nwant %v", got, want)
	}
}

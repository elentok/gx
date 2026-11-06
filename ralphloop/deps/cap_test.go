package deps

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCapInstalls_ConcurrentCalls_NeverExceedLimit(t *testing.T) {
	var running, peak atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	capped := CapInstalls(func(_ context.Context, path, logDir string) ([]string, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		running.Add(-1)
		return nil, nil
	})

	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() { _, _ = capped(context.Background(), "p", "l") })
	}
	for range maxConcurrentInstalls {
		<-entered
	}
	close(release)
	wg.Wait()

	if got := peak.Load(); got != maxConcurrentInstalls {
		t.Fatalf("peak concurrent installs = %d, want %d", got, maxConcurrentInstalls)
	}
}

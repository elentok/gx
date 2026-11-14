package index

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/testutil"
)

func runKeepFresh(t *testing.T, disable bool) *atomic.Int32 {
	t.Helper()
	idx := New(t.TempDir(), broker.New(1), nil)
	var polls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		idx.KeepFresh(ctx, WatchConfig{
			PollInterval: 10 * time.Millisecond,
			DisableWatch: disable,
			Start:        func(context.Context) bool { return true },
			OnPoll:       func() { polls.Add(1) },
		})
	}()
	t.Cleanup(func() { cancel(); <-done })
	return &polls
}

func TestKeepFresh_OnPollRunsEachTickWhenWatchIsDisabled(t *testing.T) {
	t.Parallel()
	polls := runKeepFresh(t, true)
	testutil.WaitFor(t, 5*time.Second, "several polls", func() bool { return polls.Load() >= 3 })
}

func TestKeepFresh_OnPollNeverRunsWithALiveWatch(t *testing.T) {
	t.Parallel()
	polls := runKeepFresh(t, false)
	time.Sleep(150 * time.Millisecond)
	if n := polls.Load(); n != 0 {
		t.Fatalf("OnPoll ran %d times with a live watch", n)
	}
}

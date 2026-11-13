package proctable

import (
	"maps"
	"sync"
	"testing"
	"time"
)

type batchProcs struct {
	mu     sync.Mutex
	starts map[int]string
}

func (b *batchProcs) Lookup(int) (Process, bool, error) { panic("per-pid Lookup must not be used") }

func (b *batchProcs) StartTimes([]int) (map[int]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return maps.Clone(b.starts), nil
}

func (b *batchProcs) set(pid int, start string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if start == "" {
		delete(b.starts, pid)
	} else {
		b.starts[pid] = start
	}
}

func TestExitWatcher_OnePollerClosesOnlyTheGoneOrReusedHolders(t *testing.T) {
	procs := &batchProcs{starts: map[int]string{1: "a", 2: "b", 3: "c"}}
	var w ExitWatcher
	exits := map[int]chan struct{}{1: make(chan struct{}), 2: make(chan struct{}), 3: make(chan struct{})}
	for pid, ch := range exits {
		w.Watch(procs, 5*time.Millisecond, pid, map[int]string{1: "a", 2: "b", 3: "c"}[pid], ch)
	}
	procs.set(1, "")      // gone
	procs.set(2, "other") // pid reused

	for _, pid := range []int{1, 2} {
		select {
		case <-exits[pid]:
		case <-time.After(5 * time.Second):
			t.Fatalf("holder %d exit not noticed", pid)
		}
	}
	select {
	case <-exits[3]:
		t.Fatal("live holder reported exited")
	default:
	}
}

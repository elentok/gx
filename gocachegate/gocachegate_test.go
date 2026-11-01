package gocachegate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type result struct {
	capPassed bool
	err       error
}

func waitAsync(d Deps, dir string) <-chan result {
	ch := make(chan result, 1)
	go func() {
		release, capPassed, err := d.WaitExclusive(context.Background(), dir)
		release()
		ch <- result{capPassed, err}
	}()
	return ch
}

func expectBlocked(t *testing.T, ch <-chan result) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("returned early: %+v", r)
	case <-time.After(150 * time.Millisecond):
	}
}

func expectDone(t *testing.T, ch <-chan result) result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("still waiting")
		return result{}
	}
}

func fastDeps() Deps {
	return Deps{
		Scan: func() (bool, error) { return false, nil },
		Poll: 5 * time.Millisecond,
	}
}

func TestSharedHolderBlocksExclusive(t *testing.T) {
	dir := t.TempDir()
	d := fastDeps()
	release := d.Register(dir)

	ch := waitAsync(d, dir)
	expectBlocked(t, ch)

	release()
	r := expectDone(t, ch)
	if r.err != nil || r.capPassed {
		t.Fatalf("got %+v", r)
	}
}

func TestGoProcessBlocksExclusive(t *testing.T) {
	dir := t.TempDir()
	var running atomic.Bool
	running.Store(true)
	d := fastDeps()
	d.Scan = func() (bool, error) { return running.Load(), nil }

	ch := waitAsync(d, dir)
	expectBlocked(t, ch)

	running.Store(false)
	if r := expectDone(t, ch); r.err != nil || r.capPassed {
		t.Fatalf("got %+v", r)
	}
}

func TestCapPassedReturns(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var logs []string
	d := fastDeps()
	d.Scan = func() (bool, error) { return true, nil }
	d.Cap = 20 * time.Millisecond
	d.Logf = func(f string, _ ...any) { mu.Lock(); logs = append(logs, f); mu.Unlock() }

	release, capPassed, err := d.WaitExclusive(context.Background(), dir)
	if err != nil || !capPassed {
		t.Fatalf("capPassed=%v err=%v", capPassed, err)
	}
	release()
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != 1 || !strings.Contains(logs[0], "cap passed") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestCapPassedWhileSharedHeld(t *testing.T) {
	dir := t.TempDir()
	d := fastDeps()
	d.Cap = 20 * time.Millisecond
	d.Logf = func(string, ...any) {}
	defer d.Register(dir)()

	release, capPassed, err := d.WaitExclusive(context.Background(), dir)
	if err != nil || !capPassed {
		t.Fatalf("capPassed=%v err=%v", capPassed, err)
	}
	release()
}

func TestUnopenableLockIsNoop(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var logs []string
	d := fastDeps()
	d.Logf = func(f string, _ ...any) { logs = append(logs, f) }

	d.Register(file)()
	if len(logs) != 1 || !strings.Contains(logs[0], "unregistered") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestContextCancelStopsWait(t *testing.T) {
	dir := t.TempDir()
	d := fastDeps()
	defer d.Register(dir)()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := d.WaitExclusive(ctx, dir); err == nil {
		t.Fatal("want ctx error")
	}
}

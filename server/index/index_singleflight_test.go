package index

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/tickets"
)

func writeSingleFlightTicket(t *testing.T, store, status string) {
	t.Helper()
	dir := filepath.Join(store, "proj")
	if err := os.MkdirAll(filepath.Join(dir, "epic", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: \"01\"\nstatus: " + status + "\ntype: implement\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "epic", "issues", "01-first.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Seam: concurrent Refresh calls run one scan at a time plus a shared trailing
// scan; every caller returns after a scan that started after its call, and the
// index ends on the newest scan.
func TestRefresh_ConcurrentCallsShareOneTrailingScan(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeSingleFlightTicket(t, store, "open")
	idx := New(store, broker.New(0), nil)

	var started, completed, inFlight, maxInFlight atomic.Int64
	firstRead := make(chan struct{})
	release := make(chan struct{})
	idx.scan = func(p string) ([]indexedTicket, map[string][]tickets.Epic, error) {
		n := started.Add(1)
		cur := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if cur <= m || maxInFlight.CompareAndSwap(m, cur) {
				break
			}
		}
		list, loaded, err := scanStore(p)
		if n == 1 {
			close(firstRead)
			<-release // the first scan holds a stale read
		}
		inFlight.Add(-1)
		completed.Store(n)
		return list, loaded, err
	}

	errs := make(chan error, 8)
	var wg sync.WaitGroup
	call := func() {
		defer wg.Done()
		before := started.Load()
		err := idx.Refresh()
		if err == nil && completed.Load() <= before {
			t.Errorf("returned on a scan that started before its call (started before call: %d)", before)
		}
		errs <- err
	}
	wg.Add(1)
	go call()
	<-firstRead

	writeSingleFlightTicket(t, store, "done")
	var arrived atomic.Int64
	for range 7 {
		wg.Add(1)
		go func() { arrived.Add(1); call() }()
	}
	for arrived.Load() < 7 {
		time.Sleep(time.Millisecond)
	}
	for {
		idx.flightMu.Lock()
		p := idx.pending
		idx.flightMu.Unlock()
		if p != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("max scans in flight = %d, want 1", got)
	}
	if got := started.Load(); got > 3 {
		t.Fatalf("scans = %d, want the first plus a shared trailing re-run", got)
	}
	if st := idx.Snapshot().Tickets[0].Status; st != "done" {
		t.Fatalf("index status = %q, want the newest scan's %q", st, "done")
	}
}

// Seam: a Refresh logs one line with its duration and caller.
func TestRefresh_LogsDurationAndCaller(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeSingleFlightTicket(t, store, "open")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	idx := New(store, broker.New(0), log)
	if err := idx.Refresh(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "msg=Refresh") ||
		!strings.Contains(lines[0], "duration=") ||
		!strings.Contains(lines[0], "caller=") ||
		!strings.Contains(lines[0], "TestRefresh_LogsDurationAndCaller") {
		t.Fatalf("log = %q, want one Refresh line with duration and caller", buf.String())
	}
}

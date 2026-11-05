package check

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
)

func TestCheck_FullTestRunWaitsForTheLock(t *testing.T) {
	repo, data := registeredCheckRepo(t, `{"lint":"true","test":{"full":"true","limits":{"suite":"300ms"}}}`)
	stateDir, err := config.DirsFrom(cmdtest.EnvFor(t)).StateDir()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := check.AcquireFullLock(stateDir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	lock.SetRun("HOLDER-RUN")

	// Runs that never lock start at once, while the lock is held.
	for _, args := range [][]string{
		{"--full", "--kind", "lint"},
		{"--kind", "test"},
		{"--staged", "--kind", "test"},
		{"--since", "HEAD", "--kind", "test"},
	} {
		done := make(chan int, 1)
		go func() { _, code := runCheckCmd(t, repo, args...); done <- code }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%v waited for the lock", args)
		}
	}

	type result struct {
		out  string
		code int
	}
	done := make(chan result, 1)
	go func() {
		out, code := runCheckCmd(t, repo, "--full", "--kind", "test")
		done <- result{out, code}
	}()
	select {
	case <-done:
		t.Fatal("full test run did not wait")
	case <-time.After(500 * time.Millisecond):
	}
	lock.Release()
	var r result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("full test run did not start after release")
	}
	if !strings.Contains(r.out, "HOLDER-RUN") {
		t.Errorf("no wait line naming the holder:\n%q", r.out)
	}
	// The suite limit is far smaller than the wait; the wait must not count.
	if r.code != 0 {
		t.Errorf("exit %d, want 0 (the wait counted in suite?)\n%s", r.code, r.out)
	}
	recs, err := check.ReadRecords(data, "p")
	if err != nil {
		t.Fatal(err)
	}
	var full []check.Record
	for _, rec := range recs {
		if rec.Scope == "full" && rec.KindFilter == "test" {
			full = append(full, rec)
		}
	}
	if len(full) != 1 {
		t.Fatalf("want one full test record, got %+v", recs)
	}
	if got := full[0].LockWaitMS; got < 450 {
		t.Errorf("LockWaitMS = %d, want about the 500ms the holder kept the lock", got)
	}
	for _, rec := range recs {
		if rec.Scope != "full" || rec.KindFilter != "test" {
			if rec.LockWaitMS != 0 {
				t.Errorf("%s run has lock wait %d", rec.Scope, rec.LockWaitMS)
			}
		}
	}
	if w := check.ReadWaiting(stateDir); len(w) != 0 {
		t.Errorf("waiting state left behind: %+v", w)
	}
}

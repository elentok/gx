package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/syshealth"
)

// procTiming is a Config option that sets the process timing fields.
type procTiming struct{ delay, window, cutoff, waitCap time.Duration }

func startProcs(t *testing.T, tm procTiming, procs ...syshealth.Process) *servertest.Harness {
	t.Helper()
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.HealthPollInterval = 5 * time.Millisecond
		c.ProcessSecondReadDelay = tm.delay
		c.ProcessShareWindow = tm.window
		c.ProcessCutoff = tm.cutoff
		c.ProcessWaitCap = tm.waitCap
	})
	h.Processes.Set(procs...)
	t.Cleanup(h.Processes.Release)
	return h
}

// proc is a process using ticks CPU ticks per read of the fake.
func proc(pid int, comm string, ticks uint64, mem uint64) syshealth.Process {
	return syshealth.Process{PID: pid, PPID: 1, StartUnix: 1000, Comm: comm, CPUTicks: ticks, MemBytes: mem}
}

func health(t *testing.T, h *servertest.Harness, wait bool) server.HealthReport {
	t.Helper()
	get := h.Client.Health
	if wait {
		get = h.Client.HealthWait
	}
	rep, err := get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func labels(l server.CauseList) []string {
	out := []string{}
	for _, r := range l.Rows {
		out = append(out, r.Label)
	}
	return out
}

func sameLabels(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

var slowSecondRead = procTiming{delay: 5 * time.Second, window: 100 * time.Millisecond, cutoff: 10 * time.Second}

// Seam: nobody calls the route, so the process table is never read (story 51).
func TestHealthProcs_NoCallNoReads(t *testing.T) {
	h := startProcs(t, slowSecondRead, proc(10, "a", 100, 1))
	time.Sleep(150 * time.Millisecond)
	if n := h.Processes.Reads(); n != 0 {
		t.Fatalf("reads = %d, want 0", n)
	}
}

// Seam: two callers inside the share window cost one read (story 51).
func TestHealthProcs_CallsInsideTheWindowShareOneRead(t *testing.T) {
	tm := slowSecondRead
	tm.window = time.Minute
	h := startProcs(t, tm, proc(10, "a", 100, 1))
	health(t, h, false)
	health(t, h, false)
	if n := h.Processes.Reads(); n != 1 {
		t.Fatalf("reads = %d, want 1", n)
	}
}

// Seam: the window runs from the read's own time, so calls 0.9 of a window apart
// still read once per window (story 51).
func TestHealthProcs_WindowRunsFromTheReadNotTheLastCall(t *testing.T) {
	tm := slowSecondRead
	tm.window = 500 * time.Millisecond
	h := startProcs(t, tm, proc(10, "a", 100, 1))
	spacing := tm.window * 9 / 10
	health(t, h, false)
	time.Sleep(spacing)
	health(t, h, false)
	if n := h.Processes.Reads(); n != 1 {
		t.Fatalf("reads after 0.9 window = %d, want 1", n)
	}
	time.Sleep(spacing)
	health(t, h, false)
	if n := h.Processes.Reads(); n != 2 {
		t.Fatalf("reads after 1.8 windows = %d, want 2", n)
	}
}

// Seam: a poll much slower than the window always gets a fresh read (story 51).
func TestHealthProcs_SlowPollGetsAFreshReadEachTime(t *testing.T) {
	h := startProcs(t, slowSecondRead, proc(10, "a", 100, 1))
	for i := 1; i <= 3; i++ {
		health(t, h, false)
		if n := h.Processes.Reads(); n != i {
			t.Fatalf("reads after call %d = %d, want %d", i, n, i)
		}
		time.Sleep(5 * slowSecondRead.window)
	}
}

// Seam: top 3 by CPU and by memory, with every meter normal (stories 35, 36).
func TestHealthProcs_TopThreeByCPUAndMemory(t *testing.T) {
	h := startProcs(t, procTiming{delay: 5 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second},
		proc(10, "a", 400, 10), proc(11, "b", 200, 50), proc(12, "c", 100, 30),
		proc(13, "d", 50, 70), proc(14, "e", 10, 60))
	var rep server.HealthReport
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if rep = health(t, h, true); rep.CPU.State == server.HealthStateNormal {
			break
		}
	}
	if rep.CPU.State != server.HealthStateNormal || rep.Memory.State != server.HealthStateNormal {
		t.Fatalf("meters = %s / %s, want normal", rep.CPU.State, rep.Memory.State)
	}
	if got := labels(rep.Causes.CPU); !sameLabels(got, "a", "b", "c") {
		t.Fatalf("cpu list = %v, want [a b c]", got)
	}
	if got := labels(rep.Causes.Memory); !sameLabels(got, "d", "e", "b") {
		t.Fatalf("memory list = %v, want [d e b]", got)
	}
	if top := rep.Causes.CPU.Rows[0]; top.CPU < 39 || top.CPU > 41 {
		t.Fatalf("top cpu share = %v, want 40", top.CPU)
	}
}

// Seam: a failing process source shows the unavailable marker on both lists
// while the meters still read (story 48).
func TestHealthProcs_FailingSourceGivesUnavailableAndMetersStillRead(t *testing.T) {
	h := startProcs(t, procTiming{delay: 5 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second})
	h.Processes.Fail(syshealth.ErrUnavailable)
	var rep server.HealthReport
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if rep = health(t, h, true); rep.CPU.State == server.HealthStateNormal {
			break
		}
	}
	if rep.CPU.State != server.HealthStateNormal || rep.Memory.State != server.HealthStateNormal {
		t.Fatalf("meters = %s / %s, want normal", rep.CPU.State, rep.Memory.State)
	}
	if rep.Causes.CPU.Marker != server.CauseMarkerUnavailable || rep.Causes.Memory.Marker != server.CauseMarkerUnavailable {
		t.Fatalf("causes = %+v, want unavailable on both", rep.Causes)
	}
}

// Seam: the first call answers at once with the reading marker and the memory
// list; a wait call blocks for the second read and gets the CPU list (story 49).
func TestHealthProcs_FirstCallIsReadingAndWaitGetsTheList(t *testing.T) {
	h := startProcs(t, procTiming{delay: 150 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second},
		proc(10, "a", 400, 10))
	start := time.Now()
	first := health(t, h, false)
	if first.Causes.CPU.Marker != server.CauseMarkerReading || len(first.Causes.Memory.Rows) != 1 {
		t.Fatalf("first call causes = %+v, want reading CPU and one memory row", first.Causes)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("first call took %v, want an immediate answer", time.Since(start))
	}
	waited := health(t, h, true)
	if time.Since(start) < 150*time.Millisecond {
		t.Fatalf("wait call returned after %v, want it to block for the second read", time.Since(start))
	}
	if waited.Causes.CPU.Marker != server.CauseMarkerNone || !sameLabels(labels(waited.Causes.CPU), "a") {
		t.Fatalf("wait call cpu list = %+v, want [a]", waited.Causes.CPU)
	}
}

// Seam: a wait call made at once on a quiet server gets the full list.
func TestHealthProcs_WaitOnAQuietServerBlocksForTheSecondRead(t *testing.T) {
	h := startProcs(t, procTiming{delay: 20 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second},
		proc(10, "a", 400, 10))
	rep := health(t, h, true)
	if rep.Causes.CPU.Marker != server.CauseMarkerNone || !sameLabels(labels(rep.Causes.CPU), "a") {
		t.Fatalf("cpu list = %+v, want [a]", rep.Causes.CPU)
	}
	if n := h.Processes.Reads(); n != 2 {
		t.Fatalf("reads = %d, want 2", n)
	}
}

// Seam: a second read that fails ends the wait with the unavailable marker (story 49).
func TestHealthProcs_WaitReturnsUnavailableWhenTheSecondReadFails(t *testing.T) {
	h := startProcs(t, procTiming{delay: 100 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second},
		proc(10, "a", 400, 10))
	health(t, h, false)
	h.Processes.Fail(errors.New("boom"))
	rep := health(t, h, true)
	if rep.Causes.CPU.Marker != server.CauseMarkerUnavailable {
		t.Fatalf("cpu list = %+v, want unavailable", rep.Causes.CPU)
	}
}

// Seam: a second read that never ends gives the reading marker after the cap (story 49).
func TestHealthProcs_WaitCapsAndAnswersReading(t *testing.T) {
	waitCap := 150 * time.Millisecond
	h := startProcs(t, procTiming{delay: 5 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second, waitCap: waitCap},
		proc(10, "a", 400, 10))
	health(t, h, false)
	h.Processes.Block()
	start := time.Now()
	rep := health(t, h, true)
	if took := time.Since(start); took < waitCap {
		t.Fatalf("wait returned after %v, want at least the cap %v", took, waitCap)
	}
	if rep.Causes.CPU.Marker != server.CauseMarkerReading {
		t.Fatalf("cpu list = %+v, want reading", rep.Causes.CPU)
	}
}

// Seam: a wait call stops when its request context ends.
func TestHealthProcs_WaitStopsWithTheRequestContext(t *testing.T) {
	h := startProcs(t, procTiming{delay: 5 * time.Millisecond, window: 50 * time.Millisecond, cutoff: 10 * time.Second},
		proc(10, "a", 400, 10))
	health(t, h, false)
	h.Processes.Block()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := h.Client.HealthWait(ctx); err == nil {
		t.Fatal("wait call succeeded, want the context error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("wait call took %v after its context ended", time.Since(start))
	}
}

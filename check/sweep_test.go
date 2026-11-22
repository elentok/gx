package check

import (
	"bytes"
	"testing"
	"time"
)

func TestSweepOrphans_StopsOnlyDeadOwners(t *testing.T) {
	const tmp = "/var/folders/xx/T/"
	procs := []OrphanProc{
		{PID: 10, Age: 49 * time.Hour, Command: tmp + "gxfb-111-abc/gx server"},
		{PID: 11, Age: 2 * time.Hour, Command: tmp + "gxfb-111-abc/gx agent-holder --dir x"},
		{PID: 12, Age: time.Hour, Command: tmp + "gxfb-222-def/gx server"},
		{PID: 13, Age: time.Hour, Command: "/usr/bin/gx server"},
		{PID: 14, Age: time.Hour, Command: "/other/tmp/gxfb-111-abc/gx server"},
	}
	alive := func(pid int) bool { return pid == 222 }
	var stopped []int
	var out bytes.Buffer

	n := SweepOrphans(&out, tmp, procs, alive, func(pid int) error { stopped = append(stopped, pid); return nil })

	if n != 2 || len(stopped) != 2 || stopped[0] != 10 || stopped[1] != 11 {
		t.Fatalf("stopped %v (n=%d), want [10 11]", stopped, n)
	}
	if got, want := out.String(), "swept 2 orphaned fixture processes (oldest 49h0m0s)\n"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

func TestSweepOrphans_NothingToStopPrintsNothing(t *testing.T) {
	var out bytes.Buffer
	procs := []OrphanProc{{PID: 1, Command: "/t/gxfb-5-x/gx server"}}
	if n := SweepOrphans(&out, "/t", procs, func(int) bool { return true }, func(int) error { t.Fatal("killed a live owner's process"); return nil }); n != 0 || out.Len() != 0 {
		t.Errorf("n=%d out=%q, want nothing", n, out.String())
	}
}

func TestParseOrphanProcs(t *testing.T) {
	got := parseOrphanProcs("  10 02-01:02:03 /t/gxfb-1-a/gx server\n  11    05:00 /t/gxfb-1-a/gx agent-holder\nbad line\n")
	if len(got) != 2 || got[0].PID != 10 || got[0].Age != 49*time.Hour+2*time.Minute+3*time.Second ||
		got[1].Age != 5*time.Minute || got[1].Command != "/t/gxfb-1-a/gx agent-holder" {
		t.Errorf("got %+v", got)
	}
}

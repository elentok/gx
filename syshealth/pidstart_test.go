package syshealth

import (
	"os"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
)

func TestProcessStartMatchesProcessRead(t *testing.T) {
	_, read := selectProcessBackend(runtime.GOOS)
	if read == nil {
		t.Skip("no process backend on " + runtime.GOOS)
	}
	table, err := read.Read()
	if err != nil {
		t.Skipf("process read unavailable: %v", err)
	}
	got, err := ProcessStart(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range table.Processes {
		if p.PID == os.Getpid() {
			if got != p.StartUnix {
				t.Errorf("ProcessStart = %d, process read = %d", got, p.StartUnix)
			}
			if d := time.Now().Unix() - got; d < 0 || d > 3600 {
				t.Errorf("start %d is %ds ago, want within the last hour", got, d)
			}
			return
		}
	}
	t.Fatal("own pid missing from the process read")
}

func TestLinuxProcessStartFromProcFS(t *testing.T) {
	procfs := fstest.MapFS{
		"stat":    {Data: []byte("cpu 1 1 1 1 1 1 1 1\nbtime 1000\n")},
		"77/stat": {Data: []byte(statLine(77, "bash", 1, 0, 0, 2500, 0))},
	}
	got, err := linuxProcessStart(procfs, 77)
	if err != nil || got != 1025 {
		t.Errorf("linuxProcessStart = %d, %v; want 1025", got, err)
	}
	if _, err := linuxProcessStart(procfs, 78); err == nil {
		t.Error("missing pid: error = nil")
	}
}

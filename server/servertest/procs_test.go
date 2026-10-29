package servertest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessTable_LooksUpOwnPid(t *testing.T) {
	procs := NewProcessTable()
	pid := os.Getpid()

	first, ok, err := procs.Lookup(pid)
	if err != nil || !ok {
		t.Fatalf("Lookup(own pid) = ok %v, err %v", ok, err)
	}
	if first.Start == "" {
		t.Error("Start is empty")
	}
	if want := filepath.Base(os.Args[0]); !strings.Contains(first.Cmdline, want) {
		t.Errorf("Cmdline = %q, want it to name %q", first.Cmdline, want)
	}

	second, ok, err := procs.Lookup(pid)
	if err != nil || !ok {
		t.Fatalf("second Lookup = ok %v, err %v", ok, err)
	}
	if second.Start != first.Start {
		t.Errorf("Start changed between lookups: %q then %q", first.Start, second.Start)
	}
}

func TestProcessTable_MissingPid(t *testing.T) {
	// Above the kernel's pid limit on both macOS and Linux.
	_, ok, err := NewProcessTable().Lookup(1 << 30)
	if err != nil || ok {
		t.Errorf("Lookup(missing) = ok %v, err %v; want false, nil", ok, err)
	}
}

// The /proc reader compiles everywhere, so its parsing runs on every OS.
func TestProcfsStart_CommWithParens(t *testing.T) {
	stat := []byte("42 (a) b (c)) S 1 42 42 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 1 0 987654 1 2 3")
	got, ok := procfsStart(stat)
	if !ok || got != "987654" {
		t.Errorf("procfsStart = %q, %v; want 987654, true", got, ok)
	}
}

package server

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/syshealth"
)

func TestFileSystemHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system")
	read := FileSystemHealth(path)
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := read(); err == nil {
		t.Fatal("a missing file should be a read error")
	}
	write("\n")
	if _, err := read(); err == nil {
		t.Fatal("an empty file should be a read error")
	}
	write("cpu=oops mem=50\n")
	if _, err := read(); err == nil {
		t.Fatal("a bad cpu should be a read error")
	}

	write("cpu=35 mem=60 total=8G pressure=warn\n")
	a, err := read()
	if err != nil {
		t.Fatal(err)
	}
	b, err := read()
	if err != nil {
		t.Fatal(err)
	}
	busy, reason := syshealth.BusyPercent(&a, b)
	if reason != "" || math.Abs(busy-35) > 0.01 {
		t.Fatalf("busy = %v (%q), want 35", busy, reason)
	}
	if b.MemPercent != 60 || b.MemTotal != 8_000_000_000 || b.MemUsed != 4_800_000_000 || b.Pressure != syshealth.PressureWarn {
		t.Fatalf("memory fields = %+v", b)
	}

	write("cpu=90 mem=60\n")
	c, _ := read()
	if busy, _ := syshealth.BusyPercent(&b, c); math.Abs(busy-90) > 0.01 {
		t.Fatalf("busy after change = %v, want 90", busy)
	}
	if c.Pressure != 0 {
		t.Fatalf("pressure = %d, want none", c.Pressure)
	}

	write("cpu=unavailable mem=60\n")
	if _, err := read(); !errors.Is(err, syshealth.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

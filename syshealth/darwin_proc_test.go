//go:build darwin && cgo

package syshealth

import (
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"testing"
)

func TestDarwinProcessReaderReal(t *testing.T) {
	r := NewDarwinProcessReader()
	table, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Processes) < 2 {
		t.Fatalf("got %d processes", len(table.Processes))
	}
	var self, init1 *Process
	for i := range table.Processes {
		switch table.Processes[i].PID {
		case os.Getpid():
			self = &table.Processes[i]
		case 1:
			init1 = &table.Processes[i]
		}
	}
	if self == nil || self.Unreadable {
		t.Fatalf("own pid %d missing or unreadable", os.Getpid())
	}
	if self.PPID != os.Getppid() || self.Comm == "" {
		t.Errorf("own row ppid %d comm %q, want ppid %d and a comm", self.PPID, self.Comm, os.Getppid())
	}

	fp, ok := darwinFootprint(os.Getpid())
	if !ok {
		t.Fatal("footprint of own pid unreadable")
	}
	diff := int64(self.MemBytes) - int64(fp)
	if diff < 0 {
		diff = -diff
	}
	if diff > 16<<20 {
		t.Errorf("MemBytes %d vs footprint %d", self.MemBytes, fp)
	}

	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(os.Getpid()))
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("ps unavailable: %v", err)
	}
	psStart, err := ParseLstart(string(out))
	if err != nil {
		t.Fatal(err)
	}
	if d := psStart - self.StartUnix; d < -1 || d > 1 {
		t.Errorf("StartUnix %d vs ps %d", self.StartUnix, psStart)
	}

	if got := r.Argv(os.Getpid()); len(got) == 0 || got[0] == "" {
		t.Errorf("own argv = %q, want readable", got)
	}

	if init1 == nil || !init1.Unreadable {
		t.Errorf("pid 1 row = %+v, want unreadable", init1)
	}
}

func TestParseProcArgs2(t *testing.T) {
	// argc=2, exec path, NUL padding, argv, then an environment string.
	b := []byte{2, 0, 0, 0}
	b = append(b, "/bin/ls\x00\x00\x00ls\x00-l\x00HOME=/x\x00"...)
	if got, want := parseProcArgs2(b), []string{"ls", "-l"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parseProcArgs2 = %q, want %q", got, want)
	}
	if got := parseProcArgs2([]byte{1}); got != nil {
		t.Errorf("short input = %q, want nil", got)
	}
}

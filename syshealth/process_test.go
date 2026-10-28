package syshealth

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// statLine builds a /proc/<pid>/stat line with the given comm and the fields
// the parser reads; every other field is 0.
func statLine(pid int, comm string, ppid, utime, stime, start, rss int) string {
	f := make([]string, 50)
	for i := range f {
		f[i] = "0"
	}
	f[0] = "S" // field 3
	f[1] = strconv.Itoa(ppid)
	f[11] = strconv.Itoa(utime)
	f[12] = strconv.Itoa(stime)
	f[19] = strconv.Itoa(start)
	f[21] = strconv.Itoa(rss)
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(f, " "))
}

func TestParsePidStat(t *testing.T) {
	tests := []struct {
		name string
		comm string
	}{
		{"plain comm", "bash"},
		{"comm with spaces", "Web Content"},
		{"comm with paren", "a) b (c)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePidStat([]byte(statLine(42, tc.comm, 7, 30, 12, 5000, 250)))
			if err != nil {
				t.Fatal(err)
			}
			want := PidStat{Comm: tc.comm, PPID: 7, CPUTicks: 42, StartTicks: 5000, RSSPages: 250}
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
	for _, in := range []string{"", "1 bash S 1", "1 (bash) S 1 2"} {
		if _, err := ParsePidStat([]byte(in)); err == nil {
			t.Errorf("ParsePidStat(%q): want error", in)
		}
	}
}

func TestParseProcStatBootTime(t *testing.T) {
	got, err := ParseProcStat([]byte("cpu  1 1 1 1 1 1 1 1\nbtime 1700000000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.BootTime != 1700000000 {
		t.Errorf("BootTime = %d", got.BootTime)
	}
}

func TestLinuxProcessReader(t *testing.T) {
	procfs := fstest.MapFS{
		"stat":      {Data: []byte("cpu  10 20 30 400 50 6 7 8\nbtime 1000\n")},
		"meminfo":   {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 500 kB\n")},
		"self":      {Data: []byte("not a pid")},
		"1/stat":    {Data: []byte(statLine(1, "init", 0, 5, 5, 250, 10))},
		"1/cmdline": {Data: []byte("/sbin/init\x00--flag\x00")},
		"2/stat":    {Data: []byte(statLine(2, "a b)", 1, 1, 0, 0, 1))},
		// pid 3 vanished: its dir exists but has no stat file.
		"3/cmdline": {Data: []byte("x")},
	}
	r := &LinuxProcessReader{FS: procfs, PageSize: 4096}
	table, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if table.Reading.CPUTotal != 531 {
		t.Errorf("Reading.CPUTotal = %d", table.Reading.CPUTotal)
	}
	want := []Process{
		{PID: 1, PPID: 0, StartUnix: 1002, CPUTicks: 10, MemBytes: 40960, Comm: "init"},
		{PID: 2, PPID: 1, StartUnix: 1000, CPUTicks: 1, MemBytes: 4096, Comm: "a b)"},
		{PID: 3, Unreadable: true},
	}
	if !reflect.DeepEqual(table.Processes, want) {
		t.Errorf("got %+v\nwant %+v", table.Processes, want)
	}
	if got, want := r.Argv(1), []string{"/sbin/init", "--flag"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Argv(1) = %q", got)
	}
	if got := r.Argv(2); got != nil {
		t.Errorf("Argv(2) = %q, want nil", got)
	}
}

func TestParseLstart(t *testing.T) {
	want := time.Date(2026, time.October, 28, 14, 21, 31, 0, time.Local).Unix()
	for _, in := range []string{"Wed Oct 28 14:21:31 2026", " Wed Oct 28 14:21:31 2026\n"} {
		got, err := ParseLstart(in)
		if err != nil || got != want {
			t.Errorf("ParseLstart(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	// ps pads a single-digit day with a space.
	want = time.Date(2026, time.October, 7, 9, 5, 1, 0, time.Local).Unix()
	if got, err := ParseLstart("Wed Oct  7 09:05:01 2026"); err != nil || got != want {
		t.Errorf("padded day = %d, %v; want %d", got, err, want)
	}
	if _, err := ParseLstart("garbage"); err == nil {
		t.Error("want error for garbage")
	}
}

func TestLinuxProcessReaderReal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc")
	}
	table, err := NewLinuxProcessReader().Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Processes) < 2 {
		t.Fatalf("got %d processes", len(table.Processes))
	}
	var self *Process
	for i := range table.Processes {
		if table.Processes[i].PID == os.Getpid() {
			self = &table.Processes[i]
		}
	}
	if self == nil || self.Unreadable {
		t.Fatalf("own pid %d missing or unreadable", os.Getpid())
	}

	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	var rssKB uint64
	for _, line := range strings.Split(string(status), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "VmRSS:" {
			rssKB, _ = strconv.ParseUint(f[1], 10, 64)
		}
	}
	vmRSS := rssKB * 1024
	diff := int64(self.MemBytes) - int64(vmRSS)
	if diff < 0 {
		diff = -diff
	}
	if diff > 16<<20 {
		t.Errorf("MemBytes %d vs VmRSS %d", self.MemBytes, vmRSS)
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
}

package check

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// FixtureDirPrefix starts the name of a gxfixture test run's build dir:
// gxfb-<owner pid>-<random>.
const FixtureDirPrefix = "gxfb-"

// OrphanProc is a process row for the sweep.
type OrphanProc struct {
	PID     int
	Age     time.Duration
	Command string
}

// FixtureOwnerPID parses the owner pid out of a gxfb-<pid>-<random> dir name.
func FixtureOwnerPID(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, FixtureDirPrefix)
	if !ok {
		return 0, false
	}
	digits, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	pid, err := strconv.Atoi(digits)
	return pid, err == nil && pid > 0
}

// SweepOrphans stops (via stop) every process whose executable lies under
// <tmp>/gxfb-<pid>-*/ while that owner pid is dead, then prints one line with
// the count and the oldest age. A process whose owner is alive is never touched.
// It returns how many it stopped.
func SweepOrphans(w io.Writer, tmp string, procs []OrphanProc, alive func(int) bool, stop func(int) error) int {
	base := filepath.Join(tmp, FixtureDirPrefix)
	n := 0
	var oldest time.Duration
	for _, p := range procs {
		rest, ok := strings.CutPrefix(p.Command, base)
		if !ok {
			continue
		}
		dir, _, _ := strings.Cut(rest, "/")
		owner, ok := FixtureOwnerPID(FixtureDirPrefix + dir)
		if !ok || alive(owner) || stop(p.PID) != nil {
			continue
		}
		n++
		oldest = max(oldest, p.Age)
	}
	if n > 0 {
		fmt.Fprintf(w, "swept %d orphaned fixture processes (oldest %s)\n", n, oldest)
	}
	return n
}

// SweepOrphansNow runs SweepOrphans over the live process table with SIGTERM.
// A failed `ps` or an empty tmp sweeps nothing: the sweep must never fail a
// check run.
func SweepOrphansNow(w io.Writer, tmp string, env []string) {
	if tmp == "" {
		return
	}
	ps := exec.Command("ps", "-axo", "pid=,etime=,command=")
	ps.Env = env
	out, err := ps.Output()
	if err != nil {
		return
	}
	SweepOrphans(w, tmp, parseOrphanProcs(string(out)), pidAlive,
		func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) })
}

// parseOrphanProcs reads `ps -axo pid=,etime=,command=` rows.
func parseOrphanProcs(out string) []OrphanProc {
	var procs []OrphanProc
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		age, ok := parseEtime(f[1])
		if err != nil || !ok {
			continue
		}
		procs = append(procs, OrphanProc{PID: pid, Age: age, Command: strings.Join(f[2:], " ")})
	}
	return procs
}

// parseEtime reads ps's [[dd-]hh:]mm:ss.
func parseEtime(s string) (time.Duration, bool) {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	secs := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, false
		}
		secs = secs*60 + n
	}
	return time.Duration(days*24*3600+secs) * time.Second, true
}

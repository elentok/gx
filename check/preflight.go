package check

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/brief"
)

// Machine is what a --full run looks at before it starts.
type Machine struct {
	// Load is the 1-minute load average.
	Load float64
	// TopProc and TopCPU name the busiest process; TopProc is empty when unknown.
	TopProc string
	TopCPU  float64
	// Orphans counts fixture processes whose owner pid is dead.
	Orphans int
}

// SampleMachine reads the real machine. tmp is where gxfb-* fixture dirs live.
func SampleMachine(tmp string, env []string) Machine {
	m := Machine{Load: brief.SystemLoad()}
	ps := exec.Command("ps", "-axo", "pcpu=,comm=")
	ps.Env = env
	if out, err := ps.Output(); err == nil {
		m.TopProc, m.TopCPU = topCPU(string(out))
	}
	if tmp != "" {
		ps := exec.Command("ps", "-axo", "pid=,etime=,command=")
		ps.Env = env
		if out, err := ps.Output(); err == nil {
			m.Orphans = CountOrphans(tmp, parseOrphanProcs(string(out)), pidAlive)
		}
	}
	return m
}

// topCPU reads `ps -axo pcpu=,comm=` rows and returns the busiest command.
func topCPU(out string) (string, float64) {
	var name string
	var best float64
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		cpu, err := strconv.ParseFloat(f[0], 64)
		if err == nil && cpu > best {
			best, name = cpu, filepath.Base(strings.Join(f[1:], " "))
		}
	}
	return name, best
}

// CountOrphans counts processes under <tmp>/gxfb-<pid>-*/ whose owner is dead,
// the ones SweepOrphans would stop.
func CountOrphans(tmp string, procs []OrphanProc, alive func(int) bool) int {
	return SweepOrphans(io.Discard, tmp, procs, alive, func(int) error { return nil })
}

// Preflight prints the machine lines for a --full run and reports whether the
// machine is loaded. Without requireQuiet a loaded machine only gets a warning;
// with it, Preflight returns the message that stops the run.
func Preflight(w io.Writer, m Machine, requireQuiet bool) (stop string) {
	fmt.Fprintf(w, "load average %.2f, top process %s (%.0f%% CPU), %d orphan fixture processes\n",
		m.Load, orUnknown(m.TopProc), m.TopCPU, m.Orphans)
	if !highLoad(m.Load) {
		return ""
	}
	msg := fmt.Sprintf("machine is loaded: load average %.2f is above %.1f per core (busiest: %s)",
		m.Load, HighLoadPerCore, orUnknown(m.TopProc))
	if requireQuiet {
		return msg
	}
	fmt.Fprintf(w, "warning: %s; timings may be off\n", msg)
	return ""
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

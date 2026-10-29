package herdrrunner

import (
	"syscall"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/syshealth"
)

// leftoverGrace is how long a SIGTERMed leftover gets before SIGKILL.
const leftoverGrace = 2 * time.Second

// leftover is a process identified by pid and start time, so a pid reused
// after the snapshot is never mistaken for it.
type leftover struct {
	pid   int
	start int64
}

// descendantsOf snapshots every descendant of the pane shell. It must run
// before the tab closes: closing re-parents the survivors to init, and the
// shell's pid then no longer finds them. Matching orphans by working directory
// is deliberately not done: a leftover can run anywhere.
func (r *Runner) descendantsOf(shell agentrunner.OwnerPID) []leftover {
	table, err := r.procTable()
	if err != nil {
		return nil
	}
	byParent := map[int][]syshealth.Process{}
	rootOK := false
	for _, p := range table {
		if p.Unreadable {
			continue
		}
		if p.PID == shell.PID && p.StartUnix == shell.Start {
			rootOK = true
		}
		byParent[p.PPID] = append(byParent[p.PPID], p)
	}
	if !rootOK {
		return nil
	}
	var out []leftover
	queue := []int{shell.PID}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range byParent[pid] {
			out = append(out, leftover{pid: c.PID, start: c.StartUnix})
			queue = append(queue, c.PID)
		}
	}
	return out
}

func (r *Runner) procTable() ([]syshealth.Process, error) {
	if r.Procs != nil {
		return r.Procs()
	}
	t, err := syshealth.DefaultProcess().Read()
	return t.Processes, err
}

// terminate sends SIGTERM to the leftovers still alive, then SIGKILL to those
// that outlast the grace. Each signal is preceded by a fresh pid and start-time
// check, so a reused pid is never hit.
func (r *Runner) terminate(left []leftover) {
	if len(left) == 0 {
		return
	}
	r.signalAlive(left, syscall.SIGTERM)
	_, sleep := r.clock()
	sleep(leftoverGrace)
	r.signalAlive(left, syscall.SIGKILL)
}

func (r *Runner) signalAlive(left []leftover, sig syscall.Signal) {
	table, err := r.procTable()
	if err != nil {
		return
	}
	alive := map[leftover]bool{}
	for _, p := range table {
		if !p.Unreadable {
			alive[leftover{pid: p.PID, start: p.StartUnix}] = true
		}
	}
	kill := r.Signal
	if kill == nil {
		kill = syscall.Kill
	}
	for _, l := range left {
		if alive[l] {
			_ = kill(l.pid, sig)
		}
	}
}

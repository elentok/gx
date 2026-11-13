package proctable

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExitPollInterval is how often adopted holders are checked for exit. Startup
// polling needs the fast PollInterval; noticing a dead holder does not.
const ExitPollInterval = time.Second

// startTimer is an optional ProcessTable capability: StartTimes returns the
// start time of every live pid among pids in one lookup. Missing pids are
// absent from the result.
type startTimer interface {
	StartTimes(pids []int) (map[int]string, error)
}

// StartTimes runs one ps for all pids, so N adopted holders cost one fork per
// poll instead of N.
func (psTable) StartTimes(pids []int) (map[int]string, error) {
	strs := make([]string, len(pids))
	for i, pid := range pids {
		strs[i] = strconv.Itoa(pid)
	}
	cmd := exec.Command("ps", "-ww", "-p", strings.Join(strs, ","), "-o", "pid=,lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	var exit *exec.ExitError
	// ps exits non-zero when some pid is missing, and still prints the rest.
	if err != nil && !errors.As(err, &exit) {
		return nil, err
	}
	starts := map[int]string{}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("ps output %q has no pid", line)
		}
		starts[pid] = strings.Join(fields[1:], " ")
	}
	return starts, nil
}

type exitWatch struct {
	pid   int
	start string
	exit  chan struct{}
}

// ExitWatcher closes a watch's exit channel once its holder is gone. Adopted
// holders aren't the server's children, so there is no Wait to block on. One
// goroutine serves every watch and stops when none is left.
type ExitWatcher struct {
	mu      sync.Mutex
	watches []exitWatch
	running bool
}

func (w *ExitWatcher) Watch(procs ProcessTable, interval time.Duration, pid int, start string, exit chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.watches = append(w.watches, exitWatch{pid: pid, start: start, exit: exit})
	if !w.running {
		w.running = true
		go w.loop(procs, interval)
	}
}

func (w *ExitWatcher) loop(procs ProcessTable, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if w.poll(procs) {
			return
		}
	}
}

// poll closes the exited watches and reports whether none are left.
func (w *ExitWatcher) poll(procs ProcessTable) bool {
	w.mu.Lock()
	watches := append([]exitWatch(nil), w.watches...)
	w.mu.Unlock()

	gone := goneWatches(procs, watches)

	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.watches[:0]
	for _, x := range w.watches {
		if gone[x.exit] {
			close(x.exit)
		} else {
			kept = append(kept, x)
		}
	}
	w.watches = kept
	if len(kept) == 0 {
		w.running = false
		return true
	}
	return false
}

// goneWatches picks the watches whose holder is gone or replaced by a reused
// pid. A failed lookup is retried next poll, never taken as an exit.
func goneWatches(procs ProcessTable, watches []exitWatch) map[chan struct{}]bool {
	gone := map[chan struct{}]bool{}
	if st, ok := procs.(startTimer); ok {
		pids := make([]int, len(watches))
		for i, x := range watches {
			pids[i] = x.pid
		}
		starts, err := st.StartTimes(pids)
		if err != nil {
			return gone
		}
		for _, x := range watches {
			if start, live := starts[x.pid]; !live || start != x.start {
				gone[x.exit] = true
			}
		}
		return gone
	}
	for _, x := range watches {
		proc, ok, err := procs.Lookup(x.pid)
		if err == nil && (!ok || proc.Start != x.start) {
			gone[x.exit] = true
		}
	}
	return gone
}

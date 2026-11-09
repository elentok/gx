// Package procwatch counts the descendants of a process, reports a root whose
// tree is larger than a threshold and kills a tree without letting it fork
// past the kill. It never decides which trees to watch, never notifies and
// never pauses a ticket: the caller supplies the roots and acts on a Storm.
package procwatch

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/elentok/gx/syshealth"
)

// DefaultInterval is how often the process table is read.
const DefaultInterval = 10 * time.Second

// Descendants returns the rows whose ancestry reaches root, root excluded. A
// parent that started after its child is a reused pid, not the real parent,
// so the walk stops there; an unreadable row has no parent to follow.
func Descendants(procs []syshealth.Process, root int) []syshealth.Process {
	byPID := index(procs)
	if _, ok := byPID[root]; !ok {
		return nil
	}
	var out []syshealth.Process
	for _, p := range byPID {
		if p.PID != root && reaches(byPID, p, root) {
			out = append(out, p)
		}
	}
	return out
}

// Count is the number of descendants of root.
func Count(procs []syshealth.Process, root int) int { return len(Descendants(procs, root)) }

func index(procs []syshealth.Process) map[int]syshealth.Process {
	byPID := make(map[int]syshealth.Process, len(procs))
	for _, p := range procs {
		if !p.Unreadable {
			byPID[p.PID] = p
		}
	}
	return byPID
}

func reaches(byPID map[int]syshealth.Process, p syshealth.Process, root int) bool {
	// The step bound stops a pid cycle in a table read mid-change.
	for range len(byPID) {
		parent, ok := byPID[p.PPID]
		if !ok || parent.StartUnix > p.StartUnix {
			return false
		}
		if parent.PID == root {
			return true
		}
		p = parent
	}
	return false
}

// Config is when a tree counts as a storm. A zero Threshold turns the watch off.
type Config struct {
	// Threshold is the descendant count above which a tree is a storm.
	Threshold int
	// Interval is the time between polls; zero means DefaultInterval.
	Interval time.Duration
}

func (c Config) interval() time.Duration {
	if c.Interval <= 0 {
		return DefaultInterval
	}
	return c.Interval
}

// Storm is a root whose tree is over the threshold.
type Storm struct {
	Root        int
	Descendants int
}

// Watcher polls the process table and reports storms.
type Watcher struct {
	Config
	// Settings, when set, is read before each poll and wins over Config; its
	// Interval times the next poll, so a config reload applies without a restart.
	Settings func() Config
	// Roots returns the pids to watch, asked on every poll.
	Roots  func() []int
	Report func(Storm)
	// Reader defaults to syshealth.DefaultProcess.
	Reader syshealth.ProcessReader
}

func (w *Watcher) config() Config {
	if w.Settings != nil {
		return w.Settings()
	}
	return w.Config
}

// Run polls until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	reader := w.Reader
	if reader == nil {
		reader = syshealth.DefaultProcess()
	}
	timer := time.NewTimer(w.config().interval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.poll(reader)
			timer.Reset(w.config().interval())
		}
	}
}

func (w *Watcher) poll(reader syshealth.ProcessReader) {
	cfg := w.config()
	if cfg.Threshold <= 0 || w.Roots == nil || w.Report == nil {
		return
	}
	roots := w.Roots()
	if len(roots) == 0 {
		return
	}
	table, err := reader.Read()
	if err != nil {
		return
	}
	for _, root := range roots {
		if n := Count(table.Processes, root); n > cfg.Threshold {
			w.Report(Storm{Root: root, Descendants: n})
		}
	}
}

// maxRounds bounds Kill when a zombie its parent never reaps keeps the tree
// from emptying.
const maxRounds = 50

// Killer kills process trees.
type Killer struct {
	// Reader defaults to syshealth.DefaultProcess.
	Reader syshealth.ProcessReader
	// Signal defaults to syscall.Kill.
	Signal func(pid int, sig syscall.Signal) error
}

// Kill freezes root and its descendants with SIGSTOP so nothing can fork, then
// SIGKILLs everything seen, and repeats on a fresh read until the tree is
// empty. A process forked between a read and its stop shows up in the next
// read, and is stopped before any kill.
func (k Killer) Kill(root int) error {
	reader := k.Reader
	if reader == nil {
		reader = syshealth.DefaultProcess()
	}
	signal := k.Signal
	if signal == nil {
		signal = func(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }
	}
	for range maxRounds {
		pids, err := tree(reader, root)
		if err != nil {
			return err
		}
		if len(pids) == 0 {
			return nil
		}
		for _, pid := range pids {
			_ = signal(pid, syscall.SIGSTOP)
		}
		more, err := tree(reader, root)
		if err != nil {
			return err
		}
		for _, pid := range more {
			_ = signal(pid, syscall.SIGSTOP)
		}
		for _, pid := range append(pids, more...) {
			_ = signal(pid, syscall.SIGKILL)
		}
	}
	return fmt.Errorf("procwatch: tree of %d not empty after %d rounds", root, maxRounds)
}

// tree is root plus its descendants, empty when root is gone.
func tree(reader syshealth.ProcessReader, root int) ([]int, error) {
	table, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if _, ok := index(table.Processes)[root]; !ok {
		return nil, nil
	}
	pids := []int{root}
	for _, p := range Descendants(table.Processes, root) {
		pids = append(pids, p.PID)
	}
	return pids, nil
}

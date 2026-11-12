// Package procwatch counts the descendants of a process, reports a root whose
// tree is larger than a threshold and kills a tree without letting it fork
// past the kill. It never decides which trees to watch, never notifies and
// never pauses a ticket: the caller supplies the roots and acts on a Storm.
package procwatch

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"syscall"
	"time"

	"github.com/elentok/gx/syshealth"
)

// DefaultInterval is how often the process table is read.
const DefaultInterval = 10 * time.Second

// Forest is a process table indexed once: by pid and by parent. Build it once
// per table read and ask it about every root, so a poll costs O(N) rather than
// a table scan per root.
type Forest struct {
	byPID    map[int]syshealth.Process
	children map[int][]int
}

// NewForest indexes procs. A parent that started after its child is a reused
// pid, not the real parent, so that edge is dropped; an unreadable row has no
// parent to follow and is no one's parent.
func NewForest(procs []syshealth.Process) *Forest {
	f := &Forest{
		byPID:    make(map[int]syshealth.Process, len(procs)),
		children: make(map[int][]int, len(procs)),
	}
	for _, p := range procs {
		if !p.Unreadable {
			f.byPID[p.PID] = p
		}
	}
	for _, p := range f.byPID {
		if parent, ok := f.byPID[p.PPID]; ok && parent.StartUnix <= p.StartUnix {
			f.children[p.PPID] = append(f.children[p.PPID], p.PID)
		}
	}
	return f
}

// Descendants returns the rows whose ancestry reaches root, root excluded.
func (f *Forest) Descendants(root int) []syshealth.Process {
	var out []syshealth.Process
	f.walk(root, func(p syshealth.Process) { out = append(out, p) })
	return out
}

// Count is the number of descendants of root.
func (f *Forest) Count(root int) int {
	n := 0
	f.walk(root, func(syshealth.Process) { n++ })
	return n
}

// walk visits each descendant of root once. The visited set stops a pid cycle
// in a table read mid-change.
func (f *Forest) walk(root int, visit func(syshealth.Process)) {
	if _, ok := f.byPID[root]; !ok {
		return
	}
	seen := map[int]bool{root: true}
	stack := []int{root}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range f.children[pid] {
			if seen[c] {
				continue
			}
			seen[c] = true
			visit(f.byPID[c])
			stack = append(stack, c)
		}
	}
}

// Descendants returns the rows whose ancestry reaches root, root excluded.
func Descendants(procs []syshealth.Process, root int) []syshealth.Process {
	return NewForest(procs).Descendants(root)
}

// Count is the number of descendants of root.
func Count(procs []syshealth.Process, root int) int { return NewForest(procs).Count(root) }

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
	// Foreign, when set, is called once for each tree gx did not launch that
	// goes over the threshold, and again only after it dropped back under. UID
	// is the user whose processes count as foreign trees. The trees under the
	// pids Roots returns are not foreign.
	Foreign func(Storm)
	UID     int
	// Reader defaults to syshealth.DefaultProcess.
	Reader syshealth.ProcessReader

	noticed map[int]bool // foreign roots over the threshold at the last poll
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
	if cfg.Threshold <= 0 || w.Roots == nil || (w.Report == nil && w.Foreign == nil) {
		return
	}
	roots := w.Roots()
	if len(roots) == 0 && w.Foreign == nil {
		return
	}
	table, err := reader.Read()
	if err != nil {
		return
	}
	forest := NewForest(table.Processes)
	if w.Report != nil {
		for _, root := range roots {
			if n := forest.Count(root); n > cfg.Threshold {
				w.Report(Storm{Root: root, Descendants: n})
			}
		}
	}
	if w.Foreign != nil {
		over := map[int]bool{}
		for _, st := range forest.ForeignStorms(w.UID, roots, cfg.Threshold) {
			over[st.Root] = true
			if !w.noticed[st.Root] {
				w.Foreign(st)
			}
		}
		w.noticed = over
	}
}

// ForeignStorms returns the trees of uid whose root's parent is not a process
// of uid (launchd, say) and that have more than threshold descendants outside
// every tree under gxRoots. A terminal holding a gx tree counts only its own
// other processes.
func ForeignStorms(procs []syshealth.Process, uid int, gxRoots []int, threshold int) []Storm {
	return NewForest(procs).ForeignStorms(uid, gxRoots, threshold)
}

// ForeignStorms is the package-level ForeignStorms over an already built forest.
func (f *Forest) ForeignStorms(uid int, gxRoots []int, threshold int) []Storm {
	gx := map[int]bool{}
	for _, r := range gxRoots {
		gx[r] = true
		f.walk(r, func(p syshealth.Process) { gx[p.PID] = true })
	}
	var storms []Storm
	for _, p := range f.byPID {
		if p.UID != uid || gx[p.PID] {
			continue
		}
		if parent, ok := f.byPID[p.PPID]; ok && parent.UID == uid && parent.StartUnix <= p.StartUnix {
			continue
		}
		n := 0
		f.walk(p.PID, func(d syshealth.Process) {
			if !gx[d.PID] {
				n++
			}
		})
		if n > threshold {
			storms = append(storms, Storm{Root: p.PID, Descendants: n})
		}
	}
	slices.SortFunc(storms, func(a, b Storm) int { return cmp.Compare(a.Root, b.Root) })
	return storms
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
	forest := NewForest(table.Processes)
	if _, ok := forest.byPID[root]; !ok {
		return nil, nil
	}
	pids := []int{root}
	for _, p := range forest.Descendants(root) {
		pids = append(pids, p.PID)
	}
	return pids, nil
}

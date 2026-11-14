package procwatch

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/syshealth"
)

const helperEnv = "PROCWATCH_TEST_HELPER"

// TestMain re-runs this binary as the process-tree helper; without the hook
// the helper would run the test suite and fork itself without end.
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode)
		return
	}
	os.Exit(m.Run())
}

// helperLifetime ends a helper whose test never killed it.
const helperLifetime = 60 * time.Second

func runHelper(mode string) {
	pidFile := os.Getenv(helperEnv + "_PIDS")
	record := func(pid int) {
		f, err := os.OpenFile(pidFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, pid)
			f.Close()
		}
	}
	spawn := func() {
		c := exec.Command(os.Args[0])
		c.Env = append(os.Environ(), helperEnv+"=leaf")
		if err := c.Start(); err == nil {
			record(c.Process.Pid)
			go c.Wait()
		}
	}
	lifetime := time.After(helperLifetime)
	switch mode {
	case "leaf":
		<-lifetime
	case "tree":
		n, _ := strconv.Atoi(os.Getenv(helperEnv + "_N"))
		for range n {
			spawn()
		}
		<-lifetime
	case "forker":
		pace := time.NewTicker(5 * time.Millisecond)
		defer pace.Stop()
		for {
			select {
			case <-lifetime:
				return
			case <-pace.C:
				spawn()
			}
		}
	}
}

type fixture struct {
	root    *exec.Cmd
	pidFile string
	done    chan struct{}
}

func startTree(t *testing.T, mode string, n int) *fixture {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "pids")
	c := exec.Command(os.Args[0])
	c.Env = append(os.Environ(), helperEnv+"="+mode, helperEnv+"_PIDS="+pidFile, fmt.Sprintf("%s_N=%d", helperEnv, n))
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	tr := &fixture{root: c, pidFile: pidFile, done: make(chan struct{})}
	go func() { c.Wait(); close(tr.done) }()
	t.Cleanup(func() {
		for _, pid := range tr.pids() {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		_ = syscall.Kill(c.Process.Pid, syscall.SIGKILL)
	})
	return tr
}

func (tr *fixture) pids() []int {
	f, err := os.Open(tr.pidFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []int
	s := bufio.NewScanner(f)
	for s.Scan() {
		if pid, err := strconv.Atoi(s.Text()); err == nil {
			out = append(out, pid)
		}
	}
	return out
}

func (tr *fixture) waitDescendants(t *testing.T, n int) {
	t.Helper()
	waitFor(t, func() bool {
		table, err := syshealth.DefaultProcess().Read()
		return err == nil && NewForest(table.Processes).Count(tr.root.Process.Pid) >= n
	})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// requireAllGone fails unless the root and every recorded descendant is dead,
// including ones reparented away from the root, which Count cannot see.
func (tr *fixture) requireAllGone(t *testing.T) {
	t.Helper()
	select {
	case <-tr.done:
	case <-time.After(10 * time.Second):
		t.Fatal("root still running")
	}
	waitFor(t, func() bool {
		for _, pid := range tr.pids() {
			if syscall.Kill(pid, 0) == nil {
				return false
			}
		}
		return true
	})
}

func TestKill_KillsWholeTreeAndLeavesNoDescendants(t *testing.T) {
	tr := startTree(t, "tree", 5)
	tr.waitDescendants(t, 5)

	if err := (Killer{}).Kill(tr.root.Process.Pid); err != nil {
		t.Fatal(err)
	}
	tr.requireAllGone(t)
	waitFor(t, func() bool {
		table, err := syshealth.DefaultProcess().Read()
		return err == nil && NewForest(table.Processes).Count(os.Getpid()) == 0
	})
}

func TestKill_FrozenForkerCannotEscape(t *testing.T) {
	tr := startTree(t, "forker", 0)
	tr.waitDescendants(t, 3)

	if err := (Killer{}).Kill(tr.root.Process.Pid); err != nil {
		t.Fatal(err)
	}
	tr.requireAllGone(t)
	n := len(tr.pids())
	time.Sleep(100 * time.Millisecond)
	if after := len(tr.pids()); after != n {
		t.Fatalf("forker kept forking after the kill: %d -> %d pids", n, after)
	}
}

type lockedReports struct {
	mu sync.Mutex
	s  []Storm
}

func (l *lockedReports) add(s Storm) { l.mu.Lock(); l.s = append(l.s, s); l.mu.Unlock() }
func (l *lockedReports) len() int    { l.mu.Lock(); defer l.mu.Unlock(); return len(l.s) }

func runWatcher(t *testing.T, w *Watcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
}

func TestWatcher_ReportsOverThresholdEachPollAndSparesUnder(t *testing.T) {
	big := startTree(t, "tree", 4)
	small := startTree(t, "tree", 2)
	big.waitDescendants(t, 4)
	small.waitDescendants(t, 2)

	var reports lockedReports
	var mu sync.Mutex
	storms := map[int]int{}
	runWatcher(t, &Watcher{
		Config: Config{Threshold: 2, Interval: 10 * time.Millisecond},
		Roots:  func() []int { return []int{big.root.Process.Pid, small.root.Process.Pid} },
		Report: func(s Storm) {
			reports.add(s)
			mu.Lock()
			storms[s.Root]++
			mu.Unlock()
		},
	})
	waitFor(t, func() bool { return reports.len() >= 3 })

	mu.Lock()
	defer mu.Unlock()
	if storms[small.root.Process.Pid] != 0 {
		t.Fatalf("tree at the threshold was reported: %v", storms)
	}
	if storms[big.root.Process.Pid] < 3 {
		t.Fatalf("storm not reported each poll: %v", storms)
	}
	if err := syscall.Kill(small.root.Process.Pid, 0); err != nil {
		t.Fatalf("tree under threshold was touched: %v", err)
	}
}

func TestWatcher_AppliesSettingsChangesLive(t *testing.T) {
	tr := startTree(t, "tree", 3)
	tr.waitDescendants(t, 3)

	var mu sync.Mutex
	cfg := Config{Threshold: 10, Interval: 10 * time.Millisecond}
	var reports lockedReports
	runWatcher(t, &Watcher{
		Settings: func() Config { mu.Lock(); defer mu.Unlock(); return cfg },
		Roots:    func() []int { return []int{tr.root.Process.Pid} },
		Report:   reports.add,
	})
	time.Sleep(100 * time.Millisecond)
	if n := reports.len(); n != 0 {
		t.Fatalf("reported %d storms over a threshold of 10", n)
	}

	mu.Lock()
	cfg.Threshold = 1
	mu.Unlock()
	waitFor(t, func() bool { return reports.len() > 0 })
}

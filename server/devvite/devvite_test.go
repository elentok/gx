package devvite_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/server/devvite"
	"github.com/elentok/gx/testutil"
)

const waitFor = 10 * time.Second

type fakeProc struct {
	exit    chan struct{}
	err     error
	once    sync.Once
	stopped atomic.Bool
}

func (p *fakeProc) Wait() error { <-p.exit; return p.err }
func (p *fakeProc) Stop()       { p.stopped.Store(true); p.finish(nil) }
func (p *fakeProc) finish(err error) {
	p.once.Do(func() { p.err = err; close(p.exit) })
}

// fakeLauncher is a scripted Launcher: no pnpm, no Vite.
type fakeLauncher struct {
	mu         sync.Mutex
	installs   int
	starts     int
	ports      []int
	procs      []*fakeProc
	installErr error
	startErr   error
	// skipNodeModules makes Install leave no node_modules behind.
	skipNodeModules bool
	installOutput   string
	startOutput     string
	ready           atomic.Bool
	nextPort        int
	// out is the writer the last Start was given: the child's output pipe.
	out io.Writer
}

func (f *fakeLauncher) FreePort() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ports) > 0 {
		p := f.ports[0]
		f.ports = f.ports[1:]
		return p, nil
	}
	f.nextPort++
	return 40000 + f.nextPort, nil
}

func (f *fakeLauncher) Install(_ context.Context, webui string, out io.Writer) error {
	f.mu.Lock()
	f.installs++
	err, skip, text := f.installErr, f.skipNodeModules, f.installOutput
	f.mu.Unlock()
	io.WriteString(out, text)
	if err != nil {
		return err
	}
	if !skip {
		return os.MkdirAll(filepath.Join(webui, "node_modules"), 0o755)
	}
	return nil
}

func (f *fakeLauncher) Start(_ context.Context, _ string, _ int, out io.Writer) (devvite.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	f.out = out
	if f.startErr != nil {
		return nil, f.startErr
	}
	io.WriteString(out, f.startOutput)
	p := &fakeProc{exit: make(chan struct{})}
	f.procs = append(f.procs, p)
	return p, nil
}

func (f *fakeLauncher) Probe(context.Context, int) error {
	if f.ready.Load() {
		return nil
	}
	return errors.New("not answering")
}

func (f *fakeLauncher) counts() (installs, starts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installs, f.starts
}

func (f *fakeLauncher) lastProc(t *testing.T) *fakeProc {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.procs) == 0 {
		t.Fatal("no process was started")
	}
	return f.procs[len(f.procs)-1]
}

// observed records the observer's calls.
type observed struct {
	mu    sync.Mutex
	snaps []devvite.Snapshot
}

func (o *observed) add(s devvite.Snapshot) {
	o.mu.Lock()
	o.snaps = append(o.snaps, s)
	o.mu.Unlock()
}

func (o *observed) states() []devvite.State {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []devvite.State
	for _, s := range o.snaps {
		out = append(out, s.State)
	}
	return out
}

type rig struct {
	sup    *devvite.Supervisor
	l      *fakeLauncher
	obs    *observed
	logDir string
	root   string
}

func newRig(t *testing.T, mutate func(*devvite.Config)) *rig {
	t.Helper()
	r := &rig{l: &fakeLauncher{}, obs: &observed{}, logDir: filepath.Join(t.TempDir(), "logs"), root: t.TempDir()}
	cfg := devvite.Config{
		LogDir: r.logDir, Launcher: r.l, Observer: r.obs.add,
		ProbeInterval: time.Millisecond, ReadyTimeout: waitFor,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	sup, err := devvite.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sup.Close() })
	r.sup = sup
	return r
}

// checkout lays out webui with the lock file; modules says whether
// node_modules exists and is newer than the lock file.
func (r *rig) checkout(t *testing.T, modules, stale bool) {
	t.Helper()
	webui := filepath.Join(r.root, "webui")
	if err := os.MkdirAll(webui, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(webui, "pnpm-lock.yaml")
	if err := os.WriteFile(lock, []byte("lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !modules {
		return
	}
	nm := filepath.Join(webui, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	old, newer := time.Now().Add(-time.Hour), time.Now()
	if stale {
		os.Chtimes(nm, old, old)
		os.Chtimes(lock, newer, newer)
	} else {
		os.Chtimes(lock, old, old)
		os.Chtimes(nm, newer, newer)
	}
}

func (r *rig) waitState(t *testing.T, want devvite.State) devvite.Snapshot {
	t.Helper()
	testutil.WaitFor(t, waitFor, "state "+string(want), func() bool { return r.sup.Snapshot().State == want })
	return r.sup.Snapshot()
}

func (r *rig) turnOnReady(t *testing.T) {
	t.Helper()
	r.l.ready.Store(true)
	r.sup.Set(true, r.root)
	r.waitState(t, devvite.StateRunning)
}

func TestFreshNodeModulesSkipsInstall(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.turnOnReady(t)
	if installs, starts := r.l.counts(); installs != 0 || starts != 1 {
		t.Fatalf("installs=%d starts=%d, want 0 and 1", installs, starts)
	}
}

func TestMissingOrStaleNodeModulesInstallsOnceBeforeStart(t *testing.T) {
	for name, tc := range map[string]struct{ modules, stale bool }{
		"missing": {false, false},
		"stale":   {true, true},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, nil)
			r.checkout(t, tc.modules, tc.stale)
			r.turnOnReady(t)
			if installs, starts := r.l.counts(); installs != 1 || starts != 1 {
				t.Fatalf("installs=%d starts=%d, want 1 and 1", installs, starts)
			}
			want := []devvite.State{devvite.StateInstalling, devvite.StateStarting, devvite.StateRunning}
			if got := r.obs.states(); !slices.Equal(got, want) {
				t.Fatalf("states = %v, want %v", got, want)
			}
		})
	}
}

func TestRunningNeedsTheProbeToSucceed(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.sup.Set(true, r.root)
	snap := r.waitState(t, devvite.StateStarting)
	if snap.Port == 0 {
		t.Fatal("starting snapshot has no port")
	}
	// The probe has been failing since the start; the state must not move on.
	moved := testutil.Poll(50*time.Millisecond, time.Millisecond, func() bool {
		return r.sup.Snapshot().State != devvite.StateStarting
	})
	if moved {
		t.Fatalf("state = %q before the probe succeeded", r.sup.Snapshot().State)
	}
	r.l.ready.Store(true)
	r.waitState(t, devvite.StateRunning)
}

func TestStartupFailuresEndDownWithAReason(t *testing.T) {
	cases := map[string]struct {
		modules     bool
		configure   func(*fakeLauncher)
		reasonHas   string
		reasonLacks []string
	}{
		"start fails":                   {true, func(f *fakeLauncher) { f.startErr = errors.New("spawn broke") }, "spawn broke", nil},
		"pnpm missing":                  {false, func(f *fakeLauncher) { f.installErr = devvite.ErrPnpmNotFound }, "pnpm", nil},
		"install fails":                 {false, func(f *fakeLauncher) { f.installErr = errors.New("lockfile out of date") }, "lockfile out of date", nil},
		"no node_modules after install": {false, func(f *fakeLauncher) { f.skipNodeModules = true }, "node_modules", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, nil)
			r.checkout(t, tc.modules, false)
			tc.configure(r.l)
			r.sup.Set(true, r.root)
			snap := r.waitState(t, devvite.StateDown)
			if !strings.Contains(snap.Reason, tc.reasonHas) {
				t.Fatalf("reason %q does not mention %q", snap.Reason, tc.reasonHas)
			}
			lower := strings.ToLower(snap.Reason)
			if strings.Contains(lower, "restart") || strings.Contains(lower, "port") {
				t.Fatalf("reason %q mentions restarts or a port", snap.Reason)
			}
		})
	}
}

func TestEarlyExitAndNeverReadyEndDown(t *testing.T) {
	t.Run("exits before ready", func(t *testing.T) {
		r := newRig(t, nil)
		r.checkout(t, true, false)
		r.sup.Set(true, r.root)
		r.waitState(t, devvite.StateStarting)
		r.waitStarted(t)
		r.l.lastProc(t).finish(errors.New("exit status 1"))
		snap := r.waitState(t, devvite.StateDown)
		if !strings.Contains(snap.Reason, "exit status 1") {
			t.Fatalf("reason = %q", snap.Reason)
		}
	})
	t.Run("never ready", func(t *testing.T) {
		r := newRig(t, func(c *devvite.Config) { c.ReadyTimeout = 30 * time.Millisecond })
		r.checkout(t, true, false)
		r.sup.Set(true, r.root)
		snap := r.waitState(t, devvite.StateDown)
		if snap.Reason == "" {
			t.Fatal("empty reason")
		}
		if !r.l.lastProc(t).stopped.Load() {
			t.Fatal("the unready process was left running")
		}
	})
}

func (r *rig) waitStarted(t *testing.T) {
	t.Helper()
	r.waitStarts(t, 1)
}

func (r *rig) waitStarts(t *testing.T, n int) {
	t.Helper()
	testutil.WaitFor(t, waitFor, fmt.Sprintf("%d Vite start(s)", n), func() bool {
		_, starts := r.l.counts()
		return starts >= n
	})
}

func TestCrashWhileRunningGoesDownAndStaysDownUntilRestart(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.turnOnReady(t)
	r.l.lastProc(t).finish(errors.New("killed"))
	r.waitState(t, devvite.StateDown)

	// Off then on is the other way out of down.
	r.sup.Set(false, "")
	r.waitState(t, devvite.StateOff)
	r.sup.Set(true, r.root)
	r.waitState(t, devvite.StateRunning)
	if _, starts := r.l.counts(); starts != 2 {
		t.Fatalf("starts = %d, want 2 (one per turn-on)", starts)
	}

	r.l.lastProc(t).finish(errors.New("killed"))
	r.waitState(t, devvite.StateDown)
	if err := r.sup.Restart(); err != nil {
		t.Fatalf("Restart while down: %v", err)
	}
	r.waitState(t, devvite.StateRunning)
	if _, starts := r.l.counts(); starts != 3 {
		t.Fatalf("starts = %d, want 3", starts)
	}
}

func TestDownStaysDownWithoutRestart(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.turnOnReady(t)
	r.l.lastProc(t).finish(errors.New("killed"))
	r.waitState(t, devvite.StateDown)
	// Same desire again must not count as a restart.
	r.sup.Set(true, r.root)
	restarted := testutil.Poll(50*time.Millisecond, time.Millisecond, func() bool {
		_, starts := r.l.counts()
		return starts != 1
	})
	if restarted {
		t.Fatal("the crashed Vite was started again without a Restart")
	}
}

func TestRestartIsRefusedWhileNotDown(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	if err := r.sup.Restart(); err == nil {
		t.Fatal("Restart while off was accepted")
	}
	r.turnOnReady(t)
	if err := r.sup.Restart(); err == nil {
		t.Fatal("Restart while running was accepted")
	}
}

func TestOffAndCloseStopTheProcess(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		r := newRig(t, nil)
		r.checkout(t, true, false)
		r.turnOnReady(t)
		p := r.l.lastProc(t)
		r.sup.Set(false, "")
		r.waitState(t, devvite.StateOff)
		if !p.stopped.Load() {
			t.Fatal("process not stopped when the supervisor went off")
		}
	})
	t.Run("close", func(t *testing.T) {
		r := newRig(t, nil)
		r.checkout(t, true, false)
		r.turnOnReady(t)
		p := r.l.lastProc(t)
		if err := r.sup.Close(); err != nil {
			t.Fatal(err)
		}
		if !p.stopped.Load() {
			t.Fatal("process not stopped by Close")
		}
	})
}

func TestChangedCheckoutStopsTheOldChildAndStartsAgain(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.turnOnReady(t)
	first := r.l.lastProc(t)

	other := &rig{root: t.TempDir()}
	other.checkout(t, true, false)
	r.sup.Set(true, other.root)
	r.waitStarts(t, 2)
	if !first.stopped.Load() {
		t.Fatal("old child not stopped")
	}
	if got := r.waitState(t, devvite.StateRunning).Root; got != other.root {
		t.Fatalf("root = %q, want %q", got, other.root)
	}
}

func TestObserverSeesChangesNotLogLines(t *testing.T) {
	r := newRig(t, nil)
	r.checkout(t, true, false)
	r.l.startOutput = "line one\nline two\n"
	r.turnOnReady(t)
	before := len(r.obs.states())
	// Log lines the child writes after startup must not call the observer.
	for i := range 20 {
		fmt.Fprintf(r.l.out, "extra %d\n", i)
	}
	if after := len(r.obs.states()); after != before {
		t.Fatalf("observer called %d times for log lines", after-before)
	}
	want := []devvite.State{devvite.StateStarting, devvite.StateRunning}
	if got := r.obs.states(); !slices.Equal(got, want) {
		t.Fatalf("states = %v, want %v", got, want)
	}
	if logs := r.sup.Snapshot().Logs; len(logs) == 0 || logs[len(logs)-1] != "extra 19" {
		t.Fatalf("tail = %v, want it to end with the extra lines", logs)
	}
}

func TestLogFileAndTail(t *testing.T) {
	r := newRig(t, func(c *devvite.Config) { c.TailLines = 3 })
	r.checkout(t, false, false)
	r.l.installOutput = "installing\n"
	r.l.startOutput = "a\nb\nc\nd\n"
	r.turnOnReady(t)

	data, err := os.ReadFile(filepath.Join(r.logDir, "dev-vite.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "installing\na\nb\nc\nd\n" {
		t.Fatalf("log file = %q", got)
	}
	if got, want := r.sup.Snapshot().Logs, []string{"b", "c", "d"}; !slices.Equal(got, want) {
		t.Fatalf("tail = %v, want %v", got, want)
	}
}

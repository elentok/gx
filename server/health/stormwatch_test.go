package health_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/procwatch"
	"github.com/elentok/gx/server"
	healthpkg "github.com/elentok/gx/server/health"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/syshealth"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

const stormAddr = "proj:epic-a/01"

func TestStormRoots_CollectsEveryOwnerPIDOfEveryRun(t *testing.T) {
	t.Parallel()
	owned := &pidRunner{Runner: runnerfake.NewRunner(), pids: []agentrunner.OwnerPID{{PID: 10}, {PID: 11}}}
	list := []runs.TrackedRun{
		{Run: runs.Run{Address: "p:e/01"}},
		{Run: runs.Run{Address: "p:e/02"}},
	}
	runnerFor := func(r runs.Run) agentrunner.Runner {
		if r.Address == "p:e/02" {
			return nil
		}
		return owned
	}
	roots, byRoot := healthpkg.StormRoots(list, runnerFor)
	if len(roots) != 2 || roots[0] != 10 || roots[1] != 11 {
		t.Errorf("roots = %v, want [10 11]", roots)
	}
	if byRoot[10].Address != "p:e/01" || byRoot[11].Address != "p:e/01" {
		t.Errorf("byRoot = %+v, want both owned by p:e/01", byRoot)
	}
}

// startTree starts a shell with n sleeping children and returns its pid.
func startTree(t *testing.T, n int) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", strings.Repeat("sleep 100000 & ", n)+"wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

func treePIDs(t *testing.T, root int) []int {
	t.Helper()
	table, err := syshealth.DefaultProcess().Read()
	if err != nil {
		t.Fatal(err)
	}
	pids := []int{root}
	for _, p := range procwatch.Descendants(table.Processes, root) {
		pids = append(pids, p.PID)
	}
	return pids
}

func waitTreeSize(t *testing.T, root, n int) {
	t.Helper()
	testutil.WaitFor(t, 10*time.Second, "the tree to reach its size", func() bool { return len(treePIDs(t, root)) > n })
}

func anyAlive(pids []int) bool {
	for _, pid := range pids {
		if syscall.Kill(pid, 0) == nil && !isZombie(pid) {
			return true
		}
	}
	return false
}

func isZombie(pid int) bool {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// killBanners is the banners about a tree gx killed. The real process table
// also holds the test machine's own trees, which raise foreign notices.
func killBanners(h *servertest.Harness) []osnotify.Notification {
	var out []osnotify.Notification
	for _, n := range h.Banners.Sent() {
		if strings.HasPrefix(n.Title, "Process storm stopped") {
			out = append(out, n)
		}
	}
	return out
}

func watchdogConfig(threshold int) map[string]any {
	return map[string]any{"watchdog": map[string]any{"max-tree-processes": threshold, "interval": "50ms"}}
}

// startStormRun starts a harness whose one tracked run owns the tree at root.
func startStormRun(t *testing.T, root, threshold int) (*servertest.Harness, string, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, cfgPath, store, watchdogConfig(threshold))
	r := &pidRunner{Runner: runnerfake.NewRunner(), pids: []agentrunner.OwnerPID{{PID: root}}}
	h := servertest.StartWithStore(t, store, servertest.WithRunner(r), servertest.WithConfigFile(cfgPath), servertest.WithRealProcesses(), func(c *server.Config) {
		c.Settings.Watchdog.MaxTreeProcesses = threshold
		c.Settings.Watchdog.Interval = 50 * time.Millisecond
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.Queue.Add(ctx, stormAddr, "claude"); err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 10*time.Second, "the run to start", func() bool { return len(h.Server.Runs()) == 1 })
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), cfgPath
}

func TestStorm_TreeOverThresholdIsKilledWholeAndTicketPaused(t *testing.T) {
	t.Parallel()
	root := startTree(t, 6)
	waitTreeSize(t, root, 6)
	before := treePIDs(t, root)
	h, ticketPath, _ := startStormRun(t, root, 3)

	testutil.WaitFor(t, 10*time.Second, "the tree to be gone", func() bool { return !anyAlive(before) })
	testutil.WaitFor(t, 10*time.Second, "the ticket to pause", func() bool {
		tk, err := schema.ParseTicket(ticketPath)
		return err == nil && tk.Status == schema.StatusPaused && tk.PauseReason == schema.PauseProcessStorm
	})
	testutil.WaitFor(t, 10*time.Second, "the banner", func() bool { return len(killBanners(h)) > 0 })
	// Further polls must not announce the same kill again.
	time.Sleep(300 * time.Millisecond) // negative wait: a second banner would arrive within a few polls
	sent := killBanners(h)
	if len(sent) != 1 {
		t.Fatalf("banners = %+v, want exactly one", sent)
	}
	count := regexp.MustCompile(`\b(\d+) processes\b`).FindStringSubmatch(sent[0].Body)
	if !strings.Contains(sent[0].Body, stormAddr) || count == nil {
		t.Errorf("banner = %+v, want the ticket and the process count", sent[0])
	} else if n, _ := strconv.Atoi(count[1]); n < 4 {
		t.Errorf("banner count = %d, want above the threshold of 3", n)
	}
}

func TestStorm_TreeUnderThresholdIsLeftAlone(t *testing.T) {
	t.Parallel()
	root := startTree(t, 2)
	waitTreeSize(t, root, 2)
	h, ticketPath, _ := startStormRun(t, root, 50)

	time.Sleep(400 * time.Millisecond) // negative wait: a kill would land within a few 50ms polls
	if !anyAlive(treePIDs(t, root)) {
		t.Error("tree under the threshold was killed")
	}
	if n := len(killBanners(h)); n != 0 {
		t.Errorf("banners = %d, want none", n)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status == schema.StatusPaused {
		t.Errorf("ticket = %+v, %v; want not paused", tk, err)
	}
}

func TestStorm_ReloadingTheThresholdChangesTheBehaviour(t *testing.T) {
	t.Parallel()
	root := startTree(t, 6)
	waitTreeSize(t, root, 6)
	before := treePIDs(t, root)
	h, store, cfgPath := startStormRunWithStore(t, root)

	time.Sleep(300 * time.Millisecond) // negative wait: a kill would land within a few 50ms polls
	if !anyAlive(before) {
		t.Fatal("tree was killed under the high threshold")
	}
	servertest.WriteConfig(t, cfgPath, store, watchdogConfig(3))
	if res, err := h.Client.Admin.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload = %+v, %v", res, err)
	}
	testutil.WaitFor(t, 10*time.Second, "the tree to be killed after the reload", func() bool { return !anyAlive(before) })
}

// startStormSession starts a fake session whose agent spawns 6 descendants,
// and returns the holder and agent pids.
func startStormSession(t *testing.T, threshold int) (*servertest.Harness, context.Context, <-chan wire.Event, string, []int) {
	t.Helper()
	h, ctx, evs, id, conv := servertest.StartFakeSession(t, servertest.WithRealProcesses(), func(c *server.Config) {
		c.Settings.Watchdog.MaxTreeProcesses = threshold
		c.Settings.Watchdog.Interval = 50 * time.Millisecond
	})
	c := servertest.WaitForHolder(t, filepath.Join(h.SessionsDir, id))
	if err := agentfake.SendProgramControl(conv, agentfake.Control{Action: agentfake.ActionDescendants, Count: 6, Depth: 1}); err != nil {
		t.Fatal(err)
	}
	return h, ctx, evs, id, []int{c.HolderPID(), c.ChildPID()}
}

func TestStorm_SessionTreeOverThresholdIsKilledWholeWithOneBanner(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, pids := startStormSession(t, 3)

	servertest.WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	testutil.WaitFor(t, 10*time.Second, "the session tree to be gone", func() bool { return !anyAlive(pids) })
	testutil.WaitFor(t, 10*time.Second, "the banner", func() bool { return len(killBanners(h)) > 0 })
	time.Sleep(300 * time.Millisecond) // negative wait: a second banner would arrive within a few polls
	sent := killBanners(h)
	if len(sent) != 1 {
		t.Fatalf("banners = %+v, want exactly one", sent)
	}
	if !regexp.MustCompile(`\b\d+ processes\b`).MatchString(sent[0].Body) {
		t.Errorf("banner = %+v, want the process count", sent[0])
	}
}

func TestStorm_SessionTreeUnderThresholdIsLeftAlone(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, pids := startStormSession(t, 50)

	time.Sleep(400 * time.Millisecond) // negative wait: a kill would land within a few 50ms polls
	if !anyAlive(pids) {
		t.Error("session tree under the threshold was killed")
	}
	if n := len(killBanners(h)); n != 0 {
		t.Errorf("banners = %d, want none", n)
	}
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Sessions {
		if s.ID == id && s.Status.State == wire.SessionExited {
			t.Error("session exited under the threshold")
		}
	}
}

func TestStorm_ForeignTreeIsNotifiedOnceAndLeftAlone(t *testing.T) {
	t.Parallel()
	uid := os.Getuid()
	// A tree gx did not launch, staged through the fake table: a real one
	// cannot have a parent the uid does not own.
	row := func(pid, ppid, uid int) syshealth.Process {
		return syshealth.Process{PID: pid, PPID: ppid, UID: uid, StartUnix: 100}
	}
	procs := []syshealth.Process{row(1, 0, 0), row(10, 1, uid)}
	for i := range 6 {
		procs = append(procs, row(11+i, 10, uid))
	}
	bystander := startTree(t, 2)
	waitTreeSize(t, bystander, 2)
	store := t.TempDir()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Settings.Watchdog.MaxTreeProcesses = 3
		c.Settings.Watchdog.Interval = 50 * time.Millisecond
	})
	h.Processes.Set(procs...)

	testutil.WaitFor(t, 10*time.Second, "the notification", func() bool { return len(h.Banners.Sent()) > 0 })
	time.Sleep(300 * time.Millisecond) // negative wait: a second notice would arrive within a few polls
	sent := h.Banners.Sent()
	if len(sent) != 1 {
		t.Fatalf("banners = %+v, want exactly one", sent)
	}
	if !strings.Contains(sent[0].Body, "did not start it") || !strings.Contains(sent[0].Body, "did not touch it") {
		t.Errorf("banner = %+v, want it to say gx did not start or touch the tree", sent[0])
	}
	if !anyAlive(treePIDs(t, bystander)) {
		t.Error("a process outside gx was killed")
	}
}

func startStormRunWithStore(t *testing.T, root int) (*servertest.Harness, string, string) {
	t.Helper()
	h, ticketPath, cfgPath := startStormRun(t, root, 50)
	return h, filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(ticketPath)))), cfgPath
}

package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// fakeCleaner is the cleanup runner: it records each (argv, dir) pair and can
// block until released.
type fakeCleaner struct {
	mu    sync.Mutex
	calls []string
	block chan struct{} // nil: do not block
	err   error
	after func() // runs inside each call, before it returns

	release func() // releases block; set by blockUntilCleanup
}

// blockUntilCleanup makes every call block until release runs or the test ends.
// A test that fails while the cleaner is blocked would otherwise leave
// Server.Stop waiting on the open request forever.
func (f *fakeCleaner) blockUntilCleanup(t *testing.T) {
	t.Helper()
	ch := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(ch) }) }
	f.mu.Lock()
	f.block, f.release = ch, release
	f.mu.Unlock()
	t.Cleanup(release)
}

func (f *fakeCleaner) run(ctx context.Context, argv []string, dir string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, strings.Join(argv, " ")+" @ "+dir)
	block, err, after := f.block, f.err, f.after
	f.mu.Unlock()
	if after != nil {
		after()
	}
	if block != nil {
		<-block
	}
	return []byte("some output"), err
}

func (f *fakeCleaner) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type cleanRig struct {
	h      *servertest.Harness
	disks  *servertest.FakeDisks
	clean  *fakeCleaner
	store  string
	repos  map[string]string
	start  *[]string
	bodies func() []string
}

const overrideCmd = `,"disk-space":{"cleanup-command":["make","tidy"]}`
const noCmd = `,"disk-space":{"cleanup-command":[]}`

// startCleanRig starts a server whose projects (name -> project.json extras)
// each have a ticket and a repo, all on one disk with 100 GB free.
func startCleanRig(t *testing.T, projects map[string]string, warn, cleanBelow config.ByteSize, opts ...func(*server.Config)) *cleanRig {
	t.Helper()
	store := t.TempDir()
	r := &cleanRig{disks: &servertest.FakeDisks{}, clean: &fakeCleaner{}, store: store, repos: map[string]string{}}
	for name, extra := range projects {
		repo := testutil.TempRepo(t)
		r.repos[name] = repo
		servertest.WriteTicket(t, store, name, "epic-a", "01", "first", "")
		if err := servertest.WriteProjectRepo(store, name, repo, extra); err != nil {
			t.Fatal(err)
		}
		r.disks.Put(repo, "d1", "/vol", 100*gb)
	}
	var mu sync.Mutex
	var bodies []string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(chat.Close)
	r.bodies = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(bodies)
	}
	all := append([]func(*server.Config){func(c *server.Config) {
		c.FreeSpace = r.disks.Read
		c.CleanupRunner = r.clean.run
		c.DiskPollInterval = diskInterval
		c.PollInterval = 20 * time.Millisecond
		c.DiskSpace = config.DiskSpaceConfig{WarnBelow: warn, CleanBelow: cleanBelow, CleanupCommand: []string{"go", "clean", "-cache"}}
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	}}, opts...)
	r.h = servertest.StartWithStore(t, store, all...)
	r.start, _, _ = registerTurn(r.h)
	return r
}

func (r *cleanRig) setFree(free uint64) {
	for _, repo := range r.repos {
		r.disks.SetFree(repo, free)
	}
}

func (r *cleanRig) countBodies(sub string) int {
	n := 0
	for _, b := range r.bodies() {
		if strings.Contains(b, sub) {
			n++
		}
	}
	return n
}

func (r *cleanRig) waitCalls(t *testing.T, n int) []string {
	t.Helper()
	waitFor(t, func() bool { return len(r.clean.recorded()) >= n })
	return r.clean.recorded()
}

// waitClean waits for n clean notices and returns the last one's body.
func (r *cleanRig) waitClean(t *testing.T, n int) string {
	t.Helper()
	waitFor(t, func() bool { return r.countBodies("disk clean") >= n })
	var last string
	for _, b := range r.bodies() {
		if strings.Contains(b, "disk clean") {
			last = b
		}
	}
	return last
}

// settle gives the watcher time to take several readings, then checks the run count.
func (r *cleanRig) wantCalls(t *testing.T, n int) {
	t.Helper()
	letWatcherRead()
	if got := r.clean.recorded(); len(got) != n {
		t.Errorf("cleanup calls = %v, want %d", got, n)
	}
}

func (r *cleanRig) queue(t *testing.T, addr string) {
	t.Helper()
	if res, err := r.h.Client.QueueAdd(context.Background(), addr, "claude"); err != nil || res.Refused {
		t.Fatalf("queue %s = %+v, %v", addr, res, err)
	}
}

// Story 4.
func TestDiskClean_DefaultCommandRunsInTheRepo(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.setFree(3 * gb)
	got := r.waitCalls(t, 1)
	if want := "go clean -cache @ " + r.repos["proj"]; got[0] != want {
		t.Errorf("calls = %v, want %q", got, want)
	}
}

// Story 5.
func TestDiskClean_OverrideRunsForItsProjectOnly(t *testing.T) {
	r := startCleanRig(t, map[string]string{"plain": "", "custom": overrideCmd}, 0, 10*gb)
	r.setFree(3 * gb)
	got := r.waitCalls(t, 2)
	slices.Sort(got)
	want := []string{"go clean -cache @ " + r.repos["plain"], "make tidy @ " + r.repos["custom"]}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

// Story 6.
func TestDiskClean_CleanupOffHoldsAndSaysSo(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": noCmd}, 0, 10*gb)
	r.setFree(3 * gb)
	body := r.waitClean(t, 1)
	if !strings.Contains(body, "no cleanup command set") || !strings.Contains(body, "dispatch is held") {
		t.Errorf("notice = %s, want no command set and held", body)
	}
	r.wantCalls(t, 0)
	r.disksWhen(t, "one held disk", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held && !d[0].Cleaning })
	if r.countBodies("disk clean") != 1 {
		t.Errorf("clean notices = %d, want 1", r.countBodies("disk clean"))
	}
}

func (r *cleanRig) disksWhen(t *testing.T, what string, pred func([]server.DiskStatus) bool) []server.DiskStatus {
	t.Helper()
	return (&diskRig{h: r.h}).disksWhen(t, what, pred)
}

// Story 3.
func TestDiskClean_ZeroThresholdNeverCleansOrHolds(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 0)
	r.setFree(1)
	r.wantCalls(t, 0)
	r.disksWhen(t, "no listed disks", noDisks)
}

// Story 21 and the resume rule: a reading while a clean runs never clears the hold.
func TestDiskClean_HoldsWhileCleaningThenResumesWithOneCleanNotice(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb, servertest.WithHerdr(servertest.UntilPtyFixture))
	r.clean.blockUntilCleanup(t)
	r.setFree(3 * gb)
	r.waitCalls(t, 1)
	r.queue(t, "proj:epic-a/01")
	r.disksWhen(t, "held and cleaning", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held && d[0].Cleaning })
	r.setFree(50 * gb)
	letWatcherRead()
	if len(*r.start) != 0 || len(r.h.Server.Runs()) != 0 {
		t.Fatalf("a ticket started during the clean: %v", *r.start)
	}
	r.disksWhen(t, "still held", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held })
	r.clean.release()
	body := r.waitClean(t, 1)
	if !strings.Contains(body, "dispatch is not held") {
		t.Errorf("notice = %s, want not held", body)
	}
	waitFor(t, func() bool { return len(*r.start) >= 1 })
	if r.countBodies("dispatch resumed") != 0 {
		t.Error("a resume notice was sent for the end of a clean")
	}
}

// Story 22.
func TestDiskClean_DedupesPairs(t *testing.T) {
	r := startCleanRig(t, map[string]string{"a": "", "b": "", "c": ""}, 0, 10*gb)
	r.setFree(3 * gb)
	r.waitCalls(t, 3)
	r.wantCalls(t, 3)

	store := t.TempDir()
	repo := testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "x", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "y", "epic-a", "01", "first", "")
	for _, p := range []string{"x", "y"} {
		if err := servertest.WriteProjectRepo(store, p, repo, ""); err != nil {
			t.Fatal(err)
		}
	}
	disks, fake := &servertest.FakeDisks{}, &fakeCleaner{}
	disks.Put(repo, "d1", "/vol", 3*gb)
	servertest.StartWithStore(t, store, func(c *server.Config) {
		c.FreeSpace = disks.Read
		c.CleanupRunner = fake.run
		c.DiskPollInterval = diskInterval
		c.DiskSpace = config.DiskSpaceConfig{CleanBelow: 10 * gb, CleanupCommand: []string{"go", "clean", "-cache"}}
	})
	waitFor(t, func() bool { return len(fake.recorded()) >= 1 })
	letWatcherRead()
	if got := fake.recorded(); len(got) != 1 {
		t.Errorf("calls for a shared repo = %v, want 1", got)
	}
}

// Story 23.
func TestDiskClean_ReArmsOnlyWellAboveTheLine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		warn      config.ByteSize
		justAbove uint64
		wellAbove uint64
	}{
		{"warn above clean", 20 * gb, 15 * gb, 25 * gb},
		{"warn off", 0, 11 * gb, 13*gb + 1},
		{"warn equals clean", 10 * gb, 11 * gb, 13*gb + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := startCleanRig(t, map[string]string{"proj": ""}, tc.warn, 10*gb)
			r.setFree(3 * gb)
			r.waitCalls(t, 1)
			for _, f := range []uint64{8 * gb, 3 * gb, 9 * gb, 3 * gb} {
				r.setFree(f)
				letWatcherRead()
			}
			r.setFree(tc.justAbove)
			letWatcherRead()
			r.setFree(3 * gb)
			r.wantCalls(t, 1)
			r.setFree(tc.wellAbove)
			letWatcherRead()
			r.setFree(3 * gb)
			r.waitCalls(t, 2)
			r.wantCalls(t, 2)
		})
	}
}

// Story 24.
func TestDiskClean_NoCleanInNormalUse(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb, servertest.WithHerdr(servertest.UntilPtyFixture))
	r.queue(t, "proj:epic-a/01")
	waitFor(t, func() bool { return len(*r.start) >= 1 })
	r.wantCalls(t, 0)

	path := filepath.Join(r.store, "proj", "epic-a", "issues", "01-first.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	finished := strings.Replace(string(raw), "---\n", "---\ncommitless: true\niteration_status: finished\n", 1)
	if err := os.WriteFile(path, []byte(finished), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(r.h.Server.Runs()) == 0 })
	r.wantCalls(t, 0)
}

// Story 25.
func TestDiskClean_NoticeCarriesFreedFreeAndHoldState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		free  uint64
		freed string
		state string
	}{
		{"held", 4 * gb, "freed 1.0 GB; 4.0 GB free now", "dispatch is held"},
		{"not held", 50 * gb, "freed 47 GB; 50 GB free now", "dispatch is not held"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
			r.clean.after = func() { r.setFree(tc.free) }
			r.setFree(3 * gb)
			body := r.waitClean(t, 1)
			if !strings.Contains(body, tc.freed) || !strings.Contains(body, tc.state) {
				t.Errorf("notice = %s, want %q and %q", body, tc.freed, tc.state)
			}
			if r.countBodies("disk clean") != 1 {
				t.Errorf("clean notices = %d, want 1", r.countBodies("disk clean"))
			}
		})
	}
}

// Story 26.
func TestDiskClean_FailureIsLoggedNoticedAndKeepsTheHold(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 0, 10*gb)
	r.clean.err = errors.New("boom")
	r.setFree(3 * gb)
	body := r.waitClean(t, 1)
	if !strings.Contains(body, "failed") || !strings.Contains(body, "go clean -cache") {
		t.Errorf("notice = %s, want the failure and the command", body)
	}
	r.disksWhen(t, "held", func(d []server.DiskStatus) bool { return len(d) == 1 && d[0].Held && !d[0].Cleaning })
	logs, _ := os.ReadFile(server.LogPath(r.h.StateDir))
	if !strings.Contains(string(logs), "go clean -cache") || !strings.Contains(string(logs), "boom") {
		t.Errorf("server log lacks the command and its error: %s", logs)
	}
}

// Story 27.
func TestDiskClean_TimeLimitKillsTheCommand(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": `,"disk-space":{"cleanup-command":["sleep","5"]}`}, 0, 10*gb,
		func(c *server.Config) {
			c.CleanupRunner = server.RunCleanupCommand
			c.CleanupTimeout = 200 * time.Millisecond
		})
	start := time.Now()
	r.setFree(3 * gb)
	body := r.waitClean(t, 1)
	if !strings.Contains(body, "failed") || !strings.Contains(body, "sleep 5") {
		t.Errorf("notice = %s, want the timed-out command reported as a failure", body)
	}
	if time.Since(start) > 4*time.Second {
		t.Error("the command was not killed by the time limit")
	}
}

// Story 12 (clean part).
func TestDiskClean_TwoProjectsOnOneDiskSendOneNotice(t *testing.T) {
	r := startCleanRig(t, map[string]string{"a": "", "b": ""}, 0, 10*gb)
	r.setFree(3 * gb)
	r.waitClean(t, 1)
	letWatcherRead()
	if n := r.countBodies("disk clean"); n != 1 {
		t.Errorf("clean notices = %d, want 1", n)
	}
}

// A restart with a ticket already queued must see the low disk with its
// projects before the first claim, so the real command runs and no notice says
// there was nothing to clean.
func TestDiskClean_RestartWithAQueuedTicketRunsTheRealCommandOnce(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 20*gb, 10*gb)
	r.clean.blockUntilCleanup(t)
	r.queue(t, "proj:epic-a/01")
	r.setFree(3 * gb)
	r.waitCalls(t, 1)
	r.clean.release()
	r.waitClean(t, 1)
	r.h.Restart(t)
	r.start, _, _ = registerTurn(r.h)
	r.waitClean(t, 2)
	r.wantCalls(t, 2)
	for _, b := range r.bodies() {
		if strings.Contains(b, "no cleanup command set") {
			t.Errorf("notice = %s, want the real command to run", b)
		}
	}
	if len(*r.start) != 0 {
		t.Errorf("a claim started on a held disk: %v", *r.start)
	}
}

// Story 57.
func TestDiskClean_RestartOnALowDiskWarnsHoldsAndCleansAgain(t *testing.T) {
	r := startCleanRig(t, map[string]string{"proj": ""}, 20*gb, 10*gb)
	r.setFree(3 * gb)
	r.waitClean(t, 1)
	if n := r.countBodies("low disk space"); n != 1 {
		t.Fatalf("warn notices = %d, want 1", n)
	}
	r.h.Restart(t)
	r.start, _, _ = registerTurn(r.h)
	r.queue(t, "proj:epic-a/01")
	r.waitClean(t, 2)
	waitFor(t, func() bool { return r.countBodies("low disk space") == 2 })
	ex, err := r.h.Client.Explain(context.Background(), "proj:epic-a/01")
	if err != nil || ex.Verdict != server.VerdictDiskLow {
		t.Errorf("explain = %+v, %v, want %q", ex, err, server.VerdictDiskLow)
	}
	if len(*r.start) != 0 {
		t.Errorf("a claim started on a held disk: %v", *r.start)
	}
	if got := r.clean.recorded(); len(got) != 2 {
		t.Errorf("cleanup calls after a restart = %v, want 2", got)
	}
}

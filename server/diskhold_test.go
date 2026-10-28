package server_test

import (
	"context"
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

// holdRig is a server whose projects (name -> repo) sit on fake disks.
type holdRig struct {
	h      *servertest.Harness
	disks  *servertest.FakeDisks
	store  string
	start  *[]string
	resume func() int
}

// startHoldRig starts a server with the clean threshold at 10 GB. Every repo
// begins on its own disk with 100 GB free; the Go cache disk is shared and roomy.
func startHoldRig(t *testing.T, repos map[string]string, clean config.ByteSize, opts ...func(*server.Config)) *holdRig {
	t.Helper()
	store := t.TempDir()
	rig := &holdRig{disks: &servertest.FakeDisks{}, store: store}
	for name, repo := range repos {
		servertest.WriteTicket(t, store, name, "epic-a", "01", "first", "")
		servertest.SetProjectRepo(t, store, name, repo)
		rig.disks.Put(repo, "disk-"+name, "/vol-"+name, 100*gb)
	}
	var mu sync.Mutex
	resumes := 0
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "dispatch resumed") {
			mu.Lock()
			resumes++
			mu.Unlock()
		}
	}))
	t.Cleanup(chat.Close)
	rig.resume = func() int {
		mu.Lock()
		defer mu.Unlock()
		return resumes
	}
	all := append([]func(*server.Config){func(c *server.Config) {
		c.FreeSpace = rig.disks.Read
		c.DiskPollInterval = diskInterval
		c.PollInterval = 20 * time.Millisecond
		c.DiskSpace = config.DiskSpaceConfig{CleanBelow: clean}
		c.GoCacheDir = func() (string, error) { return "/gocache", nil }
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	}}, opts...)
	rig.h = servertest.StartWithStore(t, store, all...)
	rig.start, _, _ = registerLaunch(rig.h)
	return rig
}

func (r *holdRig) queue(t *testing.T, addr string) {
	t.Helper()
	if res, err := r.h.Client.QueueAdd(context.Background(), addr, "claude"); err != nil || res.Refused {
		t.Fatalf("queue %s = %+v, %v", addr, res, err)
	}
}

func (r *holdRig) explain(t *testing.T, addr string) server.Explanation {
	t.Helper()
	ex, err := r.h.Client.Explain(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

// explainWhen polls until the verdict is want.
func (r *holdRig) explainWhen(t *testing.T, addr, want string) server.Explanation {
	t.Helper()
	var ex server.Explanation
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ex = r.explain(t, addr); ex.Verdict == want {
			return ex
		}
	}
	t.Fatalf("explain %s = %+v, want verdict %q", addr, ex, want)
	return ex
}

func (r *holdRig) started(n int) func() bool { return func() bool { return len(*r.start) >= n } }

func (r *holdRig) neverStarts(t *testing.T) {
	t.Helper()
	time.Sleep(300 * time.Millisecond)
	if len(*r.start) != 0 || len(r.h.Server.Runs()) != 0 {
		t.Errorf("an agent started on a held disk: %v, runs %d", *r.start, len(r.h.Server.Runs()))
	}
}

func assertLowReason(t *testing.T, ex server.Explanation, disk, free string) {
	t.Helper()
	if ex.Verdict != server.VerdictDiskLow || !strings.Contains(ex.Reason, disk) || !strings.Contains(ex.Reason, free) {
		t.Errorf("explain = %+v, want %q naming %s and %s", ex, server.VerdictDiskLow, disk, free)
	}
}

// Seam: claim gate (story 29).
func TestDiskHold_LowDiskStopsClaimAndExplains(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb)
	r.disks.SetFree(repo, 3*gb)
	r.queue(t, "proj:epic-a/01")
	r.neverStarts(t)
	assertLowReason(t, r.explainWhen(t, "proj:epic-a/01", server.VerdictDiskLow), "/vol-proj", "3.0 GB")
}

// Seam: pre-claim reading (story 11): the timer never fires, so only the
// claim path can notice the drop. It holds the disk and starts the clean.
func TestDiskHold_PreClaimReadingHoldsBeforeTheTimer(t *testing.T) {
	repo := testutil.TempRepo(t)
	cleaner := &fakeCleaner{}
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb, func(c *server.Config) {
		c.DiskPollInterval = time.Hour
		c.CleanupRunner = cleaner.run
		c.DiskSpace.CleanupCommand = []string{"go", "clean", "-cache"}
	})
	r.disks.SetFree(repo, 3*gb)
	r.queue(t, "proj:epic-a/01")
	r.neverStarts(t)
	assertLowReason(t, r.explainWhen(t, "proj:epic-a/01", server.VerdictDiskLow), "/vol-proj", "3.0 GB")
	waitFor(t, func() bool { return len(cleaner.recorded()) >= 1 })
	if got, want := cleaner.recorded(), []string{"go clean -cache @ " + repo}; !slices.Equal(got, want) {
		t.Errorf("clean calls = %q, want %q", got, want)
	}
}

// Seam: the Go cache disk alone holds the project (story 10 part, story 29).
func TestDiskHold_GoCacheDiskAloneHolds(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb)
	r.disks.Put("/gocache", "disk-cache", "/vol-cache", 2*gb)
	r.queue(t, "proj:epic-a/01")
	r.neverStarts(t)
	assertLowReason(t, r.explainWhen(t, "proj:epic-a/01", server.VerdictDiskLow), "/vol-cache", "2.0 GB")
}

// Seam: running work keeps running (story 30).
func TestDiskHold_RunningAgentFinishesDuringHold(t *testing.T) {
	repo := testutil.TempRepo(t)
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	disks := &servertest.FakeDisks{}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.FreeSpace = disks.Read
		c.DiskPollInterval = diskInterval
		c.PollInterval = 20 * time.Millisecond
		c.DiskSpace = config.DiskSpaceConfig{CleanBelow: 10 * gb}
	})
	launched, release := make(chan struct{}), make(chan struct{})
	h.RegisterLaunch(func(p servertest.Prompt) {
		close(launched)
		<-release
		commitWork(t, p, "epic-a")
	})
	if res, err := h.Client.QueueAdd(context.Background(), "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("queue = %+v, %v", res, err)
	}
	select {
	case <-launched:
	case <-time.After(10 * time.Second):
		t.Fatal("agent never launched")
	}
	disks.SetFree(repo, gb)
	letWatcherRead()
	if len(h.Server.Runs()) != 1 {
		t.Fatalf("runs during hold = %d, want the agent still running", len(h.Server.Runs()))
	}
	close(release)
	waitFor(t, func() bool {
		b, _ := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"))
		return strings.Contains(string(b), "status: done")
	})
}

// Seam: other disks dispatch (story 31).
func TestDiskHold_OtherDiskStillDispatches(t *testing.T) {
	low, fine := testutil.TempRepo(t), testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"low": low, "fine": fine}, 10*gb)
	r.disks.SetFree(low, gb)
	r.queue(t, "low:epic-a/01")
	r.queue(t, "fine:epic-a/01")
	waitFor(t, func() bool {
		for _, run := range r.h.Server.Runs() {
			if run.Address == "fine:epic-a/01" {
				return true
			}
		}
		return false
	})
	r.explainWhen(t, "low:epic-a/01", server.VerdictDiskLow)
	for _, run := range r.h.Server.Runs() {
		if strings.HasPrefix(run.Address, "low:") {
			t.Errorf("run on the held disk: %+v", run)
		}
	}
}

// Seam: one-offs are held (story 32).
func TestDiskHold_OneOffOnHeldDiskWaits(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb)
	r.disks.SetFree(repo, gb)
	res, err := r.h.Client.OneOff(context.Background(), server.OneOffRequest{Prompt: "do it", Project: "proj"})
	if err != nil || res.Refused {
		t.Fatalf("one-off = %+v, %v", res, err)
	}
	r.neverStarts(t)
	assertLowReason(t, r.explainWhen(t, res.Address, server.VerdictDiskLow), "/vol-proj", "1.0 GB")
}

// Seam: resume (stories 34, 35): the ticket starts alone and the chat hears
// one notice, however many readings follow.
func TestDiskHold_ResumeStartsTicketAndNotifiesOnce(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb)
	r.disks.SetFree(repo, 3*gb)
	r.queue(t, "proj:epic-a/01")
	r.explainWhen(t, "proj:epic-a/01", server.VerdictDiskLow)
	r.disks.SetFree(repo, 10*gb)
	waitFor(t, r.started(1))
	waitFor(t, func() bool { return r.resume() >= 1 })
	letWatcherRead()
	time.Sleep(2 * testFlushInterval)
	if n := r.resume(); n != 1 {
		t.Errorf("resume notices = %d, want 1", n)
	}
}

// Seam: one disk, one resume (story 12, resume part).
func TestDiskHold_TwoProjectsOnOneDiskGiveOneEntryAndOneNotice(t *testing.T) {
	a, b := testutil.TempRepo(t), testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"a": a, "b": b}, 10*gb)
	r.disks.Put(a, "shared", "/shared", 3*gb)
	r.disks.Put(b, "shared", "/shared", 3*gb)
	got := waitDisks(t, r, func(d []server.DiskStatus) bool { return len(d) == 1 })
	if !got[0].Held || len(got[0].Projects) != 2 {
		t.Errorf("entry = %+v, want one held disk with both projects", got[0])
	}
	r.disks.SetFree(a, 50*gb)
	r.disks.SetFree(b, 50*gb)
	waitFor(t, func() bool { return r.resume() >= 1 })
	letWatcherRead()
	time.Sleep(2 * testFlushInterval)
	if n := r.resume(); n != 1 {
		t.Errorf("resume notices = %d, want 1", n)
	}
}

func waitDisks(t *testing.T, r *holdRig, pred func([]server.DiskStatus) bool) []server.DiskStatus {
	t.Helper()
	var last []server.DiskStatus
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		snap, err := r.h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if last = snap.Disks; pred(last) {
			return last
		}
	}
	t.Fatalf("snapshot disks = %+v", last)
	return nil
}

// Seam: a held disk is listed with the warn threshold off (story 16).
func TestDiskHold_HeldDiskListedWithWarnOff(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb)
	r.disks.SetFree(repo, 3*gb)
	got := waitDisks(t, r, func(d []server.DiskStatus) bool { return len(d) == 1 })
	if !got[0].Held || got[0].Warned {
		t.Errorf("entry = %+v, want held and not warned", got[0])
	}
	r.disks.SetFree(repo, 50*gb)
	waitDisks(t, r, func(d []server.DiskStatus) bool { return len(d) == 0 })
}

// Seam: clean threshold off never holds (story 3, hold part).
func TestDiskHold_CleanThresholdOffNeverHolds(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 0)
	r.disks.SetFree(repo, 1)
	r.queue(t, "proj:epic-a/01")
	waitFor(t, r.started(1))
}

// Seam: retry and relaunch are refused on a held disk and pass on a free one.
func TestDiskHold_RelaunchAndRetryRefused(t *testing.T) {
	repo := testutil.TempRepo(t)
	r := startHoldRig(t, map[string]string{"proj": repo}, 10*gb, func(c *server.Config) { c.DiskPollInterval = time.Hour })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := r.h.Client.TicketPause(ctx, "proj:epic-a/01", "broken"); err != nil || res.Refused {
		t.Fatalf("pause = %+v, %v", res, err)
	}
	r.disks.SetFree(repo, 3*gb)
	for name, call := range map[string]func() (server.QueueResult, error){
		"retry":    func() (server.QueueResult, error) { return r.h.Client.TicketRetry(ctx, "proj:epic-a/01", false) },
		"relaunch": func() (server.QueueResult, error) { return r.h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false) },
	} {
		res, err := call()
		if err != nil || !res.Refused || res.Reason != server.ReasonDiskLow || !strings.Contains(res.Message, "/vol-proj") || !strings.Contains(res.Message, "3.0 GB") {
			t.Errorf("%s = %+v, %v; want refusal %s naming the disk and its free space", name, res, err, server.ReasonDiskLow)
		}
	}
	if len(*r.start) != 0 {
		t.Errorf("agent started on a held disk: %v", *r.start)
	}
	r.disks.SetFree(repo, 50*gb)
	if res, err := r.h.Client.TicketRelaunch(ctx, "proj:epic-a/01", false); err != nil || res.Reason == server.ReasonDiskLow {
		t.Errorf("relaunch on a free disk = %+v, %v; want no disk refusal", res, err)
	}
}

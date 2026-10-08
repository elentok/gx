package server_test

// Server-harness ports of ralphloop's run_realgit_codex_launch scenarios
// (seam A). The originals stay in ralphloop until cutover.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// A failed launch parks the ticket needs-repair with one chat message, and the
// next ticks do not claim it again.
func TestLaunchPort_FailureAfterClaimParksNeedsRepair(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	var mu sync.Mutex
	var bodies []string
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	defer chat.Close()
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond // several ticks pass while the test waits
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")}
	})
	var prompts atomic.Int32
	h.RegisterLaunch(func(servertest.Prompt) { prompts.Add(1) })
	var starts atomic.Int32
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		starts.Add(1)
		return nil, herdrfake.Identities{}, errors.New("Herdr rejected Codex integration")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "codex"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventIterationLaunchFailed, "proj:epic-a/01")
	// The chat batches its sends; wait for the one message, then for any retry's extras.
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		mu.Lock()
		got := len(bodies)
		mu.Unlock()
		if got > 0 {
			break
		}
	}
	time.Sleep(time.Second)

	mu.Lock()
	if len(bodies) != 1 || !strings.Contains(bodies[0], "Herdr rejected Codex integration") {
		t.Errorf("chat sends = %v, want exactly 1 carrying the launch error", bodies)
	}
	mu.Unlock()
	if got := starts.Load(); got != 1 {
		t.Errorf("agent start calls = %d, want exactly 1", got)
	}
	if got := prompts.Load(); got != 0 {
		t.Errorf("prompts = %d, want none after a failed launch", got)
	}
	matches, _ := filepath.Glob(filepath.Join(store, "proj", "epic-a", "issues", "01-*.md"))
	if len(matches) != 1 {
		t.Fatalf("ticket files = %v", matches)
	}
	body := readFile(t, matches[0])
	if !strings.Contains(body, "status: needs-repair") {
		t.Errorf("ticket after launch failure:\n%s\nwant status: needs-repair", body)
	}
	for _, unwanted := range []string{"status: done", "status: needs-answer", "status: claimed", "status: open"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("ticket after launch failure must not contain %q:\n%s", unwanted, body)
		}
	}
}

// A server that restarts mid-iteration reattaches to the live agent instead of
// prompting a fresh one, and the iteration lands exactly once.
func TestLaunchPort_RestartReattachesAndLandsOnce(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLiveAgent(h)
	var prompts, starts atomic.Int32
	h.Herdr.Register("agent", "prompt", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		prompts.Add(1)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "start", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		starts.Add(1)
		return map[string]any{}, herdrfake.Identities{}, nil
	})
	leaveIteration(t, h, store, repo)

	h.Restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventRootCompleted, "")

	if prompts.Load() != 0 || starts.Load() != 0 {
		t.Errorf("agent starts = %d, prompts = %d, want the live session reused", starts.Load(), prompts.Load())
	}
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if got := git("show", "epic-a:agent.txt"); got != "work" {
		t.Errorf("feature branch agent.txt = %q, want the iteration's commit landed", got)
	}
	if n := strings.Count(git("log", "--format=%s", "epic-a"), "agent work"); n != 1 {
		t.Errorf("feature branch has %d agent commits, want exactly 1", n)
	}
	if body, err := os.ReadFile(filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")); err != nil || !strings.Contains(string(body), "status: done") {
		t.Errorf("ticket not done: %v\n%s", err, body)
	}
}

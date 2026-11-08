package launchtest_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets/schema"
)

const vimLogNeedle = "editor mode despite the editorMode normal launch override"

type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) count(needle string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Count(b.buf.String(), needle)
}

// vimLaunch queues proj:epic-a/01 on the real PTY runner over the fake claude
// with env, and returns the harness, the ticket path, the file counting the
// fake's launches (one byte each) and the server's log. Tests using it do not
// run in parallel: the log is the process-wide one.
func vimLaunch(t *testing.T, env ...string) (*servertest.Harness, string, string, *syncBuf) {
	t.Helper()
	logs := &syncBuf{}
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	counter := filepath.Join(t.TempDir(), "launches")
	for i := range env {
		env[i] = strings.ReplaceAll(env[i], "COUNTER", counter)
	}
	h := servertest.StartWithStore(t, store, servertest.WithEnv(env...), servertest.WithLaunchGrace(3*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	return h, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md"), counter, logs
}

func launchCount(counter string) int {
	data, _ := os.ReadFile(counter)
	return len(data)
}

func TestVimLaunch_ClearingWedgeIsRelaunchedOnce(t *testing.T) {
	h, _, counter, logs := vimLaunch(t, agentfake.TUIVimWedgeEnv+"=NORMAL:1:COUNTER")
	if !servertest.Poll(30*time.Second, func() bool { return len(h.Server.Runs()) == 1 && launchCount(counter) == 2 }) {
		t.Fatalf("runs = %+v, launches = %d, want one live agent after one relaunch", h.Server.Runs(), launchCount(counter))
	}
	if n := logs.count(vimLogNeedle); n != 1 {
		t.Errorf("vim log lines = %d, want 1", n)
	}
	if failed := ofType(runLog(t, h), events.LaunchFailed); len(failed) != 0 {
		t.Errorf("launch-failed events = %+v, want none: a vim relaunch is not a failed attempt", failed)
	}
}

func TestVimLaunch_PersistentFooterLogsTwiceAndTypesAnyway(t *testing.T) {
	// A hang count of 0 hangs nothing and only counts the launches.
	h, ticketPath, counter, logs := vimLaunch(t,
		agentfake.TUIIgnoreEditorModeEnv+"=1",
		agentfake.TUIHangLaunchesEnv+"=0:COUNTER")
	if !servertest.Poll(30*time.Second, func() bool { return len(h.Server.Runs()) == 1 && logs.count(vimLogNeedle) == 2 }) {
		t.Fatalf("runs = %+v, vim log lines = %d, want one live agent and two lines", h.Server.Runs(), logs.count(vimLogNeedle))
	}
	if n := launchCount(counter); n != 2 {
		t.Errorf("launches = %d, want 2", n)
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status == schema.StatusPaused {
		t.Errorf("ticket = %+v (%v), want it running, not paused", tk.Status, err)
	}
}

func TestVimLaunch_PersistentFooterWithAHangEndsPromptStalledAndRetryWorks(t *testing.T) {
	h, ticketPath, counter, logs := vimLaunch(t,
		agentfake.TUIIgnoreEditorModeEnv+"=1",
		fmt.Sprintf("%s=3:COUNTER", agentfake.TUIHangLaunchesEnv))
	servertest.WaitStatus(t, ticketPath, schema.StatusPaused)
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil || string(tk.PauseReason) != string(events.AgentPromptStalled) {
		t.Fatalf("pause reason = %q (%v), want agent_prompt_stalled", tk.PauseReason, err)
	}
	if n := launchCount(counter); n > 3 {
		t.Errorf("launches = %d, want at most 3", n)
	}
	if n := logs.count(vimLogNeedle); n < 2 {
		t.Errorf("vim log lines = %d, want at least 2", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if res, err := h.Client.Tickets.Retry(ctx, "proj:epic-a/01", false); err != nil || res.Refused {
		t.Fatalf("retry: %+v, %v", res, err)
	}
	if !servertest.Poll(30*time.Second, func() bool { return len(h.Server.Runs()) == 1 }) {
		t.Fatalf("runs = %+v, want one live agent after Retry", h.Server.Runs())
	}
	if tk, err := schema.ParseTicket(ticketPath); err != nil || tk.Status == schema.StatusPaused {
		t.Errorf("ticket = %+v (%v), want the pause ended", tk.Status, err)
	}
}

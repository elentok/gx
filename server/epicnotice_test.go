package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

const (
	epicMergedText    = "epic merged"
	epicNotMergedText = "epic complete (not merged)"
)

// chatRecorder is a server option sending chat to a recording webhook, and a
// count of how often s was sent.
func chatRecorder(t *testing.T) (opt func(*server.Config), count func(s string) int) {
	t.Helper()
	var mu sync.Mutex
	var sent strings.Builder
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent.Write(b)
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	opt = func(c *server.Config) {
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
	}
	count = func(s string) int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(sent.String(), s)
	}
	return opt, count
}

// waitForChat polls until s was sent, and reports how often.
func waitForChat(count func(string) int, s string) int {
	servertest.Poll(10*time.Second, func() bool { return count(s) > 0 })
	return count(s)
}

func TestEpicNotice_MergeSendsEpicMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	}, chat)
	f.runEpic(t, server.EventRootCompleted)

	if got := waitForChat(count, epicMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicMergedText, got)
	}
	if got := count(epicNotMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicNotMergedText, got)
	}
}

func TestEpicNotice_AutoFFMergeOffSendsNotMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}}, func(_ *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
	}, chat)
	servertest.SetProjectRepoNoAutoMerge(t, f.store, "proj", f.repo)
	f.runEpic(t, server.EventRootCompleted)

	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := count(epicMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicMergedText, got)
	}
}

func TestEpicNotice_NeedsRebasePauseSendsNotMerged(t *testing.T) {
	t.Parallel()
	chat, count := chatRecorder(t)
	f := newSchedFixture(t, atMergeCap(map[string]servertest.TicketOpts{"01": {}}), func(f *schedFixture, p *servertest.Turn, id string) {
		commitWork(t, p, id)
		testutil.WriteFile(t, f.repo, "moved.txt", "moved")
		testutil.CommitAll(t, f.repo, "main moves on")
	}, chat)
	f.runEpic(t, server.EventRootPaused)

	if got := waitForChat(count, epicNotMergedText); got != 1 {
		t.Errorf("%q sent %d times, want once", epicNotMergedText, got)
	}
	if got := count(epicMergedText); got != 0 {
		t.Errorf("%q sent %d times, want none", epicMergedText, got)
	}
}

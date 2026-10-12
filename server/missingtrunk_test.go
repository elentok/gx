package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestMissingTrunk_UnknownBranchStopsClaimsUntilCreated(t *testing.T) {
	missingTrunkCase(t, `trunk "develop" from project.json is not a local branch`,
		func(f *schedFixture) { servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "develop") },
		func(f *schedFixture) { f.git(t, "branch", "develop") })
}

func TestMissingTrunk_UnreadableProjectFileStopsClaimsUntilFixed(t *testing.T) {
	projectFile := func(f *schedFixture) string { return filepath.Join(f.store, "proj", "project.json") }
	missingTrunkCase(t, "unreadable project.json",
		func(f *schedFixture) {
			if err := os.WriteFile(projectFile(f), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		func(f *schedFixture) { servertest.SetProjectTrunk(t, f.store, "proj", f.repo, "main") })
}

// missingTrunkCase queues a one-ticket epic with its trunk broken, checks the
// project announces Missing trunk once while claiming nothing, then fixes it
// and checks the epic runs to completion after one recovery announcement.
func missingTrunkCase(t *testing.T, wantReason string, breakTrunk, fixTrunk func(f *schedFixture)) {
	t.Helper()
	var mu sync.Mutex
	var sent strings.Builder
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent.Write(b)
		mu.Unlock()
	}))
	defer chat.Close()
	chatCount := func(s string) int {
		mu.Lock()
		defer mu.Unlock()
		return strings.Count(sent.String(), s)
	}
	f := newSchedFixture(t, map[string]servertest.TicketOpts{"01": {}},
		func(_ *schedFixture, p servertest.Prompt, id string) { commitWork(t, p, id) },
		func(c *server.Config) {
			c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: testFlushInterval}
		})
	breakTrunk(f)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	evs := f.queueEpic(ctx, t, "01")
	counts := map[string]int{}
	next := func(want string) {
		t.Helper()
		for {
			select {
			case ev, ok := <-evs:
				if !ok {
					t.Fatalf("event stream closed waiting for %s", want)
				}
				counts[ev.Type]++
				if ev.Type == want {
					return
				}
			case <-ctx.Done():
				t.Fatalf("timed out waiting for %s; saw %v", want, counts)
			}
		}
	}
	next(server.EventTrunkMissing)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && chatCount("proj: trunk missing") == 0; time.Sleep(50 * time.Millisecond) {
	}
	time.Sleep(time.Second) // many polls pass; the state must stay announced once

	snap, err := f.h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Pending) != 1 || snap.Pending[0].Verdict != server.VerdictTrunkMissing || !strings.Contains(snap.Pending[0].Reason, wantReason) {
		t.Fatalf("queue rows = %+v, want one %q naming %q", snap.Pending, server.VerdictTrunkMissing, wantReason)
	}
	if body := readFile(t, f.ticketFile(t, "01")); !strings.Contains(body, "status: open") {
		t.Fatalf("ticket claimed or parked while trunk missing:\n%s", body)
	}
	if n := chatCount("proj: trunk missing"); n != 1 {
		t.Fatalf("trunk-missing chat notices = %d, want 1", n)
	}

	fixTrunk(f)
	next(server.EventTrunkFound)
	next(server.EventRootCompleted)
	if counts[server.EventTrunkMissing] != 1 || counts[server.EventTrunkFound] != 1 {
		t.Fatalf("events = %v, want one trunk-missing and one trunk-found", counts)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && chatCount("proj: trunk found") == 0; time.Sleep(50 * time.Millisecond) {
	}
	if n := chatCount("proj: trunk found"); n != 1 {
		t.Fatalf("trunk-found chat notices = %d, want 1", n)
	}
	if n := chatCount("proj: trunk missing"); n != 1 {
		t.Fatalf("trunk-missing chat notices = %d, want 1", n)
	}
}

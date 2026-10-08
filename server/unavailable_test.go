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
	"github.com/elentok/gx/testutil"
)

func TestUnavailable_MissingPathExplainsNotifiesOnceAndSetPathRecovers(t *testing.T) {
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
	unavailableSends := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, b := range bodies {
			if strings.Contains(b, "unavailable") {
				n++
			}
		}
		return n
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = 50 * time.Millisecond
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")}
	})
	registerLaunch(h)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	if res, err := h.Client.QueueAdd(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}

	var ex server.Explanation
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		var err error
		if ex, err = h.Client.Explain(ctx, "proj:epic-a/01"); err != nil {
			t.Fatal(err)
		}
		if ex.Verdict == server.VerdictProjectUnavailable {
			break
		}
	}
	if ex.Verdict != server.VerdictProjectUnavailable || !strings.Contains(ex.Reason, repo) {
		t.Fatalf("explain = %+v, want project unavailable naming %s", ex, repo)
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && unavailableSends() == 0; time.Sleep(50 * time.Millisecond) {
	}
	time.Sleep(time.Second) // many polls pass; the notification must stay single
	if n := unavailableSends(); n != 1 {
		t.Fatalf("unavailable notifications = %d, want 1", n)
	}

	fresh := testutil.TempRepo(t)
	if res, err := h.Client.SetProjectPath(ctx, server.ProjectRequest{Name: "proj", Path: fresh}); err != nil || res.Refused {
		t.Fatalf("set-path: %+v, %v", res, err)
	}
	ex, err := h.Client.Explain(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Verdict == server.VerdictProjectUnavailable {
		t.Fatalf("explain after set-path = %+v, want available again", ex)
	}
}

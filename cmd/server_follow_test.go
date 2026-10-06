package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

type lockedBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

func TestServerTicketsFollow_PrintsOnlyTheNamedTicketsEvents(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic", "02", "second", "")
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- runServerTicketsFollow(ctx, h.Client, "proj:epic/02", false, &out) }()

	// The initial snapshot line proves the follower is subscribed from a known seq.
	waitFor(t, ctx, func() bool { return strings.Contains(out.String(), "proj:epic/02\topen") })
	for _, name := range []string{"01-first.md", "02-second.md"} {
		path := filepath.Join(store, "proj", "epic", "issues", name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(body), "status: open", "status: done", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, ctx, func() bool { return strings.Contains(out.String(), "ticket-changed\tproj:epic/02") })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "proj:epic/01") {
		t.Errorf("output mentions another ticket:\n%s", out.String())
	}
}

func waitFor(t *testing.T, ctx context.Context, cond func() bool) {
	t.Helper()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for output")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

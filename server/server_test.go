package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
)

// testFlushInterval stands in for the chat batcher's 6s tick in tests that
// count sends but never assert two notices share one batch.
const testFlushInterval = servertest.FlushInterval

func TestMain(m *testing.M) {
	servertest.RunSessionProcesses()
	herdrfake.RunHelperProcess()
	// One zone away from UTC for every test: a test that swapped it mid-run
	// moved the budget day under its parallel neighbours.
	time.Local = time.FixedZone("test", 5*3600)
	code := m.Run()
	servertest.RemoveFakeCodex()
	os.Exit(code)
}

func TestHandshake_ReturnsAPIVersionAndBuild(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)

	got, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIVersion != wire.APIVersion || got.Build != "test-build" || got.Pid != os.Getpid() {
		t.Errorf("handshake = %+v", got)
	}
}

func TestSnapshot_ListsStoreTicketsByAddressWithSequence(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic-a", "02", "second", "01")
	servertest.WriteTicket(t, store, "other", "epic-b", "01", "third", "")
	h := servertest.StartWithStore(t, store)

	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tk := range snap.Tickets {
		got = append(got, tk.Address)
	}
	want := []string{"other:epic-b/01", "proj:epic-a/01", "proj:epic-a/02"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("addresses = %v, want %v", got, want)
	}
	if snap.Tickets[2].Status != "open" || len(snap.Tickets[2].BlockedBy) != 1 {
		t.Errorf("ticket = %+v", snap.Tickets[2])
	}
	if f := snap.Tickets[2].File; !strings.HasSuffix(f, filepath.Join("proj", "epic-a", "issues", "02-second.md")) {
		t.Errorf("file = %q, want the ticket's markdown path", f)
	}
	again, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again.Seq != snap.Seq {
		t.Errorf("seq changed without a store change: %d -> %d", snap.Seq, again.Seq)
	}
}

func TestEvents_SnapshotThenSubscribeSeesTicketEditAsOneEventWithNextSeq(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	servertest.WriteTicket(t, store, "proj", "epic", "02", "second", "")
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.PollInterval = 50 * time.Millisecond })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(store, "proj", "epic", "issues", "02-second.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(body), "status: open", "status: done", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		want := wire.Event{Seq: snap.Seq + 1, Type: wire.EventTicketChanged, Address: "proj:epic/02"}
		if ev != want {
			t.Errorf("event = %+v, want %+v", ev, want)
		}
	case <-ctx.Done():
		t.Fatal("no event for the edit")
	}
	if _, err := h.Client.Events(ctx, snap.Seq+100); err == nil {
		t.Error("subscribing from an unknown seq succeeded")
	}
}

// Seam A: with the watch off and a slow poll, only the ping can make a direct
// write's event arrive in time.
func TestEvents_TicketChangedPingPublishesAtOnce(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.PollInterval = time.Hour
		c.DisableWatch = true
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(store, "proj", "epic", "issues", "01-first.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), "status: open", "status: draft", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.Client.Tickets.Changed(ctx, "proj:epic/01"); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		want := wire.Event{Seq: snap.Seq + 1, Type: wire.EventTicketChanged, Address: "proj:epic/01"}
		if ev != want {
			t.Errorf("event = %+v, want %+v", ev, want)
		}
	case <-ctx.Done():
		t.Fatal("no event after the ping")
	}
}

func TestServer_StopReleasesLockSoNewServerCanStart(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)

	if err := h.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := h.Client.Handshake(context.Background()); err == nil {
		t.Error("stopped server still answers")
	}
	next, err := server.New(server.Config{StateDir: h.StateDir, Runner: runnerfake.NewRunner()})
	if err != nil {
		t.Fatalf("new server on same state dir: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := next.Serve(ctx); err != nil {
		t.Errorf("second server: %v", err)
	}
}

func TestServer_StateDirAndSocketArePrivate(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)

	dir, err := os.Stat(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Errorf("state dir mode = %o, want 700", dir.Mode().Perm())
	}
	sock, err := os.Stat(server.SocketPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if sock.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %o, want 600", sock.Mode().Perm())
	}
}

func TestServer_SecondServerOnSameStateDirIsRefused(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)

	_, err := server.New(server.Config{StateDir: h.StateDir, Runner: runnerfake.NewRunner()})
	// The first server lives in this process, so the lock error names the
	// same-process case after the retry window.
	want := "lock still held by this process (pid " + strconv.Itoa(os.Getpid()) + ")"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if _, err := h.Client.Handshake(context.Background()); err != nil {
		t.Errorf("first server stopped answering: %v", err)
	}
}

func TestSnapshot_HandEditShowsUpWithWatchAndWithPollOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		disableWatch bool
	}{{"watch", false}, {"poll only", true}} {
		t.Run(tc.name, func(t *testing.T) {
			store := t.TempDir()
			servertest.WriteTicket(t, store, "proj", "epic", "01", "first", "")
			h := servertest.StartWithStore(t, store, func(c *server.Config) {
				c.DisableWatch = tc.disableWatch
				// With the watch on, a long poll proves the watch did the work.
				c.PollInterval = 100 * time.Millisecond
				if !tc.disableWatch {
					c.PollInterval = time.Hour
				}
			})

			path := filepath.Join(store, "proj", "epic", "issues", "01-first.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			edited := strings.Replace(string(body), "status: open", "status: done", 1)
			if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
				t.Fatal(err)
			}

			var snap wire.Snapshot
			servertest.WaitFor(t, 5*time.Second, "the edit to reach the snapshot", func() bool {
				var err error
				if snap, err = h.Client.Snapshot(context.Background()); err != nil {
					t.Fatal(err)
				}
				return snap.Tickets[0].Status == "done"
			})
			if snap.Seq == 0 {
				t.Error("seq did not advance on change")
			}
		})
	}
}

func TestNew_RequiresARunner(t *testing.T) {
	t.Parallel()
	_, err := server.New(server.Config{StateDir: t.TempDir(), TicketStore: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "Runner is required") {
		t.Fatalf("err = %v, want it to name the missing runner", err)
	}
}

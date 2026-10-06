package server_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/herdrfake"
)

func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}

func TestHandshake_ReturnsAPIVersionAndBuild(t *testing.T) {
	h := servertest.Start(t)

	got, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.APIVersion != server.APIVersion || got.Build != "test-build" || got.Pid != os.Getpid() {
		t.Errorf("handshake = %+v", got)
	}
}

func TestSnapshot_ListsStoreTicketsByAddressWithSequence(t *testing.T) {
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
	again, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again.Seq != snap.Seq {
		t.Errorf("seq changed without a store change: %d -> %d", snap.Seq, again.Seq)
	}
}

func TestEvents_SnapshotThenSubscribeSeesTicketEditAsOneEventWithNextSeq(t *testing.T) {
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
		want := server.Event{Seq: snap.Seq + 1, Type: server.EventTicketChanged, Address: "proj:epic/02"}
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

func TestServer_StopReleasesLockSoNewServerCanStart(t *testing.T) {
	h := servertest.Start(t)

	if err := h.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := h.Client.Handshake(context.Background()); err == nil {
		t.Error("stopped server still answers")
	}
	next, err := server.New(server.Config{StateDir: h.StateDir})
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
	h := servertest.Start(t)

	_, err := server.New(server.Config{StateDir: h.StateDir})
	want := "already running (pid " + strconv.Itoa(os.Getpid()) + ")"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if _, err := h.Client.Handshake(context.Background()); err != nil {
		t.Errorf("first server stopped answering: %v", err)
	}
}

func TestSnapshot_HandEditShowsUpWithWatchAndWithPollOnly(t *testing.T) {
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

			deadline := time.Now().Add(5 * time.Second)
			for {
				snap, err := h.Client.Snapshot(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if snap.Tickets[0].Status == "done" {
					if snap.Seq == 0 {
						t.Error("seq did not advance on change")
					}
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("edit never reached the snapshot: %+v", snap.Tickets[0])
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

func TestTCP_SameRoutesAnswerOverLoopback(t *testing.T) {
	h := servertest.StartWithTCP(t)

	resp, err := http.Get("http://" + h.TCPAddr + "/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot over TCP: %s", resp.Status)
	}
	got, err := h.Client.Handshake(context.Background())
	if err != nil || got.TCPAddr != h.TCPAddr {
		t.Errorf("handshake TCPAddr = %q (err %v), want %q", got.TCPAddr, err, h.TCPAddr)
	}
}

func TestTCP_OffByDefault(t *testing.T) {
	got, err := servertest.Start(t).Client.Handshake(context.Background())
	if err != nil || got.TCPAddr != "" {
		t.Errorf("handshake TCPAddr = %q (err %v), want empty", got.TCPAddr, err)
	}
}

func TestTCP_RefusesNonLoopbackAddr(t *testing.T) {
	_, err := server.New(server.Config{StateDir: t.TempDir(), TicketStore: t.TempDir(), TCPAddr: "0.0.0.0:0"})
	if err == nil {
		t.Fatal("expected error for non-loopback address")
	}
}

func TestHerdr_DownAtStartIsReportedAndRecoveryStreamsOneEventEach(t *testing.T) {
	h := servertest.StartHerdrDown(t, func(c *server.Config) { c.HerdrRetryInterval = 20 * time.Millisecond })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.HerdrUnavailable {
		t.Fatal("snapshot does not report herdr unavailable")
	}
	events, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := func() server.Event {
		select {
		case ev := <-events:
			return ev
		case <-ctx.Done():
			t.Fatal("event missing")
			return server.Event{}
		}
	}
	if ev := next(); ev.Type != server.EventHerdrUnavailable {
		t.Fatalf("first event = %+v", ev)
	}

	// Several failed retries must not repeat the event.
	time.Sleep(100 * time.Millisecond)
	h.SetHerdrDown(false)
	if ev := next(); ev.Type != server.EventHerdrAvailable {
		t.Fatalf("second event = %+v, want herdr-available", ev)
	}
	after, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.HerdrUnavailable || after.Seq != 2 {
		t.Errorf("after recovery: %+v", after)
	}
}

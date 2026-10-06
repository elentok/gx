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

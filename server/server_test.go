package server_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

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

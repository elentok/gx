// Package servertest is seam A: an in-process server on a temp state dir,
// temp socket and temp ticket store, with the fake herdr executable first in
// PATH, driven only through the API client. Like herdrfake.Start it calls
// t.Setenv, so tests using it must not be parallel, and the package's
// TestMain must call herdrfake.RunHelperProcess.
package servertest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/herdrfake"
)

// Harness is a running test server.
type Harness struct {
	Client      *apiclient.Client
	Herdr       *herdrfake.State
	StateDir    string
	TicketStore string
}

// Start runs a server until the test ends.
func Start(t *testing.T) *Harness {
	t.Helper()
	// Unix socket paths are capped near 100 bytes; t.TempDir() can exceed that.
	stateDir, err := os.MkdirTemp("", "gxs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })

	h := &Harness{
		Herdr:       herdrfake.NewState(t),
		StateDir:    filepath.Join(stateDir, "state"),
		TicketStore: t.TempDir(),
	}
	herdrfake.StartState(t, h.Herdr)

	srv, err := server.New(server.Config{StateDir: h.StateDir, Build: "test-build"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("server: %v", err)
		}
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
	return h
}

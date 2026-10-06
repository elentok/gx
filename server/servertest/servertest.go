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
	"sync"
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
	// Stop does what SIGTERM does to the real server: shut down, release the
	// lock, and return Serve's result. Safe to call more than once.
	Stop func() error
}

// WriteTicket creates <store>/<project>/<epic>/issues/<id>-<slug>.md (and the
// project's project.json) with minimal valid frontmatter. Call it before Start.
func WriteTicket(t *testing.T, store, project, epic, id, slug, blockedBy string) {
	t.Helper()
	projectDir := filepath.Join(store, project)
	issues := filepath.Join(projectDir, epic, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(`{"name":"`+project+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fm := "---\nid: \"" + id + "\"\nstatus: open\ntype: implement\n"
	if blockedBy != "" {
		fm += "blocked_by: [\"" + blockedBy + "\"]\n"
	}
	fm += "---\n\n# " + slug + "\n"
	if err := os.WriteFile(filepath.Join(issues, id+"-"+slug+".md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Start runs a server until the test ends. Tickets already written to
// TicketStore are indexed at startup; since the harness creates TicketStore
// itself, use StartWithStore to seed it first.
func Start(t *testing.T) *Harness {
	t.Helper()
	return StartWithStore(t, t.TempDir())
}

// StartWithStore is Start over a caller-seeded ticket store.
// Options tweak the server config before it starts.
func StartWithStore(t *testing.T, store string, opts ...func(*server.Config)) *Harness {
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
		TicketStore: store,
	}
	herdrfake.StartState(t, h.Herdr)

	cfg := server.Config{StateDir: h.StateDir, Build: "test-build", TicketStore: h.TicketStore}
	for _, o := range opts {
		o(&cfg)
	}
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	var once sync.Once
	h.Stop = func() error {
		var err error
		once.Do(func() { cancel(); err = <-done })
		return err
	}
	t.Cleanup(func() {
		if err := h.Stop(); err != nil {
			t.Errorf("server: %v", err)
		}
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
	return h
}

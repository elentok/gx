package serverctl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

// heldServer starts a server whose migration step blocks until release is called.
func heldServer(t *testing.T) (h *servertest.Harness, release func()) {
	t.Helper()
	store := t.TempDir()
	dir := filepath.Join(store, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	gate := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	reg := &migrations.Registry{}
	reg.Register(migrations.Step{Name: "hold", Run: func(migrations.Env) ([]tickets.MigrationChange, error) {
		enteredOnce.Do(func() { close(entered) })
		<-gate
		return nil, nil
	}})
	release = func() { releaseOnce.Do(func() { close(gate) }) }
	h = servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = reg })
	// Registered after the harness so it runs first: the held step must finish
	// before the server stops, or it writes into the store while TempDir is
	// being removed.
	t.Cleanup(release)
	<-entered
	return h, release
}

// Seam: `gx server status` says migrating while a migration is held, and not after.
func TestServerStatus_SaysMigratingWhileAMigrationIsHeld(t *testing.T) {
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	h, release := heldServer(t)
	ctx := context.Background()

	var out bytes.Buffer
	if err := runServerStatusWith(ctx, dirs, h.Client, true, &out); err != nil {
		t.Fatal(err)
	}
	var info StatusInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !info.Running || !info.Migrating {
		t.Errorf("status --json = %s, want running and migrating", out.String())
	}
	out.Reset()
	if err := runServerStatusWith(ctx, dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\nmigrating: ") {
		t.Errorf("status = %q, want a migrating line", out.String())
	}

	release()
	select {
	case <-h.Server.MigrationsDone():
	case <-time.After(10 * time.Second):
		t.Fatal("migrations did not end")
	}
	out.Reset()
	if err := runServerStatusWith(ctx, dirs, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "migrating") {
		t.Errorf("status after the run = %q, want no migrating line", out.String())
	}
}

// Seam: `gx server start` returns on the first handshake, however long the
// migration runs.
func TestServerStart_ReturnsWhileAMigrationIsHeld(t *testing.T) {
	h, _ := heldServer(t)
	spawn := func() error { t.Error("spawned a second server"); return nil }
	var out bytes.Buffer
	start := time.Now()
	if err := runServerStart(context.Background(), h.Client, &out, spawn, nil); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > stopTimeout/2 {
		t.Errorf("start took %s with a migration held", time.Since(start))
	}
	if !strings.HasPrefix(out.String(), "already running") {
		t.Errorf("output = %q", out.String())
	}
}

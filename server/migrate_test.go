package server_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/migratefail"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

func migrateStore(t *testing.T, projects ...string) string {
	t.Helper()
	store := t.TempDir()
	for _, p := range projects {
		dir := filepath.Join(store, p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func okStep(name string, ran *[]string) migrations.Step {
	return migrations.Step{Name: name, Run: func(env migrations.Env) ([]tickets.MigrationChange, error) {
		// The harness store also holds the scratch project; leave it out of the count.
		if filepath.Base(env.Root) != server.ScratchProject {
			*ran = append(*ran, name)
		}
		return nil, nil
	}}
}

func appliedOf(t *testing.T, store, project string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(store, project, migrations.AppliedFile))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func waitMigrations(t *testing.T, h *servertest.Harness) {
	t.Helper()
	select {
	case <-h.Server.MigrationsDone():
	case <-time.After(10 * time.Second):
		t.Fatal("migrations did not end")
	}
}

// Seam: a start migrates, records, keeps going past a failure, clears a stale
// record on a pass, and a second start runs nothing.
func TestMigrate_StartRunsPendingStepsAndRecords(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "bad", "good")
	var ran []string
	reg := &migrations.Registry{}
	reg.Register(okStep("one", &ran))
	reg.Register(migrations.Step{Name: "boom", Run: func(env migrations.Env) ([]tickets.MigrationChange, error) {
		if filepath.Base(env.Root) == "bad" {
			return nil, errors.New("exploded")
		}
		return nil, nil
	}})
	reg.Register(okStep("three", &ran))
	// A stale record for the passing project is cleared.
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Migrations = reg
		if err := migratefail.Record(c.StateDir, "good", "old"); err != nil {
			t.Fatal(err)
		}
	})
	waitMigrations(t, h)

	if got := appliedOf(t, store, "good"); got != "one\nboom\nthree\n" {
		t.Errorf("good applied = %q", got)
	}
	if got := appliedOf(t, store, "bad"); got != "one\n" {
		t.Errorf("bad applied = %q", got)
	}
	fails, err := migratefail.Read(h.StateDir, []string{"bad", "good"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fails) != 1 || fails[0].Project != "bad" || !strings.Contains(fails[0].Error, "exploded") {
		t.Errorf("failures = %v", fails)
	}
	if len(ran) != 3 {
		t.Errorf("ran = %v, want one (bad), then one and three (good); nothing after the failure", ran)
	}

	// Second start over the same store: only the failed step is pending.
	ran = nil
	h2 := servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = reg })
	waitMigrations(t, h2)
	if len(ran) != 0 {
		t.Errorf("second start ran %v", ran)
	}
}

// Seam: the handshake answers while a step is held open.
func TestMigrate_HandshakeAnswersWhileStepHeld(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "proj")
	entered := make(chan struct{})
	release := make(chan struct{})
	reg := &migrations.Registry{}
	var enteredOnce sync.Once
	reg.Register(migrations.Step{Name: "hold", Run: func(migrations.Env) ([]tickets.MigrationChange, error) {
		// The harness store also holds the scratch project, so this runs per project.
		enteredOnce.Do(func() { close(entered) })
		<-release
		return nil, nil
	}})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = reg })
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.Client.Handshake(ctx); err != nil {
		t.Fatalf("handshake while held: %v", err)
	}
	select {
	case <-h.Server.MigrationsDone():
		t.Fatal("done before release")
	default:
	}
	close(release)
	waitMigrations(t, h)
	if got := appliedOf(t, store, "proj"); got != "hold\n" {
		t.Errorf("applied = %q", got)
	}
}

// Seam: unknown names in lists give one warning for the whole set.
func TestMigrate_UnknownAppliedNamesWarnOnce(t *testing.T) {
	t.Parallel()
	store := migrateStore(t, "a", "b")
	for _, p := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(store, p, migrations.AppliedFile), []byte("from-the-future\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, func(c *server.Config) { c.Migrations = &migrations.Registry{} })
	waitMigrations(t, h)
	data, err := os.ReadFile(server.LogPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "from-the-future"); n != 1 {
		t.Errorf("warning count = %d, want 1\n%s", n, data)
	}
}

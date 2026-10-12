package server_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

// noProcs runs no process, so every agent directory is dead.
type noProcs struct{}

func (noProcs) Lookup(int) (nativerunner.Process, bool, error) {
	return nativerunner.Process{}, false, nil
}

func agentDir(t *testing.T, root, label string, touched time.Time) string {
	t.Helper()
	dir := filepath.Join(root, label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := nativerunner.WriteMeta(dir, nativerunner.Meta{Runner: "headless", PID: 4242, Label: label}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nativerunner.OutFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{nativerunner.MetaFile, nativerunner.OutFile} {
		if err := os.Chtimes(filepath.Join(dir, f), touched, touched); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPrune_HeadlessDirsArePrunedUnderAHerdrConfig(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	ticket := filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")
	if err := ralphloop.Claim(ticket); err != nil {
		t.Fatal(err)
	}
	if err := ralphloop.Park(filepath.Join(store, "proj"), "epic-a", "01", ticket, events.IterationError, "stuck"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	retention := 36 * time.Hour
	expired := time.Now().Add(-retention - time.Hour)
	old := agentDir(t, root, "old", expired)
	parked := agentDir(t, root, "epic-a-iter-01", expired)
	fresh := agentDir(t, root, "fresh", time.Now())

	configured := newRenamedFake("herdr", nativerunner.VerdictLive)
	servertest.StartWithStore(t, store, func(c *server.Config) {
		c.Runner = configured
		c.RunnerFor = func(name, project string) agentrunner.Runner {
			if name == "headless" && project == "proj" {
				return &nativerunner.Headless{Root: root, Procs: noProcs{}}
			}
			if name == configured.Name() {
				return configured
			}
			return nil
		}
		c.LogRetention = retention
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(old, nativerunner.OutFile)); errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the expired headless dir was never pruned")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, kept := range []string{parked, fresh} {
		if _, err := os.Stat(filepath.Join(kept, nativerunner.OutFile)); err != nil {
			t.Errorf("%s was pruned: %v", filepath.Base(kept), err)
		}
	}
}

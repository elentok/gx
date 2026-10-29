package server_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// startWithPath starts a server whose PATH is only dir.
func startWithPath(t *testing.T, dir string) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.Env = append(c.Env, "PATH="+dir)
	})
}

func TestSnapshot_FlagsNodeMissingFromThePath(t *testing.T) {
	t.Parallel()
	h := startWithPath(t, t.TempDir())
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.NodeMissing {
		t.Error("node_missing = false with no node on PATH, want true")
	}
}

func TestSnapshot_NodeOnThePathIsNotFlagged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := startWithPath(t, dir)
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.NodeMissing {
		t.Error("node_missing = true with node on PATH, want false")
	}
}

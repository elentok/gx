package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// watchHarness starts a server watching dir/config.json with a short
// debounce, and streams its events from now on.
func watchHarness(t *testing.T, ctx context.Context) (h *servertest.Harness, path, store string, evs <-chan server.Event) {
	t.Helper()
	store = t.TempDir()
	path = filepath.Join(t.TempDir(), "config.json")
	writeWatched(t, path, store, 3)
	h = servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgents = 3
		c.WatchConfig = true
		c.ConfigWatchDebounce = 20 * time.Millisecond
	})
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if evs, err = h.Client.Events(ctx, snap.Seq); err != nil {
		t.Fatal(err)
	}
	return h, path, store, evs
}

func writeWatched(t *testing.T, path, store string, n int) {
	t.Helper()
	servertest.WriteConfig(t, path, store, map[string]any{
		"execution-queue": map[string]any{"max-agents": n},
		"server":          map[string]any{"watch-config": true},
	})
}

// saveByRename is how most editors save: a temp file renamed over the original.
func saveByRename(t *testing.T, path, store string, n int) {
	t.Helper()
	tmp := path + ".tmp"
	writeWatched(t, tmp, store, n)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// noReloadEvent fails if a config-reloaded event arrives within a few debounces.
func noReloadEvent(t *testing.T, evs <-chan server.Event, why string) {
	t.Helper()
	timeout := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-evs:
			if ev.Type == server.EventConfigReloaded {
				t.Fatalf("%s: got %+v, want no reload", why, ev)
			}
		case <-timeout:
			return
		}
	}
}

// Seam: rename-style and in-place saves each reload once; identical bytes
// and a temp file of another name do not.
func TestConfigWatch_SavesReloadOnceEach(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, path, store, evs := watchHarness(t, ctx)

	noReloadEvent(t, evs, "startup")
	saveByRename(t, path, store, 4)
	ev := servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
	if ev.Level != server.EventLevelSuccess || !strings.Contains(ev.Message, "execution-queue.max-agents") {
		t.Errorf("rename save = %+v, want max-agents changed", ev)
	}
	noReloadEvent(t, evs, "after the rename save")

	writeWatched(t, path, store, 5)
	servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
	noReloadEvent(t, evs, "after the in-place save")

	writeWatched(t, path, store, 5)
	noReloadEvent(t, evs, "identical bytes")
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "other.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	noReloadEvent(t, evs, "another file")
}

// Seam: a delete reports config-missing once, and a re-created file reloads.
func TestConfigWatch_MissingFileReportedOnce(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, path, store, evs := watchHarness(t, ctx)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	ev := servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
	if ev.Level != server.EventLevelError || !strings.Contains(ev.Message, "does not exist") {
		t.Errorf("delete = %+v, want a config-missing failure", ev)
	}
	if err := os.WriteFile(path+".tmp", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	noReloadEvent(t, evs, "still missing")

	writeWatched(t, path, store, 3)
	if ev := servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, ""); ev.Level != server.EventLevelSuccess {
		t.Errorf("re-created = %+v, want success", ev)
	}
}

// Seam: a watcher-triggered reload that turns the watcher off returns, and
// later saves no longer reload; a command reload turning it on re-arms it.
func TestConfigWatch_TurnsItselfOffWithoutDeadlock(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, path, store, evs := watchHarness(t, ctx)

	servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"watch-config": false}})
	ev := servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
	if !strings.Contains(ev.Message, "server.watch-config") {
		t.Errorf("turn off = %+v, want server.watch-config changed", ev)
	}
	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 7}, "server": map[string]any{"watch-config": false}})
	noReloadEvent(t, evs, "watcher off")

	// The command still works, so the watcher's reload released the lock.
	writeWatched(t, path, store, 4)
	if res := reload(t, h); res.Refused || !strings.Contains(strings.Join(res.Changed, ","), "server.watch-config") {
		t.Fatalf("turn on = %+v", res)
	}
	servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
	writeWatched(t, path, store, 5)
	servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
}

// Seam: a config.json symlink reloads when its target is saved.
func TestConfigWatch_FollowsSymlinkTarget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.json")
	writeWatched(t, target, store, 3)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.MaxAgents = 3
		c.WatchConfig = true
		c.ConfigWatchDebounce = 20 * time.Millisecond
	})
	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := h.Client.Events(ctx, snap.Seq)
	if err != nil {
		t.Fatal(err)
	}
	saveByRename(t, target, store, 4)
	servertest.WaitForEvent(ctx, t, evs, server.EventConfigReloaded, "")
}

package server_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// A changed commit-debounce is reported and applied to the running store
// commit loop: an edit is committed after the new, short debounce.
func TestConfigReload_StoreCommitDebounceApplies(t *testing.T) {
	store := t.TempDir()
	if err := os.WriteFile(filepath.Join(store, "seed.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) {
		c.StoreCommitDebounce = time.Hour
	})
	// The loop starts after the server serves; its start-up flush commits the
	// seed, so the loop is running once that commit lands.
	deadline := time.Now().Add(5 * time.Second)
	for storeLog(t, store) == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	before := storeLog(t, store)
	if before == "" {
		t.Fatal("store commit loop never flushed the seed")
	}

	servertest.WriteConfig(t, path, store, map[string]any{
		"ticket-store": map[string]any{"path": store, "commit-debounce": 1},
	})
	if res := reload(t, h); res.Refused || !slices.Contains(res.Changed, "ticket-store.commit-debounce") {
		t.Fatalf("reload = %+v, want commit-debounce changed", res)
	}
	if err := os.WriteFile(filepath.Join(store, "edit.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for storeLog(t, store) == before && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := storeLog(t, store); got == before {
		t.Fatalf("no commit after the reloaded debounce; log = %q", got)
	}
}

package settings_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// Seam: both Leader keys reach the snapshot, and a reload shows a change in the next one.
func TestSnapshot_LeaderKeysFollowConfig(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	write := func(mac, other string) {
		servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"web": map[string]any{
			"leader-key-mac": mac, "leader-key": other,
		}}})
	}
	write("cmd+k", "ctrl+k")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))

	check := func(mac, other string) {
		t.Helper()
		snap, err := h.Client.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if snap.LeaderKeyMac != mac || snap.LeaderKey != other {
			t.Fatalf("snapshot leader keys = %q, %q; want %q, %q", snap.LeaderKeyMac, snap.LeaderKey, mac, other)
		}
	}
	reload(t, h)
	check("cmd+k", "ctrl+k")

	write("alt+x", "")
	reload(t, h)
	check("alt+x", "")
}

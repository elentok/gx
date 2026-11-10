package settings_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestStartup_WarnsOnceAboutDevURLAndLeavesTheFileAlone(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, path, store, map[string]any{
		"server": map[string]any{"web": map[string]any{"dev-url": "http://127.0.0.1:5173"}},
	})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))

	log, err := os.ReadFile(server.LogPath(h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	var warnings int
	for _, line := range bytes.Split(log, []byte("\n")) {
		if strings.Contains(string(line), `"level":"WARN"`) && strings.Contains(string(line), "server.web.dev-url") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("dev-url warnings = %d, want 1\n%s", warnings, log)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("config file changed by startup:\n%s\nwas:\n%s", after, before)
	}
}

package settings_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/servertest"
)

// startupOnlyCases changes each Restart-flagged registry key in a config file.
// It is the one place a new flagged key needs a line.
func startupOnlyCases(t *testing.T) map[string]map[string]any {
	other := t.TempDir()
	return map[string]map[string]any{
		"ticket-store.path": {"ticket-store": map[string]any{"path": other}},
		"server.tcp-listen": {"server": map[string]any{"tcp-listen": true}},
		"server.web.listen": {"server": map[string]any{"web": map[string]any{"listen": "127.0.0.1:0"}}},
		"agent-runner":      {"agent-runner": "headless"},
		"runner-wait":       {"runner-wait": "7s"},
	}
}

// The registry's Restart flags are the only startup-only list: every flagged
// key is reported when it changes, and nothing else is.
func TestStartupOnly_ReportsExactlyTheRegistryFlags(t *testing.T) {
	t.Parallel()
	cases := startupOnlyCases(t)

	var flagged []string
	for _, e := range registry.Default().All() {
		if e.Restart {
			flagged = append(flagged, e.Key.String())
		}
	}
	got := slices.Sorted(maps.Keys(cases))
	if slices.Sort(flagged); !slices.Equal(got, flagged) {
		t.Fatalf("cases cover %v, registry flags %v: want the same keys", got, flagged)
	}

	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	for key, fields := range cases {
		servertest.WriteConfig(t, path, store, fields)
		res := reload(t, h)
		want := "restart needed for " + key
		if res.Refused || !slices.Contains(res.Warnings, want) {
			t.Errorf("changing %s: %+v, want warning %q", key, res, want)
		}
		if len(res.Warnings) != 1 {
			t.Errorf("changing %s: warnings %v, want only %q", key, res.Warnings, want)
		}
	}

	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); len(res.Warnings) != 0 {
		t.Errorf("back at the startup values: warnings %v, want none", res.Warnings)
	}
}

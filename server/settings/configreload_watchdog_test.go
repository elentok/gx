package settings_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/settings"
)

func TestSettingsFromConfig_CarriesWatchdog(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Watchdog = config.WatchdogConfig{MaxTreeProcesses: 7, Interval: 3 * time.Second, UlimitHeadroom: 9}
	if got := settings.FromConfig(cfg).Watchdog; got != cfg.Watchdog {
		t.Errorf("Watchdog = %+v, want %+v", got, cfg.Watchdog)
	}
}

func TestConfigReload_WatchdogKeysApply(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	servertest.WriteConfig(t, path, store, map[string]any{"watchdog": map[string]any{
		"max-tree-processes": 50, "interval": "2s", "ulimit-headroom": 10,
	}})
	res := reload(t, h)
	for _, key := range []string{"watchdog.max-tree-processes", "watchdog.interval", "watchdog.ulimit-headroom"} {
		if !slices.Contains(res.Changed, key) {
			t.Errorf("reload = %+v, want %s changed", res, key)
		}
	}
}

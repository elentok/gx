package server_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestSettingsFromConfig_CarriesDiskSpace(t *testing.T) {
	cfg := config.Default()
	cfg.DiskSpace = config.DiskSpaceConfig{WarnBelow: 30e9, CleanBelow: 5e9, CleanupCommand: []string{"make", "clean"}}

	s := server.SettingsFromConfig(cfg)
	if s.DiskSpace.WarnBelow != 30e9 || s.DiskSpace.CleanBelow != 5e9 || !slices.Equal(s.DiskSpace.CleanupCommand, []string{"make", "clean"}) {
		t.Errorf("settings = %+v, want the config's disk-space section", s.DiskSpace)
	}
}

// A reload reports each changed disk-space key, and the live settings carry it.
func TestConfigReload_DiskSpaceKeysApply(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, nil)
	if res := reload(t, h); res.Refused {
		t.Fatalf("baseline = %+v", res)
	}

	servertest.WriteConfig(t, path, store, map[string]any{"disk-space": map[string]any{
		"warn-below":      "50G",
		"clean-below":     "5G",
		"cleanup-command": []string{"make", "clean"},
	}})
	res := reload(t, h)
	for _, key := range []string{"disk-space.warn-below", "disk-space.clean-below", "disk-space.cleanup-command"} {
		if !slices.Contains(res.Changed, key) {
			t.Errorf("reload = %+v, want %s changed", res, key)
		}
	}
	got := h.Server.LiveDiskSpace()
	if got.WarnBelow != 50e9 || got.CleanBelow != 5e9 || !slices.Equal(got.CleanupCommand, []string{"make", "clean"}) {
		t.Errorf("live = %+v, want 50G, 5G, make clean", got)
	}
}

func TestConfigReload_BadDiskSizeRefuses(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	servertest.WriteConfig(t, path, store, map[string]any{"disk-space": map[string]any{"warn-below": "lots"}})
	if res := reload(t, h); !res.Refused {
		t.Errorf("reload = %+v, want refused", res)
	}
}

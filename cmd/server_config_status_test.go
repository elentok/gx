package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestConfigStatus_ShowsLastReloadAndPendingKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()

	var out bytes.Buffer
	if err := runServerStatusWith(ctx, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "config:") {
		t.Errorf("fresh status = %q, want no config block", out.String())
	}

	servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"tcp-listen": true}})
	if _, err := h.Client.ReloadConfig(ctx); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runConfigStatus(ctx, h.Client, &out, true); err != nil {
		t.Fatal(err)
	}
	var st server.ConfigStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.LastReload == nil || st.LastReload.Outcome != server.EventLevelWarning || !slices.Equal(st.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("config-status --json = %s", out.String())
	}

	out.Reset()
	if err := runServerStatusWith(ctx, h.Client, true, &out); err != nil {
		t.Fatal(err)
	}
	var info StatusInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Config == nil || info.Config.LastReload == nil || !slices.Equal(info.Config.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("status --json = %s, want the config block", out.String())
	}

	out.Reset()
	if err := runServerStatusWith(ctx, h.Client, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"config:\n  last reload: warning (command, ", "\n  restart needed for: server.tcp-listen\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status = %q, want %q", out.String(), want)
		}
	}
}

func TestServerStatus_BrokenConfigStillReportsTheError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, ".config", "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "gx", "config.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := servertest.Start(t)

	var out bytes.Buffer
	if err := runServerStatusWith(context.Background(), h.Client, false, &out); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.HasPrefix(out.String(), "config error: config ") || !strings.Contains(out.String(), "\nrunning\n") {
		t.Errorf("status = %q, want the config error on top", out.String())
	}
}

package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestReloadConfig_PrintsChangesWarningsAndRefusals(t *testing.T) {
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path), func(c *server.Config) { c.MaxAgents = 3 })
	ctx := context.Background()

	servertest.WriteConfig(t, path, store, map[string]any{
		"execution-queue": map[string]any{"max-agents": 4},
		"server":          map[string]any{"tcp-listen": true},
	})
	var out bytes.Buffer
	if err := runReloadConfig(ctx, h.Client, &out, false); err != nil {
		t.Fatalf("success: %v", err)
	}
	for _, want := range []string{"gx config reloaded successfully\n", "changed: ", "execution-queue.max-agents", "warning: restart needed for server.tcp-listen\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want %q", out.String(), want)
		}
	}

	servertest.WriteConfig(t, path, store, map[string]any{"execution-queue": map[string]any{"max-agents": 4}})
	out.Reset()
	if err := runReloadConfig(ctx, h.Client, &out, false); err != nil || !strings.Contains(out.String(), "\nno changes\n") {
		t.Errorf("unchanged = %q, %v; want no changes", out.String(), err)
	}

	servertest.WriteConfig(t, path, store, map[string]any{"evolve": "sometimes"})
	err := runReloadConfig(ctx, h.Client, &out, false)
	if err == nil || !strings.Contains(err.Error(), server.ReasonInvalidConfig) {
		t.Errorf("invalid = %v, want a %s error", err, server.ReasonInvalidConfig)
	}
}

func TestReloadConfig_FailsWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out bytes.Buffer
	err := runReloadConfig(context.Background(), cl, &out, false)
	if err == nil || !strings.Contains(err.Error(), "server not running") {
		t.Errorf("err = %v, want server not running", err)
	}
}

package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func devSnapshot(t *testing.T, h *servertest.Harness) wire.DevStatus {
	t.Helper()
	snap, err := h.Client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap.Dev
}

func devConfig(t *testing.T, path, store string, web map[string]any) {
	t.Helper()
	servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"web": web}})
}

func TestSnapshotDev_NoCheckout(t *testing.T) {
	store := t.TempDir()
	h := servertest.StartWithStore(t, store)
	dev := devSnapshot(t, h)
	if dev.State != wire.DevStateOff || dev.HasCheckout || dev.Checkout != "" {
		t.Fatalf("dev = %+v, want off without a checkout", dev)
	}
	if !strings.Contains(dev.Reason, "gx has no Dev checkout") {
		t.Fatalf("reason = %q", dev.Reason)
	}
}

func TestSnapshotDev_DevDirAndListenerReason(t *testing.T) {
	store := t.TempDir()
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "webui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "webui", "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "config.json")
	devConfig(t, cfg, store, map[string]any{"listen": ""})
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(cfg))

	if dev := devSnapshot(t, h); dev.HasCheckout || !strings.Contains(dev.Reason, "the web listener is off") {
		t.Fatalf("before reload dev = %+v", dev)
	}

	devConfig(t, cfg, store, map[string]any{"listen": "", "dev-dir": checkout})
	servertest.Reload(t, h)
	dev := devSnapshot(t, h)
	if !dev.HasCheckout || dev.Checkout != checkout || dev.State != wire.DevStateOff {
		t.Fatalf("after reload dev = %+v, want checkout %s", dev, checkout)
	}
	if dev.Reason != "the web listener is off" {
		t.Fatalf("reason = %q", dev.Reason)
	}
}

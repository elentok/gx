package servertest

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/runnerfake"
)

// SetHarness sets the default harness through the client.
func SetHarness(t *testing.T, h *Harness, req wire.SetDefaultHarnessRequest) wire.SetDefaultHarnessResult {
	t.Helper()
	res, err := h.Client.SetDefaultHarness(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// SwitchDefaultHarness rewrites the config file's default harness and reloads it.
func SwitchDefaultHarness(t *testing.T, h *Harness, path, store, harness string) {
	t.Helper()
	WriteConfig(t, path, store, map[string]any{"agents": map[string]any{"default-harness": harness}})
	if res, err := h.Client.ReloadConfig(context.Background()); err != nil || res.Refused {
		t.Fatalf("reload: %+v, %v", res, err)
	}
}

// WaitKind waits for the session labelled label to start and returns its agent kind.
func WaitKind(t *testing.T, fake *runnerfake.Runner, label string) agentrunner.Kind {
	t.Helper()
	var kind agentrunner.Kind
	WaitFor(t, 10*time.Second, label+" starts", func() bool {
		kind = fake.Kind(label)
		return kind != ""
	})
	return kind
}

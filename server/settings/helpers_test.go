package settings_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// reload asks the running server to reload its config and returns the result.
func reload(t *testing.T, h *servertest.Harness) wire.ReloadResult {
	t.Helper()
	res, err := h.Client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

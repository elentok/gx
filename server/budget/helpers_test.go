package budget_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

// assertPaused checks a parsed ticket is paused with the given class and, when
// reason is non-empty, the given reason.
func assertPaused(t *testing.T, tk schema.Ticket, class schema.PauseClass, reason schema.PauseReason) {
	t.Helper()
	if tk.Status != schema.StatusPaused {
		t.Errorf("Status = %q, want %q", tk.Status, schema.StatusPaused)
		return
	}
	if class != "" && tk.PausedClass() != class {
		t.Errorf("PausedClass = %q, want %q", tk.PausedClass(), class)
	}
	if reason != "" && tk.PauseReason != reason {
		t.Errorf("PauseReason = %q, want %q", tk.PauseReason, reason)
	}
}

// reload asks the running server to reload its config and returns the result.
func reload(t *testing.T, h *servertest.Harness) wire.ReloadResult {
	t.Helper()
	res, err := h.Client.ReloadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func waitRuns(ctx context.Context, t *testing.T, h *servertest.Harness, n int) {
	t.Helper()
	deadline, _ := ctx.Deadline()
	servertest.WaitFor(t, time.Until(deadline), fmt.Sprintf("%d runs", n), func() bool { return len(h.Server.Runs()) == n })
}

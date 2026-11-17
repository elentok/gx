package askflows_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"

	"github.com/elentok/gx/nativerunner/holder"
)

func TestSessionScreen_LiveReturnsHolderRows(t *testing.T) {
	t.Parallel()
	h, ctx, _, id, conv := startFakeSession(t, servertest.WithEnv(agentfake.ProgramStartDelayEnv+"=1s"))
	holder := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	// The ack returns once the fake reads input; the tty echo of keys typed
	// earlier proves nothing.
	servertest.Report(t, conv, agentfake.Control{State: "idle"})
	if err := holder.SendKeys("typed-on-screen"); err != nil {
		t.Fatal(err)
	}

	var got string
	if !servertest.Poll(10*time.Second, func() bool {
		res, err := h.Client.Sessions.Screen(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got = strings.Join(res.Rows, "\n")
		return strings.Contains(got, "typed-on-screen")
	}) {
		t.Fatalf("rows = %q, want the typed text", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("rows = %q, want the plain holder screen", got)
	}
}

func TestSessionScreen_ExitedReturnsSavedScreen(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, _ := startFakeSession(t)
	dir := filepath.Join(h.SessionsDir, id)
	waitForHolder(t, dir)
	if _, err := h.Client.Sessions.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if err := os.WriteFile(filepath.Join(dir, holder.ScreenFile), []byte("saved one\nsaved two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.Sessions.Screen(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "|"); got != "saved one|saved two" {
		t.Errorf("rows = %q, want the saved screen", got)
	}
}

func TestSessionScreen_UnknownSessionIsRefused(t *testing.T) {
	t.Parallel()
	h, ctx, _, _, _ := startFakeSession(t)
	if _, err := h.Client.Sessions.Screen(ctx, "no-such-session"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("unknown session err = %v, want a 404", err)
	}
}

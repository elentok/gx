package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
)

func TestSessionScreen_LiveReturnsHolderRows(t *testing.T) {
	h, ctx, _, id, _ := startFakeSession(t)
	holder := waitForHolder(t, filepath.Join(h.SessionsDir, id))
	if err := holder.SendKeys("typed-on-screen"); err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, holder, "typed-on-screen")

	res, err := h.Client.SessionScreen(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "\n"); !strings.Contains(got, "typed-on-screen") || strings.Contains(got, "\x1b") {
		t.Errorf("rows = %q, want the plain holder screen", got)
	}
}

func TestSessionScreen_ExitedReturnsSavedScreen(t *testing.T) {
	h, ctx, evs, id, _ := startFakeSession(t)
	dir := filepath.Join(h.SessionsDir, id)
	waitForHolder(t, dir)
	if _, err := h.Client.StopSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionExited })
	if err := os.WriteFile(filepath.Join(dir, nativerunner.ScreenFile), []byte("saved one\nsaved two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := h.Client.SessionScreen(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "|"); got != "saved one|saved two" {
		t.Errorf("rows = %q, want the saved screen", got)
	}
}

func TestSessionScreen_UnknownSessionIsRefused(t *testing.T) {
	h, ctx, _, _, _ := startFakeSession(t)
	if _, err := h.Client.SessionScreen(ctx, "no-such-session"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("unknown session err = %v, want a 404", err)
	}
}

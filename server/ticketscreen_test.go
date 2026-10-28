package server_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func startTicketScreen(t *testing.T) *servertest.Harness {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", testutil.TempRepo(t))
	return servertest.StartWithStore(t, store)
}

// ticketAgentDir writes a ticket agent directory of the given runner, with
// the screen its holder saved when it ended.
func ticketAgentDir(t *testing.T, stateDir, runner, screen string) {
	t.Helper()
	label, _, _ := ralphloop.IterationIdentity("epic-a", "01", "")
	dir := filepath.Join(nativerunner.AgentsRoot(stateDir, "proj"), label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := nativerunner.WriteMeta(dir, nativerunner.Meta{Runner: runner, Label: label}); err != nil {
		t.Fatal(err)
	}
	if screen != "" {
		if err := os.WriteFile(filepath.Join(dir, nativerunner.ScreenFile), []byte(screen), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTicketScreen_PtyAgentReturnsItsSavedScreen(t *testing.T) {
	t.Parallel()
	h := startTicketScreen(t)
	ticketAgentDir(t, h.StateDir, "pty", "saved one\nsaved two\n")
	ctx := context.Background()

	res, err := h.Client.TicketScreen(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "|"); got != "saved one|saved two" {
		t.Errorf("rows = %q, want the saved screen", got)
	}
	d, err := h.Client.Detail(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasScreen {
		t.Error("detail.HasScreen = false for a pty agent, want true")
	}
}

// startTicketScreenShortPath starts the harness under /tmp: the holder socket
// path must fit a unix socket's length cap, which the default temp dir exceeds.
func startTicketScreenShortPath(t *testing.T) *servertest.Harness {
	t.Helper()
	return startTicketScreen(t)
}

// fakeHolder serves the agent directory's holder.sock: ping always answers,
// and a screen request answers with live, or drops the connection when
// live is nil, as a holder does that exits after the ping.
func fakeHolder(t *testing.T, dir string, live []string) {
	t.Helper()
	ln, err := net.Listen("unix", filepath.Join(dir, nativerunner.HolderSocketFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			var req struct {
				Op string `json:"op"`
			}
			if json.NewDecoder(conn).Decode(&req) == nil {
				switch {
				case req.Op == "ping":
					_ = json.NewEncoder(conn).Encode(map[string]any{"version": 1, "pid": 1})
				case live != nil:
					_ = json.NewEncoder(conn).Encode(map[string]any{"screen": live})
				}
			}
			conn.Close()
		}
	}()
}

func ticketAgentPath(stateDir string) string {
	label, _, _ := ralphloop.IterationIdentity("epic-a", "01", "")
	return filepath.Join(nativerunner.AgentsRoot(stateDir, "proj"), label)
}

func TestTicketScreen_LiveHolderReturnsItsRows(t *testing.T) {
	t.Parallel()
	h := startTicketScreenShortPath(t)
	ticketAgentDir(t, h.StateDir, "pty", "saved\n")
	fakeHolder(t, ticketAgentPath(h.StateDir), []string{"live one", "live two"})

	res, err := h.Client.TicketScreen(context.Background(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "|"); got != "live one|live two" {
		t.Errorf("rows = %q, want the live screen", got)
	}
}

func TestTicketScreen_HolderGoneAfterPingReturnsSavedRows(t *testing.T) {
	t.Parallel()
	h := startTicketScreenShortPath(t)
	ticketAgentDir(t, h.StateDir, "pty", "saved one\nsaved two\n")
	fakeHolder(t, ticketAgentPath(h.StateDir), nil)

	res, err := h.Client.TicketScreen(context.Background(), "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Rows, "|"); got != "saved one|saved two" {
		t.Errorf("rows = %q, want the saved screen", got)
	}
}

func TestTicketScreen_AgentWithoutPtyHasNoScreen(t *testing.T) {
	t.Parallel()
	h := startTicketScreen(t)
	ticketAgentDir(t, h.StateDir, "headless", "")
	ctx := context.Background()

	if _, err := h.Client.TicketScreen(ctx, "proj:epic-a/01"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("headless agent err = %v, want a 404", err)
	}
	d, err := h.Client.Detail(ctx, "proj:epic-a/01")
	if err != nil {
		t.Fatal(err)
	}
	if d.HasScreen {
		t.Error("detail.HasScreen = true for a headless agent, want false")
	}
}

func TestTicketScreen_TicketWithoutAgentIsRefused(t *testing.T) {
	t.Parallel()
	h := startTicketScreen(t)

	if _, err := h.Client.TicketScreen(context.Background(), "proj:epic-a/01"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("no-agent err = %v, want a 404", err)
	}
}

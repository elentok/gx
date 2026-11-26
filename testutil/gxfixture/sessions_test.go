package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/testutil/agentfake"
)

// mkControlFIFO makes the control FIFO of conv and holds its read end, as the
// fake does, returning a read of what was sent so far.
func mkControlFIFO(t *testing.T, home, conv string) func() string {
	t.Helper()
	path := agentfake.TUIControlPath(home, conv)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(fd) })
	return func() string {
		buf := make([]byte, 512)
		n, _ := syscall.Read(fd, buf)
		return string(buf[:max(n, 0)])
	}
}

func TestControlSender_TargetsTheSessionsOwnControlFile(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	dir := filepath.Join(root, "data", "gx", "sessions", "sess-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sessions.WriteMeta(dir, sessions.Meta{ID: "sess-1", ConversationID: "conv-own"}); err != nil {
		t.Fatal(err)
	}
	send := sessionSeeder{&fixture{root: root}}.controlSender("sess-1", "")

	// A late fake of another session appears first, the session's own file later.
	stray := mkControlFIFO(t, home, "conv-late")
	done := make(chan error, 1)
	go func() { done <- send(agentfake.Control{Action: agentfake.ActionExit, Code: 1}) }()
	select {
	case err := <-done:
		t.Fatalf("sent before the session's own control file existed: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	own := mkControlFIFO(t, home, "conv-own")
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if got := own(); !strings.Contains(got, agentfake.ActionExit) {
		t.Fatalf("own control file: want the exit control, got %q", got)
	}
	if got := stray(); got != "" {
		t.Fatalf("stray control file: want nothing, got %q", got)
	}
}

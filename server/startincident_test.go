package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
)

func TestStartIncident_MaintainerStartFilesATicketAndRemovesTheNote(t *testing.T) {
	t.Parallel()
	state := shortStateDir(t)
	note := server.StartIncidentPath(state)
	if err := os.WriteFile(note, []byte("job state = spawn failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, store := cpuSpikeServer(t, withEvolve(config.EvolveMaintainer), func(c *server.Config) { c.StateDir = state })

	// The server files it while starting, so the ticket may land after the
	// harness returns.
	var issues []string
	deadline := time.Now().Add(5 * time.Second)
	for len(issues) == 0 && time.Now().Before(deadline) {
		issues, _ = filepath.Glob(filepath.Join(store, projects.OutboxProject, queue.OneOffsEpic, "issues", "*-server-start-*.md"))
		time.Sleep(20 * time.Millisecond)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want one", issues)
	}
	body, _ := os.ReadFile(issues[0])
	for _, want := range []string{"type: diagnose", "spawn failed"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("ticket lacks %q:\n%s", want, body)
		}
	}
	for !os.IsNotExist(statErr(note)) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if err := statErr(note); !os.IsNotExist(err) {
		t.Errorf("note still there after filing: %v", err)
	}
}

func TestStartIncident_OutsideMaintainerModeKeepsTheNote(t *testing.T) {
	t.Parallel()
	state := shortStateDir(t)
	note := server.StartIncidentPath(state)
	if err := os.WriteFile(note, []byte("job state = spawn failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, store := cpuSpikeServer(t, func(c *server.Config) { c.StateDir = state })

	if issues, _ := filepath.Glob(filepath.Join(store, projects.OutboxProject, queue.OneOffsEpic, "issues", "*-server-start-*.md")); len(issues) != 0 {
		t.Errorf("filed %v outside maintainer mode", issues)
	}
	if _, err := os.Stat(note); err != nil {
		t.Errorf("note was removed without a ticket: %v", err)
	}
}

// shortStateDir is a state dir short enough for the server's unix socket.
func shortStateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gxi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func statErr(path string) error { _, err := os.Stat(path); return err }

package promptflows_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

const missingMarkerLog = "without the composer marker"

// A marker never drawn raises the notice and logs the version once, after the
// prompt was typed.
func TestFirstPromptMarker_NeverSeenIsReportedAfterTheTyping(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h, ctx, evs, id, conv := servertest.StartFakeSessionWith(t, wire.CreateSessionRequest{Prompt: "fix the bug"}, func(c *server.Config) {
		c.PromptGrace = firstPromptGrace
	}, servertest.WithEnv(agentfake.ProgramInputEnv+"=1", "CLAUDE_CONFIG_DIR="+dir, agentfake.ProgramInsertNoMarkerEnv+"=1"))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSubmitted(t, conv, "\"fix the bug\"\n")
	servertest.WaitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool {
		return slices.Contains(s.Notices, wire.SessionNoticeNoComposerMarker)
	})
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	got := string(logs)
	if n := strings.Count(got, missingMarkerLog); n != 1 {
		t.Fatalf("missing-marker lines = %d, want 1:\n%s", n, got)
	}
	typed := strings.Index(got, "first prompt typed")
	if typed < 0 || typed > strings.Index(got, missingMarkerLog) {
		t.Errorf("the missing-marker line must come after the prompt was typed:\n%s", got)
	}
}

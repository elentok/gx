package launchtest_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
)

// A launch with no note keeps the shared prompt file, adds no sentence and
// removes a note file an earlier launch left behind.
func TestLaunch_NoNoteKeepsSharedPromptAndRemovesStaleFiles(t *testing.T) {
	t.Parallel()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicketWith(t, store, "proj", "epic-a", "01", "first", servertest.TicketOpts{})
	servertest.SetProjectRepo(t, store, "proj", repo)
	dir := filepath.Join(store, "proj", "epic-a", "launch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := []string{filepath.Join(dir, "01-note.md"), filepath.Join(dir, "01-prompt.md")}
	for _, p := range stale {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fake := runnerfake.NewRunner()
	h := servertest.StartWithStore(t, store, servertest.WithRunner(rigs.NewNamedFake(fake, "pty")))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if res, err := h.Client.Queue.Add(ctx, "proj:epic-a/01", "claude"); err != nil || res.Refused {
		t.Fatalf("add: %+v, %v", res, err)
	}
	t.Cleanup(func() { fake.SetState(rigs.ResumeLabel, agentrunner.StateDone, "") })
	servertest.WaitFor(t, 10*time.Second, "the ticket launches", func() bool { return len(fake.Prompts(rigs.ResumeLabel)) > 0 })

	for _, p := range stale {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone: %v", p, err)
		}
	}
	args := fake.Args(rigs.ResumeLabel)
	i := slices.Index(args, "--append-system-prompt-file")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("args lack a prompt file: %v", args)
	}
	if want := harness.GXSystemPromptFile(); args[i+1] != want {
		t.Errorf("prompt file = %s, want the shared %s", args[i+1], want)
	}
}

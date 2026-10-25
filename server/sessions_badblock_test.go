package server_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

const badRoundText = "❓ **Q1** Which store?\n\n```gx-question\noptions: [unclosed\n```\n"

// goIdle takes a working session idle and back to working, so the idle
// handling has run when it returns.
func goIdle(t *testing.T, h *servertest.Harness, ctx context.Context, evs <-chan server.Event, id, conv string) {
	t.Helper()
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	settle(t, h, ctx, evs, id, conv)
}

// settle moves the session to working; that change orders the caller after the
// previous idle handling.
func settle(t *testing.T, h *servertest.Harness, ctx context.Context, evs <-chan server.Event, id, conv string) {
	t.Helper()
	report(t, conv, agentfake.Control{State: "working"})
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
}

func badBlockTickets(t *testing.T, store string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(store, "gx", "follow-ups", "issues", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(raw))
	}
	return out
}

func seedGxProject(t *testing.T) string {
	t.Helper()
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "gx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "gx", "project.json"), []byte(`{"name":"gx"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestBadRoundBlock_FilesOneAggregatedFollowUp(t *testing.T) {
	store := seedGxProject(t)
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", store)
	settle(t, h, ctx, evs, id, conv)

	writeAssistantMessage(t, path, badRoundText)
	goIdle(t, h, ctx, evs, id, conv)
	got := badBlockTickets(t, store)
	if len(got) != 1 || !strings.Contains(got[0], "Design round has an unreadable gx-question block") || !strings.Contains(got[0], "options: [unclosed") {
		t.Fatalf("tickets after round one = %q, want one holding the raw block", got)
	}

	// The same round seen again adds nothing.
	goIdle(t, h, ctx, evs, id, conv)
	if again := badBlockTickets(t, store); len(again) != 1 || again[0] != got[0] {
		t.Fatalf("tickets after the same round = %q, want unchanged", again)
	}

	// A new round adds a line to the same follow-up.
	writeAssistantMessage(t, path, strings.Replace(badRoundText, "Which store?", "Which db?", 1))
	goIdle(t, h, ctx, evs, id, conv)
	next := badBlockTickets(t, store)
	if len(next) != 1 || strings.Count(next[0], "```gx-question") != 2 || !strings.Contains(next[0], "Seen again") {
		t.Fatalf("tickets after round two = %q, want one with a seen-again entry", next)
	}
}

func TestBadRoundBlock_BlockPastTranscriptCapIsSeen(t *testing.T) {
	store := seedGxProject(t)
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", store)
	settle(t, h, ctx, evs, id, conv)

	long := "❓ **Q1** Which store?\n\n" + strings.Repeat("Background on the choice. ", 150) + "\n\n" +
		strings.SplitN(badRoundText, "\n\n", 2)[1]
	writeAssistantMessage(t, path, long)
	goIdle(t, h, ctx, evs, id, conv)
	if got := badBlockTickets(t, store); len(got) != 1 || !strings.Contains(got[0], "options: [unclosed") {
		t.Fatalf("tickets = %q, want one for the bad block past the old 2048-byte cap", got)
	}
}

func TestBadRoundBlock_NoGxProjectWritesNothing(t *testing.T) {
	store := t.TempDir()
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", store)
	settle(t, h, ctx, evs, id, conv)
	writeAssistantMessage(t, path, badRoundText)
	goIdle(t, h, ctx, evs, id, conv)
	if _, err := os.Stat(filepath.Join(store, "gx")); !os.IsNotExist(err) {
		t.Fatalf("gx dir stat err = %v, want not exist", err)
	}
}

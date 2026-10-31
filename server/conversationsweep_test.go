package server_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/transcript"
)

func TestStartupSweep_CountsUncountedDesignRecordsOfBothHarnesses(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	at := time.Now()
	seed := []conversations.Record{
		{ID: "design", Dir: dir, Harness: "claude", Ticket: "proj:map", Design: true},
		{ID: "old-no-flag", Dir: dir, Harness: "claude", Ticket: "proj:map/01"},
		{ID: "codex", Dir: dir, Harness: "codex", Ticket: "proj:map", Design: true},
		{ID: "codex-no-rollout", Dir: dir, Harness: "codex", Ticket: "proj:map", Design: true},
		{ID: "plain", Dir: dir, Harness: "claude", Ticket: "proj:epic/01"},
	}
	for _, r := range seed {
		switch r.ID {
		case "codex":
			writeCodexRound(t, filepath.Join(home, ".codex"), r.ID)
		case "codex-no-rollout":
		default:
			writeGrillingRoundsAt(t, transcript.PathIn(home, dir, r.ID), at)
		}
	}
	h := servertest.StartWithStore(t, designMapStore(t), servertest.WithTestHolder(), servertest.WithEnv("HOME="+home), func(c *server.Config) {
		// Sweep order follows RecordedAt, so the skipped records are visited last.
		for i, r := range seed {
			r.RecordedAt = at.Add(time.Duration(i) * time.Second)
			if err := conversations.Save(c.ConversationsDir, r); err != nil {
				t.Fatal(err)
			}
		}
	})

	servertest.WaitFor(t, 10*time.Second, "one grilling round on design, old-no-flag and codex", func() bool {
		recs := recordsByID(t, h.ConversationsDir)
		return len(recs["design"].Grilling) == 1 && len(recs["old-no-flag"].Grilling) == 1 && len(recs["codex"].Grilling) == 1
	})
	recs := recordsByID(t, h.ConversationsDir)
	if recs["codex-no-rollout"].Grilling != nil || recs["plain"].Grilling != nil {
		t.Errorf("codex-no-rollout = %+v, plain = %+v; want both uncounted", recs["codex-no-rollout"].Grilling, recs["plain"].Grilling)
	}
}

func writeCodexRound(t *testing.T, codexHome, id string) {
	t.Helper()
	dir := filepath.Join(codexHome, "sessions", "2026", "01", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"2026-01-02T03:04:05Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"❓ **Q1** - **Alpha**: one two"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-01-02T03-04-05-"+id+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

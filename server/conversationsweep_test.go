package server_test

import (
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/transcript"
)

func TestStartupSweep_CountsUncountedClaudeDesignRecords(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	at := time.Now()
	seed := []conversations.Record{
		{ID: "design", Dir: dir, Harness: "claude", Ticket: "proj:map", Design: true},
		{ID: "old-no-flag", Dir: dir, Harness: "claude", Ticket: "proj:map/01"},
		{ID: "codex", Dir: dir, Harness: "codex", Ticket: "proj:map", Design: true},
		{ID: "plain", Dir: dir, Harness: "claude", Ticket: "proj:epic/01"},
	}
	for _, r := range seed {
		writeGrillingRoundsAt(t, transcript.PathIn(home, dir, r.ID), at)
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

	deadline := time.Now().Add(10 * time.Second)
	for {
		recs := recordsByID(t, h.ConversationsDir)
		if len(recs["design"].Grilling) == 1 && len(recs["old-no-flag"].Grilling) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("design = %+v, old-no-flag = %+v; want one round each", recs["design"].Grilling, recs["old-no-flag"].Grilling)
		}
		time.Sleep(20 * time.Millisecond)
	}
	recs := recordsByID(t, h.ConversationsDir)
	if recs["codex"].Grilling != nil || recs["plain"].Grilling != nil {
		t.Errorf("codex = %+v, plain = %+v; want both uncounted", recs["codex"].Grilling, recs["plain"].Grilling)
	}
}

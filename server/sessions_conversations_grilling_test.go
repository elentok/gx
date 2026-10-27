package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

// writeGrillingRoundsAt writes one round per timestamp to the transcript at
// path, each answered by a person message.
func writeGrillingRoundsAt(t *testing.T, path string, at ...time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i, ts := range at {
		stamp := ts.UTC().Format(time.RFC3339Nano)
		for _, line := range []map[string]any{
			{"type": "assistant", "uuid": "round-" + string(rune('a'+i)), "timestamp": stamp, "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "❓ **Q1** - **Alpha**: one two"}}}},
			{"type": "user", "uuid": "answer-" + string(rune('a'+i)), "timestamp": stamp, "message": map[string]any{"role": "user", "content": "yes"}},
		} {
			if err := enc.Encode(line); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func designMapStore(t *testing.T) string {
	t.Helper()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "map", "01", "first", "")
	if err := os.WriteFile(filepath.Join(store, "proj", "map", "ticket.md"), []byte("---\nkind: map\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

// HOME is set only on the server.
func TestSessionConversations_DesignCountsSavedWhenAConversationEnds(t *testing.T) {
	home := t.TempDir()
	h := servertest.StartWithStore(t, designMapStore(t), servertest.WithTestHolder(), servertest.WithEnv("HOME="+home))
	checkDesignCountsSaved(t, h, func(dir, conv string, at ...time.Time) {
		writeGrillingRoundsAt(t, transcript.PathIn(home, dir, conv), at...)
	})
}

func checkDesignCountsSaved(t *testing.T, h *servertest.Harness, writeGrillingRounds func(dir, conv string, at ...time.Time)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.CreateSession(ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, h, id)["conversation_id"].(string)
	dir, _ := readMeta(t, h, id)["dir"].(string)
	newer := conv + "-cleared"
	local := time.Date(2026, 3, 4, 23, 59, 0, 0, time.Local)
	writeGrillingRounds(dir, conv, local, local.Add(2*time.Minute))
	clearFakeSession(ctx, t, h, evs, id, conv, newer)

	recs := recordsByID(t, h.ConversationsDir)
	if !recs[conv].Design || !recs[newer].Design {
		t.Errorf("design flags = %v, %v; want both true", recs[conv].Design, recs[newer].Design)
	}
	first := recs[conv].Grilling
	if len(first) != 2 || first[0].At.Local().Day() != 4 || first[1].At.Local().Day() != 5 {
		t.Errorf("first grilling = %+v; want two rounds, on local days 4 and 5", first)
	}
	if recs[newer].Grilling != nil {
		t.Errorf("running conversation grilling = %+v; want none yet", recs[newer].Grilling)
	}

	writeGrillingRounds(dir, newer, local)
	stopFakeSession(ctx, t, h, evs, id)
	if second := recordsByID(t, h.ConversationsDir)[newer].Grilling; len(second) != 1 {
		t.Errorf("second grilling = %+v; want one round after exit", second)
	}
}

func TestSessionConversations_NonDesignSessionLeavesNoCounts(t *testing.T) {
	home := t.TempDir()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"}, servertest.WithEnv("HOME="+home))
	dir, _ := readMeta(t, h, id)["dir"].(string)
	writeGrillingRoundsAt(t, transcript.PathIn(home, dir, conv), time.Now())
	stopFakeSession(ctx, t, h, evs, id)

	rec := recordsByID(t, h.ConversationsDir)[conv]
	if rec.Design || rec.Grilling != nil {
		t.Errorf("record = %+v; want no Design flag and no counts", rec)
	}
}

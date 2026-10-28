package server_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/transcript"
)

// clearFakeSession makes the fake report a new conversation, as /clear does,
// and waits until the session's meta holds it.
func clearFakeSession(ctx context.Context, t *testing.T, h *servertest.Harness, evs <-chan server.Event, id, oldConv, newConv string) {
	t.Helper()
	if err := agentfake.SendProgramControl(oldConv, agentfake.Control{Action: agentfake.ActionConversation, ConversationID: newConv}); err != nil {
		t.Fatal(err)
	}
	report(t, oldConv, agentfake.Control{State: "idle"})
	waitForSession(ctx, t, h, evs, id, func(server.SessionInfo) bool { return readMeta(t, h, id)["conversation_id"] == newConv })
}

func recordsByID(t *testing.T, root string) map[string]conversations.Record {
	t.Helper()
	list, err := conversations.List(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]conversations.Record{}
	for _, r := range list {
		out[r.ID] = r
	}
	return out
}

func TestSessionConversations_LinkedSessionKeepsEveryConversation(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"})
	newer := conv + "-cleared"
	clearFakeSession(ctx, t, h, evs, id, conv, newer)

	recs := recordsByID(t, h.ConversationsDir)
	if len(recs) != 2 {
		t.Fatalf("records = %+v; want both conversations", recs)
	}
	for _, c := range []string{conv, newer} {
		r := recs[c]
		if r.Ticket != "proj:epic/01" || r.Harness != string(agentfake.HarnessKind) || r.Dir == "" {
			t.Errorf("record %s = %+v; want ticket, harness and dir", c, r)
		}
	}
	if _, ok, err := conversations.Started(h.ConversationsDir); err != nil || !ok {
		t.Errorf("started = %v, %v; want recording start noted", ok, err)
	}
}

func TestSessionConversations_ReLinkMovesTheRunningConversation(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"})
	newer := conv + "-cleared"
	clearFakeSession(ctx, t, h, evs, id, conv, newer)

	if res, err := h.Client.LinkSession(ctx, server.LinkSessionRequest{ID: id, Ticket: "proj:epic/02"}); err != nil || res.Refused {
		t.Fatalf("link = %+v, %v", res, err)
	}
	recs := recordsByID(t, h.ConversationsDir)
	if got := recs[conv].Ticket; got != "proj:epic/01" {
		t.Errorf("earlier conversation ticket = %q; want it to stay with the first ticket", got)
	}
	if got := recs[newer].Ticket; got != "proj:epic/02" {
		t.Errorf("running conversation ticket = %q; want the latest ticket", got)
	}
}

func TestSessionConversations_SurviveRemoveAndSkipUnlinked(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"})
	stopFakeSession(ctx, t, h, evs, id)
	if res, err := h.Client.RemoveSession(ctx, id); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(h.SessionsDir, id)); !os.IsNotExist(err) {
		t.Fatalf("session dir still there: %v", err)
	}
	if recs := recordsByID(t, h.ConversationsDir); recs[conv].Ticket != "proj:epic/01" {
		t.Errorf("records after remove = %+v; want %s kept", recs, conv)
	}

	h2, _, _, _, _ := startFakeSession(t)
	if recs := recordsByID(t, h2.ConversationsDir); len(recs) != 0 {
		t.Errorf("unlinked session recorded %+v; want none", recs)
	}
}

// writeTranscriptTurns writes a Claude transcript for the conversation, one
// assistant turn per input-token count, each with 10 output tokens.
func writeTranscriptTurns(t *testing.T, h *servertest.Harness, dir, conv string, inputs ...int) {
	t.Helper()
	path := transcript.PathIn(h.Env("HOME"), dir, conv)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i, in := range inputs {
		ts := time.Now().Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		lines = append(lines, fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":%d,"output_tokens":10}}}`, ts, in))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionConversations_TotalsSavedWhenAConversationEnds(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"})
	dir, _ := readMeta(t, h, id)["dir"].(string)
	newer := conv + "-cleared"
	writeTranscriptTurns(t, h, dir, conv, 100, 200)
	clearFakeSession(ctx, t, h, evs, id, conv, newer)

	recs := recordsByID(t, h.ConversationsDir)
	first := recs[conv].Totals
	if first == nil || first.TotalTokens != 320 || first.Cost <= 0 {
		t.Errorf("first totals = %+v; want 320 tokens and a cost after /clear", first)
	}
	if recs[newer].Totals != nil {
		t.Errorf("running conversation totals = %+v; want none yet", recs[newer].Totals)
	}

	writeTranscriptTurns(t, h, dir, newer, 50)
	stopFakeSession(ctx, t, h, evs, id)
	second := recordsByID(t, h.ConversationsDir)[newer].Totals
	if second == nil || second.TotalTokens != 60 || second.Cost <= 0 {
		t.Errorf("second totals = %+v; want 60 tokens and a cost after exit", second)
	}
}

func TestSessionConversations_NoTranscriptLeavesNoTotals(t *testing.T) {
	t.Parallel()
	h, ctx, evs, id, conv := startFakeSessionWith(t, server.CreateSessionRequest{Ticket: "proj:epic/01"})
	stopFakeSession(ctx, t, h, evs, id)
	if r := recordsByID(t, h.ConversationsDir)[conv]; r.Totals != nil {
		t.Errorf("totals = %+v; want none without a transcript", r.Totals)
	}
}

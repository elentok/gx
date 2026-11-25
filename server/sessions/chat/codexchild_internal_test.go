package chat

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

func TestCodexChildPath_MissExpiresWhenTheTestClockMoves(t *testing.T) {
	t.Parallel()
	c := claudeChat(newFakeSource(), t.TempDir())
	c.cfg.TranscriptMissTTL = time.Minute
	now := time.Now()
	c.now = func() time.Time { return now }
	lookups := 0
	c.rolloutPath = func(string, string) (string, bool, error) { lookups++; return "", false, nil }

	c.codexChildPath("th", Meta{})
	c.codexChildPath("th", Meta{})
	if lookups != 1 {
		t.Fatalf("lookups inside the TTL = %d, want 1", lookups)
	}
	now = now.Add(2 * time.Minute)
	c.codexChildPath("th", Meta{})
	if lookups != 2 {
		t.Errorf("lookups after the TTL = %d, want 2", lookups)
	}
}

func TestRead_CodexSubagentRowsTakeDetailsFromTheChildRollout(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	c := claudeChat(f.src, t.TempDir())
	c.cfg.TranscriptMissTTL = time.Minute
	lookups, parses := &atomic.Int32{}, &atomic.Int32{}
	c.rolloutPath = func(home, id string) (string, bool, error) {
		lookups.Add(1)
		return codexsession.RolloutPathIn(home, id)
	}
	c.reads.codexChild = func(p string) (codexChild, error) {
		parses.Add(1)
		return readCodexChild(p)
	}
	f.srv = serveChat(t, f.src, c)

	ts := func(ago time.Duration) string { return time.Now().UTC().Add(-ago).Format(time.RFC3339Nano) }
	spawn := func(call, name string, ago time.Duration) string {
		return fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":%q,"arguments":"{\"task_name\":\"%s\"}"}}`, ts(ago), call, name)
	}
	started := func(call, thread string, ago time.Duration) string {
		return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":"started","id":%q,"agent_thread_id":%q,"agent_path":"/root/x","model":"started-model"}}}`, ts(ago), call, thread)
	}
	writeLines(t, f.path,
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"cli_version":"0.150.0"}}`, ts(time.Minute)),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"task_started"}}`, ts(55*time.Second)),
		spawn("c1", "has-file", 50*time.Second), started("c1", "th-file", 49*time.Second),
		spawn("c2", "no-file", 40*time.Second), started("c2", "th-none", 39*time.Second),
	)
	child := filepath.Join(f.src.codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-th-file.jsonl")
	turn := `{"type":"turn_context","payload":{"model":"child-model"}}`
	count := func(total int) string {
		return fmt.Sprintf(`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":%d}}}}`, total)
	}
	writeLines(t, child, turn, count(100))

	rows := func() map[string]wire.SessionTask {
		out := map[string]wire.SessionTask{}
		for _, r := range f.chat(t, "/v1/sessions/chat").Tasks {
			out[r.Label] = r
		}
		return out
	}
	got := rows()
	if r := got["has-file"]; r.Kind != "subagent" || r.Model != "child-model" || r.Tokens != 100 {
		t.Errorf("has-file = %+v, want child-model and 100 tokens", r)
	}
	if r := got["no-file"]; r.Model != "started-model" || r.Tokens != 0 {
		t.Errorf("no-file = %+v, want the started model and no tokens", r)
	}
	rows()
	if parses.Load() != 1 {
		t.Errorf("unchanged child parsed %d times, want 1", parses.Load())
	}
	if lookups.Load() != 2 {
		t.Errorf("path lookups = %d, want 2 (one per thread id)", lookups.Load())
	}
	writeLines(t, child, turn, count(100), count(900))
	if r := rows()["has-file"]; r.Tokens != 900 {
		t.Errorf("grown child: tokens = %d, want 900", r.Tokens)
	}
	if parses.Load() != 2 {
		t.Errorf("grown child parsed %d times in total, want 2", parses.Load())
	}
}

func TestCodexSubagent_ResolvesThroughThreadIDOnly(t *testing.T) {
	t.Parallel()
	c := claudeChat(newFakeSource(), t.TempDir())
	c.rolloutPath = func(_, id string) (string, bool, error) { return "/r/" + id, true, nil }
	ref, ok := c.codexSubagent(transcript.TaskRecord{CallID: "c1", ID: "th1"}, Meta{})
	if !ok || ref.Path != "/r/th1" || ref.AgentID != "th1" {
		t.Fatalf("ref = %+v, %v", ref, ok)
	}
	if _, ok := c.codexSubagent(transcript.TaskRecord{CallID: "c1"}, Meta{}); ok {
		t.Fatal("a call with no started event resolved")
	}
}

package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/server/wire"
)

// codexPage is a Codex session with one spawn_agent call "c1" whose child is
// thread "th1". extra lines follow the started event in the parent's rollout.
type codexPage struct {
	f     *readFixture
	child string
}

func newCodexPage(t *testing.T, ago time.Duration, extra ...string) *codexPage {
	t.Helper()
	f := newCodexFixture(t)
	c := claudeChat(f.src, t.TempDir())
	c.cfg.TranscriptMissTTL = time.Hour
	f.srv = serveChat(t, f.src, c)
	ts := func(d time.Duration) string { return time.Now().UTC().Add(-d).Format(time.RFC3339Nano) }
	lines := []string{
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"cli_version":"0.150.0"}}`, ts(ago+time.Minute)),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"c1","arguments":"{\"task_name\":\"rev\"}"}}`, ts(ago)),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":"started","id":"c1","agent_thread_id":"th1","agent_path":"/root/rev","model":"started-model"}}}`, ts(ago)),
	}
	writeLines(t, f.path, append(lines, extra...)...)
	return &codexPage{f: f, child: filepath.Join(f.src.codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-th1.jsonl")}
}

func (p *codexPage) read(t *testing.T, call string) wire.SessionChatResponse {
	t.Helper()
	code, body := p.f.getQuery(t, "/v1/sessions/chat", "subagent="+call)
	var got wire.SessionChatResponse
	if err := json.Unmarshal(body, &got); err != nil || code != http.StatusOK || got.Subagent == nil {
		t.Fatalf("subagent=%s = %d %q (%v)", call, code, body, err)
	}
	return got
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func completedEvent() string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":"completed","id":"x","agent_thread_id":"th1","agent_path":"/root/rev"}}}`, nowStamp())
}

func codexChildLines(role string, withHistory bool) []string {
	msg := func(ord int, role, text string) string {
		return fmt.Sprintf(`{"ordinal":%d,"type":"response_item","payload":{"type":"message","role":%q,"content":[{"type":"output_text","text":%q}]}}`, ord, role, text)
	}
	meta := fmt.Sprintf(`{"ordinal":0,"type":"session_meta","payload":{"cli_version":"0.150.0","subagent_history_start_ordinal":3,"source":{"subagent":{"thread_spawn":{"agent_role":%s}}}}}`, role)
	lines := []string{meta}
	if withHistory {
		lines = append(lines, msg(1, "user", "PARENT-PROMPT"), msg(2, "assistant", "PARENT-REPLY"))
	}
	return append(lines,
		msg(3, "user", "NEW_TASK header"), msg(4, "assistant", "child work"), msg(5, "assistant", "child answer"),
		`{"type":"turn_context","payload":{"model":"child-model"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":77}}}}`)
}

func TestCodexChild_PageCutsHistoryAndHidesThePrompt(t *testing.T) {
	t.Parallel()
	p := newCodexPage(t, time.Minute)
	writeLines(t, p.child, codexChildLines(`"explorer"`, true)...)
	got := p.read(t, "c1")
	body, _ := json.Marshal(got)
	for _, bad := range []string{"PARENT-PROMPT", "PARENT-REPLY", "NEW_TASK"} {
		if strings.Contains(string(body), bad) {
			t.Errorf("response has %q: %s", bad, body)
		}
	}
	h := got.Subagent
	if h.Prompt != "Prompt not readable" || h.Result != "child answer" || h.Type != "explorer" || h.Title != "rev" || h.ToolCalls != 0 {
		t.Errorf("header = %+v", h)
	}
	if h.Status != "running" || h.Model != "child-model" || h.Tokens != 77 {
		t.Errorf("header = %+v, want running child-model 77", h)
	}
	row := p.f.chat(t, "/v1/sessions/chat").Tasks
	if len(row) != 1 || row[0].Model != h.Model || row[0].Tokens != h.Tokens {
		t.Errorf("row = %+v, header = %+v: want the same model and tokens", row, h)
	}
}

func TestCodexChild_NullRoleHasNoType(t *testing.T) {
	t.Parallel()
	p := newCodexPage(t, time.Minute)
	writeLines(t, p.child, codexChildLines(`null`, false)...)
	if h := p.read(t, "c1").Subagent; h.Type != "" || h.ToolCalls != 0 {
		t.Errorf("header = %+v, want no type and no tool calls", h)
	}
}

func TestCodexChild_StatusFollowsTheEndRules(t *testing.T) {
	t.Parallel()
	wait := `{"timestamp":"` + nowStamp() + `","type":"response_item","payload":{"type":"function_call","name":"wait_agent","call_id":"w1","arguments":"{}"}}`
	waitOut := `{"timestamp":"` + nowStamp() + `","type":"response_item","payload":{"type":"function_call_output","call_id":"w1","output":"{\"status\":{\"th1\":{\"completed\":\"done\"}}}"}}`
	for _, tc := range []struct {
		name  string
		ago   time.Duration
		extra []string
		setup func(*readFixture)
		want  string
	}{
		{"spawn returned, no end signal", time.Minute, nil, nil, "running"},
		{"completed event", time.Minute, []string{completedEvent()}, nil, "done"},
		{"wait completion", time.Minute, []string{wait, waitOut}, nil, "done"},
		{"exited session", time.Minute, nil, func(f *readFixture) { f.src.meta.Status.State = wire.SessionExited }, "stopped"},
		{"new run", time.Minute, nil, func(f *readFixture) { f.src.meta.RunStartedAt = time.Now() }, "stopped"},
		{"age-out", 3 * time.Hour, nil, nil, "stopped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newCodexPage(t, tc.ago, tc.extra...)
			writeLines(t, p.child, codexChildLines(`null`, false)...)
			if tc.setup != nil {
				tc.setup(p.f)
			}
			if h := p.read(t, "c1").Subagent; h.Status != tc.want || h.Result != "child answer" {
				t.Errorf("header = %+v, want status %s", h, tc.want)
			}
		})
	}
}

func TestCodexChild_MissingRollout(t *testing.T) {
	t.Parallel()
	t.Run("running child is starting", func(t *testing.T) {
		t.Parallel()
		p := newCodexPage(t, time.Minute)
		if h := p.read(t, "c1").Subagent; h.Status != "running" || h.Unavailable != "starting" || h.Title != "rev" {
			t.Errorf("header = %+v", h)
		}
	})
	t.Run("ended child is gone", func(t *testing.T) {
		t.Parallel()
		p := newCodexPage(t, time.Minute, completedEvent())
		if h := p.read(t, "c1").Subagent; h.Status != "not_available" || h.Unavailable != "gone" || h.Title != "rev" {
			t.Errorf("header = %+v", h)
		}
	})
	t.Run("only a compressed rollout", func(t *testing.T) {
		t.Parallel()
		p := newCodexPage(t, time.Minute)
		writeLines(t, filepath.Join(p.f.src.codexHome, "archived_sessions", "rollout-2026-01-01T10-00-00-th1.jsonl.zst"), "zst")
		if h := p.read(t, "c1").Subagent; h.Status != "not_available" || h.Unavailable != "compressed" {
			t.Errorf("header = %+v", h)
		}
	})
	t.Run("a rollout that appears after a miss is found at once", func(t *testing.T) {
		t.Parallel()
		p := newCodexPage(t, time.Minute)
		if h := p.read(t, "c1").Subagent; h.Unavailable != "starting" {
			t.Fatalf("first read = %+v, want starting", h)
		}
		writeLines(t, p.child, codexChildLines(`null`, false)...)
		if h := p.read(t, "c1").Subagent; h.Unavailable != "" || h.Result != "child answer" {
			t.Errorf("second read = %+v, want the child's page", h)
		}
	})
}

func TestCodexChild_UnknownCallIs404(t *testing.T) {
	t.Parallel()
	p := newCodexPage(t, time.Minute)
	if code, body := p.f.getQuery(t, "/v1/sessions/chat", "subagent=nope"); code != http.StatusNotFound || !strings.Contains(string(body), "no such subagent") {
		t.Errorf("= %d %q, want 404 no such subagent", code, body)
	}
}

func TestCodexChild_ReadsOneLookupAndOneParse(t *testing.T) {
	t.Parallel()
	p := newCodexPage(t, time.Minute)
	writeLines(t, p.child, codexChildLines(`null`, false)...)
	lookups, parses := &atomic.Int32{}, &atomic.Int32{}
	c := claudeChat(p.f.src, t.TempDir())
	c.rolloutPath = func(home, id string) (string, bool, error) {
		lookups.Add(1)
		return codexsession.RolloutPathIn(home, id)
	}
	c.reads.codexChild = func(path string) (codexChild, error) {
		parses.Add(1)
		return readCodexChild(path)
	}
	p.f.srv = serveChat(t, p.f.src, c)
	p.read(t, "c1")
	p.read(t, "c1")
	if lookups.Load() != 1 || parses.Load() != 1 {
		t.Errorf("lookups = %d, parses = %d, want 1 and 1", lookups.Load(), parses.Load())
	}
}

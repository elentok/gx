package watchflows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"
	"github.com/elentok/gx/testutil/agentfake"
)

// subWatch is a Claude or Codex chat session served on a web mux, with a parent
// transcript holding one subagent call "a1" (Claude) or "c1" (Codex).
type subWatch struct {
	h      *servertest.Harness
	base   string
	cookie *http.Cookie
	evs    <-chan wire.Event
	id     string
	parent string
	// dir is where the Claude subagent files go; child is the Codex rollout.
	dir   string
	child string
}

func startSubWatch(t *testing.T, codex bool, parentLines func(now string) []string) *subWatch {
	t.Helper()
	cfg, codexHome := t.TempDir(), t.TempDir()
	h := servertest.StartWithStore(t, t.TempDir(), servertest.WithTestHolder(), servertest.WithChatHarness(string(agentfake.HarnessKind)), servertest.WithDefaultHarness(string(agentfake.HarnessKind)), servertest.WithChatPollInterval(chatPollTestInterval),
		servertest.WithEnv("CLAUDE_CONFIG_DIR="+cfg, "CODEX_HOME="+codexHome), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	evs, err := h.Client.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := h.Client.Sessions.Create(ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	meta := readMeta(t, h, id)
	conv, _ := meta["conversation_id"].(string)
	f := &subWatch{h: h, evs: evs, id: id}
	if codex {
		meta["harness"] = "codex"
		data, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		day := filepath.Join(codexHome, "sessions", "2026", "01", "01")
		f.parent = filepath.Join(day, "rollout-2026-01-01T10-00-00-"+conv+".jsonl")
		f.child = filepath.Join(day, "rollout-2026-01-01T10-00-01-th1.jsonl")
	} else {
		f.parent = filepath.Join(cfg, "projects", "-repo", conv+".jsonl")
		f.dir = strings.TrimSuffix(f.parent, ".jsonl") + "/subagents"
	}
	servertest.WriteTranscript(t, f.parent, parentLines(time.Now().UTC().Format(time.RFC3339Nano))...)
	hs, err := h.Client.Handshake(ctx)
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	f.base, f.cookie = hs.WebURL, servertest.Login(t, h.StateDir, hs.WebURL)
	return f
}

func claudeCall(now string) []string {
	return []string{fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"a1","name":"Agent","input":{"description":"explore","prompt":"find foo"}}]}}`, now)}
}

func claudeCallDone(now string) []string {
	return append(claudeCall(now), fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"a1","content":"ok"}]},"toolUseResult":{"status":"completed"}}`, now))
}

func codexCall(now string) []string {
	return []string{
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"cli_version":"0.150.0"}}`, now),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","call_id":"c1","arguments":"{\"task_name\":\"rev\"}"}}`, now),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","kind":"started","id":"c1","agent_thread_id":"th1","agent_path":"/root/rev","model":"m"}}}`, now),
	}
}

const (
	subFileLine   = `{"type":"assistant","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"work"}]}}`
	codexChildMsg = `{"ordinal":4,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"child work"}]}}`
)

// writeSubagent writes the Claude sidecar and file of call a1.
func (f *subWatch) writeSubagent(t *testing.T) string {
	t.Helper()
	path := f.dir + "/agent-ag1.jsonl"
	servertest.WriteTranscript(t, path, subFileLine)
	if err := os.WriteFile(f.dir+"/agent-ag1.meta.json", []byte(`{"toolUseId":"a1","agentType":"Explore"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// openTab opens an event stream of client; closing it is the page going away.
func (f *subWatch) openTab(t *testing.T, client string) (closeTab func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.base+"/v1/events?client="+client, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.cookie)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
	return cancel
}

func (f *subWatch) watch(t *testing.T, req subagent.WatchRequest) wire.SessionResult {
	t.Helper()
	body, _ := json.Marshal(req)
	code, res := servertest.WebPostJSON(t, f.base+"/v1/sessions/subagent-watch", string(body), f.cookie, "", servertest.JSONPost(f.base))
	var out wire.SessionResult
	if code != http.StatusOK || json.Unmarshal([]byte(res), &out) != nil {
		t.Fatalf("subagent-watch = %d %s", code, res)
	}
	return out
}

// eventWithin reports whether a subagent event for the session arrives within d.
func (f *subWatch) eventWithin(d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case ev := <-f.evs:
			if ev.Type == wire.EventSessionSubagentChanged && ev.Address == f.id && ev.Call != "" {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// discardQueued drops the events already queued, so a later wait sees only new ones.
func (f *subWatch) discardQueued() {
	for {
		select {
		case <-f.evs:
		default:
			return
		}
	}
}

func (f *subWatch) open(t *testing.T, call string) wire.SessionResult {
	t.Helper()
	return f.watch(t, subagent.WatchRequest{Client: "page-1", Session: f.id, CallID: call})
}

func TestSessionChatChanged_StartingClaudeSubagentPublishesWhenItsFileAppears(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	f.openTab(t, "page-1")
	if res := f.open(t, "a1"); res.Refused {
		t.Fatalf("watch refused: %+v", res)
	}
	f.writeSubagent(t)
	if !f.eventWithin(5 * time.Second) {
		t.Fatal("no subagent event when the sidecar and file appeared")
	}
}

func TestSessionChatChanged_StartingCodexChildPublishesWhenItsRolloutAppears(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, true, codexCall)
	f.openTab(t, "page-1")
	if res := f.open(t, "c1"); res.Refused {
		t.Fatalf("watch refused: %+v", res)
	}
	servertest.WriteTranscript(t, f.child, codexChildMsg)
	if !f.eventWithin(5 * time.Second) {
		t.Fatal("no subagent event when the child rollout appeared")
	}
}

func TestSessionChatChanged_SubagentFileGrowthPublishesAndUnchangedDoesNot(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	f.open(t, "a1")
	// Negative wait: no event marks the absence of a change.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event for an unchanged file")
	}
	servertest.AppendLines(t, path, subFileLine)
	if !f.eventWithin(5 * time.Second) {
		t.Fatal("no subagent event when the file grew")
	}
}

func TestSessionChatChanged_SubagentWatchRefusesUnknownClientCallAndSession(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	if res := f.open(t, "a1"); !res.Refused || res.Reason != wire.ReasonSubagentWatchUnknownClient {
		t.Fatalf("unknown client = %+v", res)
	}
	f.openTab(t, "page-1")
	if res := f.open(t, "nope"); !res.Refused || res.Reason != wire.ReasonUnknownSubagent {
		t.Fatalf("unknown call = %+v", res)
	}
	if res := f.watch(t, subagent.WatchRequest{Client: "page-1", Session: "nope", CallID: "a1"}); !res.Refused || res.Reason != wire.ReasonSessionUnknown {
		t.Fatalf("unknown session = %+v", res)
	}
}

func TestSessionChatChanged_SubagentWatchEndsWithTheStream(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	closeTab := f.openTab(t, "page-1")
	f.open(t, "a1")
	closeTab()
	servertest.WaitFor(t, 5*time.Second, "the stream's client to be gone", func() bool {
		return f.open(t, "a1").Reason == wire.ReasonSubagentWatchUnknownClient
	})
	f.discardQueued()
	servertest.AppendLines(t, path, subFileLine)
	// Negative wait: no event marks the end of a watch.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event after the stream closed")
	}
}

func TestSessionChatChanged_SubagentWatchEndsWhenThePageReportsClosed(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	f.open(t, "a1")
	if res := f.watch(t, subagent.WatchRequest{Client: "page-1", Session: f.id, CallID: "a1", Closed: true}); res.Refused {
		t.Fatalf("close refused: %+v", res)
	}
	f.discardQueued()
	servertest.AppendLines(t, path, subFileLine)
	// Negative wait: no event marks the end of a watch.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event after the page reported closed")
	}
}

func TestSessionChatChanged_FinishedSubagentIsNotWatched(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCallDone)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	if res := f.open(t, "a1"); res.Refused {
		t.Fatalf("watch refused: %+v", res)
	}
	servertest.AppendLines(t, path, subFileLine)
	// Negative wait: nothing was started, so no event can mark it.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event for a finished call")
	}
}

// callResult is the parent's line that ends call a1.
func callResult() string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"a1","content":"ok"}]},"toolUseResult":{"status":"completed"}}`, time.Now().UTC().Format(time.RFC3339Nano))
}

func TestSessionChatChanged_RunningSubagentWatchEndsWhenItsCallEnds(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	path := f.writeSubagent(t)
	f.openTab(t, "page-1")
	f.open(t, "a1")
	servertest.AppendLines(t, f.parent, callResult())
	// Negative wait: the watch needs a tick to see the call end.
	f.eventWithin(5 * chatPollTestInterval)
	f.discardQueued()
	servertest.AppendLines(t, path, subFileLine)
	// Negative wait: no event marks the end of a watch.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event after the call ended")
	}
}

func TestSessionChatChanged_StartingSubagentWatchEndsWhenItsCallEndsWithNoFile(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	f.openTab(t, "page-1")
	f.open(t, "a1")
	servertest.AppendLines(t, f.parent, callResult())
	// Negative wait: the watch needs a tick to see the call end.
	f.eventWithin(5 * chatPollTestInterval)
	f.discardQueued()
	f.writeSubagent(t)
	// Negative wait: no event marks the end of a watch.
	if f.eventWithin(10 * chatPollTestInterval) {
		t.Fatal("subagent event for a file that appeared after the call ended")
	}
}

func TestSessionChatChanged_StartingSubagentWatchKeepsWatchingTheFileThatAppeared(t *testing.T) {
	t.Parallel()
	f := startSubWatch(t, false, claudeCall)
	f.openTab(t, "page-1")
	f.open(t, "a1")
	path := f.writeSubagent(t)
	if !f.eventWithin(5 * time.Second) {
		t.Fatal("no subagent event when the file appeared")
	}
	f.discardQueued()
	servertest.AppendLines(t, path, subFileLine)
	if !f.eventWithin(5 * time.Second) {
		t.Fatal("no subagent event when the appeared file grew")
	}
}

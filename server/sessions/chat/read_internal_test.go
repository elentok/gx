package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

const (
	testCwd  = "/work/proj"
	testConv = "019f7c18-ff7f-7382-a353-e80c020defdb"
)

// readFixture serves the chat routes over a fakeSource, with one session whose
// transcript the test writes.
type readFixture struct {
	src    *fakeSource
	srv    *httptest.Server
	path   string // where the session's transcript or rollout lives
	config string
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func serveChat(t *testing.T, src *fakeSource, c *Chat) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, r := range Routes(func() *Chat { return c }) {
		mux.HandleFunc(r.Pattern, r.Handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newClaudeFixture is a Claude session of a chat-capable harness.
func newClaudeFixture(t *testing.T) *readFixture {
	t.Helper()
	config := t.TempDir()
	src := newFakeSource()
	src.meta = Meta{ID: "s1", Harness: "claude", Dir: testCwd, ConversationID: testConv, Flow: FlowRef{Flow: tickets.FlowDesign, Key: "gx:epic/03"}}
	return &readFixture{
		src:    src,
		config: config,
		srv:    serveChat(t, src, claudeChat(src, config)),
		path:   filepath.Join(config, "projects", transcript.Slugify(testCwd), testConv+".jsonl"),
	}
}

// newCodexFixture is a Codex session whose rollout lives under a fresh CODEX_HOME.
func newCodexFixture(t *testing.T) *readFixture {
	t.Helper()
	src := newFakeSource()
	src.codexHome = t.TempDir()
	src.meta = Meta{ID: "s1", Harness: "codex", Dir: testCwd, ConversationID: testConv}
	return &readFixture{
		src:  src,
		srv:  serveChat(t, src, claudeChat(src, t.TempDir())),
		path: filepath.Join(src.codexHome, "sessions", "2026", "01", "01", "rollout-2026-01-01T10-00-00-"+testConv+".jsonl"),
	}
}

func (f *readFixture) get(t *testing.T, route string) (int, []byte) {
	t.Helper()
	res, err := http.Get(f.srv.URL + route + "?session=s1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, body
}

func (f *readFixture) chat(t *testing.T, route string) wire.SessionChatResponse {
	t.Helper()
	code, body := f.get(t, route)
	var got wire.SessionChatResponse
	if err := json.Unmarshal(body, &got); err != nil || code != http.StatusOK {
		t.Fatalf("%s = %d %q (%v)", route, code, body, err)
	}
	return got
}

func chatOf(t *testing.T, lines ...string) wire.SessionChatResponse {
	t.Helper()
	f := newClaudeFixture(t)
	writeLines(t, f.path, lines...)
	return f.chat(t, "/v1/sessions/chat")
}

func TestTranscriptRead(t *testing.T) {
	t.Parallel()
	read := func(t *testing.T, f *readFixture) wire.SessionTranscriptResponse {
		t.Helper()
		code, body := f.get(t, "/v1/sessions/transcript")
		var got wire.SessionTranscriptResponse
		if err := json.Unmarshal(body, &got); err != nil || code != http.StatusOK {
			t.Fatalf("transcript = %d %q (%v)", code, body, err)
		}
		return got
	}
	for _, tc := range []struct {
		name    string
		fixture func(*testing.T) *readFixture
		harness string
	}{
		{"claude", newClaudeFixture, "claude"},
		{"codex", newCodexFixture, "codex"},
	} {
		t.Run(tc.name+" found", func(t *testing.T) {
			t.Parallel()
			f := tc.fixture(t)
			writeLines(t, f.path, `{"type":"user"}`)
			want := wire.SessionTranscriptResponse{Harness: tc.harness, ConversationID: testConv, Path: f.path, Found: true}
			if got := read(t, f); got != want {
				t.Errorf("transcript = %+v, want %+v", got, want)
			}
		})
		t.Run(tc.name+" missing", func(t *testing.T) {
			t.Parallel()
			want := wire.SessionTranscriptResponse{Harness: tc.harness, ConversationID: testConv}
			if got := read(t, tc.fixture(t)); got != want {
				t.Errorf("transcript = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRead_ShowsVisibleMessagesInOrder(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	writeLines(t, f.path,
		`{"type":"user","uuid":"u1","timestamp":"2026-01-01T10:00:00Z","message":{"content":"<command-name>/gx-design</command-name><command-args>proj:map</command-args>"}}`,
		`{"type":"user","uuid":"u2","isMeta":true,"timestamp":"2026-01-01T10:00:01Z","message":{"content":"caveat"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"thinking","thinking":"hmm"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:03Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"user","uuid":"u3","timestamp":"2026-01-01T10:00:04Z","message":{"content":[{"type":"tool_result","tool_use_id":"t","content":"x"}]}}`,
		`{"type":"user","uuid":"u4","timestamp":"2026-01-01T10:00:05Z","message":{"content":"SQLite"}}`,
		`{"type":"system","uuid":"y1","timestamp":"2026-01-01T10:00:06Z","content":"note"}`,
	)
	got := f.chat(t, "/v1/sessions/chat")
	if got.ConversationID != testConv || got.Unsupported {
		t.Fatalf("chat = %+v, want conversation %s", got, testConv)
	}
	var ids []string
	for _, m := range got.Messages {
		ids = append(ids, m.ID+":"+m.Role+":"+m.Kind+":"+m.Text)
	}
	want := "u1:person:command:/gx-design proj:map,a2:agent:text:Which store?,u4:person:prompt:SQLite"
	if strings.Join(ids, ",") != want {
		t.Errorf("messages = %v, want %s", ids, want)
	}
}

func TestRead_FoldsToolRunsAndMarksCompaction(t *testing.T) {
	t.Parallel()
	got := chatOf(t,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Looking"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Read"}]}}`,
		`{"type":"user","uuid":"r1","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"x"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-01-01T10:00:03Z","message":{"content":[{"type":"thinking","thinking":"hmm"},{"type":"tool_use","id":"t2","name":"Grep"}]}}`,
		`{"type":"user","uuid":"r2","timestamp":"2026-01-01T10:00:04Z","message":{"content":[{"type":"tool_result","tool_use_id":"t2","content":"x"}]}}`,
		`{"type":"assistant","uuid":"a4","timestamp":"2026-01-01T10:00:05Z","message":{"content":[{"type":"tool_use","id":"t3","name":"Edit"}]}}`,
		`{"type":"assistant","uuid":"a5","timestamp":"2026-01-01T10:00:06Z","message":{"content":[{"type":"text","text":"Done"}]}}`,
		`{"type":"assistant","uuid":"a6","timestamp":"2026-01-01T10:00:07Z","message":{"content":[{"type":"tool_use","id":"t4","name":"Bash"}]}}`,
		`{"type":"system","uuid":"c1","subtype":"compact_boundary","timestamp":"2026-01-01T10:00:08Z"}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-01-01T10:00:09Z","message":{"content":"go on"}}`,
	)
	var seen []string
	for _, m := range got.Messages {
		seen = append(seen, fmt.Sprintf("%s:%s:%d:%s", m.ID, m.Kind, m.ToolCount, m.LastTool))
	}
	want := "a1:text:0:,a2:tools:3:Edit,a5:text:0:,a6:tools:1:Bash,c1:compacted:0:,u1:prompt:0:"
	if strings.Join(seen, ",") != want {
		t.Errorf("messages = %v, want %s", seen, want)
	}
}

func TestRead_CapsAtTheLast300Messages(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	var lines []string
	for i := 0; i < 310; i++ {
		lines = append(lines, fmt.Sprintf(`{"type":"user","uuid":"u%d","timestamp":"2026-01-01T10:00:00Z","message":{"content":"m%d"}}`, i, i))
	}
	writeLines(t, f.path, lines...)
	first := f.chat(t, "/v1/sessions/chat")
	if len(first.Messages) != 300 || first.Messages[0].ID != "u10" || first.Messages[299].ID != "u309" || !first.Truncated {
		t.Errorf("chat has %d messages from %s, truncated=%v; want the last 300 from u10, truncated", len(first.Messages), first.Messages[0].ID, first.Truncated)
	}
	if second := f.chat(t, "/v1/sessions/chat"); !reflect.DeepEqual(first, second) {
		t.Error("a second read with no file change differs from the first")
	}
}

func TestRead_OnlyTheNewestConversation(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	writeLines(t, f.path, `{"type":"assistant","uuid":"old","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"before clear"}]}}`)
	writeLines(t, filepath.Join(filepath.Dir(f.path), "conv-2.jsonl"),
		`{"type":"assistant","uuid":"new","timestamp":"2026-01-01T10:01:00Z","message":{"content":[{"type":"text","text":"after clear"}]}}`)
	f.src.meta.ConversationID = "conv-2"
	got := f.chat(t, "/v1/sessions/chat")
	if got.ConversationID != "conv-2" || len(got.Messages) != 1 || got.Messages[0].ID != "new" {
		t.Errorf("chat = %+v, want only conv-2's message", got)
	}
}

func TestRead_NonChatHarnessIsUnsupported(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	f.src.readers = map[string]bool{}
	got := f.chat(t, "/v1/sessions/chat")
	if !got.Unsupported || len(got.Messages) != 0 {
		t.Errorf("chat = %+v, want unsupported and empty", got)
	}
}

func TestRead_CodexRolloutHasTheClaudeShape(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path,
		`{"timestamp":"2026-01-01T10:00:00Z","ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
		`{"timestamp":"2026-01-01T10:00:01Z","ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
		`{"timestamp":"2026-01-01T10:00:02Z","ordinal":2,"type":"response_item","payload":{"type":"custom_tool_call","name":"exec"}}`,
		`{"timestamp":"2026-01-01T10:00:03Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
		`{"timestamp":"2026-01-01T10:00:04Z","ordinal":4,"type":"event_msg","payload":{"type":"task_complete"}}`,
	)
	got := f.chat(t, "/v1/sessions/chat")
	var ids []string
	for _, m := range got.Messages {
		ids = append(ids, fmt.Sprintf("%s:%s:%s:%s:%d", m.ID, m.Role, m.Kind, m.Text, m.ToolCount))
	}
	want := "o1:person:prompt:hello:0,o2:agent:tools::1,o3:agent:text:hi:0"
	if got.Unsupported || got.Unreadable || strings.Join(ids, ",") != want {
		t.Errorf("chat = %+v, messages %v, want %s", got, ids, want)
	}
}

func TestRead_InFlightCarriesCodexCalls(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path,
		`{"ordinal":0,"type":"event_msg","payload":{"type":"task_started"}}`,
		`{"ordinal":1,"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"a","arguments":"{\"cmd\":\"one\"}"}}`,
		`{"ordinal":2,"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"b","arguments":"{\"cmd\":\"two\"}"}}`,
		`{"ordinal":3,"type":"response_item","payload":{"type":"function_call_output","call_id":"a","output":"ok"}}`,
	)
	got := f.chat(t, "/v1/sessions/chat")
	if got.Current == nil || got.Current.Kind != "shell" || got.Current.Label != "two" || len(got.Others) != 0 {
		t.Errorf("current/others = %+v/%+v, want shell two and no others", got.Current, got.Others)
	}
}

func TestHasExecShape(t *testing.T) {
	t.Parallel()
	for version, want := range map[string]bool{
		"": false, "0.144.0": false, "0.143.9": false, "0.144.1": true, "0.166.0-alpha.5.1": true, "1.0.0": true,
	} {
		if got := hasExecShape(version); got != want {
			t.Errorf("hasExecShape(%q) = %v, want %v", version, got, want)
		}
	}
}

// codexExecRollout is a rollout with a yielded exec cell that a wait poll has
// not finished, so the cell is running at the time of the read.
func codexExecRollout(version string) []string {
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }
	texts := func(ss ...string) string {
		var items []map[string]string
		for _, s := range ss {
			items = append(items, map[string]string{"type": "input_text", "text": s})
		}
		b, _ := json.Marshal(items)
		s, _ := json.Marshal(string(b))
		return string(s)
	}
	return []string{
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"cli_version":%q}}`, ts(time.Minute), version),
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"task_started"}}`, ts(50*time.Second)),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":"c1","input":"const r = await tools.exec_command({cmd:\"make test\"});"}}`, ts(40*time.Second)),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c1","output":%s}}`, ts(30*time.Second), texts("Script running with cell ID 7")),
		fmt.Sprintf(`{"timestamp":%q,"type":"response_item","payload":{"type":"function_call","name":"wait","call_id":"w1","arguments":"{\"cell_id\":7}"}}`, ts(20*time.Second)),
	}
}

func TestRead_CodexExecCellIsATaskAndTheWorkingRow(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path, codexExecRollout("0.144.1")...)
	got := f.chat(t, "/v1/sessions/chat")
	if len(got.Tasks) != 1 || got.Tasks[0].Kind != "shell" || got.Tasks[0].Label != "make test" || got.Tasks[0].Finished {
		t.Errorf("tasks = %+v, want one running shell row labelled make test", got.Tasks)
	}
	if got.Current == nil || got.Current.Kind != "shell" || got.Current.Label != "make test" || len(got.Others) != 0 {
		t.Errorf("current/others = %+v/%+v, want shell make test and no others", got.Current, got.Others)
	}
}

func TestRead_CodexBeforeExecShapeHasNoTasks(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"", "0.144.0"} {
		f := newCodexFixture(t)
		writeLines(t, f.path, codexExecRollout(version)...)
		got := f.chat(t, "/v1/sessions/chat")
		if len(got.Tasks) != 0 {
			t.Errorf("version %q: tasks = %+v, want none", version, got.Tasks)
		}
		if got.Current == nil || got.Current.Label != "wait" || got.Current.Kind != "other" {
			t.Errorf("version %q: current = %+v, want the old wait label", version, got.Current)
		}
	}
}

func TestRead_CodexTurnsWithoutMessagesAreUnreadable(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path,
		`{"ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
		`{"ordinal":1,"type":"event_msg","payload":{"type":"task_complete"}}`,
	)
	got := f.chat(t, "/v1/sessions/chat")
	if !got.Unreadable || got.CodexVersion != "0.166.0" || len(got.Messages) != 0 {
		t.Errorf("chat = %+v, want unreadable with version 0.166.0", got)
	}
}

var archivedRollout = []string{
	`{"timestamp":"2026-01-01T10:00:00Z","ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
	`{"timestamp":"2026-01-01T10:00:01Z","ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
	`{"timestamp":"2026-01-01T10:00:03Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
}

// writeZstd compresses lines into the .jsonl.zst that replaces plainPath.
func writeZstd(t *testing.T, plainPath string, lines ...string) {
	t.Helper()
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := plainPath + ".zst"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, enc.EncodeAll([]byte(strings.Join(lines, "\n")+"\n"), nil), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestArchive_RolloutIsFlaggedAndNotDecoded(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	// Not valid zstd: a plain read that opened the archive would fail.
	archive := f.path + ".zst"
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("not zstd"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := f.chat(t, "/v1/sessions/chat")
	if !got.Archived || len(got.Messages) != 0 {
		t.Errorf("chat = %+v, want archived with no messages", got)
	}
	if code, _ := f.get(t, "/v1/sessions/chat/archive"); code != http.StatusUnprocessableEntity {
		t.Errorf("corrupt archive: status = %d, want 422", code)
	}
}

func TestArchive_RouteMatchesThePlainRollout(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeLines(t, f.path, archivedRollout...)
	plain := f.chat(t, "/v1/sessions/chat")
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	writeZstd(t, f.path, archivedRollout...)
	got := f.chat(t, "/v1/sessions/chat/archive")
	if len(plain.Messages) != 2 || len(got.Messages) != len(plain.Messages) {
		t.Fatalf("archive messages = %+v, plain = %+v", got.Messages, plain.Messages)
	}
	for i := range plain.Messages {
		if !reflect.DeepEqual(got.Messages[i], plain.Messages[i]) {
			t.Errorf("message %d = %+v, want %+v", i, got.Messages[i], plain.Messages[i])
		}
	}
}

func TestArchive_RouteWithoutAnArchiveIs404(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	if code, _ := f.get(t, "/v1/sessions/chat/archive"); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestArchive_ConversationIdThatIsNotAUUIDFindsNoArchive(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	writeZstd(t, f.path, archivedRollout...)
	f.src.meta.ConversationID = "019f" // a substring of the archive's name
	if code, _ := f.get(t, "/v1/sessions/chat/archive"); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func assistantText(uuid, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"type":"assistant","uuid":%q,"timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":%s}]}}`, uuid, b)
}

func personPrompt(uuid, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"type":"user","uuid":%q,"timestamp":"2026-01-01T10:00:01Z","message":{"content":%s}}`, uuid, b)
}

const goodRound = "Intro\n\n" +
	"❓ **Q1** - **Which store?**: SQLite or files?\n\n➡️ SQLite\n\n```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: SQLite\n  - id: b\n    label: Files\n```\n\n---\n\n" +
	"❓ **Q2** - **Keep going?**: Ok?\n\n➡️ Yes\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```"

// roundQs is the printed round's questions, nil when the response lists no round card.
func roundQs(c wire.SessionChatResponse) []wire.QuestionCardQuestion {
	for _, card := range c.Questions {
		if card.Source == wire.CardSourceRound {
			return card.Questions
		}
	}
	return nil
}

func TestRead_RoundParsesCardsAndStripsFences(t *testing.T) {
	t.Parallel()
	got := chatOf(t, assistantText("a1", goodRound))
	if len(roundQs(got)) != 2 {
		t.Fatalf("round = %+v, want 2 cards", roundQs(got))
	}
	c1, c2 := roundQs(got)[0], roundQs(got)[1]
	if c1.Label != "1" || c1.Title != "Which store?" || c1.Kind != "options" || c1.Recommended != "a" || len(c1.Options) != 2 || c1.Note != "" {
		t.Errorf("card 1 = %+v", c1)
	}
	if c2.Label != "2" || c2.Title != "Keep going?" || c2.Kind != "yes-no" || c2.Recommended != "yes" {
		t.Errorf("card 2 = %+v", c2)
	}
	if strings.Contains(c1.Body, "---") || strings.Contains(c1.Body, "gx-question") || !strings.Contains(c1.Body, "➡️ SQLite") {
		t.Errorf("card 1 body = %q, want the text up to the rule, without the fence", c1.Body)
	}
	if text := got.Messages[0].Text; strings.Contains(text, "gx-question") || !strings.Contains(text, "Intro") {
		t.Errorf("message text = %q, want fences stripped", text)
	}
}

func TestRead_QuestionsListsTheRoundAsOneCard(t *testing.T) {
	t.Parallel()
	got := chatOf(t, assistantText("a1", goodRound))
	if len(got.Questions) != 1 || got.Questions[0].Source != wire.CardSourceRound || len(got.Questions[0].Questions) != 2 {
		t.Fatalf("questions = %+v, want one round card of 2 questions", got.Questions)
	}
	again := chatOf(t, assistantText("a1", goodRound))
	other := chatOf(t, assistantText("a1", strings.Replace(goodRound, "Keep going?", "Stop?", 1)))
	if again.Questions[0].ID != got.Questions[0].ID || other.Questions[0].ID == got.Questions[0].ID {
		t.Errorf("ids = %q %q %q, want stable then different", got.Questions[0].ID, again.Questions[0].ID, other.Questions[0].ID)
	}
	if none := chatOf(t, assistantText("a2", "hello")); none.Questions == nil || len(none.Questions) != 0 {
		t.Errorf("no round: questions = %#v, want empty non-nil", none.Questions)
	}
}

func TestRead_RoundCardCarriesLinks(t *testing.T) {
	t.Parallel()
	text := "❓ **Q1** - **Pick a variant?**: See the mock.\n\n```gx-question\ntype: yes-no\nlinks:\n  - label: Mock\n    url: https://claude.ai/artifact/abc\n```"
	got := chatOf(t, assistantText("a1", text))
	if len(roundQs(got)) != 1 || len(roundQs(got)[0].Links) != 1 || roundQs(got)[0].Links[0].URL != "https://claude.ai/artifact/abc" || roundQs(got)[0].Links[0].Label != "Mock" {
		t.Fatalf("round = %+v, want the card's link", roundQs(got))
	}
}

func TestRead_RoundDefaultsAreNotCards(t *testing.T) {
	t.Parallel()
	defaults := "\n\n**Decided unless you object**\n\n- Naming: use camelCase\n- Order: alphabetical"
	for name, sep := range map[string]string{"with rule": "\n\n---", "without rule": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			round := "❓ **Q1** - **Which store?**: SQLite or files?\n\n➡️ SQLite\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```" + sep + defaults
			got := chatOf(t, assistantText("a1", round))
			if len(roundQs(got)) != 1 {
				t.Fatalf("round = %+v, want 1 card", roundQs(got))
			}
			if body := roundQs(got)[0].Body; strings.Contains(body, "camelCase") || strings.Contains(body, "Decided") {
				t.Errorf("card body = %q, want no defaults text", body)
			}
		})
	}
}

func TestRead_RoundReadsDriftedHeadersLeniently(t *testing.T) {
	t.Parallel()
	yesNo := "\n\n```gx-question\ntype: yes-no\n```"
	got := chatOf(t, assistantText("a1",
		"❓ **Q1 - Plain title**: body"+yesNo+"\n\n"+
			"❓ **Q2** Third thing? More"+yesNo+"\n\n"+
			"❓ **Q3** - **Bold title** body"+yesNo))
	var titles []string
	for _, c := range roundQs(got) {
		titles = append(titles, fmt.Sprintf("%s=%s", c.Label, c.Title))
	}
	if want := "1=Plain title,2=Third thing?,3=Bold title"; strings.Join(titles, ",") != want {
		t.Errorf("cards = %v, want %s", titles, want)
	}
}

func TestRead_RoundBadOrMissingBlockGivesFreeTextCard(t *testing.T) {
	t.Parallel()
	good := "\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```"
	got := chatOf(t, assistantText("a1",
		"❓ **Q1** - **Bad**: x\n\n```gx-question\ntype: options\n```\n\n---\n\n"+
			"❓ **Q2** - **Missing**: y\n\n---\n\n"+
			"❓ **Q3** - **Twice**: z"+good+good+"\n\n---\n\n"+
			"❓ **Q4** - **Fine**: w"+good))
	if len(roundQs(got)) != 4 {
		t.Fatalf("round = %+v, want 4 cards", roundQs(got))
	}
	for _, c := range roundQs(got)[:3] {
		if c.Kind != "text" || c.Note == "" || len(c.Options) != 0 || c.Recommended != "" {
			t.Errorf("card %s = %+v, want a free-text card with a note", c.Label, c)
		}
	}
	if c := roundQs(got)[3]; c.Kind != "yes-no" || c.Recommended != "yes" || c.Note != "" {
		t.Errorf("card 4 = %+v, want it answerable", c)
	}
}

func TestRead_CommandOutputIsNotAnAgentReply(t *testing.T) {
	t.Parallel()
	out := personPrompt("o1", "<local-command-stdout>Total cost: $1.20</local-command-stdout>")
	got := chatOf(t, assistantText("a1", goodRound), out)
	if len(roundQs(got)) != 2 {
		t.Errorf("round = %+v, want the open round kept past the output entry", roundQs(got))
	}
	last := got.Messages[len(got.Messages)-1]
	if last.ID != "o1" || last.Kind != "output" || last.Text != "Total cost: $1.20" {
		t.Errorf("last message = %+v, want the output entry", last)
	}
	if strings.Contains(got.Messages[0].Text, "gx-question") {
		t.Errorf("agent text kept its fence: %q", got.Messages[0].Text)
	}
}

func TestRead_RoundIsNullOnceThePersonAnswers(t *testing.T) {
	t.Parallel()
	got := chatOf(t, assistantText("a1", goodRound), personPrompt("u1", "yes to both"))
	if roundQs(got) != nil {
		t.Errorf("round = %+v, want null after a person prompt", roundQs(got))
	}
	got = chatOf(t, assistantText("a1", goodRound),
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_use","id":"t","name":"Bash"}]}}`)
	if len(roundQs(got)) != 2 {
		t.Errorf("round = %+v, want it kept when a tool call follows", roundQs(got))
	}
}

func TestRead_NextReadsTheRunEndLine(t *testing.T) {
	t.Parallel()
	for _, next := range []string{"map — go on", "anything the agent picks", "implement"} {
		got := chatOf(t, assistantText("a1", fmt.Sprintf("Done.\n\n✅ Design: all good. Next: %s", next)))
		if got.Next == nil || got.Next.Action.Reason == "" || len(got.Next.Action.Action) == 0 {
			t.Errorf("Next: %s => %+v, want a Continue action", next, got.Next)
		}
	}
	if got := chatOf(t, assistantText("a1", "no run end here")); got.Next != nil {
		t.Errorf("next = %+v, want null without a run-end line", got.Next)
	}
}

func TestRead_RoundWinsOverTheRunEndLine(t *testing.T) {
	t.Parallel()
	got := chatOf(t, assistantText("a1", "✅ Design: halfway. Next: design detector"), assistantText("a2", goodRound))
	if len(roundQs(got)) != 2 || got.Next != nil {
		t.Errorf("round = %d cards, next = %+v, want the round and no Continue", len(roundQs(got)), got.Next)
	}
}

func TestRead_BackgroundListsLiveTasksAndDropsAgedOutOnes(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }
	launch := func(use, input, result string) []string {
		return []string{
			fmt.Sprintf(`{"type":"assistant","uuid":"a-%s","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":"X","input":%s}]}}`, use, ts(time.Minute), use, input),
			fmt.Sprintf(`{"type":"user","uuid":"r-%s","timestamp":%q,"toolUseResult":%s,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}}`, use, ts(time.Minute), result, use),
		}
	}
	long := strings.Repeat("x", 80)
	var lines []string
	lines = append(lines, launch("t1", `{"command":"`+long+`","description":""}`, `{"backgroundTaskId":"b1"}`)...)
	lines = append(lines, launch("t2", `{"description":"watch the logs","command":"tail -f"}`, `{"taskId":"m1","timeoutMs":3600000}`)...)
	lines = append(lines, launch("t3", `{"description":"survey code"}`, `{"status":"async_launched","agentId":"g1"}`)...)
	old := launch("t4", `{"description":"stale"}`, `{"backgroundTaskId":"b4"}`)
	for i := range old {
		old[i] = strings.ReplaceAll(old[i], ts(time.Minute), ts(3*time.Hour))
	}
	lines = append(lines, old...)

	got := chatOf(t, lines...)
	var seen []string
	for _, b := range got.Background {
		seen = append(seen, b.Kind+":"+b.Label)
	}
	want := "shell:" + long[:60] + ",monitor:watch the logs,agent:survey code"
	if strings.Join(seen, ",") != want {
		t.Errorf("background = %v, want %s", seen, want)
	}
	if got.Background[0].StartedAt == "" {
		t.Error("background task has no started_at")
	}
}

func TestRead_TasksListShellsAndCallsOfAClaudeSession(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }
	use := func(id, ago, input string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":"Bash","input":%s}]}}`, ts(parseAgo(ago)), id, input)
	}
	launched := func(id, task, ago string) string {
		return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"Output is being written to: /o/%s.output"}]},"toolUseResult":{"backgroundTaskId":%q}}`, ts(parseAgo(ago)), id, task, task)
	}
	done := func(task, ago string) string {
		body := "<task-notification>\n<task-id>" + task + "</task-id>\n<status>completed</status>\n<summary>done (exit code 0)</summary>\n</task-notification>"
		return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts(parseAgo(ago)), body)
	}
	lines := []string{
		use("u1", "5m", `{"command":"make","description":"build","run_in_background":true}`),
		launched("u1", "b1", "5m"),
		use("u2", "4m", `{"command":"lint","description":"lint","run_in_background":true}`),
		launched("u2", "b2", "4m"),
		done("b1", "2m"),
		use("u3", "1m", `{"command":"sleep 30","description":"wait"}`),
	}

	f := newClaudeFixture(t)
	writeLines(t, f.path, lines...)
	got := f.chat(t, "/v1/sessions/chat").Tasks
	if labels := taskLabels(got); !reflect.DeepEqual(labels, []string{"build", "lint", "wait"}) {
		t.Fatalf("tasks = %v", labels)
	}
	if !got[0].Finished || got[0].Outcome != "success" || got[0].FinishedAt == "" || got[1].Finished || got[2].Kind != "call" || got[2].Finished {
		t.Errorf("rows = %+v", got)
	}

	// Once the last task ends the group closes.
	writeLines(t, f.path, append(lines, done("b2", "30s"), `{"type":"system","subtype":"turn_duration","timestamp":"`+ts(10*time.Second)+`"}`)...)
	if got := f.chat(t, "/v1/sessions/chat").Tasks; len(got) != 0 {
		t.Errorf("after the last task = %v", taskLabels(got))
	}

	// An exited session keeps no running rows.
	f = newClaudeFixture(t)
	f.src.meta.Status.State = wire.SessionExited
	writeLines(t, f.path, lines...)
	if got := f.chat(t, "/v1/sessions/chat").Tasks; len(got) != 0 {
		t.Errorf("exited session = %v", taskLabels(got))
	}
}

func TestRead_TasksListMonitorsAndSubagents(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }
	use := func(name, id string, ago time.Duration, input string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, ts(ago), id, name, input)
	}
	result := func(id string, ago time.Duration, res string) string {
		return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]},"toolUseResult":%s}`, ts(ago), id, res)
	}
	note := func(task, status string, ago time.Duration) string {
		body := "<task-notification>\n<task-id>" + task + "</task-id>\n"
		if status != "" {
			body += "<status>" + status + "</status>\n"
		}
		body += "<summary>s</summary>\n</task-notification>"
		return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","timestamp":%q,"content":%q}`, ts(ago), body)
	}
	agent := []string{
		use("Agent", "a1", 5*time.Minute, `{"description":"explore","prompt":"p"}`),
		result("a1", 5*time.Minute, `{"status":"async_launched","agentId":"ag1","resolvedModel":"m","outputFile":"/o/ag1.output"}`),
	}
	monitor := []string{
		use("Monitor", "m1", 4*time.Minute, `{"command":"tail","description":"watch","timeout_ms":600000}`),
		result("m1", 4*time.Minute, `{"taskId":"mon1","timeoutMs":600000}`),
		note("mon1", "", 3*time.Minute),
	}

	f := newClaudeFixture(t)
	writeLines(t, f.path, append(append([]string{}, agent...), monitor...)...)
	got := f.chat(t, "/v1/sessions/chat").Tasks
	if labels := taskLabels(got); !reflect.DeepEqual(labels, []string{"explore", "watch"}) {
		t.Fatalf("tasks = %v", labels)
	}
	if got[0].Kind != "subagent" || got[0].Model != "m" || got[0].Finished || got[1].Kind != "monitor" || got[1].EventCount != 1 || got[1].Finished {
		t.Errorf("rows = %+v", got)
	}

	// The subagent alone holds the group; its final notice clears it.
	writeLines(t, f.path, append(append([]string{}, agent...), note("ag1", "", time.Minute))...)
	if got := f.chat(t, "/v1/sessions/chat").Tasks; len(got) != 1 {
		t.Errorf("running subagent = %v", taskLabels(got))
	}
	writeLines(t, f.path, append(append([]string{}, agent...), note("ag1", "completed", time.Minute))...)
	if got := f.chat(t, "/v1/sessions/chat").Tasks; len(got) != 0 {
		t.Errorf("after final notice = %v", taskLabels(got))
	}
}

func parseAgo(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestRead_InFlightCarriesUnmatchedCalls(t *testing.T) {
	t.Parallel()
	prompt := `{"type":"user","message":{"content":"go"}}`
	use := func(id, cmd string) string {
		return fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":%q,"name":"Bash","input":{"command":%q}}]}}`, id, cmd)
	}
	result := func(id string) string {
		return fmt.Sprintf(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":%q,"content":"ok"}]}}`, id)
	}

	got := chatOf(t, prompt, use("a", "one"), use("b", "two"), use("c", "three"), result("a"))
	if got.Current == nil || got.Current.Kind != "shell" || got.Current.Label != "three" {
		t.Errorf("current = %+v, want shell three", got.Current)
	}
	if len(got.Others) != 1 || got.Others[0].Label != "two" {
		t.Errorf("others = %+v, want [two]", got.Others)
	}

	got = chatOf(t, prompt, use("a", "one"), result("a"))
	if got.Current != nil || got.Others != nil {
		t.Errorf("current/others = %+v/%+v, want neither", got.Current, got.Others)
	}
}

func TestPoll_UnchangedTranscriptPublishesNothing(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	writeLines(t, f.path, "{}")
	c := claudeChat(f.src, f.config)
	// The interval keeps the 16 buffered notes from filling before the test closes the poll.
	c.cfg.ChatPollInterval = 20 * time.Millisecond
	p := c.NewPoll("dir", "s1")
	defer p.Close()
	p.SetActive(true)
	for range 5 { // each tick that has finished is one more look at the file
		<-f.src.noted
	}
	select {
	case ev := <-f.src.published:
		t.Fatalf("published %v for an unchanged transcript", ev)
	default:
	}
}

var taskT0 = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func taskRec(kind, label string, start time.Duration) transcript.TaskRecord {
	return transcript.TaskRecord{Kind: kind, Label: label, StartedAt: taskT0.Add(start)}
}

func (r taskRecEnd) end(at time.Duration, outcome string) transcript.TaskRecord {
	r.rec.End = &transcript.TaskEnd{At: taskT0.Add(at), Outcome: outcome, Source: transcript.TaskEndNotification}
	return r.rec
}

type taskRecEnd struct{ rec transcript.TaskRecord }

func taskLabels(rows []wire.SessionTask) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Label)
	}
	return out
}

func TestAssembleTasks_RulesInOrder(t *testing.T) {
	t.Parallel()
	meta := Meta{RunStartedAt: taskT0.Add(-time.Hour)}
	now := taskT0.Add(10 * time.Minute)
	run := taskRec(transcript.TaskKindShell, "run", 0)

	endedOK := taskRecEnd{taskRec(transcript.TaskKindShell, "ok", time.Minute)}.end(2*time.Minute, "success")
	stopped := taskRecEnd{taskRec(transcript.TaskKindShell, "stopped", time.Minute)}.end(3*time.Minute, "stopped")

	callTurnEnd := taskRec(transcript.TaskKindCall, "call", time.Minute)
	callTurnEnd.TurnEnd = taskT0.Add(4 * time.Minute)

	trailer := taskRec(transcript.TaskKindShell, "trailer", time.Minute)
	trailer.Trailer = &transcript.TaskEnd{At: taskT0.Add(5 * time.Minute), Outcome: "failure"}

	monitor := taskRec(transcript.TaskKindMonitor, "mon", time.Minute)
	monitor.Timeout = 3 * time.Minute

	stoppedMonitor := taskRecEnd{monitor}.end(2*time.Minute, "stopped")
	stoppedMonitor.Label = "mon-stopped"

	got := AssembleTasks([]transcript.TaskRecord{run, endedOK, stopped, callTurnEnd, trailer, monitor, stoppedMonitor}, meta, now)
	want := map[string]wire.SessionTask{
		"run":         {Kind: "shell"},
		"ok":          {Kind: "shell", Finished: true, Outcome: "success", FinishedAt: taskT0.Add(2 * time.Minute).Format(time.RFC3339)},
		"stopped":     {Kind: "shell", Finished: true, Outcome: "killed", FinishedAt: taskT0.Add(3 * time.Minute).Format(time.RFC3339)},
		"call":        {Kind: "call", Finished: true, FinishedAt: taskT0.Add(4 * time.Minute).Format(time.RFC3339)},
		"trailer":     {Kind: "shell", Finished: true, Outcome: "failure", FinishedAt: taskT0.Add(5 * time.Minute).Format(time.RFC3339)},
		"mon":         {Kind: "monitor", Finished: true, FinishedAt: taskT0.Add(4 * time.Minute).Format(time.RFC3339)},
		"mon-stopped": {Kind: "monitor", Finished: true, Outcome: "killed", FinishedAt: taskT0.Add(2 * time.Minute).Format(time.RFC3339)},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v", taskLabels(got))
	}
	for _, g := range got {
		w := want[g.Label]
		w.Label, w.StartedAt = g.Label, g.StartedAt
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %+v, want %+v", g.Label, g, w)
		}
	}
}

func TestAssembleTasks_NoDurationRules(t *testing.T) {
	t.Parallel()
	now := taskT0.Add(3 * time.Hour)
	// Grouping end of a no-duration row: start+cap for age-out, RunStartedAt for
	// an earlier run, and an exited session closes its group. So only an aged-out
	// row can share a group with a running task; the others leave no group.
	old := taskRec(transcript.TaskKindShell, "old", 0)
	for _, tc := range []struct {
		name string
		meta Meta
	}{
		{"exited", Meta{Status: wire.SessionStatus{State: wire.SessionExited}}},
		{"earlier run", Meta{RunStartedAt: taskT0.Add(time.Hour)}},
	} {
		if got := AssembleTasks([]transcript.TaskRecord{old}, tc.meta, now); got != nil {
			t.Errorf("%s: want a closed group, got %v", tc.name, taskLabels(got))
		}
	}
	run := taskRec(transcript.TaskKindShell, "run", time.Hour+30*time.Minute)
	got := AssembleTasks([]transcript.TaskRecord{old, run}, Meta{}, now)
	if len(got) != 2 || !got[0].Finished || !got[0].NoDuration || got[0].FinishedAt != "" || got[0].Outcome != "" || got[1].Finished {
		t.Errorf("aged out: rows = %+v", got)
	}
}

func TestAssembleTasks_ForegroundCallWithoutTurnEndRuns(t *testing.T) {
	t.Parallel()
	call := taskRec(transcript.TaskKindCall, "call", 0)
	got := AssembleTasks([]transcript.TaskRecord{call}, Meta{RunStartedAt: taskT0.Add(-time.Hour)}, taskT0.Add(time.Minute))
	if len(got) != 1 || got[0].Finished {
		t.Fatalf("rows = %+v", got)
	}
	// An earlier run closes it with no duration, like a background task.
	got = AssembleTasks([]transcript.TaskRecord{call}, Meta{RunStartedAt: taskT0.Add(time.Minute)}, taskT0.Add(2*time.Minute))
	if got != nil {
		t.Fatalf("rows = %+v", got)
	}
}

// The group of a no-duration row ends at start+cap for age-out and at
// RunStartedAt for an earlier run: a task starting after that opens a new group
// and the older rows drop out.
func TestAssembleTasks_OnlyTheOpenGroup(t *testing.T) {
	t.Parallel()
	meta := Meta{RunStartedAt: taskT0.Add(-time.Hour)}
	a := taskRecEnd{taskRec(transcript.TaskKindShell, "a", 0)}.end(time.Minute, "success")
	b := taskRec(transcript.TaskKindShell, "b", 2*time.Minute)
	c := taskRec(transcript.TaskKindShell, "c", 3*time.Minute)
	now := taskT0.Add(5 * time.Minute)

	if got := taskLabels(AssembleTasks([]transcript.TaskRecord{c, a, b}, meta, now)); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("open group = %v", got)
	}
	done := taskRecEnd{b}.end(3*time.Minute, "success")
	cDone := taskRecEnd{c}.end(4*time.Minute, "failure")
	if got := AssembleTasks([]transcript.TaskRecord{a, done, cDone}, meta, now); got != nil {
		t.Errorf("last task finished: rows = %v", taskLabels(got))
	}
	// A finished task of the open group stays listed while another runs.
	overlap := taskRecEnd{b}.end(4*time.Minute, "success")
	if got := taskLabels(AssembleTasks([]transcript.TaskRecord{a, overlap, c}, meta, now)); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("finished sibling = %v", got)
	}
	// A later task starting after the group ended opens a new one.
	d := taskRec(transcript.TaskKindMonitor, "d", 4*time.Minute+30*time.Second)
	if got := taskLabels(AssembleTasks([]transcript.TaskRecord{a, done, cDone, d}, meta, now)); !reflect.DeepEqual(got, []string{"d"}) {
		t.Errorf("new group = %v", got)
	}
}

func TestAssembleTasks_RunningSubagentHoldsTheGroupOpen(t *testing.T) {
	t.Parallel()
	meta := Meta{RunStartedAt: taskT0.Add(-time.Hour)}
	shell := taskRecEnd{taskRec(transcript.TaskKindShell, "sh", 0)}.end(time.Minute, "success")
	agent := taskRec(transcript.TaskKindSubagent, "agent", time.Second)
	got := AssembleTasks([]transcript.TaskRecord{shell, agent}, meta, taskT0.Add(5*time.Minute))
	if !reflect.DeepEqual(taskLabels(got), []string{"sh", "agent"}) {
		t.Errorf("rows = %v", taskLabels(got))
	}
}

// A read after a Monitor's timeout changes the row with no re-parse: the cache
// serves the same records and only the clock moves.
func TestChatRead_TasksFollowTheClockWithoutReparse(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	f.src.meta.RunStartedAt = taskT0.Add(-time.Hour)
	writeLines(t, f.path, `{"type":"user","message":{"role":"user","content":"hi"}}`)
	var nowNanos atomic.Int64
	nowNanos.Store(taskT0.Add(time.Minute).UnixNano())
	c := claudeChat(f.src, f.config)
	c.now = func() time.Time { return time.Unix(0, nowNanos.Load()).UTC() }
	f.srv = serveChat(t, f.src, c)

	loads := 0
	mon := taskRec(transcript.TaskKindMonitor, "mon", 0)
	mon.Timeout = 5 * time.Minute
	_, err := c.chats.Read(f.path, harness.Claude, false, func(string) (Snapshot, error) {
		loads++
		return Snapshot{Records: []transcript.TaskRecord{mon}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := f.chat(t, "/v1/sessions/chat").Tasks; len(got) != 1 || got[0].Finished {
		t.Fatalf("before the timeout: %+v", got)
	}
	nowNanos.Store(taskT0.Add(6 * time.Minute).UnixNano())
	// The timeout closes the only task, so the group is gone.
	if got := f.chat(t, "/v1/sessions/chat").Tasks; got != nil {
		t.Fatalf("after the timeout: %+v", got)
	}
	if loads != 1 {
		t.Fatalf("parsed %d times", loads)
	}
}

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
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

const (
	testCwd  = "/work/proj"
	testConv = "conv"
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
	src.meta = Meta{ID: "s1", Harness: "claude", Dir: testCwd, ConversationID: testConv}
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

func TestRead_RoundParsesCardsAndStripsFences(t *testing.T) {
	t.Parallel()
	got := chatOf(t, assistantText("a1", goodRound))
	if len(got.Round) != 2 {
		t.Fatalf("round = %+v, want 2 cards", got.Round)
	}
	c1, c2 := got.Round[0], got.Round[1]
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

func TestRead_RoundDefaultsAreNotCards(t *testing.T) {
	t.Parallel()
	defaults := "\n\n**Decided unless you object**\n\n- Naming: use camelCase\n- Order: alphabetical"
	for name, sep := range map[string]string{"with rule": "\n\n---", "without rule": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			round := "❓ **Q1** - **Which store?**: SQLite or files?\n\n➡️ SQLite\n\n```gx-question\ntype: yes-no\nrecommended: yes\n```" + sep + defaults
			got := chatOf(t, assistantText("a1", round))
			if len(got.Round) != 1 {
				t.Fatalf("round = %+v, want 1 card", got.Round)
			}
			if body := got.Round[0].Body; strings.Contains(body, "camelCase") || strings.Contains(body, "Decided") {
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
	for _, c := range got.Round {
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
	if len(got.Round) != 4 {
		t.Fatalf("round = %+v, want 4 cards", got.Round)
	}
	for _, c := range got.Round[:3] {
		if c.Kind != "text" || c.Note == "" || len(c.Options) != 0 || c.Recommended != "" {
			t.Errorf("card %s = %+v, want a free-text card with a note", c.Label, c)
		}
	}
	if c := got.Round[3]; c.Kind != "yes-no" || c.Recommended != "yes" || c.Note != "" {
		t.Errorf("card 4 = %+v, want it answerable", c)
	}
}

func TestRead_CommandOutputIsNotAnAgentReply(t *testing.T) {
	t.Parallel()
	out := personPrompt("o1", "<local-command-stdout>Total cost: $1.20</local-command-stdout>")
	got := chatOf(t, assistantText("a1", goodRound), out)
	if len(got.Round) != 2 {
		t.Errorf("round = %+v, want the open round kept past the output entry", got.Round)
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
	if got.Round != nil {
		t.Errorf("round = %+v, want null after a person prompt", got.Round)
	}
	got = chatOf(t, assistantText("a1", goodRound),
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:02Z","message":{"content":[{"type":"tool_use","id":"t","name":"Bash"}]}}`)
	if len(got.Round) != 2 {
		t.Errorf("round = %+v, want it kept when a tool call follows", got.Round)
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
	if len(got.Round) != 2 || got.Next != nil {
		t.Errorf("round = %d cards, next = %+v, want the round and no Continue", len(got.Round), got.Next)
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

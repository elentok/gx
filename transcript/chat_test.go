package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadChat_ShowsVisibleAndHidesTheRest(t *testing.T) {
	lines := []string{
		`{"type":"user","uuid":"u0","timestamp":"2026-01-01T10:00:00Z","message":{"content":"<command-name>/clear</command-name>\n<command-message>clear</command-message>\n<command-args></command-args>"}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-01-01T10:00:01Z","isMeta":true,"message":{"content":"<local-command-caveat>x</local-command-caveat>"}}`,
		`{"type":"user","uuid":"u2","timestamp":"2026-01-01T10:00:02Z","message":{"content":"<local-command-stdout></local-command-stdout>"}}`,
		`{"type":"user","uuid":"u3","timestamp":"2026-01-01T10:00:03Z","message":{"content":"<command-name>/gx-design</command-name><command-args>proj:map</command-args>"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:04Z","message":{"content":[{"type":"thinking","thinking":"hmm"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:05Z","message":{"content":[{"type":"text","text":"Hello"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-01-01T10:00:06Z","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash"}]}}`,
		`{"type":"user","uuid":"u4","timestamp":"2026-01-01T10:00:07Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"out"}]}}`,
		`{"type":"assistant","uuid":"s1","isSidechain":true,"timestamp":"2026-01-01T10:00:08Z","message":{"content":[{"type":"text","text":"sub"}]}}`,
		`{"type":"user","uuid":"c1","isCompactSummary":true,"timestamp":"2026-01-01T10:00:09Z","message":{"content":"summary"}}`,
		`{"type":"system","uuid":"y1","timestamp":"2026-01-01T10:00:10Z","content":"note"}`,
		`{"type":"attachment","uuid":"x1","timestamp":"2026-01-01T10:00:11Z"}`,
		`not json`,
		`{"type":"user","uuid":"n1","timestamp":"2026-01-01T10:00:11Z","origin":{"kind":"task-notification"},"message":{"content":"<task-notification>\n<task-id>a1</task-id>\n</task-notification>"}}`,
		`{"type":"user","uuid":"n2","timestamp":"2026-01-01T10:00:11Z","message":{"content":"<task-notification>\n<task-id>a2</task-id>\n</task-notification>"}}`,
		`{"type":"user","uuid":"u5","timestamp":"2026-01-01T10:00:12Z","message":{"content":"Which store?"}}`,
	}
	path := filepath.Join(t.TempDir(), "c.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msgs, ok, err := ReadChat(path)
	if err != nil || !ok {
		t.Fatalf("ReadChat = %v, %v", ok, err)
	}
	type view struct{ ID, Role, Kind, Text string }
	var got []view
	for _, m := range msgs {
		got = append(got, view{m.ID, m.Role, m.Kind, m.Text})
	}
	want := []view{
		{"u0", ChatRolePerson, ChatKindCommand, "/clear"},
		{"u3", ChatRolePerson, ChatKindCommand, "/gx-design proj:map"},
		{"a2", ChatRoleAgent, ChatKindText, "Hello"},
		{"a3", ChatRoleAgent, ChatKindTools, ""},
		{"u5", ChatRolePerson, ChatKindPrompt, "Which store?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v, want %+v", got, want)
	}
	if msgs[0].At.IsZero() {
		t.Error("At not parsed")
	}
}

func TestReadChat_CommandOutput(t *testing.T) {
	line := func(id, content string) string {
		b, _ := json.Marshal(map[string]any{"type": "user", "uuid": id, "timestamp": "2026-01-01T10:00:00Z", "message": map[string]any{"content": content}})
		return string(b)
	}
	lines := []string{
		line("c1", "<command-name>/cost</command-name><command-args></command-args>"),
		line("o0", "<local-command-stdout></local-command-stdout>"),
		line("o1", "<local-command-stdout>Total cost: $1.20\n\x1b[1mTotal duration:\x1b[0m 5m\nUsage: 3 models</local-command-stdout>"),
		line("c2", "<command-name>/compact</command-name><command-args></command-args>"),
		line("o2", "<local-command-stdout>Compacted </local-command-stdout>"),
		line("c3", "<command-name>/big</command-name><command-args></command-args>"),
		line("o3", "<local-command-stdout>"+strings.Repeat("x", MaxCommandOutputBytes+100)+"</local-command-stdout>"),
	}
	msgs, err := ReadChatFrom(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	var ids, kinds []string
	for _, m := range msgs {
		ids, kinds = append(ids, m.ID), append(kinds, m.Kind)
	}
	if want := []string{"c1", "o1", "c2", "c3", "o3"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if msgs[1].Kind != ChatKindOutput || msgs[1].Role != ChatRoleAgent {
		t.Errorf("o1 = %s/%s", msgs[1].Role, msgs[1].Kind)
	}
	if want := "Total cost: $1.20\nTotal duration: 5m\nUsage: 3 models"; msgs[1].Text != want {
		t.Errorf("text = %q, want %q", msgs[1].Text, want)
	}
	if big := msgs[4].Text; !strings.HasSuffix(big, "\n"+commandOutputCutMarker) || len(big) > MaxCommandOutputBytes+len(commandOutputCutMarker)+1 {
		t.Errorf("oversized output not cut with marker: len=%d", len(big))
	}
}

func TestReadChat_MissingFile(t *testing.T) {
	_, ok, err := ReadChat(filepath.Join(t.TempDir(), "none.jsonl"))
	if ok || err != nil {
		t.Fatalf("ReadChat = %v, %v", ok, err)
	}
}

func TestReadChat_ToolRunKeepsOneLinePerCall(t *testing.T) {
	lines := []string{
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}},{"type":"tool_use","name":"Read","input":{"file_path":"/x/a.go"}},{"type":"tool_use","name":"TodoWrite","input":{}}]}}`,
	}
	msgs, err := ReadChatFrom(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("ReadChatFrom = %+v, %v", msgs, err)
	}
	want := []string{"Bash: ls -la", "Read: /x/a.go", "TodoWrite"}
	if msgs[0].ToolCount != 3 || !reflect.DeepEqual(msgs[0].Calls, want) {
		t.Errorf("tools entry = %+v, want 3 calls %v", msgs[0], want)
	}
}

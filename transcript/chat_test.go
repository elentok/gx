package transcript

import (
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
		{"u5", ChatRolePerson, ChatKindPrompt, "Which store?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v, want %+v", got, want)
	}
	if msgs[0].At.IsZero() {
		t.Error("At not parsed")
	}
}

func TestReadChat_MissingFile(t *testing.T) {
	_, ok, err := ReadChat(filepath.Join(t.TempDir(), "none.jsonl"))
	if ok || err != nil {
		t.Fatalf("ReadChat = %v, %v", ok, err)
	}
}

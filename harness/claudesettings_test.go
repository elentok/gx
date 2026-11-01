package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func settingsValues(args []string) []string {
	var out []string
	for i, a := range args {
		if a == "--settings" && i+1 < len(args) {
			out = append(out, args[i+1])
		}
	}
	return out
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad settings JSON %q: %v", s, err)
	}
	return m
}

func TestMergeClaudeSettings_TwoFlagsBecomeOne(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(file, []byte(`{"theme":"dark","hooks":{"PreToolUse":[{"matcher":"Edit"}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--model", "x",
		"--settings", `{"tui":"default","statusLine":{"type":"command","command":"sl"},"hooks":{"PreToolUse":[{"matcher":"Bash"}]},"editorMode":"vim"}`,
		"--settings", file,
	}
	got := MergeClaudeSettings(args)
	vals := settingsValues(got)
	if len(vals) != 1 {
		t.Fatalf("want one --settings, got %q", got)
	}
	m := decode(t, vals[0])
	if m["editorMode"] != "normal" || m["tui"] != "default" || m["theme"] != "dark" {
		t.Errorf("merged = %v", m)
	}
	if m["statusLine"].(map[string]any)["command"] != "sl" {
		t.Errorf("statusLine lost: %v", m)
	}
	if n := len(m["hooks"].(map[string]any)["PreToolUse"].([]any)); n != 2 {
		t.Errorf("hooks = %v, want both matchers", m["hooks"])
	}
	if got[0] != "--model" || got[1] != "x" {
		t.Errorf("other args moved: %q", got)
	}
}

func TestMergeClaudeSettings_NoFlagAppends(t *testing.T) {
	got := MergeClaudeSettings([]string{"--model", "x"})
	vals := settingsValues(got)
	if len(vals) != 1 || decode(t, vals[0])["editorMode"] != "normal" {
		t.Errorf("got %q", got)
	}
}

func TestMergeClaudeSettings_Idempotent(t *testing.T) {
	once := MergeClaudeSettings([]string{"--settings", `{"hooks":{"PreToolUse":[{"matcher":"Bash"}]}}`})
	twice := MergeClaudeSettings(once)
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("not idempotent:\n%q\n%q", once, twice)
	}
}

func TestMergeClaudeSettings_EqualsForm(t *testing.T) {
	got := MergeClaudeSettings([]string{`--settings={"a":1}`})
	vals := settingsValues(got)
	if len(vals) != 1 || len(got) != 2 || decode(t, vals[0])["a"] != float64(1) {
		t.Errorf("got %q", got)
	}
}

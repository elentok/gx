package codexsession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	injectedLine  = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /repo"},{"type":"input_text","text":"<environment_context>x</environment_context>"}]}}`
	developerLine = `{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system"}]}}`
)

func userLine(text string) string {
	return `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}`
}

func metaLineJSON(id, cwd, started, source string) string {
	return `{"timestamp":"` + started + `","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `","timestamp":"` + started + `","source":` + source + `}}`
}

func writeLaunchRollout(t *testing.T, home, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "11", "08", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFirstPrompt_SkipsInjectedAndPairsSkillRecord(t *testing.T) {
	home := t.TempDir()
	with := writeLaunchRollout(t, home, "rollout-a.jsonl",
		metaLineJSON("a", "/repo", "2026-11-08T10:00:00Z", `"cli"`),
		developerLine, injectedLine, userLine("$gx-bump go"), userLine("<skill><name>gx-bump</name></skill>"))
	without := writeLaunchRollout(t, home, "rollout-b.jsonl",
		metaLineJSON("b", "/repo", "2026-11-08T10:00:00Z", `"cli"`), developerLine, injectedLine)

	got, ok, err := FirstPrompt(with, 0)
	if err != nil || !ok || got.Text != "$gx-bump go" || got.Skill != "<skill><name>gx-bump</name></skill>" {
		t.Errorf("FirstPrompt(with) = %+v, %t, %v", got, ok, err)
	}
	if _, ok, err := FirstPrompt(without, 0); err != nil || ok {
		t.Errorf("FirstPrompt(without) ok = %t, err = %v; want not delivered", ok, err)
	}
	if _, ok, err := FirstPrompt(filepath.Join(home, "missing.jsonl"), 0); err != nil || ok {
		t.Errorf("FirstPrompt(missing) ok = %t, err = %v; want not delivered", ok, err)
	}
}

func TestLocateRollout_SkipsGuardianAndSubagentRollouts(t *testing.T) {
	home := t.TempDir()
	since := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	writeLaunchRollout(t, home, "rollout-guardian.jsonl", metaLineJSON("g", "/repo", "2026-11-08T10:05:00Z", `{"guardian":{}}`))
	writeLaunchRollout(t, home, "rollout-sub.jsonl", metaLineJSON("s", "/repo", "2026-11-08T10:06:00Z", `{"subagent":{"thread_spawn":{}}}`))
	writeLaunchRollout(t, home, "rollout-old.jsonl", metaLineJSON("o", "/repo", "2026-11-08T09:00:00Z", `"cli"`))
	writeLaunchRollout(t, home, "rollout-other.jsonl", metaLineJSON("x", "/elsewhere", "2026-11-08T10:07:00Z", `"cli"`))
	want := writeLaunchRollout(t, home, "rollout-real.jsonl", metaLineJSON("r", "/repo", "2026-11-08T10:01:00Z", `"cli"`))

	got, ok, err := LocateRollout(home, "/repo", since)
	if err != nil || !ok || got != want {
		t.Errorf("LocateRollout() = %q, %t, %v; want %q", got, ok, err, want)
	}
	if _, ok, err := LocateRollout(t.TempDir(), "/repo", since); err != nil || ok {
		t.Errorf("LocateRollout(empty home) ok = %t, err = %v", ok, err)
	}
}

func TestFirstPrompt_ResumeOffsetIgnoresOlderUserLines(t *testing.T) {
	home := t.TempDir()
	path := writeLaunchRollout(t, home, "rollout-r.jsonl",
		metaLineJSON("r", "/repo", "2026-11-08T10:00:00Z", `"cli"`), injectedLine, userLine("old prompt"))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	offset := info.Size()

	if _, ok, err := FirstPrompt(path, offset); err != nil || ok {
		t.Fatalf("before the resume prompt: ok = %t, err = %v; want not delivered", ok, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(userLine("new prompt") + "\n"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := FirstPrompt(path, offset)
	if err != nil || !ok || got.Text != "new prompt" {
		t.Errorf("after the resume prompt = %+v, %t, %v; want new prompt", got, ok, err)
	}
}

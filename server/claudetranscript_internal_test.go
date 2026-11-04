package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/transcript"
)

// A copy of the transcript under another project dir makes a glob ambiguous
// and so a miss: finding the expected one proves the glob was never run.
func TestClaudeTranscript_StatsTheExpectedSlugDirBeforeGlobbing(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	other := filepath.Join(config, "projects", "-elsewhere", "conv.jsonl")
	for _, p := range []string{want, other} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := withProjects(&Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}}})
	got, ok := s.claudeTranscript("s1", sessionMeta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("claudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

func TestClaudeTranscript_ReadsTheConfigDirFromTheServerEnv(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := withProjects(&Server{log: slog.New(slog.DiscardHandler), cfg: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}}})
	got, ok := s.claudeTranscript("s1", sessionMeta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("claudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

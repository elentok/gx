package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeClaude writes an executable claude into a new dir and returns an env whose
// PATH is only that dir.
func fakeClaude(t *testing.T, script string) []string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return []string{"PATH=" + dir + ":/usr/bin:/bin"}
}

func TestClaudeSupportsPrefill(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   bool
	}{
		{"a build that knows the flag", `echo "error: option '--prefill <text>' argument missing" >&2; exit 1`, true},
		{"a build that does not", `echo "error: unknown option '--prefill'" >&2; exit 1`, false},
		{"a build that starts instead of failing", `exit 0`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClaudeSupportsPrefill(fakeClaude(t, tt.script)); got != tt.want {
				t.Errorf("ClaudeSupportsPrefill = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeSupportsPrefill_NoClaudeOnPathIsNo(t *testing.T) {
	if ClaudeSupportsPrefill([]string{"PATH=" + t.TempDir()}) {
		t.Error("ClaudeSupportsPrefill = true with no claude on PATH")
	}
}

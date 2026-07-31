package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify_ReplacesSlashesAndDots(t *testing.T) {
	got := Slugify("/Users/david/dev/gx/main")
	want := "-Users-david-dev-gx-main"
	if got != want {
		t.Errorf("Slugify() = %q, want %q", got, want)
	}
}

func TestSlugify_DotDirectory(t *testing.T) {
	got := Slugify("/Users/david/.dotfiles")
	want := "-Users-david--dotfiles"
	if got != want {
		t.Errorf("Slugify() = %q, want %q", got, want)
	}
}

func TestPath_JoinsHomeSlugAndSessionID(t *testing.T) {
	t.Setenv("HOME", "/home/fake")
	got, err := Path("/repo/worktree", "session-123")
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	want := filepath.Join("/home/fake", ".claude", "projects", "-repo-worktree", "session-123.jsonl")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLastAssistantUsage_ReturnsOnlyTheLastAssistantLine(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","message":{}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"cache_read_input_tokens":100,"cache_creation_input_tokens":0,"output_tokens":50}}}`,
		`{"type":"assistant","message":{"model":"claude-sonnet-5","usage":{"input_tokens":2,"cache_read_input_tokens":200000,"cache_creation_input_tokens":1000,"output_tokens":60}}}`,
	)

	usage, ok, err := LastAssistantUsage(path)
	if err != nil {
		t.Fatalf("LastAssistantUsage() error = %v", err)
	}
	if !ok {
		t.Fatal("LastAssistantUsage() ok = false, want true")
	}
	if usage.InputTokens != 2 || usage.CacheReadInputTokens != 200000 || usage.CacheCreationInputTokens != 1000 {
		t.Errorf("usage = %+v, want the last assistant line's fields, not summed across lines", usage)
	}
}

func TestLastAssistantUsage_NoAssistantLines_NotOK(t *testing.T) {
	path := writeTranscript(t, `{"type":"user","message":{}}`)

	_, ok, err := LastAssistantUsage(path)
	if err != nil {
		t.Fatalf("LastAssistantUsage() error = %v", err)
	}
	if ok {
		t.Error("LastAssistantUsage() ok = true, want false with no assistant lines")
	}
}

func TestLastAssistantUsage_LastLineFarBeforeInitialTailWindow_StillFound(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"cache_read_input_tokens":42,"cache_creation_input_tokens":0,"output_tokens":1}}}`,
	}
	// Pad well past initialTailBytes so the first (smallest) tail read
	// can't possibly contain the assistant line above, forcing at least
	// one doubling pass.
	padding := strings.Repeat("x", initialTailBytes*3)
	for range 500 {
		lines = append(lines, `{"type":"user","message":{}}`+"// "+padding)
	}
	path := writeTranscript(t, lines...)

	usage, ok, err := LastAssistantUsage(path)
	if err != nil {
		t.Fatalf("LastAssistantUsage() error = %v", err)
	}
	if !ok {
		t.Fatal("LastAssistantUsage() ok = false, want true")
	}
	if usage.CacheReadInputTokens != 42 {
		t.Errorf("usage.CacheReadInputTokens = %d, want 42 (found across the doubling passes)", usage.CacheReadInputTokens)
	}
}

func TestLastAssistantUsage_NoAssistantLineAnywhere_ScansWholeFile(t *testing.T) {
	padding := strings.Repeat("x", initialTailBytes*3)
	lines := make([]string, 0, 500)
	for range 500 {
		lines = append(lines, `{"type":"user","message":{}}`+"// "+padding)
	}
	path := writeTranscript(t, lines...)

	_, ok, err := LastAssistantUsage(path)
	if err != nil {
		t.Fatalf("LastAssistantUsage() error = %v", err)
	}
	if ok {
		t.Error("LastAssistantUsage() ok = true, want false with no assistant line anywhere")
	}
}

func TestLastAssistantUsage_MissingFile_NotOKNoError(t *testing.T) {
	_, ok, err := LastAssistantUsage(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("LastAssistantUsage() error = %v, want nil for a missing file", err)
	}
	if ok {
		t.Error("LastAssistantUsage() ok = true, want false for a missing file")
	}
}

func TestUsage_Occupancy_SumsInputSideFieldsOnly(t *testing.T) {
	u := Usage{InputTokens: 10, CacheReadInputTokens: 20000, CacheCreationInputTokens: 500, OutputTokens: 9999}
	if got, want := u.Occupancy(), 10+20000+500; got != want {
		t.Errorf("Occupancy() = %d, want %d (excluding OutputTokens)", got, want)
	}
}

func TestLastAssistantOccupancy_ReadsThroughPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cwd := "/repo/worktree"
	slugDir := filepath.Join(dir, ".claude", "projects", Slugify(cwd))
	if err := os.MkdirAll(slugDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sessionPath := filepath.Join(slugDir, "sess-1.jsonl")
	if err := os.WriteFile(sessionPath, []byte(`{"type":"assistant","message":{"usage":{"input_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":3}}}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	occ, ok, err := LastAssistantOccupancy(cwd, "sess-1")
	if err != nil {
		t.Fatalf("LastAssistantOccupancy() error = %v", err)
	}
	if !ok {
		t.Fatal("LastAssistantOccupancy() ok = false, want true")
	}
	if occ != 6 {
		t.Errorf("LastAssistantOccupancy() = %d, want 6", occ)
	}
}

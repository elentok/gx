package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestReadAll_ParsesEveryLineInOrder(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z","message":{}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:05.000Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":1,"cache_read_input_tokens":100,"output_tokens":50}}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:10.000Z","message":{"model":"claude-opus-5","usage":{"input_tokens":2,"cache_read_input_tokens":200,"output_tokens":60}}}`,
	)

	lines, ok, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !ok {
		t.Fatal("ReadAll() ok = false, want true")
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0].Type != "user" || lines[1].Type != "assistant" || lines[2].Type != "assistant" {
		t.Errorf("lines types = [%q %q %q], want [user assistant assistant]", lines[0].Type, lines[1].Type, lines[2].Type)
	}
	if lines[2].Usage.Model != "claude-opus-5" || lines[2].Usage.InputTokens != 2 {
		t.Errorf("lines[2].Usage = %+v, want the third line's own usage fields", lines[2].Usage)
	}
	if !lines[2].Timestamp.After(lines[0].Timestamp) {
		t.Errorf("lines[2].Timestamp = %v, want it after lines[0].Timestamp = %v", lines[2].Timestamp, lines[0].Timestamp)
	}
}

func TestReadAll_SkipsMalformedAndUntimestampedLines(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:00.000Z","message":{"usage":{"input_tokens":1}}}`,
		`not json at all`,
		`{"type":"assistant","message":{"usage":{"input_tokens":2}}}`, // no timestamp field
		`{"type":"assistant","timestamp":"2026-01-01T00:00:05.000Z","message":{"usage":{"input_tokens":3}}}`,
	)

	lines, ok, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !ok {
		t.Fatal("ReadAll() ok = false, want true")
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (malformed/untimestamped lines skipped): %+v", len(lines), lines)
	}
	if lines[0].Usage.InputTokens != 1 || lines[1].Usage.InputTokens != 3 {
		t.Errorf("lines = %+v, want input_tokens [1 3]", lines)
	}
}

func TestReadAll_MissingFile_NotOKNoError(t *testing.T) {
	lines, ok, err := ReadAll(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("ReadAll() error = %v, want nil for a missing file", err)
	}
	if ok || lines != nil {
		t.Errorf("ReadAll() = (%v, %v), want (nil, false) for a missing file", lines, ok)
	}
}

func TestUsage_Occupancy_SumsInputSideFieldsOnly(t *testing.T) {
	u := Usage{InputTokens: 10, CacheReadInputTokens: 20000, CacheCreationInputTokens: 500, OutputTokens: 9999}
	if got, want := u.Occupancy(), 10+20000+500; got != want {
		t.Errorf("Occupancy() = %d, want %d (excluding OutputTokens)", got, want)
	}
}

func TestFirstLineTimestamp_ReturnsEarliestParseableLine(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z","message":{}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:05.000Z","message":{}}`,
	)

	ts, ok, err := FirstLineTimestamp(path)
	if err != nil {
		t.Fatalf("FirstLineTimestamp() error = %v", err)
	}
	if !ok {
		t.Fatal("FirstLineTimestamp() ok = false, want true")
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("FirstLineTimestamp() = %v, want %v", ts, want)
	}
}

func TestFirstLineTimestamp_SkipsMalformedAndUntimestampedLeadingLines(t *testing.T) {
	path := writeTranscript(t,
		`not json at all`,
		`{"type":"user","message":{}}`, // no timestamp field
		`{"type":"assistant","timestamp":"2026-01-01T00:00:05.000Z","message":{}}`,
	)

	ts, ok, err := FirstLineTimestamp(path)
	if err != nil {
		t.Fatalf("FirstLineTimestamp() error = %v", err)
	}
	if !ok {
		t.Fatal("FirstLineTimestamp() ok = false, want true")
	}
	want := time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)
	if !ts.Equal(want) {
		t.Errorf("FirstLineTimestamp() = %v, want %v", ts, want)
	}
}

func TestFirstLineTimestamp_MissingFile_NotOKNoError(t *testing.T) {
	ts, ok, err := FirstLineTimestamp(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Fatalf("FirstLineTimestamp() error = %v, want nil for a missing file", err)
	}
	if ok || !ts.IsZero() {
		t.Errorf("FirstLineTimestamp() = (%v, %v), want (zero, false) for a missing file", ts, ok)
	}
}

func TestElapsed_ComputesNowMinusFirstLineTimestamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cwd := "/repo/worktree"
	slugDir := filepath.Join(dir, ".claude", "projects", Slugify(cwd))
	if err := os.MkdirAll(slugDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	start := time.Now().Add(-5 * time.Minute).UTC()
	sessionPath := filepath.Join(slugDir, "sess-1.jsonl")
	line := `{"type":"assistant","timestamp":"` + start.Format(time.RFC3339Nano) + `","message":{}}` + "\n"
	if err := os.WriteFile(sessionPath, []byte(line), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	elapsed, ok, err := Elapsed(cwd, "sess-1")
	if err != nil {
		t.Fatalf("Elapsed() error = %v", err)
	}
	if !ok {
		t.Fatal("Elapsed() ok = false, want true")
	}
	if elapsed < 5*time.Minute || elapsed > 6*time.Minute {
		t.Errorf("Elapsed() = %v, want approximately 5m", elapsed)
	}
}

func TestElapsed_EmptyCwdOrSessionID_NotOKNoError(t *testing.T) {
	elapsed, ok, err := Elapsed("", "sess-1")
	if err != nil || ok || elapsed != 0 {
		t.Errorf("Elapsed(\"\", ...) = (%v, %v, %v), want (0, false, nil)", elapsed, ok, err)
	}
	elapsed, ok, err = Elapsed("/repo/worktree", "")
	if err != nil || ok || elapsed != 0 {
		t.Errorf("Elapsed(..., \"\") = (%v, %v, %v), want (0, false, nil)", elapsed, ok, err)
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

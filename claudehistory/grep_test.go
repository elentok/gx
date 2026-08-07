package claudehistory

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// ---- GrepDecodeMatch tests ----

const grepFixtureUser = `{"type":"user","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"user","content":"How do I implement a fibonacci sequence?"},"timestamp":"2026-01-01T10:00:00Z"}`

const grepFixtureAssistant = `{"type":"assistant","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"assistant","content":[{"type":"text","text":"You can implement fibonacci using recursion or iteration."}]},"timestamp":"2026-01-01T10:00:01Z"}`

const grepFixtureAITitle = `{"type":"ai-title","aiTitle":"Fibonacci implementation","sessionId":"s1"}`

const grepFixtureLastPrompt = `{"type":"last-prompt","lastPrompt":"fibonacci help","sessionId":"s1"}`

const grepFixtureSystem = `{"type":"system","subtype":"turn_duration","durationMs":100}`

const grepFixtureMode = `{"type":"mode","mode":"normal","sessionId":"s1"}`

func TestGrepDecodeMatchUser(t *testing.T) {
	snippet, hl, preview := GrepDecodeMatch([]byte(grepFixtureUser), "fibonacci")
	if !strings.Contains(snippet, "fibonacci") {
		t.Errorf("expected snippet to contain 'fibonacci', got %q", snippet)
	}
	if preview == "" {
		t.Error("expected non-empty preview")
	}
	if len(hl) == 0 {
		t.Error("expected highlight ranges for 'fibonacci'")
	}
}

func TestGrepDecodeMatchAssistant(t *testing.T) {
	snippet, hl, preview := GrepDecodeMatch([]byte(grepFixtureAssistant), "fibonacci")
	if !strings.Contains(snippet, "fibonacci") {
		t.Errorf("expected snippet to contain 'fibonacci', got %q", snippet)
	}
	if preview == "" {
		t.Error("expected non-empty preview")
	}
	if len(hl) == 0 {
		t.Error("expected highlight ranges")
	}
}

func TestGrepDecodeMatchAITitle(t *testing.T) {
	snippet, hl, preview := GrepDecodeMatch([]byte(grepFixtureAITitle), "fibonacci")
	if snippet != "Fibonacci implementation" {
		t.Errorf("expected 'Fibonacci implementation', got %q", snippet)
	}
	if preview != "Fibonacci implementation" {
		t.Errorf("expected preview 'Fibonacci implementation', got %q", preview)
	}
	if len(hl) == 0 {
		t.Error("expected highlight ranges for case-insensitive match")
	}
}

func TestGrepDecodeMatchLastPrompt(t *testing.T) {
	snippet, _, preview := GrepDecodeMatch([]byte(grepFixtureLastPrompt), "fibonacci")
	if !strings.Contains(snippet, "fibonacci") {
		t.Errorf("expected snippet to contain 'fibonacci', got %q", snippet)
	}
	if preview == "" {
		t.Error("expected non-empty preview")
	}
}

func TestGrepDecodeMatchSystemIgnored(t *testing.T) {
	snippet, hl, preview := GrepDecodeMatch([]byte(grepFixtureSystem), "turn")
	if snippet != "" || preview != "" || hl != nil {
		t.Errorf("system record should produce empty decode, got snippet=%q", snippet)
	}
}

func TestGrepDecodeMatchModeIgnored(t *testing.T) {
	snippet, _, _ := GrepDecodeMatch([]byte(grepFixtureMode), "normal")
	if snippet != "" {
		t.Errorf("mode record should produce empty decode, got %q", snippet)
	}
}

func TestGrepDecodeMatchHighlightRanges(t *testing.T) {
	// Verify highlight ranges point at the query text within the snippet.
	snippet, hl, _ := GrepDecodeMatch([]byte(grepFixtureUser), "fibonacci")
	if len(hl) == 0 {
		t.Fatal("expected highlight ranges")
	}
	runes := []rune(snippet)
	start := hl[0]
	end := start + len(hl)
	if end > len(runes) {
		t.Fatalf("highlight range [%d,%d) out of bounds (snippet len=%d)", start, end, len(runes))
	}
	highlighted := strings.ToLower(string(runes[start:end]))
	if highlighted != "fibonacci" {
		t.Errorf("highlight runes spell %q, want 'fibonacci'", highlighted)
	}
}

func TestGrepDecodeMatchLongTextCentered(t *testing.T) {
	// Build a user message where the query is far into the text.
	prefix := strings.Repeat("a ", 60) // 120 chars before the keyword
	longContent := prefix + "fibonacci is cool"
	line := `{"type":"user","isMeta":false,"isSidechain":false,"message":{"role":"user","content":"` +
		longContent + `"},"timestamp":"2026-01-01T10:00:00Z"}`

	snippet, hl, _ := GrepDecodeMatch([]byte(line), "fibonacci")
	if !strings.Contains(snippet, "fibonacci") {
		t.Errorf("snippet should contain 'fibonacci', got %q", snippet)
	}
	if len(hl) == 0 {
		t.Error("expected highlight ranges")
	}
	// Snippet should not be longer than ~130 runes (max + prefix ellipsis).
	if len([]rune(snippet)) > 135 {
		t.Errorf("snippet too long: %d runes", len([]rune(snippet)))
	}
}

func TestGrepDecodeMatchEscapedJSON(t *testing.T) {
	// JSON-encoded content with escaped quotes — decoded text should be legible.
	line := `{"type":"user","isMeta":false,"isSidechain":false,"message":{"role":"user","content":"What does \"fibonacci\" mean?"},"timestamp":"2026-01-01T10:00:00Z"}`
	snippet, _, _ := GrepDecodeMatch([]byte(line), "fibonacci")
	if strings.Contains(snippet, `\"`) {
		t.Errorf("decoded snippet should not contain raw JSON escapes, got %q", snippet)
	}
	if !strings.Contains(snippet, `"fibonacci"`) {
		t.Errorf("expected decoded quotes in snippet, got %q", snippet)
	}
}

func TestGrepDecodeMatchEmpty(t *testing.T) {
	snippet, hl, preview := GrepDecodeMatch(nil, "query")
	if snippet != "" || hl != nil || preview != "" {
		t.Error("empty input should produce empty result")
	}
}

func TestGrepDecodeMatchInvalidJSON(t *testing.T) {
	snippet, _, _ := GrepDecodeMatch([]byte("not json"), "query")
	if snippet != "" {
		t.Errorf("invalid JSON should produce empty snippet, got %q", snippet)
	}
}

// ---- decodeMessageText tests ----

func TestDecodeMessageTextStringContent(t *testing.T) {
	raw := json.RawMessage(`{"role":"user","content":"  hello world  "}`)
	got := decodeMessageText(raw, strings.TrimSpace)
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestDecodeMessageTextBlockContent(t *testing.T) {
	raw := json.RawMessage(`{"role":"assistant","content":[{"type":"tool_use","text":""},{"type":"text","text":"  block text  "}]}`)
	got := decodeMessageText(raw, strings.TrimSpace)
	if got != "block text" {
		t.Errorf("got %q, want %q", got, "block text")
	}
}

func TestDecodeMessageTextPostProcessApplied(t *testing.T) {
	raw := json.RawMessage(`{"role":"user","content":"<command-message>run</command-message>actual text"}`)
	got := decodeMessageText(raw, cleanUserText)
	if got != "actual text" {
		t.Errorf("got %q, want %q", got, "actual text")
	}
}

func TestDecodeMessageTextNoPostProcess(t *testing.T) {
	raw := json.RawMessage(`{"role":"assistant","content":"<command-message>run</command-message>actual text"}`)
	got := decodeMessageText(raw, strings.TrimSpace)
	if got != "<command-message>run</command-message>actual text" {
		t.Errorf("expected cleanup hook not applied, got %q", got)
	}
}

// ---- GrepTranscripts tests ----

// grepFixtureDir creates a temp project dir with two .jsonl files.
// Returns the dir path.
func grepFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	older := `{"type":"mode","sessionId":"old"}
{"type":"user","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"user","content":"What is a fibonacci sequence?"},"timestamp":"2026-01-01T08:00:00Z"}
{"type":"ai-title","aiTitle":"Fibonacci question","sessionId":"old"}
`
	newer := `{"type":"mode","sessionId":"new"}
{"type":"user","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"user","content":"How do I sort a fibonacci list?"},"timestamp":"2026-01-02T08:00:00Z"}
{"type":"ai-title","aiTitle":"Sort fibonacci","sessionId":"new"}
`
	if err := os.WriteFile(dir+"/old.jsonl", []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/new.jsonl", []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func requireRg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not available")
	}
}

func TestGrepTranscriptsSingleDir(t *testing.T) {
	requireRg(t)
	dir := grepFixtureDir(t)
	results, err := GrepTranscripts("fibonacci", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'fibonacci'")
	}
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.Snippet), "fibonacci") {
			t.Errorf("snippet should contain 'fibonacci': %q", r.Snippet)
		}
	}
}

func TestGrepTranscriptsNewestFirst(t *testing.T) {
	requireRg(t)
	dir := grepFixtureDir(t)
	results, err := GrepTranscripts("fibonacci", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Skip("need at least 2 results to test ordering")
	}
	// new.jsonl has timestamp 2026-01-02, old.jsonl has 2026-01-01.
	// Results should come from new.jsonl first.
	if !strings.Contains(results[0].FilePath, "new.jsonl") {
		t.Errorf("expected new.jsonl first, got %q", results[0].FilePath)
	}
}

func TestGrepTranscriptsNoMatches(t *testing.T) {
	requireRg(t)
	dir := grepFixtureDir(t)
	results, err := GrepTranscripts("xyznotfound999", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for no matches, got %d", len(results))
	}
}

// TestGrepTranscriptsRgAbsent forces exec.LookPath("rg") to fail by clearing
// PATH, so this exercises the ErrRgNotFound path deterministically regardless
// of whether rg is actually installed on the machine running the test.
func TestGrepTranscriptsRgAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := grepFixtureDir(t)
	_, err := GrepTranscripts("fibonacci", []string{dir})
	if !errors.Is(err, ErrRgNotFound) {
		t.Fatalf("expected ErrRgNotFound, got: %v", err)
	}
}

func TestGrepTranscriptsSurfacesRgStderr(t *testing.T) {
	requireRg(t)
	dir := grepFixtureDir(t)

	orig := runRg
	runRg = func(args []string) ([]byte, string, int, error) {
		return nil, "regex parse error: unclosed group", 2, errors.New("exit status 2")
	}
	defer func() { runRg = orig }()

	_, err := GrepTranscripts("(", []string{dir})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unclosed group") {
		t.Errorf("expected error to include rg stderr detail, got: %v", err)
	}
}

func TestGrepTranscriptsScopeProject(t *testing.T) {
	requireRg(t)
	// Create two dirs; query in both but only pass one dir.
	d1 := t.TempDir()
	d2 := t.TempDir()
	c1 := `{"type":"user","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"user","content":"fibonacci in project one"},"timestamp":"2026-01-01T08:00:00Z"}` + "\n"
	c2 := `{"type":"user","parentUuid":null,"isSidechain":false,"isMeta":false,"message":{"role":"user","content":"fibonacci in project two"},"timestamp":"2026-01-01T08:00:00Z"}` + "\n"
	if err := os.WriteFile(d1+"/a.jsonl", []byte(c1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d2+"/b.jsonl", []byte(c2), 0o644); err != nil {
		t.Fatal(err)
	}

	// Search only d1
	results, err := GrepTranscripts("fibonacci", []string{d1})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if strings.Contains(r.FilePath, d2) {
			t.Errorf("result from d2 leaked into d1-only search: %q", r.FilePath)
		}
	}
}

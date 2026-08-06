package claudehistory

import (
	"os"
	"testing"
	"time"
)

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fixture-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

const fixtureWithAITitle = `{"type":"mode","mode":"normal","sessionId":"sess-001"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"hello world"},"timestamp":"2026-01-01T10:00:00Z"}
{"type":"ai-title","aiTitle":"A helpful title","sessionId":"sess-001"}
{"type":"user","parentUuid":"a1","isSidechain":false,"message":{"role":"user","content":"second message"},"timestamp":"2026-01-01T10:02:00Z"}
{"type":"last-prompt","lastPrompt":"hello world","sessionId":"sess-001"}
`

const fixtureWithLastPrompt = `{"type":"mode","mode":"normal","sessionId":"sess-002"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"/beads 3dg.1"},"timestamp":"2026-01-02T10:00:00Z"}
{"type":"last-prompt","lastPrompt":"/beads 3dg.1","sessionId":"sess-002"}
{"type":"user","parentUuid":"u1","isSidechain":false,"message":{"role":"user","content":"follow-up"},"timestamp":"2026-01-02T10:05:00Z"}
`

const fixtureFirstUserMessage = `{"type":"mode","mode":"normal","sessionId":"sess-003"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"Please fix the bug in my code"},"timestamp":"2026-01-03T09:00:00Z"}
`

const fixtureSlashNoise = `{"type":"mode","mode":"normal","sessionId":"sess-004"}
{"type":"last-prompt","lastPrompt":"/grill generate a logo","sessionId":"sess-004"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"something"},"timestamp":"2026-01-04T09:00:00Z"}
`

const fixtureXMLNoise = `{"type":"mode","mode":"normal","sessionId":"sess-005"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"<command-message>beads</command-message>\n<command-name>/beads</command-name>\n<command-args>ji2.2</command-args>"},"timestamp":"2026-01-05T09:00:00Z"}
{"type":"last-prompt","lastPrompt":"/beads ji2.2","sessionId":"sess-005"}
`

const fixtureCustomTitle = `{"type":"mode","mode":"normal","sessionId":"sess-006"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"scan apps recursively"},"timestamp":"2026-01-06T09:00:00Z"}
{"type":"ai-title","aiTitle":"A generated title","sessionId":"sess-006"}
{"type":"custom-title","customTitle":"launcher-scan-utils","sessionId":"sess-006"}
{"type":"last-prompt","lastPrompt":"agree","sessionId":"sess-006"}
`

const fixtureAcknowledgementLastPrompt = `{"type":"mode","mode":"normal","sessionId":"sess-007"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"scan apps recursively"},"timestamp":"2026-01-07T09:00:00Z"}
{"type":"last-prompt","lastPrompt":"scan apps recursively","sessionId":"sess-007"}
{"type":"last-prompt","lastPrompt":"agree","sessionId":"sess-007"}
`

func TestConversationMetaAITitle(t *testing.T) {
	path := writeFixture(t, fixtureWithAITitle)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "A helpful title" {
		t.Errorf("expected 'A helpful title', got %q", c.Title)
	}
	if c.SessionID != "sess-001" {
		t.Errorf("expected sess-001, got %q", c.SessionID)
	}
	want := time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC)
	if !c.LastAccessed.Equal(want) {
		t.Errorf("expected last-accessed %v, got %v", want, c.LastAccessed)
	}
}

func TestConversationMetaLastPromptFallback(t *testing.T) {
	path := writeFixture(t, fixtureWithLastPrompt)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	// lastPrompt "/beads 3dg.1" → strip "/beads" → "3dg.1"
	if c.Title != "3dg.1" {
		t.Errorf("expected '3dg.1', got %q", c.Title)
	}
}

func TestConversationMetaFirstUserFallback(t *testing.T) {
	path := writeFixture(t, fixtureFirstUserMessage)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Please fix the bug in my code" {
		t.Errorf("expected first user message as title, got %q", c.Title)
	}
}

func TestConversationMetaSlashNoiseStripped(t *testing.T) {
	path := writeFixture(t, fixtureSlashNoise)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	// "/grill generate a logo" → strip "/grill" → "generate a logo"
	if c.Title != "generate a logo" {
		t.Errorf("expected 'generate a logo', got %q", c.Title)
	}
}

func TestConversationMetaXMLNoiseStripped(t *testing.T) {
	path := writeFixture(t, fixtureXMLNoise)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	// XML noise in first user message → fall through to lastPrompt "/beads ji2.2" → "ji2.2"
	if c.Title != "ji2.2" {
		t.Errorf("expected 'ji2.2', got %q", c.Title)
	}
}

func TestConversationMetaCustomTitleTakesPriority(t *testing.T) {
	path := writeFixture(t, fixtureCustomTitle)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "launcher-scan-utils" {
		t.Errorf("expected customTitle to win over aiTitle/lastPrompt, got %q", c.Title)
	}
}

func TestConversationMetaAcknowledgementLastPromptSkipped(t *testing.T) {
	path := writeFixture(t, fixtureAcknowledgementLastPrompt)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	// lastPrompt is just "agree" (a closing acknowledgement) → falls through
	// to the first user message instead.
	if c.Title != "scan apps recursively" {
		t.Errorf("expected acknowledgement lastPrompt to be skipped, got %q", c.Title)
	}
}

func TestConversationMetaSessionIDFallback(t *testing.T) {
	fixture := `{"type":"mode","mode":"normal","sessionId":"sess-fallback"}` + "\n"
	path := writeFixture(t, fixture)
	c, err := ConversationMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "sess-fallback" {
		t.Errorf("expected sessionID fallback, got %q", c.Title)
	}
}

func TestListConversationsSortedByLastAccessed(t *testing.T) {
	dir := t.TempDir()
	// Write two fixtures: older first, newer second
	write := func(name, content string) {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(dir+"/a.jsonl", `{"type":"mode","sessionId":"old"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"old"},"timestamp":"2026-01-01T08:00:00Z"}
{"type":"ai-title","aiTitle":"Old conversation","sessionId":"old"}
`)
	write(dir+"/b.jsonl", `{"type":"mode","sessionId":"new"}
{"type":"user","parentUuid":null,"isSidechain":false,"message":{"role":"user","content":"new"},"timestamp":"2026-01-02T08:00:00Z"}
{"type":"ai-title","aiTitle":"New conversation","sessionId":"new"}
`)

	convs, err := ListConversations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convs))
	}
	if convs[0].Title != "New conversation" {
		t.Errorf("expected newest first, got %q", convs[0].Title)
	}
	if convs[1].Title != "Old conversation" {
		t.Errorf("expected oldest second, got %q", convs[1].Title)
	}
}

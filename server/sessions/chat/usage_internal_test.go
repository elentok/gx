package chat

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/transcript"
)

func TestConversationUsageCodexCategories(t *testing.T) {
	src := newFakeSource()
	src.codexHome = t.TempDir()
	path := filepath.Join(src.codexHome, "sessions", "rollout-"+testConv+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":60,"cache_write_input_tokens":10},"total_token_usage":{"input_tokens":300,"cached_input_tokens":200,"cache_write_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":30,"total_tokens":350},"model_context_window":475000}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Deps{Source: src, Log: slog.New(slog.DiscardHandler)})
	got := c.ConversationUsage(Meta{ID: "s1", Harness: "codex", ConversationID: testConv})
	if got == nil {
		t.Fatal("no usage")
	}
	if got.Scope != "thread" || got.TotalTokens != 350 || got.InputTokens != 30 || got.FirstContext != 100 || got.ModelContextWindow != 475000 {
		t.Fatalf("usage = %+v", got)
	}
	if *got.Totals.InputTokens != 80 || *got.Totals.CacheReadTokens != 200 || *got.Totals.CacheCreationTokens != 20 || *got.Totals.OutputTokens != 50 || *got.Totals.ReasoningTokens != 30 {
		t.Fatalf("categories = %+v", got.Totals)
	}
	if got.MainTokens != nil || got.SubagentTokens != nil || got.EstimatedCost != nil {
		t.Fatalf("invented accounting: %+v", got)
	}
}

func TestConversationUsageRefreshesSubagentsWithoutParentWrites(t *testing.T) {
	config := t.TempDir()
	src := newFakeSource()
	m := Meta{ID: "s1", Harness: "claude", Dir: testCwd, ConversationID: testConv}
	c := claudeChat(src, config)
	path := filepath.Join(config, "projects", transcript.Slugify(testCwd), testConv+".jsonl")
	parent := `{"type":"assistant","timestamp":"2026-01-01T00:00:00Z","message":{"model":"claude-sonnet-4-5","usage":{"input_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"output_tokens":40}}}`
	writeLines(t, path, parent, `{"type":"system","subtype":"compact_boundary"}`)
	got := c.ConversationUsage(m)
	if got == nil || got.TotalTokens != 100 || !got.ContextStale || got.Compactions != 1 {
		t.Fatalf("parent = %+v", got)
	}
	sub := filepath.Join(filepath.Dir(path), testConv, "subagents", "agent-a.jsonl")
	writeLines(t, sub, parent, parent)
	got = c.ConversationUsage(m)
	if got == nil || got.TotalTokens != 300 || *got.MainTokens != 100 || *got.SubagentTokens != 200 {
		t.Fatalf("new subagent = %+v", got)
	}
	writeLines(t, sub, parent, parent, parent)
	got = c.ConversationUsage(m)
	if got == nil || got.TotalTokens != 400 || *got.Totals.InputTokens != 40 || *got.Totals.OutputTokens != 160 {
		t.Fatalf("updated subagent = %+v", got)
	}
	if got.FirstContext != 60 || got.PeakContext != 60 || !got.ContextStale || got.Scope != "conversation" {
		t.Fatalf("subagents changed parent context: %+v", got)
	}
	writeLines(t, path, parent, `{"type":"system","subtype":"compact_boundary"}`, parent)
	if got = c.ConversationUsage(m); got == nil || got.ContextStale {
		t.Fatalf("fresh parent = %+v", got)
	}
}

package chat

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/codexsession"
)

func TestReportedCodex_OneRolloutLookupForModelEffortAndTokens(t *testing.T) {
	t.Parallel()
	src := newFakeSource()
	src.codexHome = t.TempDir()
	path := filepath.Join(src.codexHome, "sessions", "rollout-"+testConv+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := `{"type":"turn_context","payload":{"model":"gpt-5","effort":"high"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":120000}}}}
`
	if err := os.WriteFile(path, []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	wantTokens, ok, err := codexsession.ReadContextTokens(path)
	if err != nil || !ok || wantTokens == 0 {
		t.Fatalf("fixture has no context reading: %d, %v, %v", wantTokens, ok, err)
	}

	lookups := 0
	c := New(Deps{Source: src, Log: slog.New(slog.DiscardHandler)})
	c.rolloutPath = func(home, id string) (string, bool, error) {
		lookups++
		return codexsession.RolloutPathIn(home, id)
	}
	m := Meta{ID: "s1", Harness: "codex", ConversationID: testConv}

	turn, tokens := c.ReportedCodex(m)
	if turn.Model != "gpt-5" || turn.Effort != "high" || tokens != wantTokens {
		t.Fatalf("ReportedCodex = %+v, %d, want gpt-5/high, %d", turn, tokens, wantTokens)
	}
	if lookups != 1 {
		t.Fatalf("rollout lookups = %d, want 1", lookups)
	}
	if model, got := c.ReportedModelAndContext(m); model != "gpt-5" || got != wantTokens {
		t.Fatalf("ReportedModelAndContext = %q, %d", model, got)
	}
}

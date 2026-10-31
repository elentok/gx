package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/ralphloop"
)

func TestSaveConversationTotals_CodexDesignSavesGrillingCounts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	codexHome := t.TempDir()
	root := t.TempDir()
	m := sessionMeta{ID: "s1", Dir: "/work/dir", Harness: string(ralphloop.AgentCodex), Ticket: "p:map", ConversationID: "c1", Design: true}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{ConversationsDir: root, Env: []string{"HOME=" + home, "CODEX_HOME=" + codexHome}}}
	s.recordConversation(m)

	dir := filepath.Join(codexHome, "sessions", "2026", "01", "02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The totals read comes first and needs the rollout's own meta and token lines.
	round := `{"timestamp":"2026-01-02T03:04:00Z","type":"session_meta","payload":{"id":"c1","cwd":"/work/dir"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":10},"last_token_usage":{"input_tokens":10}}}}` + "\n" +
		`{"timestamp":"2026-01-02T03:04:05Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"❓ **Q1** - **Alpha**: one two"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-01-02T03-04-05-c1.jsonl"), []byte(round), 0o644); err != nil {
		t.Fatal(err)
	}

	s.saveConversationTotals(m, m.ConversationID)
	rec, err := conversations.Read(root, m.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Design || len(rec.Grilling) != 1 || rec.Grilling[0].MessageID != "o3" {
		t.Errorf("record = %+v; want the Design flag and one counted round", rec)
	}
}

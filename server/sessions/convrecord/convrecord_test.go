package convrecord_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/sessions/convrecord"
)

type fakeHost struct{ claude, codex string }

func (h fakeHost) EnvHome() (string, error)   { return h.claude, nil }
func (h fakeHost) CodexHome() (string, error) { return h.codex, nil }
func (h fakeHost) OnMapEpic(string) bool      { return false }

func TestSaveTotals_CodexDesignSavesGrillingCounts(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	root := t.TempDir()
	s := convrecord.Session{ID: "s1", Dir: "/work/dir", Harness: string(ralphloop.AgentCodex), Ticket: "p:map", ConversationID: "c1", Design: true}
	rec := &convrecord.Recorder{Dir: root, Log: slog.New(slog.DiscardHandler), Host: fakeHost{claude: t.TempDir(), codex: codexHome}}
	rec.Record(s)

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

	rec.SaveTotals(s, s.ConversationID)
	got, err := conversations.Read(root, s.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Design || len(got.Grilling) != 1 || got.Grilling[0].MessageID != "o3" {
		t.Errorf("record = %+v; want the Design flag and one counted round", got)
	}
}

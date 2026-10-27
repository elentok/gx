package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

func TestSaveConversationTotals_CodexDesignLeavesNoGrillingCounts(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	m := sessionMeta{ID: "s1", Dir: "/work/dir", Harness: string(ralphloop.AgentCodex), Ticket: "p:map", ConversationID: "c1", Design: true}
	s := &Server{log: slog.New(slog.DiscardHandler), cfg: Config{ConversationsDir: root, Env: []string{"HOME=" + home}}}
	s.recordConversation(m)

	path := transcript.PathIn(home, m.Dir, m.ConversationID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	round := `{"type":"assistant","uuid":"m1","timestamp":"2026-01-02T03:04:05Z","message":{"role":"assistant","content":[{"type":"text","text":"❓ **Q1** - **Alpha**: one two"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(round), 0o644); err != nil {
		t.Fatal(err)
	}

	s.saveConversationTotals(m, m.ConversationID)
	rec, err := conversations.Read(root, m.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Design || rec.Grilling != nil {
		t.Errorf("record = %+v; want the Design flag and no counts", rec)
	}
}

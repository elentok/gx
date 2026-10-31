package server

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/ralphloop"
)

func TestLastLoggedAssistantText_Codex(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	rollout := `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"all done here"}]}}` + "\n"
	dir := filepath.Join(home, "sessions", "2026", "08")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout-c1.jsonl"), []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&logs, nil)), cfg: Config{Env: []string{"CODEX_HOME=" + home}}}

	for _, tc := range []struct{ name, session, want string }{
		{"rollout", "c1", "all done here"},
		{"died before the first prompt", "c2", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := s.lastLoggedAssistantText(ralphloop.Event{Agent: ralphloop.AgentCodex, Cwd: "/w", AgentSession: tc.session})
			if got != tc.want {
				t.Errorf("text = %q, want %q", got, tc.want)
			}
		})
	}
	if logs.Len() != 0 {
		t.Errorf("logged %q, want nothing", logs.String())
	}
}

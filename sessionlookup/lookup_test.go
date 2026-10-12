package sessionlookup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookup_MixedDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	put := func(path, content string) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	turn := func(text string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]}}` + "\n"
	}
	put("agents/headless/meta.json", `{"runner":"headless","session_id":"h1","label":"headless"}`)
	put("agents/headless/out.jsonl", turn("headless-log"))
	put("agents/pty/meta.json", `{"runner":"pty","session_id":"p1","label":"pty","cwd":"/w"}`)
	put("agents/pty/raw.log", "\x1b[2Jterminal bytes")
	put("agents/pty-gone/meta.json", `{"runner":"pty","session_id":"p2","label":"pty-gone","cwd":"/w"}`)
	put("agents/pty-gone/raw.log", "\x1b[2Jterminal bytes")
	put(".claude/projects/-w/p1.jsonl", turn("pty-transcript"))
	put(".claude/projects/-w/p1/subagents/agent-a.jsonl", turn("pty-sub"))
	put("codex/sessions/2026/rollout-c1.jsonl",
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"codex-log"}]}}`+"\n")

	roots := Roots{Home: root, CodexHome: filepath.Join(root, "codex")}
	agents := filepath.Join(root, "agents")
	for _, tc := range []struct {
		name, dir, id, source, contains string
		subs                            int
	}{
		{"headless", "headless", "h1", Native, "headless-log", 0},
		{"pty", "pty", "p1", Claude, "pty-transcript", 1},
		{"pty without transcript", "pty-gone", "p2", Missing, "", 0},
		{"codex", "", "c1", Codex, "codex-log", 0},
		{"missing", "headless", "nope", Missing, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := ""
			if tc.dir != "" {
				dir = filepath.Join(agents, tc.dir)
			}
			s, err := Lookup(roots, dir, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if s.Source != tc.source || !strings.Contains(s.Markdown, tc.contains) || len(s.Subagents) != tc.subs {
				t.Errorf("Lookup(%s) = %+v", tc.id, s)
			}
			if strings.Contains(s.Markdown, "terminal bytes") {
				t.Errorf("Lookup(%s) read raw.log", tc.id)
			}
		})
	}
}

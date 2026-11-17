package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeEffortLevel(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home}
	if got := ClaudeEffortLevel(env); got != "" {
		t.Fatalf("no config: got %q", got)
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude.json"), `{"effortLevel":"low"}`)
	if got := ClaudeEffortLevel(env); got != "low" {
		t.Fatalf("global: got %q", got)
	}
	write(filepath.Join(dir, "settings.json"), `{"effortLevel":"xhigh"}`)
	if got := ClaudeEffortLevel(env); got != "xhigh" {
		t.Fatalf("settings beats global: got %q", got)
	}
	write(filepath.Join(dir, "settings.json"), `{"effortLevel":"bogus"}`)
	if got := ClaudeEffortLevel(env); got != "low" {
		t.Fatalf("unknown level ignored: got %q", got)
	}
}

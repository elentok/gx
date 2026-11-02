package agentfake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/claudeenv"
)

func TestLaunchEditorMode_LaunchSettingBeatsPersonsVim(t *testing.T) {
	home := t.TempDir()
	for _, kv := range claudeenv.In(home) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := launchEditorMode(nil); got != "vim" {
		t.Errorf("no flag: got %q, want the person's vim", got)
	}
	if got := launchEditorMode([]string{"--settings", `{"editorMode":"normal"}`}); got != "normal" {
		t.Errorf("with flag: got %q, want normal", got)
	}
}

// With CLAUDE_CONFIG_DIR set, claude's global config lives in that dir, so the
// poisoned HOME files are not read.
func TestLaunchEditorMode_IgnoresHomeWhenConfigDirIsSet(t *testing.T) {
	home := t.TempDir()
	for _, kv := range claudeenv.In(home) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	if got := launchEditorMode(nil); got != "" {
		t.Errorf("got %q, want no editor mode", got)
	}
}

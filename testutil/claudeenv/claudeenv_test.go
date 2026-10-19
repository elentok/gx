package claudeenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolate_ConfigDirIsCleanAndHomeIsPoisoned(t *testing.T) {
	cleanup := Isolate()
	home := os.Getenv("HOME")

	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if !strings.HasPrefix(cfg, home) {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want it under HOME %q", cfg, home)
	}
	if _, err := os.Stat(filepath.Join(cfg, ".claude.json")); !os.IsNotExist(err) {
		t.Errorf("config dir has a .claude.json (err=%v), want it clean", err)
	}
	for _, p := range []string{".claude.json", filepath.Join(".claude", "settings.json")} {
		b, err := os.ReadFile(filepath.Join(home, p))
		if err != nil || !strings.Contains(string(b), `"vim"`) {
			t.Errorf("%s = %q, %v; want the vim canary", p, b, err)
		}
	}
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		if v := os.Getenv(k); v != "" {
			t.Errorf("%s = %q, want it unset so paths derive from HOME", k, v)
		}
	}

	cleanup()
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Errorf("cleanup left %s behind (err=%v)", home, err)
	}
}

package claudeenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIn_ReturnsEnvEntriesAndLeavesTheProcessEnvAlone(t *testing.T) {
	before := os.Environ()
	home := t.TempDir()

	env := In(home)

	if !slices.Equal(before, os.Environ()) {
		t.Error("In changed the process env")
	}
	get := func(key string) string {
		v := ""
		for _, kv := range env {
			if k, val, _ := strings.Cut(kv, "="); k == key {
				v = val
			}
		}
		return v
	}
	if got := get("HOME"); got != home {
		t.Errorf("HOME = %q, want %q", got, home)
	}
	cfg := get("CLAUDE_CONFIG_DIR")
	if !strings.HasPrefix(cfg, home) {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want it under %q", cfg, home)
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
		if !slices.Contains(env, k+"=") {
			t.Errorf("env has no empty %s entry", k)
		}
	}
}

func TestIn_GoCachesStayOutsideTempHome(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	home := t.TempDir()

	env := In(home)

	for _, k := range []string{"GOCACHE", "GOMODCACHE"} {
		var got string
		for _, kv := range env {
			if key, val, _ := strings.Cut(kv, "="); key == k {
				got = val
			}
		}
		if got == "" {
			t.Errorf("env has no %s entry", k)
		} else if strings.HasPrefix(got, home) {
			t.Errorf("%s = %q is under the temp HOME %q, want the real cache", k, got, home)
		}
	}
}


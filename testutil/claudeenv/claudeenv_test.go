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

func TestIsolate_GoCachesStayOutsideTempHome(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	// Register the restore; Isolate sets these with os.Setenv.
	for _, k := range []string{"GOCACHE", "GOMODCACHE", "CLAUDE_CONFIG_DIR"} {
		t.Setenv(k, os.Getenv(k))
	}
	// An earlier test may have left HOME pointing at a deleted dir, which go
	// would recreate. Give it a dir that t.TempDir removes.
	t.Setenv("HOME", t.TempDir())
	cleanup := Isolate()
	defer cleanup()
	home := os.Getenv("HOME")

	// go may leave a background telemetry process that writes under HOME after
	// the child exits, so the child gets a HOME other than the one cleanup removes.
	cmd := exec.Command("go", "env", "GOCACHE", "GOMODCACHE")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 2 {
		t.Fatalf("go env printed %q, want two paths", out)
	}
	for _, p := range lines {
		if strings.HasPrefix(p, home) {
			t.Errorf("%q is under the temp HOME %q, want the real cache", p, home)
		}
	}
}

func TestRemoveTree_ReadOnlyTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	sub := filepath.Join(root, "pkg", "mod", "x@v1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(sub, "f.go"), sub, filepath.Join(root, "pkg", "mod"), filepath.Join(root, "pkg"), root} {
		perm := os.FileMode(0o555)
		if strings.HasSuffix(p, ".go") {
			perm = 0o444
		}
		if err := os.Chmod(p, perm); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeTree(root); err != nil {
		t.Errorf("removeTree = %v, want nil", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("%s still exists (err=%v)", root, err)
	}
}

func TestRemoveTree_ReturnsErrorWhenRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })

	if err := removeTree(home); err == nil {
		t.Error("removeTree = nil, want the removal error")
	}
}

package agentfake

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/commandlist"
)

// probeEnv puts a "claude" and a "codex" (both this test binary) on PATH and
// returns the env a probe is launched with, the cwd and the HOME dir.
func probeEnv(t *testing.T, cfg *ProbeConfig) (env []string, cwd, home string) {
	t.Helper()
	// The test binary runs one fake per process; the test's agent picks which.
	fakeEnv := TUIEnv
	if strings.HasPrefix(t.Name(), "TestCodex") {
		fakeEnv = CodexEnv
	}
	return probeEnvFor(t, fakeEnv, cfg)
}

func probeEnvFor(t *testing.T, fakeEnv string, cfg *ProbeConfig) (env []string, cwd, home string) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.Symlink(os.Args[0], filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home, cwd = t.TempDir(), t.TempDir()
	env = append(os.Environ(), fakeEnv+"=1", "HOME="+home)
	if cfg != nil {
		env = append(env, CommandsFileEnv+"="+writeProbeConfig(t, filepath.Join(t.TempDir(), "commands.json"), *cfg))
	}
	return env, cwd, home
}

func writeProbeConfig(t *testing.T, path string, cfg ProbeConfig) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(entries []commandlist.Entry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func contains(entries []commandlist.Entry, name string) bool {
	for _, e := range entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("probe wrote files into %s: %v", dir, left)
	}
}

func TestClaudeProbe_ListsConfiguredCommands(t *testing.T) {
	env, cwd, home := probeEnv(t, &ProbeConfig{Entries: []ProbeEntry{
		{Name: "myskill", Description: "Mine", ArgumentHint: "[x]"},
		{Name: "compact", Builtin: true},
		{Name: "srv:review (MCP)"},
	}})
	got, err := commandlist.ProbeClaude(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"myskill", "compact", "mcp__srv__review"} {
		if !contains(got, want) {
			t.Errorf("missing %q in %v", want, names(got))
		}
	}
	assertEmptyDir(t, home)
	assertEmptyDir(t, cwd)
}

func TestClaudeProbe_CwdFileOverridesGlobal(t *testing.T) {
	env, cwd, _ := probeEnv(t, &ProbeConfig{Entries: []ProbeEntry{{Name: "globalskill"}}})
	writeProbeConfig(t, filepath.Join(cwd, CommandsCwdFile), ProbeConfig{Entries: []ProbeEntry{{Name: "featureskill"}}})
	got, err := commandlist.ProbeClaude(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "featureskill") || contains(got, "globalskill") {
		t.Fatalf("got %v", names(got))
	}
}

func TestClaudeProbe_Delay(t *testing.T) {
	env, cwd, _ := probeEnv(t, &ProbeConfig{Delay: "400ms", Entries: []ProbeEntry{{Name: "slow"}}})
	start := time.Now()
	got, err := commandlist.ProbeClaude(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 400*time.Millisecond || !contains(got, "slow") {
		t.Fatalf("elapsed %s, got %v", time.Since(start), names(got))
	}
}

func TestClaudeProbe_Fail(t *testing.T) {
	env, cwd, home := probeEnv(t, &ProbeConfig{Fail: true})
	if _, err := commandlist.ProbeClaude(context.Background(), cwd, env); err == nil {
		t.Fatal("want an error")
	}
	assertEmptyDir(t, home)
}

func TestCodexProbe_ListsSkillsOnceEach(t *testing.T) {
	env, cwd, home := probeEnv(t, &ProbeConfig{Entries: []ProbeEntry{
		{Name: "dup-skill", Description: "D"},
		{Name: "dup-skill", Description: "D"},
		{Name: "off-skill", Disabled: true},
		{Name: "other"},
	}})
	got, err := commandlist.ProbeCodexSkills(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got); len(n) != 2 || n[0] != "dup-skill" || n[1] != "other" {
		t.Fatalf("got %v", n)
	}
	assertEmptyDir(t, home)
	assertEmptyDir(t, cwd)
}

func TestCodexProbe_CwdFileOverridesGlobal(t *testing.T) {
	env, cwd, _ := probeEnv(t, &ProbeConfig{Entries: []ProbeEntry{{Name: "globalskill"}}})
	writeProbeConfig(t, filepath.Join(cwd, CommandsCwdFile), ProbeConfig{Entries: []ProbeEntry{{Name: "featureskill"}}})
	got, err := commandlist.ProbeCodexSkills(context.Background(), cwd, env)
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got); len(n) != 1 || n[0] != "featureskill" {
		t.Fatalf("got %v", n)
	}
}

func TestCodexProbe_Delay(t *testing.T) {
	env, cwd, _ := probeEnv(t, &ProbeConfig{Delay: "400ms", Entries: []ProbeEntry{{Name: "slow"}}})
	start := time.Now()
	if _, err := commandlist.ProbeCodexSkills(context.Background(), cwd, env); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("answered after %s", time.Since(start))
	}
}

func TestCodexProbe_Fail(t *testing.T) {
	env, cwd, _ := probeEnv(t, &ProbeConfig{Fail: true})
	if _, err := commandlist.ProbeCodexSkills(context.Background(), cwd, env); err == nil {
		t.Fatal("want an error")
	}
}

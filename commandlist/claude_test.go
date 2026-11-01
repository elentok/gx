package commandlist_test

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/commandlist"
)

const recordedReply = "testdata/claude-initialize.jsonl"

func parseRecorded(t *testing.T) []commandlist.Entry {
	t.Helper()
	f, err := os.Open(recordedReply)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := commandlist.ParseClaudeReply(f)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestParseClaudeReply_Recorded(t *testing.T) {
	got := parseRecorded(t)
	if len(got) != 5 {
		t.Fatalf("got %d entries, want 5: %v", len(got), names(got))
	}

	skill, _ := find(got, "probe-skill")
	if skill.Source != commandlist.SourceSkill {
		t.Errorf("skill source = %q", skill.Source)
	}
	// Unescaping is Merge's job; the parser keeps the reply text as is.
	if !strings.Contains(skill.Description, "&lt;epic-slug&gt;") {
		t.Errorf("skill description = %q", skill.Description)
	}
	if c, _ := find(got, "mycmd"); c.Source != commandlist.SourceSkill {
		t.Errorf("custom command source = %q", c.Source)
	}

	clear, _ := find(got, "clear")
	if clear.Source != commandlist.SourceBuiltIn || !slices.Equal(clear.Aliases, []string{"reset", "new"}) || clear.ArgHint != "[name]" { // oldword:ok /clear's real alias in Claude
		t.Errorf("clear = %+v", clear)
	}

	mcp, ok := find(got, "mcp__probe_srv__review_pr")
	if !ok || mcp.Source != commandlist.SourceMCP || mcp.Description != "Review a PR" {
		t.Errorf("mcp entry = %+v (found %v)", mcp, ok)
	}
}

func TestParseClaudeReply_Errors(t *testing.T) {
	for name, in := range map[string]string{
		"no reply": `{"type":"system","subtype":"hook_started"}` + "\n",
		"bad json": "not json\n",
		"empty":    "",
	} {
		if _, err := commandlist.ParseClaudeReply(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// stubClaude puts a `claude` shell script first on PATH and returns the dir it
// records the run into. body runs after the recording.
func stubClaude(t *testing.T, body string) string {
	t.Helper()
	bin, out := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"echo \"$@\" > \"$STUB_OUT/args\"\n" +
		"pwd -P > \"$STUB_OUT/pwd\"\n" +
		"env > \"$STUB_OUT/env\"\n" + body
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

func stubEnv(out string, extra ...string) []string {
	abs, _ := filepath.Abs(recordedReply)
	return append(append(os.Environ(), "STUB_OUT="+out, "STUB_REPLY="+abs), extra...)
}

func readRecord(t *testing.T, out, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProbeClaude_Stub(t *testing.T) {
	out := stubClaude(t, "read line\ncat \"$STUB_REPLY\"\n")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := stubEnv(out, "GX_AGENT_SESSION=abc", "GX_CODEX_AGENT_DIR=/a/dir", "KEEP_ME=1")

	got, err := commandlist.ProbeClaude(context.Background(), dir, env)
	if err != nil {
		t.Fatal(err)
	}

	// Merged: the reply's entries, then built-ins it did not mention.
	if _, ok := find(got, "probe-skill"); !ok {
		t.Error("reply entry missing")
	}
	if e, _ := find(got, "clear"); e.Description != "Start a new session with empty context; previous session stays on disk (resumable with /resume)" {
		t.Errorf("reply did not win over the built-in: %q", e.Description)
	}
	if _, ok := find(got, "review"); !ok {
		t.Error("built-ins not merged in")
	}
	if e, _ := find(got, "probe-skill"); strings.Contains(e.Description, "&lt;") {
		t.Errorf("description not unescaped: %q", e.Description)
	}

	args := strings.Fields(readRecord(t, out, "args"))
	for _, want := range []string{"-p", "--no-session-persistence", "--input-format", "--output-format", "stream-json"} {
		if !slices.Contains(args, want) {
			t.Errorf("args %v lack %q", args, want)
		}
	}
	if pwd := strings.TrimSpace(readRecord(t, out, "pwd")); pwd != dir {
		t.Errorf("cwd = %q, want %q", pwd, dir)
	}
	stubSeen := readRecord(t, out, "env")
	for _, bad := range []string{"GX_AGENT_SESSION=", "GX_CODEX_AGENT_DIR="} {
		if strings.Contains(stubSeen, bad) {
			t.Errorf("child env has %s", bad)
		}
	}
	if !strings.Contains(stubSeen, "KEEP_ME=1") {
		t.Error("child env lost KEEP_ME")
	}
}

func TestProbeClaude_StubFailures(t *testing.T) {
	cases := map[string]string{
		"non-zero exit": "exit 3\n",
		"bad json":      "read line\necho nope\n",
		"no reply":      "read line\nexit 0\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			out := stubClaude(t, body)
			if _, err := commandlist.ProbeClaude(context.Background(), t.TempDir(), stubEnv(out)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestProbeClaude_TimeoutKillsChild(t *testing.T) {
	out := stubClaude(t, "read line\nsleep 30\n")
	start := time.Now()
	_, err := commandlist.ProbeClaudeTimeout(context.Background(), t.TempDir(), stubEnv(out), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("probe took %s, the hung child was not killed", d)
	}
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// Real binary; never in CI.
func TestProbeClaude_RealBinaryLeavesNoTranscript(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("real claude is not run in CI")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude is not installed")
	}
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip(err)
		}
		cfg = filepath.Join(home, ".claude")
	}
	projects := filepath.Join(cfg, "projects")
	before := countFiles(t, projects)

	got, err := commandlist.ProbeClaude(context.Background(), t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Error("empty command list")
	}
	if after := countFiles(t, projects); after != before {
		t.Errorf("projects dir went from %d to %d files", before, after)
	}
}

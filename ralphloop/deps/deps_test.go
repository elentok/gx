package deps

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/transcript"
)

// stubBin writes an executable script named name into a fresh directory and
// returns that directory (a pathEnv for installDependenciesWith/lookPathIn),
// so the install step's exec.Command calls resolve to it instead of the
// real package manager, without mutating the process-wide PATH env var
// (t.Setenv, which would block this test from running under t.Parallel()).
func stubBin(t *testing.T, name string, logPath string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	writeStub(t, dir, name, logPath, exitCode)
	return dir
}

// writeStub writes the stub script into dir. The script appends one line
// "cwd=<dir> CI=<value> <$0> <args>" to logPath; a non-zero exitCode also
// prints "boom" first, as a failing package manager would.
func writeStub(t *testing.T, dir, name, logPath string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\necho \"cwd=$(pwd) CI=$CI $0 $*\" >> " + logPath + "\n"
	if exitCode != 0 {
		script += "echo boom\n"
	}
	script += "exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}
}

func TestVerifySkillWith(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		agent    agent.AgentKind
		skill    string
		statErr  error
		homeErr  error
		wantErr  string
		wantPath string
	}{
		{
			name:  "claude skill installed",
			agent: agent.AgentClaude,
			skill: "implement",
		},
		{
			name:     "claude skill missing",
			agent:    agent.AgentClaude,
			skill:    "implement",
			statErr:  os.ErrNotExist,
			wantErr:  `skill "implement" not found`,
			wantPath: filepath.Join("home", ".claude", "skills", "implement", "SKILL.md"),
		},
		{
			name:  "codex prompt installed",
			agent: agent.AgentCodex,
			skill: "implement",
		},
		{
			name:     "codex prompt missing",
			agent:    agent.AgentCodex,
			skill:    "implement",
			statErr:  os.ErrNotExist,
			wantErr:  `skill "implement" not found`,
			wantPath: filepath.Join("home", ".codex", "prompts", "implement.md"),
		},
		{
			name:    "home directory unresolvable",
			agent:   agent.AgentClaude,
			skill:   "implement",
			homeErr: errors.New("no home"),
			wantErr: "resolving home directory",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var statPath string
			err := verifySkillWith(tc.agent, tc.skill,
				func() (string, error) { return "home", tc.homeErr },
				func(path string) (os.FileInfo, error) {
					statPath = path
					if tc.statErr != nil {
						return nil, tc.statErr
					}
					return nil, nil
				},
			)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("verifySkillWith() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("verifySkillWith() error = %v, want containing %q", err, tc.wantErr)
			}
			if tc.wantPath != "" && statPath != tc.wantPath {
				t.Errorf("stat path = %q, want %q", statPath, tc.wantPath)
			}
		})
	}
}

func TestLookPathIn_ResolvesAgainstExplicitPathInsteadOfProcessEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "mybin")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Deliberately never t.Setenv PATH here — lookPathIn must not need it.
	got, err := lookPathIn("mybin", dir)
	if err != nil {
		t.Fatalf("lookPathIn() error = %v", err)
	}
	if got != binPath {
		t.Errorf("lookPathIn() = %q, want %q", got, binPath)
	}

	if _, err := lookPathIn("mybin", t.TempDir()); err == nil {
		t.Error("lookPathIn() with an unrelated PATH = nil error, want not-found")
	}
}

func TestInstallDependenciesWith_UsesExplicitPathInsteadOfProcessEnv(t *testing.T) {
	t.Parallel()
	repo := installRepo(t, map[string]string{"package.json": pkgJSON, "package-lock.json": "{}"}, nil)
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	binDir := stubBin(t, "npm", logPath, 0)

	// Deliberately never t.Setenv PATH here — installDependenciesWith must
	// not need it.
	commands, err := installDependenciesWith(repo, binDir, "")
	if err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if want := []string{repo + ": npm ci"}; !equalStrings(commands, want) {
		t.Errorf("commands = %v, want %v", commands, want)
	}
	if _, err := os.ReadFile(logPath); err != nil {
		t.Fatalf("stub was not invoked: %v", err)
	}
}

func TestDefaultDepsWithOverrides_HomeOverridesVerifySkillLookup(t *testing.T) {
	t.Parallel()
	overrideHome := t.TempDir()
	skillPath := filepath.Join(overrideHome, ".claude", "skills", "implement", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deps := DefaultDepsWithOverrides(DepsOverrides{Home: overrideHome})
	if err := deps.VerifySkill(agent.AgentClaude, "implement"); err != nil {
		t.Errorf("VerifySkill() error = %v, want nil: override home has the skill file", err)
	}
	if err := deps.VerifySkill(agent.AgentClaude, "missing"); err == nil {
		t.Error("VerifySkill() error = nil, want error for a skill absent from the override home")
	}
}

func TestDefaultDepsWithOverrides_NoHomeReadsNoTranscript(t *testing.T) {
	t.Parallel()
	const cwd, session = "/work/tree", "sess-1"
	home := t.TempDir()
	fixture.WriteFakeTranscript(t, home, cwd, session, time.Now(), [3]any{"claude-sonnet-5", 1000, 0})

	if _, ok, err := DefaultDepsWithOverrides(DepsOverrides{Home: home}).ReadOccupancy(cwd, session); err != nil || !ok {
		t.Fatalf("ReadOccupancy() with the home = ok %v, err %v; want the transcript found", ok, err)
	}
	if _, ok, err := DefaultDepsWithOverrides(DepsOverrides{}).ReadOccupancy(cwd, session); err != nil || ok {
		t.Errorf("ReadOccupancy() with no home = ok %v, err %v; want not found", ok, err)
	}
	if err := DefaultDepsWithOverrides(DepsOverrides{}).VerifySkill(agent.AgentClaude, "implement"); err == nil {
		t.Error("VerifySkill() with no home = nil, want an error")
	}
	if ok, err := DefaultDepsWithOverrides(DepsOverrides{}).VerifyCodexSession(cwd, session); err != nil || ok {
		t.Errorf("VerifyCodexSession() with no codex home = ok %v, err %v; want not found", ok, err)
	}
}

func TestDefaultDepsWithOverrides_ReadTurnsResolvesPathLikeReadOccupancy(t *testing.T) {
	t.Parallel()
	const cwd, session = "/work/tree", "sess-1"
	home := t.TempDir()
	fixture.WriteFakeTranscript(t, home, cwd, session, time.Now(), [3]any{"claude-sonnet-5", 1000, 0})

	turns, ok, err := DefaultDepsWithOverrides(DepsOverrides{Home: home}).ReadTurns(cwd, session, time.Time{})
	if err != nil || !ok || turns.Model != "claude-sonnet-5" || turns.Count != 1 {
		t.Fatalf("ReadTurns() = %+v, %v, %v; want the transcript's first model and 1 turn", turns, ok, err)
	}
	if _, ok, err := DefaultDepsWithOverrides(DepsOverrides{}).ReadTurns(cwd, session, time.Time{}); err != nil || ok {
		t.Errorf("ReadTurns() with no home = ok %v, err %v; want not found", ok, err)
	}
}

func TestDefaultDepsWithOverrides_RunnerWaitReachesRunner(t *testing.T) {
	t.Parallel()
	runner := DefaultDepsWithOverrides(DepsOverrides{RunnerWait: 7 * time.Second}).Runner.(*herdrrunner.Runner)
	if got := runner.WaitN(); got != 7*time.Second {
		t.Errorf("WaitN() = %v, want 7s", got)
	}
}

func TestDefaultDepsWithOverrides_PathOverridesPreflightLookup(t *testing.T) {
	t.Parallel()
	binDir := t.TempDir()
	script := "#!/bin/sh\necho logged in\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile codex stub: %v", err)
	}

	deps := DefaultDepsWithOverrides(DepsOverrides{Path: binDir})
	err := deps.PreflightAgent(agent.AgentCodex)
	if err == nil || !strings.Contains(err.Error(), "Herdr executable not found") {
		t.Errorf("PreflightAgent() error = %v, want a Herdr-not-found error (codex resolved via override PATH, herdr did not)", err)
	}
}

func TestDefaultDepsWithOverrides_CodexHomeOverridesContextAndRateLimitReads(t *testing.T) {
	t.Parallel()
	codexHome := t.TempDir()
	path := filepath.Join(codexHome, "sessions", "2026", "08", "01", "rollout-2026-08-01T10-00-00-session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/repo/iter-01"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":151000}},"rate_limits":{"primary":{"used_percent":100,"resets_at":1786170140}}}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deps := DefaultDepsWithOverrides(DepsOverrides{CodexHome: codexHome})
	tokens, ok, err := deps.ReadCodexContext("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadCodexContext() error = %v", err)
	}
	if !ok || tokens != 151000 {
		t.Errorf("ReadCodexContext() = (%d, %t), want (151000, true)", tokens, ok)
	}

	if n, ok, err := deps.ReadCodexCompactions("/repo/iter-01", "session-1"); err != nil || !ok || n != 0 {
		t.Errorf("ReadCodexCompactions() = (%d, %t, %v), want (0, true, nil)", n, ok, err)
	}
	if n, ok, err := deps.ReadCodexCompactionsAfter("/repo/iter-01", "session-1", time.Time{}); err != nil || !ok || n != 0 {
		t.Errorf("ReadCodexCompactionsAfter() = (%d, %t, %v), want (0, true, nil)", n, ok, err)
	}
	empty := DefaultDepsWithOverrides(DepsOverrides{})
	if _, ok, err := empty.ReadCodexCompactions("/repo/iter-01", "session-1"); err != nil || ok {
		t.Errorf("empty Codex home: ReadCodexCompactions ok=%t err=%v, want unavailable", ok, err)
	}

	limit, ok, err := deps.Runner.(*herdrrunner.Runner).CodexQuota("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("CodexQuota() error = %v", err)
	}
	if !ok || limit.Quota != "primary" {
		t.Errorf("CodexQuota() = (%+v, %t), want exhausted primary, ok=true", limit, ok)
	}
}

func TestDefaultDepsWithOverrides_HomeOverridesOccupancyAndCompactionReads(t *testing.T) {
	t.Parallel()
	overrideHome := t.TempDir()
	path := transcript.PathIn(overrideHome, "/repo/iter-01", "session-1")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	contents := `{"type":"assistant","timestamp":"2026-08-01T10:00:00Z","message":{"model":"claude-sonnet-5","usage":{"input_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":0,"output_tokens":50}}}
{"type":"system","subtype":"compact_boundary","timestamp":"2026-08-01T10:01:00Z","compactMetadata":{"trigger":"manual"}}
`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deps := DefaultDepsWithOverrides(DepsOverrides{Home: overrideHome})

	occupancy, ok, err := deps.ReadOccupancy("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadOccupancy() error = %v", err)
	}
	if !ok || occupancy != 105 {
		t.Errorf("ReadOccupancy() = (%d, %t), want (105, true)", occupancy, ok)
	}

	count, ok, err := deps.ReadCompactions("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadCompactions() error = %v", err)
	}
	if !ok || count != 1 {
		t.Errorf("ReadCompactions() = (%d, %t), want (1, true)", count, ok)
	}
}

// fixedNow returns a clock that never advances, for tests where elapsed
// time isn't under test.
func fixedNow() func() time.Time {
	t := time.Now()
	return func() time.Time { return t }
}

package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/transcript"
)

// stubBin writes an executable script named name into a fresh directory and
// returns that directory (a pathEnv for installDependenciesWith/lookPathIn),
// so InstallDependencies's exec.Command calls resolve to it instead of the
// real package manager, without mutating the process-wide PATH env var
// (t.Setenv, which would block this test from running under t.Parallel()).
// The script appends its own invocation ("name arg1 arg2 ...") as one line
// to logPath, and exits with exitCode.
func stubBin(t *testing.T, name string, logPath string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$0 $*\" >> " + logPath + "\nexit " + strconv.Itoa(exitCode) + "\n"
	scriptPath := filepath.Join(dir, name)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}
	return dir
}

func TestVerifySkillWith(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		agent    AgentKind
		skill    string
		statErr  error
		homeErr  error
		wantErr  string
		wantPath string
	}{
		{
			name:  "claude skill installed",
			agent: AgentClaude,
			skill: "implement",
		},
		{
			name:     "claude skill missing",
			agent:    AgentClaude,
			skill:    "implement",
			statErr:  os.ErrNotExist,
			wantErr:  `skill "implement" not found`,
			wantPath: filepath.Join("home", ".claude", "skills", "implement", "SKILL.md"),
		},
		{
			name:  "codex prompt installed",
			agent: AgentCodex,
			skill: "implement",
		},
		{
			name:     "codex prompt missing",
			agent:    AgentCodex,
			skill:    "implement",
			statErr:  os.ErrNotExist,
			wantErr:  `skill "implement" not found`,
			wantPath: filepath.Join("home", ".codex", "prompts", "implement.md"),
		},
		{
			name:    "home directory unresolvable",
			agent:   AgentClaude,
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

func TestInstallDependencies_NoMarker_SkipsSilently(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	command, err := InstallDependencies(dir)
	if err != nil {
		t.Fatalf("InstallDependencies() error = %v", err)
	}
	if command != "" {
		t.Errorf("command = %q, want empty (no marker matched)", command)
	}
}

func TestInstallDependencies_GoModOnly_SkipsSilently(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	command, err := InstallDependencies(dir)
	if err != nil {
		t.Fatalf("InstallDependencies() error = %v", err)
	}
	if command != "" {
		t.Errorf("command = %q, want empty: go.mod alone should not trigger an install step (go build/test populate the module cache lazily)", command)
	}
}

func TestInstallDependencies_EachMarker_RunsExpectedCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		marker      string
		wantCommand string
		wantBin     string
	}{
		{"package-lock.json", "npm ci", "npm"},
		{"pnpm-lock.yaml", "pnpm install --frozen-lockfile", "pnpm"},
		{"yarn.lock", "yarn install --frozen-lockfile", "yarn"},
		{"poetry.lock", "poetry install", "poetry"},
		{"uv.lock", "uv sync", "uv"},
	}

	for _, c := range cases {
		t.Run(c.marker, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.marker), []byte(""), 0644); err != nil {
				t.Fatalf("WriteFile %s: %v", c.marker, err)
			}
			logPath := filepath.Join(t.TempDir(), "invocations.log")
			binDir := stubBin(t, c.wantBin, logPath, 0)

			command, err := installDependenciesWith(dir, binDir)
			if err != nil {
				t.Fatalf("installDependenciesWith() error = %v", err)
			}
			if command != c.wantCommand {
				t.Errorf("command = %q, want %q", command, c.wantCommand)
			}

			logged, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("stub was not invoked: %v", err)
			}
			if !strings.Contains(string(logged), c.wantBin) {
				t.Errorf("invocation log = %q, want it to mention %q", logged, c.wantBin)
			}
		})
	}
}

func TestInstallDependencies_MarkerPrecedence_NpmWinsOverPnpm(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, marker := range []string{"package-lock.json", "pnpm-lock.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, marker), []byte(""), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", marker, err)
		}
	}
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	binDir := stubBin(t, "npm", logPath, 0)

	command, err := installDependenciesWith(dir, binDir)
	if err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if command != "npm ci" {
		t.Errorf("command = %q, want npm ci to win when both markers are present", command)
	}
}

func TestInstallDependencies_CommandFails_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile package-lock.json: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	binDir := stubBin(t, "npm", logPath, 1)

	command, err := installDependenciesWith(dir, binDir)
	if err == nil {
		t.Fatal("installDependenciesWith() error = nil, want failure surfaced")
	}
	if command != "npm ci" {
		t.Errorf("command = %q, want npm ci returned alongside the error", command)
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
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile package-lock.json: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "invocations.log")
	binDir := t.TempDir()
	script := "#!/bin/sh\necho \"$0 $*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "npm"), []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile stub: %v", err)
	}

	// Deliberately never t.Setenv PATH here — installDependenciesWith must
	// not need it.
	command, err := installDependenciesWith(dir, binDir)
	if err != nil {
		t.Fatalf("installDependenciesWith() error = %v", err)
	}
	if command != "npm ci" {
		t.Errorf("command = %q, want npm ci", command)
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
	if err := deps.VerifySkill(AgentClaude, "implement"); err != nil {
		t.Errorf("VerifySkill() error = %v, want nil: override home has the skill file", err)
	}
	if err := deps.VerifySkill(AgentClaude, "missing"); err == nil {
		t.Error("VerifySkill() error = nil, want error for a skill absent from the override home")
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
	err := deps.PreflightAgent(AgentCodex)
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

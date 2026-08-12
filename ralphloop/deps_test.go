package ralphloop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/herdr"
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

	limit, ok, err := deps.ReadCodexRateLimit("/repo/iter-01", "session-1")
	if err != nil {
		t.Fatalf("ReadCodexRateLimit() error = %v", err)
	}
	if !ok || limit.Quota != "primary" {
		t.Errorf("ReadCodexRateLimit() = (%+v, %t), want exhausted primary, ok=true", limit, ok)
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

// pollTimeoutErr matches isPollTimeout's "timed out" substring check.
var pollTimeoutErr = errors.New("timed out waiting for state")

// fixedNow returns a clock that never advances, for tests where elapsed
// time isn't under test.
func fixedNow() func() time.Time {
	t := time.Now()
	return func() time.Time { return t }
}

// stepNow returns a clock that advances by each of steps on successive
// calls (the first call returns the base instant unmodified), for tests
// asserting how promptWithNudge divides a deadline across its phases.
func stepNow(steps ...time.Duration) func() time.Time {
	t := time.Now()
	calls := 0
	return func() time.Time {
		if calls > 0 && calls-1 < len(steps) {
			t = t.Add(steps[calls-1])
		}
		calls++
		return t
	}
}

func TestPromptWithNudge_SucceedsWithoutTimeout_NeverNudges(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if opts.TimeoutMs != promptNudgeGraceMs {
			t.Errorf("prompt TimeoutMs = %d, want %d", opts.TimeoutMs, promptNudgeGraceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "working")
	}
	if promptCalls != 1 || sendKeysCalls != 0 || waitCalls != 0 {
		t.Errorf("promptCalls=%d sendKeysCalls=%d waitCalls=%d, want 1/0/0 (no nudge needed)", promptCalls, sendKeysCalls, waitCalls)
	}
}

func TestPromptWithNudge_TimesOutThenNudgeSucceeds(t *testing.T) {
	t.Parallel()
	var sendKeysTarget string
	var sendKeysKeys []string
	var waitOpts herdr.AgentWaitOptions

	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysTarget = target
		sendKeysKeys = keys
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitOpts = opts
		return herdr.Agent{AgentStatus: "working"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "working" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "working")
	}
	if sendKeysTarget != "pane-1" || len(sendKeysKeys) != 1 || sendKeysKeys[0] != "enter" {
		t.Errorf("sendKeys called with target=%q keys=%v, want pane-1/[enter]", sendKeysTarget, sendKeysKeys)
	}
	if waitOpts.Target != "pane-1" || len(waitOpts.Until) != 1 || waitOpts.Until[0] != "working" {
		t.Errorf("wait called with %+v, want Target=pane-1 Until=[working]", waitOpts)
	}
}

func TestPromptWithNudge_ExhaustsNudges_ReturnsTimeoutError(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, pollTimeoutErr
	}

	_, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if !isPollTimeout(err) {
		t.Fatalf("promptWithNudge() error = %v, want a poll-timeout error", err)
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls = %d, want 1 (only the initial submission)", promptCalls)
	}
	if sendKeysCalls != promptMaxNudges || waitCalls != promptMaxNudges {
		t.Errorf("sendKeysCalls=%d waitCalls=%d, want both = promptMaxNudges (%d)", sendKeysCalls, waitCalls, promptMaxNudges)
	}
}

func TestPromptWithNudge_FastCompletionBeforeWorking_ReturnsSuccessImmediately(t *testing.T) {
	t.Parallel()
	var promptCalls, sendKeysCalls, waitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if len(opts.Until) != 3 || opts.Until[0] != "working" || opts.Until[1] != "idle" || opts.Until[2] != "done" {
			t.Errorf("prompt Until = %v, want [working idle done] (start detection accepts either)", opts.Until)
		}
		return herdr.Agent{AgentStatus: "done"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		sendKeysCalls++
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "hello",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 300_000,
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptCalls != 1 || sendKeysCalls != 0 || waitCalls != 0 {
		t.Errorf("promptCalls=%d sendKeysCalls=%d waitCalls=%d, want 1/0/0 (a final state observed before working returns immediately)", promptCalls, sendKeysCalls, waitCalls)
	}
}

func TestPromptWithNudge_StartConfirmed_WaitsForCompletionWithCallersTimeout(t *testing.T) {
	t.Parallel()
	var promptCalls int
	var completionWaitOpts herdr.AgentWaitOptions
	var completionWaitCalls int
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptCalls++
		if opts.TimeoutMs != promptNudgeGraceMs {
			t.Errorf("prompt TimeoutMs = %d, want the short grace window %d, not the caller's completion timeout", opts.TimeoutMs, promptNudgeGraceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		t.Fatal("sendKeys should not be called: start was observed within the grace window")
		return nil
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 300_000,
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptCalls != 1 {
		t.Errorf("promptCalls = %d, want 1 (the prompt is submitted exactly once)", promptCalls)
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 300_000 {
		t.Errorf("completion wait TimeoutMs = %d, want the caller's own 300000ms, not clobbered by the start-detection grace window", completionWaitOpts.TimeoutMs)
	}
	if len(completionWaitOpts.Until) != 2 || completionWaitOpts.Until[0] != "idle" || completionWaitOpts.Until[1] != "done" {
		t.Errorf("completion wait Until = %v, want [idle done]", completionWaitOpts.Until)
	}
}

func TestPromptWithNudge_SendKeysFails_ReturnsErrorImmediately(t *testing.T) {
	t.Parallel()
	sendKeysErr := errors.New("pane not found")
	var waitCalls int

	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		return sendKeysErr
	}
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	_, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target: "pane-1",
		Text:   "hello",
		Wait:   true,
		Until:  []string{"working"},
	})
	if err == nil || !errors.Is(err, sendKeysErr) {
		t.Fatalf("promptWithNudge() error = %v, want it to wrap %v", err, sendKeysErr)
	}
	if waitCalls != 0 {
		t.Errorf("waitCalls = %d, want 0 (should not wait after a failed nudge)", waitCalls)
	}
}

func TestPromptWithNudge_SlowStart_CompletionGetsRemainingBudget(t *testing.T) {
	t.Parallel()
	promptAttempts := 0
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		promptAttempts++
		return herdr.Agent{}, pollTimeoutErr
	}
	sendKeys := func(target string, keys ...string) error {
		return nil
	}
	var completionWaitOpts herdr.AgentWaitOptions
	var completionWaitCalls, nudgeWaitCalls int
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		if slices.Contains(opts.Until, "working") {
			nudgeWaitCalls++
			return herdr.Agent{AgentStatus: "working"}, nil
		}
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	// 3s elapses between the deadline being captured and the first
	// start/nudge attempt (simulating a slow submit); the clock holds
	// steady after that.
	agent, err := promptWithNudge(prompt, sendKeys, wait, stepNow(3_000*time.Millisecond))(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 10_000,
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if promptAttempts != 1 {
		t.Errorf("promptAttempts = %d, want 1 (the prompt is submitted exactly once)", promptAttempts)
	}
	if nudgeWaitCalls != 1 {
		t.Errorf("nudgeWaitCalls = %d, want 1", nudgeWaitCalls)
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 7_000 {
		t.Errorf("completion wait TimeoutMs = %d, want 7000 (the original 10000ms minus the 3000ms already spent on start)", completionWaitOpts.TimeoutMs)
	}
}

func TestPromptWithNudge_CompletionDeadlineAlreadyExpired_ReturnsTimeoutWithoutWaiting(t *testing.T) {
	t.Parallel()
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		t.Fatal("sendKeys should not be called: start succeeded on the first attempt")
		return nil
	}
	waitCalls := 0
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		waitCalls++
		return herdr.Agent{}, nil
	}

	// 1s elapses before the start attempt, then a further 5s before the
	// completion-budget check, exhausting the caller's 5s deadline.
	_, err := promptWithNudge(prompt, sendKeys, wait, stepNow(1_000*time.Millisecond, 5_000*time.Millisecond))(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 5_000,
	})
	if !isPollTimeout(err) {
		t.Fatalf("promptWithNudge() error = %v, want a poll-timeout error reporting the overall deadline", err)
	}
	if waitCalls != 0 {
		t.Errorf("waitCalls = %d, want 0 (an already-expired deadline must not wait, unbounded or otherwise)", waitCalls)
	}
}

func TestPromptWithNudge_ZeroTimeout_BoundedStartThenUnlimitedCompletion(t *testing.T) {
	t.Parallel()
	prompt := func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if opts.TimeoutMs != promptNudgeGraceMs {
			t.Errorf("prompt TimeoutMs = %d, want the bounded grace window %d even with no caller deadline", opts.TimeoutMs, promptNudgeGraceMs)
		}
		return herdr.Agent{AgentStatus: "working"}, nil
	}
	sendKeys := func(target string, keys ...string) error {
		return nil
	}
	var completionWaitOpts herdr.AgentWaitOptions
	completionWaitCalls := 0
	wait := func(opts herdr.AgentWaitOptions) (herdr.Agent, error) {
		completionWaitCalls++
		completionWaitOpts = opts
		return herdr.Agent{AgentStatus: "done"}, nil
	}

	agent, err := promptWithNudge(prompt, sendKeys, wait, fixedNow())(herdr.AgentPromptOptions{
		Target:    "pane-1",
		Text:      "/compact",
		Wait:      true,
		Until:     []string{"idle", "done"},
		TimeoutMs: 0,
	})
	if err != nil {
		t.Fatalf("promptWithNudge() error = %v", err)
	}
	if agent.AgentStatus != "done" {
		t.Errorf("agent.AgentStatus = %q, want %q", agent.AgentStatus, "done")
	}
	if completionWaitCalls != 1 {
		t.Fatalf("completion wait calls = %d, want 1", completionWaitCalls)
	}
	if completionWaitOpts.TimeoutMs != 0 {
		t.Errorf("completion wait TimeoutMs = %d, want 0 (unlimited: a zero caller timeout means no deadline)", completionWaitOpts.TimeoutMs)
	}
}

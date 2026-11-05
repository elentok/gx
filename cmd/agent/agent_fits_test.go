package agent

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/nativerunner/holder"
)

func fitsDeps(stdout *bytes.Buffer, used int) Deps {
	d := agentTestDeps(stdout, map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1"})
	d.ReadClaudeOccupancy = func(string, string) (int, bool, error) { return used, true, nil }
	return d
}

func TestExecute_AgentContextWindow_Fits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		step     string
		used     int
		wantExit int
	}{
		{"spec", 100_000, 0},
		{"spec", 110_000, 1},
		{"review", 109_999, 0},
		{"review", 110_000, 1},
		{"resolve-tickets", 79_999, 0},
		{"resolve-tickets", 80_000, 1},
		{"chart", 69_999, 0},
		{"chart", 70_000, 1},
		{"grill-findings", 0, 0}, // a blank session always fits
		{"create-tickets", 0, 0},
		{"branch", 0, 0},
		{"create-tickets", 49_999, 0},
		{"create-tickets", 50_000, 1},
		{"done", 129_000, 0},
		{"done", 500_000, 0},
	}
	for _, tc := range cases {
		var stdout bytes.Buffer
		err := execute(t, []string{"agent", "context-window", "--fits", tc.step}, fitsDeps(&stdout, tc.used))
		got := 0
		var exitErr *cmdbase.ExitError
		if errors.As(err, &exitErr) {
			got = exitErr.Code
		} else if err != nil {
			t.Fatalf("%s@%d: %v", tc.step, tc.used, err)
		}
		if got != tc.wantExit {
			t.Errorf("%s@%d: exit %d, want %d", tc.step, tc.used, got, tc.wantExit)
		}
	}
}

func TestExecute_AgentContextWindow_FitsPrintsUsedGrowthCeiling(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	if err := execute(t, []string{"agent", "context-window", "--fits", "spec"}, fitsDeps(&stdout, 100_000)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"100000", "25000", "130000"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q missing %q", stdout.String(), want)
		}
	}
}

func TestExecute_AgentContextWindow_FitsGrillFindingsAliases(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"grill-findings", "create-tickets", "branch"} {
		var out bytes.Buffer
		_ = execute(t, []string{"agent", "context-window", "--fits", step}, fitsDeps(&out, 10_000))
		if !strings.Contains(out.String(), "80000") {
			t.Errorf("%s: stdout %q missing the 80000 growth", step, out.String())
		}
	}
}

func TestExecute_AgentContextWindow_FitsUnknownStepErrors(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := execute(t, []string{"agent", "context-window", "--fits", "bogus"}, fitsDeps(&stdout, 1))
	var exitErr *cmdbase.ExitError
	if err == nil || errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want a plain error", err)
	}
}

func fitsExit(t *testing.T, args []string, d Deps) (int, error) {
	t.Helper()
	err := execute(t, args, d)
	var exitErr *cmdbase.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, nil
	}
	return 0, err
}

func TestExecute_AgentContextWindow_FitsSwitchOffStops(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 1_000)
	env := map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1", holder.AgentSessionEnv: "gx-1"}
	d.Getenv = func(k string) string { return env[k] }
	d.ReadAutoAdvance = func(id string) (bool, error) { return id != "gx-1", nil }
	code, err := fitsExit(t, []string{"agent", "context-window", "--fits", "spec"}, d)
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v, want exit 1", code, err)
	}
}

func TestExecute_AgentContextWindow_FitsSwitchOnUsesTable(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 1_000)
	env := map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1", holder.AgentSessionEnv: "gx-1"}
	d.Getenv = func(k string) string { return env[k] }
	d.ReadAutoAdvance = func(string) (bool, error) { return true, nil }
	code, err := fitsExit(t, []string{"agent", "context-window", "--fits", "spec"}, d)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v, want exit 0", code, err)
	}
}

func TestExecute_AgentContextWindow_FitsSwitchUnreachableErrors(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 1_000)
	env := map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1", holder.AgentSessionEnv: "gx-1"}
	d.Getenv = func(k string) string { return env[k] }
	d.ReadAutoAdvance = func(string) (bool, error) { return false, errors.New("no server") }
	code, err := fitsExit(t, []string{"agent", "context-window", "--fits", "spec"}, d)
	if err == nil || code != 0 {
		t.Fatalf("code=%d err=%v, want a plain error", code, err)
	}
}

func TestExecute_AgentContextWindow_FitsCodexSwitchOffStops(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 0)
	env := map[string]string{holder.AgentSessionEnv: "gx-1"}
	d.Getenv = func(k string) string { return env[k] }
	d.VerifyCodexSession = func(string, string) (bool, error) { return true, nil }
	d.ReadCodexContext = func(string, string) (int, bool, error) { return 1_000, true, nil }
	d.ReadAutoAdvance = func(string) (bool, error) { return false, nil }
	code, err := fitsExit(t, []string{"agent", "context-window", "--agent", "codex", "--session", "c-1", "--fits", "spec"}, d)
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v, want exit 1", code, err)
	}
}

func freshCodexDeps(stdout *bytes.Buffer, env map[string]string) Deps {
	d := agentTestDeps(stdout, env)
	d.VerifyCodexSession = func(string, string) (bool, error) { return true, nil }
	d.ReadCodexContext = func(string, string) (int, bool, error) { return 0, false, nil }
	return d
}

func TestExecute_AgentContextWindow_FitsFreshCodexSessionUsesZero(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := freshCodexDeps(&stdout, nil)
	code, err := fitsExit(t, []string{"agent", "context-window", "--agent", "codex", "--session", "c-1", "--fits", "spec"}, d)
	if err != nil || code != 0 || !strings.Contains(stdout.String(), "used=0 ") {
		t.Fatalf("code=%d err=%v out=%q, want exit 0 with used=0", code, err, stdout.String())
	}
	stdout.Reset()
	code, err = fitsExit(t, []string{"agent", "context-window", "--agent", "codex", "--session", "c-1", "--fits", "grill-findings"}, d)
	if err != nil || code != 0 {
		t.Fatalf("grill-findings: code=%d err=%v, want a blank session to fit", code, err)
	}
}

func TestExecute_AgentContextWindow_PlainFreshCodexSessionStillErrors(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	err := execute(t, []string{"agent", "context-window", "--agent", "codex", "--session", "c-1"}, freshCodexDeps(&stdout, nil))
	if err == nil || !strings.Contains(err.Error(), "no context usage yet") {
		t.Fatalf("err = %v, want the no-context-usage error", err)
	}
}

func TestExecute_AgentContextWindow_FitsClaudeMissingTranscriptIsAnError(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 0)
	d.ReadClaudeOccupancy = func(string, string) (int, bool, error) { return 0, false, nil }
	code, err := fitsExit(t, []string{"agent", "context-window", "--fits", "spec"}, d)
	if err == nil || code != 0 {
		t.Fatalf("code=%d err=%v, want a plain error", code, err)
	}
}

func TestExecute_AgentContextWindow_BothEnvVarsPickTheGXSessionHarness(t *testing.T) {
	t.Parallel()
	env := map[string]string{"CLAUDE_CODE_SESSION_ID": "sess-1", "CODEX_THREAD_ID": "c-1", holder.AgentSessionEnv: "gx-1"}
	var stdout bytes.Buffer
	d := freshCodexDeps(&stdout, env)
	d.ReadCodexContext = func(string, string) (int, bool, error) { return 42, true, nil }
	d.ReadSessionHarness = func(id string) (string, error) {
		if id != "gx-1" {
			t.Errorf("harness read for %q, want gx-1", id)
		}
		return "codex", nil
	}
	if err := execute(t, []string{"agent", "context-window"}, d); err != nil || strings.TrimSpace(stdout.String()) != "42" {
		t.Fatalf("out=%q err=%v, want the Codex count 42", stdout.String(), err)
	}

	// Without a gx session the ambiguity stays.
	delete(env, holder.AgentSessionEnv)
	if err := execute(t, []string{"agent", "context-window"}, d); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v, want the ambiguity error", err)
	}
}

func TestExecute_AgentContextWindow_FitsReadErrorIsNotChain(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fitsDeps(&stdout, 0)
	d.ReadClaudeOccupancy = func(string, string) (int, bool, error) { return 0, false, errors.New("boom") }
	var exitErr *cmdbase.ExitError
	err := execute(t, []string{"agent", "context-window", "--fits", "spec"}, d)
	if err == nil || errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want a plain error (caller treats any failure as stop)", err)
	}
}

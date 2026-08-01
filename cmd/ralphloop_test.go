package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func TestNewRalphLoopCmd_SkillFlagDefaultsToImplement(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	flag := cmd.Flags().Lookup("skill")
	if flag == nil {
		t.Fatal("missing --skill flag")
	}
	if flag.DefValue != "implement" {
		t.Errorf("--skill default = %q, want %q", flag.DefValue, "implement")
	}
}

func TestNewRalphLoopCmd_MaxParallelFlagDefaultsToTwo(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	flag := cmd.Flags().Lookup("max-parallel")
	if flag == nil {
		t.Fatal("missing --max-parallel flag")
	}
	if flag.DefValue != "2" {
		t.Errorf("--max-parallel default = %q, want %q", flag.DefValue, "2")
	}
}

func TestNewRalphLoopCmd_RequiresExactlyOneArg(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	if err := cmd.Args(cmd, nil); err == nil {
		t.Error("Args(no args) = nil, want error")
	}
	if err := cmd.Args(cmd, []string{"epic", "extra"}); err == nil {
		t.Error("Args(two args) = nil, want error")
	}
	if err := cmd.Args(cmd, []string{"epic"}); err != nil {
		t.Errorf("Args(one arg) = %v, want nil", err)
	}
}

func TestRunRalphLoop_NotInGitRepo_ReturnsError(t *testing.T) {
	nonRepoDir := t.TempDir()
	d := deps{getwd: func() (string, error) { return nonRepoDir, nil }}

	err := runRalphLoop("some-epic", "claude", "implement", 2, 150_000, d)
	if err == nil {
		t.Fatal("runRalphLoop() error = nil, want error outside a git repo")
	}
	if !strings.Contains(err.Error(), "no git repo found") {
		t.Errorf("runRalphLoop() error = %v, want a no-git-repo error", err)
	}
}

func TestNewRalphLoopCmd_AgentFlagDefaultsToClaude(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	flag := cmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("missing --agent flag")
	}
	if flag.DefValue != "claude" {
		t.Errorf("--agent default = %q, want %q", flag.DefValue, "claude")
	}
}

func TestNewRalphLoopCmd_AgentFlagValidatesSupportedAgents(t *testing.T) {
	for _, tc := range []struct {
		agent   string
		wantErr bool
	}{
		{agent: "claude"},
		{agent: "codex"},
		{agent: "other", wantErr: true},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			cmd := newRalphLoopCmd(deps{})
			if err := cmd.Flags().Set("agent", tc.agent); err != nil {
				t.Fatalf("setting --agent: %v", err)
			}
			err := cmd.PreRunE(cmd, []string{"epic"})
			if (err != nil) != tc.wantErr {
				t.Errorf("PreRunE() error = %v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

// TestRunRalphLoop_NonTerminalStdout_RunsHeadless confirms runRalphLoop's
// isTerminalWriter gate (cmd/spinner.go) routes a non-terminal stdout (a
// bytes.Buffer, same as CI/piped output) to the unchanged headless
// ralphloop.Run path rather than trying to launch the standalone TUI, which
// would hang waiting on a real terminal. An epic with no tickets makes
// ralphloop.Run return immediately with its "nothing to do" message, so this
// also confirms the headless writer is still wired to d.stdout.
func TestRunRalphLoop_NonTerminalStdout_RunsHeadless(t *testing.T) {
	dir := testutil.TempRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".scratch", "empty-epic", "issues"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	restoreWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(restoreWd) })

	var out bytes.Buffer
	d := deps{getwd: os.Getwd, stdout: &out}
	if err := runRalphLoop("empty-epic", "claude", "implement", 2, 150_000, d); err != nil {
		t.Fatalf("runRalphLoop() error = %v", err)
	}
	if !strings.Contains(out.String(), "no tickets found") {
		t.Errorf("output = %q, want the headless no-tickets message", out.String())
	}
}

func TestNewRalphLoopCmd_SmartZoneFlagDefaultsTo150000(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	flag := cmd.Flags().Lookup("smart-zone")
	if flag == nil {
		t.Fatal("missing --smart-zone flag")
	}
	if flag.DefValue != "150000" {
		t.Errorf("--smart-zone default = %q, want %q", flag.DefValue, "150000")
	}
}

func TestNewRalphLoopCmd_HasResumeSubcommand(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	resume, _, err := cmd.Find([]string{"resume", "some-epic"})
	if err != nil {
		t.Fatalf("Find(resume) error = %v", err)
	}
	if resume.Name() != "resume" {
		t.Errorf("Find(resume) = %q, want the resume subcommand", resume.Name())
	}
}

func TestNewRalphLoopCmd_HasReportSubcommand(t *testing.T) {
	cmd := newRalphLoopCmd(deps{})
	report, _, err := cmd.Find([]string{"report", "some-epic"})
	if err != nil {
		t.Fatalf("Find(report) error = %v", err)
	}
	if report.Name() != "report" {
		t.Errorf("Find(report) = %q, want the report subcommand", report.Name())
	}
}

func TestRunRalphLoopReport_NoRunLog_PrintsNoOpMessage(t *testing.T) {
	dir := t.TempDir()
	restoreWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(restoreWd) })

	var out bytes.Buffer
	d := deps{stdout: &out}
	if err := runRalphLoopReport("some-epic", d); err != nil {
		t.Fatalf("runRalphLoopReport() error = %v", err)
	}
	if !strings.Contains(out.String(), "some-epic") {
		t.Errorf("output = %q, want it to mention the epic name", out.String())
	}
}

func TestRunRalphLoopResume_WritesSignalAndReportsIt(t *testing.T) {
	dir := t.TempDir()
	restoreWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(restoreWd) })

	if err := os.MkdirAll(filepath.Join(dir, ".scratch", "some-epic"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	var out bytes.Buffer
	d := deps{stdout: &out}
	if err := runRalphLoopResume("some-epic", d); err != nil {
		t.Fatalf("runRalphLoopResume() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".scratch", "some-epic", "resume-signal")); err != nil {
		t.Errorf("resume signal file not created: %v", err)
	}
	if !strings.Contains(out.String(), "some-epic") {
		t.Errorf("output = %q, want it to mention the epic name", out.String())
	}
}

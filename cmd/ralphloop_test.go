package cmd

import (
	"strings"
	"testing"
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

	err := runRalphLoop("some-epic", "implement", 2, d)
	if err == nil {
		t.Fatal("runRalphLoop() error = nil, want error outside a git repo")
	}
	if !strings.Contains(err.Error(), "no git repo found") {
		t.Errorf("runRalphLoop() error = %v, want a no-git-repo error", err)
	}
}

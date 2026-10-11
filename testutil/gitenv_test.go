package testutil

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInit_TurnsOffGitAutoMaintenance(t *testing.T) {
	repo := TempRepo(t)
	out, err := exec.Command("git", "-C", repo, "config", "--get", "maintenance.auto").Output()
	if err != nil {
		t.Fatalf("git config: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "false" {
		t.Fatalf("maintenance.auto = %q, want false", got)
	}
}

func TestAppendGitConfig_KeepsExistingEntriesAndSkipsDuplicates(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_0", "x")
	t.Setenv("GIT_CONFIG_KEY_1", "")
	t.Setenv("GIT_CONFIG_VALUE_1", "")

	appendGitConfig("maintenance.auto", "false")
	appendGitConfig("maintenance.auto", "false")

	out, err := exec.Command("git", "config", "--get-regexp", `^(user\.name|maintenance\.auto)$`).Output()
	if err != nil {
		t.Fatalf("git config: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "user.name x") || strings.Count(got, "maintenance.auto false") != 1 {
		t.Fatalf("git sees:\n%s", got)
	}
}

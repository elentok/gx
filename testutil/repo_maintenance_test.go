package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Test repos carry maintenance.auto=false in their own config, so it holds
// without any GIT_CONFIG_* in the process env.
func TestTempRepos_TurnOffGitAutoMaintenanceInRepoConfig(t *testing.T) {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_CONFIG_") {
			env = append(env, kv)
		}
	}
	for name, repo := range map[string]string{
		"TempRepo":     TempRepo(t),
		"TempBareRepo": TempBareRepo(t),
	} {
		cmd := exec.Command("git", "-C", repo, "config", "--get", "maintenance.auto")
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: git config: %v", name, err)
		}
		if got := strings.TrimSpace(string(out)); got != "false" {
			t.Errorf("%s: maintenance.auto = %q, want false", name, got)
		}
	}
}

package git

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestWithConfigEnv_KeepsExistingEntries(t *testing.T) {
	base := []string{"A=1", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=x"}
	env := WithMaintenanceOff(base)

	cmd := exec.Command("git", "config", "--get-regexp", `^(user\.name|maintenance\.auto)$`)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "user.name x") || !strings.Contains(got, "maintenance.auto false") {
		t.Fatalf("git sees:\n%s", got)
	}
	if !slices.Contains(env, "A=1") {
		t.Errorf("unrelated entry dropped: %v", env)
	}
	if len(base) != 4 {
		t.Errorf("input env changed: %v", base)
	}
}

func TestWithConfigEnv_SkipsDuplicateKey(t *testing.T) {
	once := WithMaintenanceOff(nil)
	twice := WithMaintenanceOff(once)
	if !slices.Equal(once, twice) {
		t.Fatalf("second call changed env:\n%v\n%v", once, twice)
	}
	if !slices.Contains(once, "GIT_CONFIG_COUNT=1") {
		t.Fatalf("env = %v", once)
	}
}

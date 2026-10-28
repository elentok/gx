package ralphloop

import (
	"strings"
	"testing"
)

// A run without a ScratchDir fails instead of resolving a ticket root from
// the process env and config.
func TestRun_ScratchDirIsRequired(t *testing.T) {
	t.Parallel()
	d, _, _ := fakeDeps()
	err := Run(RunOptions{EpicName: "my-epic", Skill: "implement", RepoDir: t.TempDir()}, d, noopEventSink{})
	if err == nil || !strings.Contains(err.Error(), "ScratchDir is required") {
		t.Fatalf("Run() error = %v, want a ScratchDir-required error", err)
	}
}

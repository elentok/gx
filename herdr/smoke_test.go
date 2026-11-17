package herdr

import (
	"os"
	"testing"
)

// A CLI with an explicit Env finds herdr on that Env's PATH and runs it as a
// real child process: the process boundary end to end. The process PATH is
// emptied so only the stub can answer.
func TestCLI_ConfiguredEnvFindsStubHerdr(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HERDR_BIN_PATH", "")
	old := fallbackDirs
	fallbackDirs = nil
	t.Cleanup(func() { fallbackDirs = old })

	cli, _ := stubHerdr(t, map[string]reply{
		"workspace list": {out: `{"result":{"workspaces":[{"workspace_id":"ws-1","label":"mylabel"}]}}`},
	})
	cli.Env = append(os.Environ(), cli.Env...)

	id, err := cli.FindWorkspace("mylabel")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "ws-1" {
		t.Fatalf("FindWorkspace() = %q, want %q", id, "ws-1")
	}
}

package herdr

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/herdrfake"
)

// A CLI with an explicit Env finds herdr on that Env's PATH and runs it with
// that Env, while the process environment is untouched. This is the process
// boundary itself, end to end, with no faked runCommand.
func TestCLI_ConfiguredEnvFindsFakeHerdr(t *testing.T) {
	_, env := herdrfake.StartEnv(t, func(argv []string) ([]byte, int) {
		if len(argv) != 2 || argv[0] != "workspace" || argv[1] != "list" {
			t.Errorf("argv = %v, want [workspace list]", argv)
			return herdrfake.CommandError("unexpected command")
		}
		return herdrfake.Result(map[string]any{
			"workspaces": []map[string]string{{"workspace_id": "ws-1", "label": "mylabel"}},
		})
	})

	id, err := CLI{Env: append(os.Environ(), env...)}.FindWorkspace("mylabel")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "ws-1" {
		t.Fatalf("FindWorkspace() = %q, want %q", id, "ws-1")
	}
	if got := os.Getenv("GX_HERDR_FAKE_HELPER_SOCKET"); got != "" {
		t.Fatalf("StartEnv changed the process environment: socket = %q", got)
	}
}

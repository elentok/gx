package herdr

import (
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/herdrfake"
)

func TestPaneShellPID_ThroughFakeHerdrProcessBoundary(t *testing.T) {
	var gotArgv []string
	herdrfake.Start(t, func(argv []string) ([]byte, int) {
		gotArgv = argv
		return herdrfake.Result(map[string]any{
			"process_info": map[string]any{"shell_pid": 4242, "foreground_processes": []any{}},
		})
	})
	pid, err := PaneShellPID("w1:p2")
	if err != nil {
		t.Fatalf("PaneShellPID() error = %v", err)
	}
	if pid != 4242 {
		t.Errorf("PaneShellPID() = %d, want 4242", pid)
	}
	if want := "pane process-info --pane w1:p2"; strings.Join(gotArgv, " ") != want {
		t.Errorf("argv = %q, want %q", gotArgv, want)
	}
}

func TestPaneShellPID_HerdrFailureIsAnError(t *testing.T) {
	herdrfake.Start(t, func([]string) ([]byte, int) { return herdrfake.CommandError("boom") })
	if _, err := PaneShellPID("w1:p2"); err == nil {
		t.Fatal("PaneShellPID() error = nil, want error")
	}
}

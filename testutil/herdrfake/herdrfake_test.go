package herdrfake

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	RunHelperProcess()
	os.Exit(m.Run())
}

func TestStart_HelperReceivesArgvAndReturnsResult(t *testing.T) {
	var gotArgv []string
	Start(t, func(argv []string) ([]byte, int) {
		gotArgv = argv
		return Result(map[string]string{"ok": "yes"})
	})

	out, err := exec.Command("herdr", "workspace", "list").CombinedOutput()
	if err != nil {
		t.Fatalf("herdr workspace list: %v\n%s", err, out)
	}
	if got, want := string(out), `{"result":{"ok":"yes"}}`; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if len(gotArgv) != 2 || gotArgv[0] != "workspace" || gotArgv[1] != "list" {
		t.Errorf("handler argv = %v, want [workspace list]", gotArgv)
	}
}

func TestStart_CommandErrorNonZeroExit(t *testing.T) {
	Start(t, func(argv []string) ([]byte, int) {
		return CommandError("boom")
	})

	out, err := exec.Command("herdr", "x").CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil error")
	}
	if !strings.Contains(string(out), "boom") {
		t.Errorf("output = %q, want it to contain %q", out, "boom")
	}
}

func TestStart_PanicsAfterParallel(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Error("Start() after t.Parallel() did not panic")
			}
		}()
		Start(t, func(argv []string) ([]byte, int) { return nil, 0 })
	})
}

package herdrfake

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	RunHelperProcess()
	os.Exit(m.Run())
}

// herdrIn is a `herdr` invocation under the env StartEnv returned. exec.Command
// resolves a bare name on the process PATH, so the fake is found on env's.
func herdrIn(env []string, args ...string) *exec.Cmd {
	var dir string
	for _, kv := range env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			dir = filepath.SplitList(path)[0]
		}
	}
	cmd := exec.Command(filepath.Join(dir, "herdr"), args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd
}

func TestStartEnv_HelperReceivesArgvAndReturnsResult(t *testing.T) {
	var gotArgv []string
	_, env := StartEnv(t, func(argv []string) ([]byte, int) {
		gotArgv = argv
		return Result(map[string]string{"ok": "yes"})
	})

	out, err := herdrIn(env, "workspace", "list").CombinedOutput()
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

func TestStartEnv_CommandErrorNonZeroExit(t *testing.T) {
	_, env := StartEnv(t, func(argv []string) ([]byte, int) {
		return CommandError("boom")
	})

	out, err := herdrIn(env, "x").CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil error")
	}
	if !strings.Contains(string(out), "boom") {
		t.Errorf("output = %q, want it to contain %q", out, "boom")
	}
}

func TestStartEnv_LeavesProcessEnvAlone(t *testing.T) {
	StartEnv(t, func(argv []string) ([]byte, int) { return nil, 0 })

	if got := os.Getenv(helperSocketEnv); got != "" {
		t.Errorf("%s = %q in the process env, want it unset", helperSocketEnv, got)
	}
}

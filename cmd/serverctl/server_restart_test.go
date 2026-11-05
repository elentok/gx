package serverctl

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
)

// testEnv is a cmdbase.Env on a fresh temp HOME with discarded output.
func testEnv(t *testing.T) cmdbase.Env {
	t.Helper()
	return cmdbase.Env{Env: cmdtest.TempEnv(t), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
}

func testDeps(t *testing.T) Deps { return Deps{Env: testEnv(t)} }

const legacyStatusTicket = "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n"

func TestServerRestart_MigrateFlagIsUnknown(t *testing.T) {
	t.Parallel()
	err := cmdtest.Run(t, testEnv(t), []string{"server", "restart", "--migrate"}, New(testDeps(t)))
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("err = %v, want unknown flag", err)
	}
}

func TestServerRestart_HelpHasNoMigrate(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	env := testEnv(t)
	env.Stdout, env.Stderr = &out, &out
	if err := cmdtest.Run(t, env, []string{"server", "restart", "--help"}, New(Deps{Env: env})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "--migrate") {
		t.Errorf("help mentions --migrate:\n%s", out.String())
	}
}

func TestServerRestart_ContinueFlagIsAcceptedAndHidden(t *testing.T) {
	t.Parallel()
	c, _, err := New(testDeps(t)).Find([]string{"restart"})
	if err != nil {
		t.Fatal(err)
	}
	f := c.Flags().Lookup("continue")
	if f == nil || !f.Hidden {
		t.Fatalf("continue flag = %+v, want a hidden flag", f)
	}
}

func TestOpenRestartLog_AppendsToRestartLogInStateDir(t *testing.T) {
	t.Parallel()
	dirs := config.DirsFrom(cmdtest.TempEnv(t))
	stateDir, err := dirs.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"one\n", "two\n"} {
		f, err := openRestartLog(dirs)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	got, err := os.ReadFile(migratefail.RestartLogPath(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one\ntwo\n" {
		t.Errorf("restart.log = %q", got)
	}
}

func TestRestartLogPath_IsFixedFileInStateDir(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	if got, want := migratefail.RestartLogPath(state), filepath.Join(state, "restart.log"); got != want {
		t.Errorf("RestartLogPath = %s, want %s", got, want)
	}
}

func TestRestartSteps_FailedStopDoesNotStart(t *testing.T) {
	t.Parallel()
	var calls []string
	step := func(name string, err error) func() error {
		return func() error { calls = append(calls, name); return err }
	}
	if err := restartSteps(step("stop", errors.New("boom")), step("start", nil)); err == nil {
		t.Fatal("want the stop error")
	}
	if got := strings.Join(calls, ","); got != "stop" {
		t.Errorf("calls = %s, want stop", got)
	}
}

func TestRestartSteps_StopThenStart(t *testing.T) {
	t.Parallel()
	var calls []string
	step := func(name string) func() error {
		return func() error { calls = append(calls, name); return nil }
	}
	if err := restartSteps(step("stop"), step("start")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "stop,start" {
		t.Errorf("calls = %s, want stop,start", got)
	}
}

package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/migratefail"
)

// writeStoreProject makes <store>/<name> a project holding one epic with the
// given tickets.
func writeStoreProject(t *testing.T, store, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	cmdtest.WriteMigrateFixtureEpic(t, dir, "epic", files)
	return dir
}

const legacyStatusTicket = "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n"

func TestServerRestart_MigrateFlagIsUnknown(t *testing.T) {
	t.Parallel()
	root := newRootCmd(deps{env: tempEnv(t)})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"server", "restart", "--migrate"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("err = %v, want unknown flag", err)
	}
}

func TestServerRestart_HelpHasNoMigrate(t *testing.T) {
	t.Parallel()
	root := newRootCmd(deps{env: tempEnv(t)})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"server", "restart", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "--migrate") {
		t.Errorf("help mentions --migrate:\n%s", out.String())
	}
}

func TestServerRestart_ContinueFlagIsAcceptedAndHidden(t *testing.T) {
	t.Parallel()
	root := newRootCmd(deps{env: tempEnv(t)})
	c, _, err := root.Find([]string{"server", "restart"})
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
	dirs := config.DirsFrom(tempEnv(t))
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

package codex

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/codexsetup"
)

func testDeps(t *testing.T, stdout, stderr io.Writer) Deps {
	t.Helper()
	return Deps{Env: cmdbase.Env{
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
		Env:    cmdtest.EnvFor(t),
	}}
}

func TestCodexSetup_YesWritesAndPrintsRestart(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, nil)
	if err := cmdtest.Run(t, d.Env, []string{"codex", "setup", "--yes", "--codex-home", home}, New(d)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(codexsetup.RulesPath(home)); err != nil {
		t.Errorf("rules file not written: %v", err)
	}
	if !strings.Contains(stdout.String(), "Restart Codex for the changes to take effect.") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestCodexSetup_HelpListsFlags(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, nil)
	if err := cmdtest.Run(t, d.Env, []string{"codex", "setup", "--help"}, New(d)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, flag := range []string{"--yes", "--codex-home"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Errorf("help lacks %s:\n%s", flag, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "gopls") {
		t.Errorf("help still mentions gopls:\n%s", stdout.String())
	}
}

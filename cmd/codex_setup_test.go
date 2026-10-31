package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/codexsetup"
)

func TestCodexSetup_YesWritesAndPrintsRestart(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	var stdout bytes.Buffer
	d := agentTestDeps(&stdout, nil)
	if err := execute([]string{"codex", "setup", "--yes", "--codex-home", home}, d); err != nil {
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
	d := agentTestDeps(&stdout, nil)
	if err := execute([]string{"codex", "setup", "--help"}, d); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, flag := range []string{"--yes", "--codex-home", "--gopls-server"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Errorf("help lacks %s:\n%s", flag, stdout.String())
		}
	}
}

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
)

// The ticket-store fixtures live in cmdtest; these keep the root tests' short
// names.
// isolateTicketStore returns the store path under the test's root env
// (envFor), which cmdtest's own env does not share.
func isolateTicketStore(t *testing.T) string {
	t.Helper()
	return filepath.Join(envValue(envFor(t), "XDG_DATA_HOME"), "gx", "tickets")
}

func addProject(t *testing.T, store, name, repoPath string) string {
	t.Helper()
	return cmdtest.AddProject(t, store, name, repoPath)
}

const logQuestionLine = cmdtest.LogQuestionLine

func writeLogFile(t *testing.T, ticketPath, content string) {
	t.Helper()
	cmdtest.WriteLogFile(t, ticketPath, content)
}

func writeTicketFile(t *testing.T, path, content string) {
	t.Helper()
	cmdtest.WriteTicketFile(t, path, content)
}

func ensureIssueEpic(t *testing.T, path string) {
	t.Helper()
	cmdtest.EnsureIssueEpic(t, path)
}

const showTicketFixture = "---\nid: \"06\"\nstatus: open\ntype: implement\n---\n\n# 06 — Hello\n\nbody text\n"

// setupShowProject is a project "mine" holding epic/06.
func setupShowProject(t *testing.T) (repo, project string) {
	t.Helper()
	store := isolateTicketStore(t)
	repo = testutil.TempRepo(t)
	project = addProject(t, store, "mine", repo)
	issues := filepath.Join(project, "epic", "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "epic"))
	testutil.WriteFile(t, issues, "06-hello.md", showTicketFixture)
	return repo, project
}

// runIn runs `gx <args>` through the full root with cwd as the working
// directory and returns stdout.
func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := deps{
		stdout: &stdout,
		stderr: bytes.NewBuffer(nil),
		getwd:  func() (string, error) { return cwd, nil },
		env:    envFor(t),
	}
	err := execute(args, d)
	return stdout.String(), err
}

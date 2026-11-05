package ticketsflow

import (
	"bytes"
	"io"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	ticketscmd "github.com/elentok/gx/cmd/tickets"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// The shared fixtures live in cmdtest; these keep the tests' short names.
func envFor(t *testing.T) []string { return cmdtest.EnvFor(t) }

func isolateTicketStore(t *testing.T) string {
	t.Helper()
	return cmdtest.IsolateTicketStore(t)
}

func addProject(t *testing.T, store, name, repoPath string) string {
	t.Helper()
	return cmdtest.AddProject(t, store, name, repoPath)
}

func writeTicketFile(t *testing.T, path, content string) {
	t.Helper()
	cmdtest.WriteTicketFile(t, path, content)
}

func ensureIssueEpic(t *testing.T, path string) {
	t.Helper()
	cmdtest.EnsureIssueEpic(t, path)
}

func writeEpicTicket(t *testing.T, scratchDir, epic, filename, content string) string {
	t.Helper()
	return cmdtest.WriteEpicTicket(t, scratchDir, epic, filename, content)
}

func ticketFrontmatter(id, extra string) string { return cmdtest.TicketFrontmatter(id, extra) }

func setupShowProject(t *testing.T) (repo, project string) {
	t.Helper()
	return cmdtest.SetupShowProject(t)
}

// testDeps builds Deps over the test's isolated env.
func testDeps(t *testing.T, stdout, stderr io.Writer, getwd func() (string, error)) Deps {
	t.Helper()
	return Deps{Env: cmdbase.Env{Stdout: stdout, Stderr: stderr, Getwd: getwd, Env: envFor(t)}, CommandTree: fakeTree}
}

// fakeTree is the small command tree lint-claims checks claims against in
// these tests: `gx tickets add --slug` and `gx tickets publish`.
func fakeTree() *cobra.Command {
	root := &cobra.Command{Use: "gx"}
	tk := &cobra.Command{Use: "tickets"}
	add := &cobra.Command{Use: "add"}
	add.Flags().String("slug", "", "")
	tk.AddCommand(add, &cobra.Command{Use: "publish"})
	root.AddCommand(tk)
	return root
}

// execute runs args against `gx tickets` built from the core verbs plus d's
// flow verbs.
func execute(t *testing.T, args []string, d Deps) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, args, ticketscmd.New(ticketscmd.Deps{Env: d.Env}, Commands(d)...))
}

// runIn runs `gx <args>` with cwd as the working directory and returns stdout.
func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	d := testDeps(t, &stdout, bytes.NewBuffer(nil), func() (string, error) { return cwd, nil })
	err := execute(t, args, d)
	return stdout.String(), err
}

package ticketsmigrate

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

// runMigrate executes args (starting with "tickets", "migrate") under a
// `tickets` parent that holds only the migrate command.
func runMigrate(t *testing.T, args []string, env cmdbase.Env) error {
	t.Helper()
	tickets := &cobra.Command{Use: "tickets"}
	tickets.AddCommand(New(Deps{Env: env}))
	return cmdtest.Run(t, env, args, tickets)
}

// runIn runs `gx <args>` with cwd as the working directory and returns stdout.
func runIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := runMigrate(t, args, cmdbase.Env{
		Stdout: &stdout,
		Getwd:  func() (string, error) { return cwd, nil },
		Env:    cmdtest.EnvFor(t),
	})
	return stdout.String(), err
}

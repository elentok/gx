package serverapi

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdtest"
)

func TestMain(m *testing.M) { cmdtest.Main(m) }

var (
	finishWebSetup           = cmdtest.FinishWebSetup
	finishWebSetupWithSecret = cmdtest.FinishWebSetupWithSecret
)

// serverParent is `gx server` with the API verbs as its children, the path a
// user types.
func serverParent(d Deps) *cobra.Command {
	server := &cobra.Command{Use: "server"}
	server.AddCommand(Commands(d)...)
	return server
}

// execute runs args (starting with "server") against serverParent(d).
func execute(t *testing.T, args []string, d Deps) error {
	t.Helper()
	return cmdtest.Run(t, d.Env, args, serverParent(d))
}

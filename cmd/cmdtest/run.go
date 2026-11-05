package cmdtest

import (
	"io"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/projectlock"
)

// Run executes cmds under a mini gx root with the settings of the real root
// (errors and usage silenced, explicit args, output on the Env writers). It
// replaces the root package's execute in package tests.
func Run(t *testing.T, env cmdbase.Env, args []string, cmds ...*cobra.Command) error {
	t.Helper()
	if args == nil {
		// A nil slice would make cobra fall back to os.Args[1:].
		args = []string{}
	}
	root := &cobra.Command{
		Use:           "gx",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
	}
	root.AddCommand(cmds...)
	root.SetArgs(args)
	if env.Stdin != nil {
		root.SetIn(env.Stdin)
	}
	root.SetOut(writerOrDiscard(env.Stdout))
	root.SetErr(writerOrDiscard(env.Stderr))
	return root.Execute()
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

// SetWaitHook overrides projectlock's global wait hook for one test: the wait
// shortens to delay and the migration notice goes to w. Both are restored in
// t.Cleanup.
func SetWaitHook(t *testing.T, w io.Writer, delay time.Duration) {
	t.Helper()
	restoreDelay := projectlock.SetWaitHookDelayForTest(delay)
	restoreNotice := cmdbase.RegisterMigrationWaitNotice(w)
	t.Cleanup(func() {
		restoreNotice()
		restoreDelay()
	})
}

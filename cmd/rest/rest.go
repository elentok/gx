// Package rest is the `gx rest` verbs: read the Rest engine's status, send one
// active minute and unlock with a sentence, all through the API.
package rest

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
}

// New builds `gx rest` with its `active` and `unlock` verbs.
func New(d Deps) *cobra.Command {
	root := verb(d, &cobra.Command{
		Use:   "rest",
		Short: "show the Rest status: Break and Bedtime periods, warning and work count (asks the server)",
		Args:  cobra.NoArgs,
	}, func(ctx context.Context, cl *apiclient.Client, _ []string) (wire.RestResult, error) {
		st, err := cl.Rest.Status(ctx)
		return wire.RestResult{Status: st}, err
	})
	root.AddCommand(
		verb(d, &cobra.Command{
			Use:   "active",
			Short: "report one active minute, as the browser heartbeat does",
			Args:  cobra.NoArgs,
		}, func(ctx context.Context, cl *apiclient.Client, _ []string) (wire.RestResult, error) {
			return cl.Rest.Active(ctx)
		}),
		verb(d, &cobra.Command{
			Use:   "unlock <sentence>",
			Short: "end the running Rest period by typing the unlock sentence",
			Args:  cobra.MinimumNArgs(1),
		}, func(ctx context.Context, cl *apiclient.Client, args []string) (wire.RestResult, error) {
			return cl.Rest.Unlock(ctx, strings.Join(args, " "))
		}),
	)
	return root
}

// verb gives cmd its --json flag and its run function.
func verb(d Deps, cmd *cobra.Command, do func(context.Context, *apiclient.Client, []string) (wire.RestResult, error)) *cobra.Command {
	var jsonOut bool
	cmd.RunE = func(c *cobra.Command, args []string) error {
		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		cl, err := d.Dial()()
		if err != nil {
			return err
		}
		return run(ctx, cl, c.OutOrStdout(), jsonOut, args, do)
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func run(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, args []string, do func(context.Context, *apiclient.Client, []string) (wire.RestResult, error)) error {
	res, err := do(ctx, cl, args)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.RestResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		return cmdbase.EncodeProvenanceRefusal(w, res, res.Refused)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, line(res.Status))
	return err
}

// line is the short human summary of a status.
func line(st wire.RestStatus) string {
	switch {
	case !st.Enabled:
		return "rest off"
	case st.Period != nil:
		return fmt.Sprintf("%s until %s", st.Period.Kind, st.Period.End.Format("15:04"))
	case st.Warning != nil:
		return fmt.Sprintf("no rest period; %s warning for %s; work %d", st.Warning.Kind, st.Warning.Start.Format("15:04"), st.WorkCount)
	}
	return fmt.Sprintf("no rest period; work %d", st.WorkCount)
}

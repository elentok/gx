package serverapi

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerHoldCmd holds a Design or epic; yes confirms exiting the Design's live sessions.
func newServerHoldCmd(d Deps) *cobra.Command {
	var jsonOut, yes bool
	cmd := &cobra.Command{
		Use:   "hold <epic>",
		Short: "hold a Design or epic (project:epic) so nothing of it starts; a Design's live sessions exit",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runHold(c, d, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.HoldResult, error) {
				return cl.Admin.HoldEpic(ctx, args[0], yes)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm exiting the Design's live sessions")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// newServerUnholdCmd clears a hold. It asks nothing and starts nothing.
func newServerUnholdCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unhold <epic>",
		Short: "clear the hold on a Design or epic (project:epic)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runHold(c, d, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.HoldResult, error) {
				return cl.Admin.UnholdEpic(ctx, args[0])
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runHold(c *cobra.Command, d Deps, jsonOut bool, do func(context.Context, *apiclient.Client) (wire.HoldResult, error)) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := d.Dial()()
	if err != nil {
		return err
	}
	res, err := do(ctx, cl)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.HoldResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	return printHold(c.OutOrStdout(), jsonOut, res)
}

// printHold prints the result; with --json it is the output, even a refusal,
// which otherwise is an error.
func printHold(w io.Writer, jsonOut bool, res wire.HoldResult) error {
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	if len(res.Sessions) > 0 {
		if _, err := fmt.Fprintf(w, "ended sessions: %s\n", strings.Join(res.Sessions, ", ")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "held: %s\n", strings.Join(res.Held, ", "))
	return err
}

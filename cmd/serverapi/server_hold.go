package serverapi

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

// newServerHoldCmd holds Designs or epics; yes confirms exiting a Design's live sessions.
func newServerHoldCmd(d Deps) *cobra.Command {
	var jsonOut, yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "hold <epic>...",
		Short: "hold Designs or epics (project:epic) so nothing of them starts; a Design's live sessions exit",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runHold(c, d, jsonOut, args, func(ctx context.Context, cl *apiclient.Client) (wire.HoldResult, error) {
				return cl.Admin.HoldEpics(ctx, args, yes, dryRun)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm exiting the Designs' live sessions")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run every rule and change nothing; lists the sessions that would exit")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

// newServerUnholdCmd clears holds. It asks nothing and starts nothing.
func newServerUnholdCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unhold <epic>...",
		Short: "clear the hold on Designs or epics (project:epic)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runHold(c, d, jsonOut, args, func(ctx context.Context, cl *apiclient.Client) (wire.HoldResult, error) {
				return cl.Admin.UnholdEpics(ctx, args)
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

func runHold(c *cobra.Command, d Deps, jsonOut bool, targets []string, do func(context.Context, *apiclient.Client) (wire.HoldResult, error)) error {
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
		res = wire.HoldResult{}
		for _, t := range targets {
			res.Results = append(res.Results, notRunning(t))
		}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	return cmdbase.PrintTargets(c.OutOrStdout(), jsonOut, res, res.Results, func(w io.Writer, r wire.TargetResult) error {
		if len(r.Sessions) > 0 {
			verb := "ended"
			if r.Status == wire.TargetFits {
				verb = "would end"
			}
			if _, err := fmt.Fprintf(w, "%s sessions: %s\n", verb, strings.Join(r.Sessions, ", ")); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w, "held: %s\n", strings.Join(res.Held, ", "))
		return err
	})
}

package serverapi

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// newServerEpicsUnarchiveCmd brings archived epics and Designs back; the target is the archive address.
func newServerEpicsUnarchiveCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unarchive <project:epic>...",
		Short: "bring archived epics and Designs back out of the project's .archive",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			res, err := cl.Admin.UnarchiveEpics(ctx, args)
			switch {
			case apiclient.IsNotRunning(err):
				res = wire.TargetsResult{}
				for _, t := range args {
					res.Results = append(res.Results, notRunning(t))
				}
			case err != nil:
				return fmt.Errorf("server write failed: %w", err)
			}
			return cmdbase.PrintTargets(c.OutOrStdout(), jsonOut, res, res.Results, printUnarchived)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

func printUnarchived(w io.Writer, r wire.TargetResult) error {
	line := "unarchived " + r.Target
	if r.NewAddress != "" {
		line += " as " + r.NewAddress
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

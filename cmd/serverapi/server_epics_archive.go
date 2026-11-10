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

// newServerEpicsArchiveCmd archives finished loop epics; yes confirms stopping their live Sessions.
func newServerEpicsArchiveCmd(d Deps) *cobra.Command {
	var jsonOut, yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "archive <project:epic>...",
		Short: "archive finished loop epics into the project's .archive; their live sessions stop",
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
			res, err := cl.Admin.ArchiveEpics(ctx, args, yes, dryRun)
			switch {
			case apiclient.IsNotRunning(err):
				res = wire.TargetsResult{}
				for _, t := range args {
					res.Results = append(res.Results, notRunning(t))
				}
			case err != nil:
				return fmt.Errorf("server write failed: %w", err)
			}
			return cmdbase.PrintTargets(c.OutOrStdout(), jsonOut, res, res.Results, printArchived)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm stopping the epics' live sessions")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "run every rule and change nothing; lists the sessions that would stop")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured results as JSON")
	return cmd
}

func printArchived(w io.Writer, r wire.TargetResult) error {
	verb := "archived"
	if r.Status == wire.TargetFits {
		verb = "would archive"
	}
	line := verb + " " + r.Target
	if r.NewAddress != "" {
		line += " as " + r.NewAddress
	}
	if r.OnTrunk != nil && !*r.OnTrunk {
		line += " (not seen on trunk)"
	}
	if len(r.Sessions) > 0 {
		line += "; sessions: " + strings.Join(r.Sessions, ", ")
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newServerStatsCmd shows the per-day stats the web UI's Stats page draws.
func newServerStatsCmd() *cobra.Command {
	var jsonOut bool
	var days int
	var project string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "show per-day throughput, cost and friction stats through the gx server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return serverRead(c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (server.Stats, error) {
					return cl.Stats(ctx, days, project)
				},
				printStats)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().IntVar(&days, "range", 0, "days to cover: 7, 30 or 90 (default 30)")
	cmd.Flags().StringVar(&project, "project", "", "only this project (default: all projects)")
	return cmd
}

func printStats(w io.Writer, s server.Stats) error {
	if _, err := fmt.Fprintln(w, "date\ttickets_merged\tepics_merged\tdesigns_completed"); err != nil {
		return err
	}
	for i, day := range s.Days {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", day, s.Throughput.TicketsMerged[i], s.Throughput.EpicsMerged[i], s.Throughput.DesignsCompleted[i]); err != nil {
			return err
		}
	}
	return nil
}

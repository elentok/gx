package serverapi

import (
	"context"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newServerStatsCmd shows the per-day stats the web UI's Stats page draws.
func newServerStatsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var days int
	var project string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "show per-day throughput, cost and friction stats through the gx server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.Stats, error) {
					return cl.Reads.Stats(ctx, days, project)
				},
				printStats)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().IntVar(&days, "range", 0, "days to cover: 7, 30 or 90 (default 7)")
	cmd.Flags().StringVar(&project, "project", "", "only this project (default: all projects)")
	return cmd
}

func printStats(w io.Writer, s wire.Stats) error {
	if _, err := fmt.Fprintln(w, "date\ttickets_merged\tepics_merged\tdesigns_completed"); err != nil {
		return err
	}
	for i, day := range s.Days {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", day, s.Throughput.TicketsMerged[i], s.Throughput.EpicsMerged[i], s.Throughput.DesignsCompleted[i]); err != nil {
			return err
		}
	}
	l := s.Launch
	if l.MedianMs == nil || l.ShareUnderTgt == nil {
		_, err := fmt.Fprintln(w, "launch: no data")
		return err
	}
	_, err := fmt.Fprintf(w, "launch median: %.1fs, %.0f%% under %ds (%d launches)\n",
		float64(*l.MedianMs)/1000, *l.ShareUnderTgt*100, l.TargetMs/1000, l.Count)
	return err
}

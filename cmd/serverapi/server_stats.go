package serverapi

import (
	"context"
	"fmt"
	"io"
	"strconv"

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
	var includeDev bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "show per-day throughput, cost and friction stats through the gx server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.Stats, error) {
					return cl.Reads.Stats(ctx, days, project, includeDev)
				},
				func(w io.Writer, s wire.Stats) error { return printStatsDev(w, s, includeDev) })
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.Flags().IntVar(&days, "range", 0, "days to cover: 7, 30 or 90 (default 7)")
	cmd.Flags().StringVar(&project, "project", "", "only this project (default: all projects)")
	cmd.Flags().BoolVar(&includeDev, "include-dev", false, "count dev web performance samples too")
	return cmd
}

func printStats(w io.Writer, s wire.Stats) error { return printStatsDev(w, s, false) }

func printStatsDev(w io.Writer, s wire.Stats, includeDev bool) error {
	if _, err := fmt.Fprintln(w, "date\ttickets_merged\tepics_merged\tdesigns_completed"); err != nil {
		return err
	}
	for i, day := range s.Days {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", day, s.Throughput.TicketsMerged[i], s.Throughput.EpicsMerged[i], s.Throughput.DesignsCompleted[i]); err != nil {
			return err
		}
	}
	l := s.Launch
	var err error
	if l.MedianMs == nil || l.ShareUnderTgt == nil {
		_, err = fmt.Fprintln(w, "launch: no data")
	} else {
		_, err = fmt.Fprintf(w, "launch median: %.1fs, %.0f%% under %ds (%d launches)\n",
			float64(*l.MedianMs)/1000, *l.ShareUnderTgt*100, l.TargetMs/1000, l.Count)
	}
	if err != nil {
		return err
	}
	return printWebPerformance(w, s, includeDev)
}

func printWebPerformance(w io.Writer, s wire.Stats, includeDev bool) error {
	p := s.WebPerformance
	cards := append(append([]wire.StatsWebPerfCard{}, p.HardLoads...), p.RouteChanges...)
	samples := p.BuildStrip != nil
	for _, c := range cards {
		samples = samples || c.Count > 0
	}
	if !samples {
		_, err := fmt.Fprintln(w, "web performance: no samples")
		return err
	}
	dev := "dev samples hidden"
	if includeDev {
		dev = "dev samples included"
	}
	if _, err := fmt.Fprintf(w, "web performance (%d days, %s)\n", s.Range, dev); err != nil {
		return err
	}
	for _, tab := range []struct {
		name  string
		cards []wire.StatsWebPerfCard
	}{{"hard loads", p.HardLoads}, {"route changes", p.RouteChanges}} {
		for _, c := range tab.cards {
			if _, err := fmt.Fprintf(w, "  %s %s: p75 %s, count %d, %s\n",
				tab.name, c.Metric, num(c.P75), c.Count, orDash(c.Status)); err != nil {
				return err
			}
		}
	}
	if b := p.BuildStrip; b != nil {
		_, err := fmt.Fprintf(w, "  build %s (%s): lcp %s, inp %s, js %s\n",
			b.Current.BuildID, b.Current.WebVersion, pct(b.LCPChangePct), pct(b.INPChangePct), pct(b.JSChangePct))
		return err
	}
	return nil
}

func num(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'g', 4, 64)
}

func pct(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%+.1f%%", *v)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// newServerTipsCmd shows the tips the web UI's Tips page lists.
func newServerTipsCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "tips",
		Short: "show the tips for the server machine's OS",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) ([]wire.Tip, error) { return cl.Reads.Tips(ctx) },
				printTips)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func printTips(w io.Writer, tips []wire.Tip) error {
	for _, t := range tips {
		if _, err := fmt.Fprintf(w, "%s\n%s\n", t.Title, t.Body); err != nil {
			return err
		}
		for _, f := range t.Folders {
			if _, err := fmt.Fprintf(w, "  %s\n", f); err != nil {
				return err
			}
		}
	}
	return nil
}

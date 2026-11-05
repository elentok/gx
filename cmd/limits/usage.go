package limits

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

// NewUsage builds `gx usage`.
func NewUsage(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "usage", Short: "usage gate", Args: cobra.NoArgs}
	var jsonOut bool
	override := &cobra.Command{
		Use:       "override [claude|codex]",
		Short:     "let new claims of a harness start until its gating usage window resets (asks the server; default harness when none named)",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude", "codex"},
		RunE: func(c *cobra.Command, args []string) error {
			harness := ""
			if len(args) > 0 {
				harness = args[0]
			}
			return runUsageOverride(d.Dial(), c, harness, jsonOut)
		},
	}
	override.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(override)

	var summaryJSON bool
	summary := &cobra.Command{
		Use:   "summary",
		Short: "show the usage mode, the Claude limit windows and the spend totals (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), summaryJSON, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.UsageSummary, error) {
					return cl.Reads.UsageSummary(ctx)
				},
				printUsageSummary)
		},
	}
	summary.Flags().BoolVar(&summaryJSON, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(summary)
	return cmd
}

func printUsageSummary(w io.Writer, u wire.UsageSummary) error {
	fmt.Fprintf(w, "mode: %s\n", u.Mode)
	for _, row := range []struct {
		name string
		win  *wire.UsageLimitWindow
	}{{"five_hour", u.Limits.FiveHour}, {"seven_day", u.Limits.SevenDay}} {
		if row.win != nil {
			fmt.Fprintf(w, "%s: %.0f%% used, resets at %d\n", row.name, row.win.UsedPercentage, row.win.ResetsAt)
		}
	}
	if u.Codex != nil {
		for _, row := range []struct {
			name string
			win  *wire.UsageLimitWindow
		}{{"codex_five_hour", u.Codex.FiveHour}, {"codex_seven_day", u.Codex.SevenDay}} {
			if row.win != nil {
				fmt.Fprintf(w, "%s: %.0f%% used, resets at %d\n", row.name, row.win.UsedPercentage, row.win.ResetsAt)
			}
		}
	}
	s := u.Spend
	since := func(short bool) string {
		if short {
			return " (since " + s.FirstDay + ")"
		}
		return ""
	}
	fmt.Fprintf(w, "today: $%.2f\nsince sunday: $%.2f%s\nsince the 1st: $%.2f%s\n",
		s.Today, s.SinceSunday, since(s.SinceSundayShort), s.SinceFirst, since(s.SinceFirstShort))
	return nil
}

// runUsageOverride mirrors runBudgetWrite.
func runUsageOverride(dial func() (*apiclient.Client, error), c *cobra.Command, harness string, jsonOut bool) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	res, err := cl.Admin.UsageOverride(ctx, harness)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.UsageOverrideResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	w := c.OutOrStdout()
	if jsonOut {
		return cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "usage gate lifted for the %s window until %s\n", strings.Join(res.Windows, " and "), res.Until.UTC().Format(time.RFC3339))
	return err
}

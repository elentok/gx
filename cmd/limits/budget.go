package limits

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// New builds `gx budget`.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "budget", Short: "daily budget", Args: cobra.NoArgs}
	var jsonOut bool
	status := &cobra.Command{
		Use:   "status",
		Short: "show today's total spend and the budget limits (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return cmdbase.ServerRead(d.Dial(), c.Context(), jsonOut, c.OutOrStdout(),
				func(ctx context.Context, cl *apiclient.Client) (wire.BudgetStatus, error) {
					return cl.Reads.Budget(ctx)
				},
				printBudget)
		},
	}
	status.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	cmd.AddCommand(status)

	var overrideJSON bool
	override := &cobra.Command{
		Use:   "override",
		Short: "lift the budget latches so new work can start (asks the server)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runBudgetOverride(d.Dial(), c, overrideJSON)
		},
	}
	override.Flags().BoolVar(&overrideJSON, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(override)
	cmd.AddCommand(newBudgetIncreaseCmd(d))
	return cmd
}

func newBudgetIncreaseCmd(d Deps) *cobra.Command {
	var soft, hard float64
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "increase --soft|--hard <dollars>",
		Short: "raise a daily limit on the running server until it restarts (edit config.json to keep it)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var req wire.BudgetIncreaseRequest
			if c.Flags().Changed("soft") {
				req.Soft = &soft
			}
			if c.Flags().Changed("hard") {
				req.Hard = &hard
			}
			if req.Soft == nil && req.Hard == nil {
				return fmt.Errorf("pass --soft or --hard")
			}
			return runBudgetWrite(d.Dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.BudgetResult, error) {
				return cl.Admin.BudgetIncrease(ctx, req)
			})
		},
	}
	cmd.Flags().Float64Var(&soft, "soft", 0, "new soft limit in dollars (pauses new starts)")
	cmd.Flags().Float64Var(&hard, "hard", 0, "new hard limit in dollars (stops live agents)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runBudgetOverride mirrors cmdbase.RunServerQueueWrite.
func runBudgetOverride(dial func() (*apiclient.Client, error), c *cobra.Command, jsonOut bool) error {
	return runBudgetWrite(dial, c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (wire.BudgetResult, error) {
		return cl.Admin.BudgetOverride(ctx)
	})
}

// runBudgetWrite runs one budget write: with --json the result or refusal is
// the output; otherwise a refusal is an error.
func runBudgetWrite(dial func() (*apiclient.Client, error), c *cobra.Command, jsonOut bool, write func(context.Context, *apiclient.Client) (wire.BudgetResult, error)) error {
	ctx := c.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cl, err := dial()
	if err != nil {
		return err
	}
	res, err := write(ctx, cl)
	switch {
	case apiclient.IsNotRunning(err):
		res = wire.BudgetResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
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
	return printBudget(w, *res.Budget)
}

func printBudget(w io.Writer, b wire.BudgetStatus) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "budget %s: %s\n", b.Day, budgetState(b))
	fmt.Fprintf(tw, "  spent\t%s\n", tickets.FormatCost(b.Total))
	if b.Override > 0 {
		fmt.Fprintf(tw, "  overridden at\t%s\t%s counts toward the limits\n", tickets.FormatCost(b.Override), tickets.FormatCost(b.Total-b.Override))
	}
	fmt.Fprintf(tw, "  soft limit\t%s\n", budgetLimit(b.SoftLimit, b.Total-b.Override, "pauses new starts"))
	fmt.Fprintf(tw, "  hard limit\t%s\n", budgetLimit(b.HardLimit, b.Total-b.Override, "stops live agents"))
	if len(b.Projects) > 0 {
		fmt.Fprintln(tw, "\n  by project")
		tracked := 0.0
		for _, project := range slices.Sorted(maps.Keys(b.Projects)) {
			fmt.Fprintf(tw, "    %s\t%s\n", project, tickets.FormatCost(b.Projects[project]))
			tracked += b.Projects[project]
		}
		if untracked := b.Total - tracked; untracked >= 0.005 {
			fmt.Fprintf(tw, "    (untracked)\t%s\n", tickets.FormatCost(untracked))
		}
	}
	return tw.Flush()
}

// budgetLimit renders one limit and what is left of it; counted is the spend
// since the override, the only spend a limit counts.
func budgetLimit(limit, counted float64, does string) string {
	if limit <= 0 {
		return "off"
	}
	left := max(limit-counted, 0)
	return fmt.Sprintf("%s\t%s left\t(%s)", tickets.FormatCost(limit), tickets.FormatCost(left), does)
}

// BudgetLine is the one-line budget summary `gx server status` prints.
func BudgetLine(b wire.BudgetStatus) string {
	line := fmt.Sprintf("%s spent today, %s", tickets.FormatCost(b.Total), budgetState(b))
	if b.Override > 0 {
		line += fmt.Sprintf(", overridden at %s", tickets.FormatCost(b.Override))
	}
	return line
}

// budgetState is the budget's effect on the queue right now.
func budgetState(b wire.BudgetStatus) string {
	switch {
	case b.HardLatched:
		return "STOPPED: hard limit reached (gx budget override)"
	case b.BudgetPaused:
		return "PAUSED: soft limit reached (gx budget override)"
	}
	return "ok"
}

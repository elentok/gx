package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/tickets"
)

func newTicketsVerifyPlanCmd() *cobra.Command {
	var planPath string
	var table bool
	c := &cobra.Command{
		Use:   "verify-plan <project:epic> --plan <plan.yaml> | --table",
		Short: "pack a verify plan into verify tickets, or print the estimate table",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if table {
				tickets.WriteEstimateTable(c.OutOrStdout())
				return nil
			}
			if len(args) != 1 || planPath == "" {
				return fmt.Errorf("verify-plan needs <project:epic> and --plan <plan.yaml>, or --table")
			}
			return runVerifyPlan(planPath, c.OutOrStdout())
		},
	}
	c.Flags().StringVar(&planPath, "plan", "", "the verify plan (YAML)")
	c.Flags().BoolVar(&table, "table", false, "print the estimate table and exit")
	return c
}

func runVerifyPlan(planPath string, w io.Writer) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	plan, err := tickets.ParseVerifyPlan(data)
	if err != nil {
		return err
	}
	for _, e := range tickets.PackVerifyPlan(plan) {
		fmt.Fprintf(w, "%s: window=%dK", e.Slug, e.ExpectedContextWindow/1000)
		if len(e.BlockedBy) > 0 {
			fmt.Fprintf(w, " blocked_by=%s", strings.Join(e.BlockedBy, ","))
		}
		if len(e.Stories) > 0 {
			fmt.Fprintf(w, " stories=%v", e.Stories)
		}
		if e.Note != "" {
			fmt.Fprintf(w, " (%s)", e.Note)
		}
		fmt.Fprintln(w)
	}
	return nil
}

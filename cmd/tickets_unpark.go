package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/elentok/gx/repair"
	"github.com/spf13/cobra"
)

func newTicketsUnparkCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "unpark <epic> <id>",
		Short: "reopen a needs-answer ticket and retire its Needs Answer section into Comments",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := d.getwd()
			if err != nil {
				return err
			}
			return runTicketsUnpark(resolveEpicArg(args[0], cwd), args[1], jsonOut, time.Now(), c.OutOrStdout(), c.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

// runTicketsUnpark is a thin wrapper over repair.Unpark.
func runTicketsUnpark(epicPath, id string, jsonMode bool, now time.Time, stdout, stderr io.Writer) error {
	res, err := repair.Unpark(epicPath, id, now)
	return finishRecovery(stdout, stderr, jsonMode, res,
		fmt.Sprintf("%s: unparked", epicTicketLabel(epicPath, id)), err)
}

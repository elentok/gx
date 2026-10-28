package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/elentok/gx/cpuwatch"
	"github.com/spf13/cobra"
)

func newServerCPUSpikeCmd(d deps) *cobra.Command {
	var jsonOut bool
	var spike cpuwatch.Spike
	c := &cobra.Command{
		Use:   "cpu-spike",
		Short: "Report a CPU spike by hand, as the server and TUIs do when they stay busy",
		Long: "Report a CPU spike by hand, as the server and TUIs do when they stay busy.\n\n" +
			"In evolve maintainer mode the server files a ticket in the gx project for an\n" +
			"agent to find the cause, subject to cpu-watchdog's cooldown.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := cl.CPUSpike(ctx, spike)
			if err != nil {
				return fmt.Errorf("server write failed: %w", err)
			}
			if jsonOut {
				return encodeProvenance(c.OutOrStdout(), res, viaServer, callerActor(os.Getwd))
			}
			if res.Address != "" {
				_, err = fmt.Fprintf(c.OutOrStdout(), "filed %s\n", res.Address)
			} else {
				_, err = fmt.Fprintf(c.OutOrStdout(), "no ticket: %s\n", res.Message)
			}
			return err
		},
	}
	c.Flags().StringVar(&spike.Source, "source", "manual", "the process that spiked")
	c.Flags().IntVar(&spike.PID, "pid", 0, "the process's pid")
	c.Flags().StringVar(&spike.Dir, "dir", "", "the evidence dir (cpu.pprof, goroutines.txt, summary.md)")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return c
}

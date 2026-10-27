package cmd

import (
	"context"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// runAgentsFix never starts the server: a dead socket is a refusal.
func runAgentsFix(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, address string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.FixAgent(ctx, server.FixAgentRequest{Address: address})
	return printSessionResult(w, jsonOut, res.SessionResult, err)
}

func newAgentsFixCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "fix <project:epic/NN>",
		Short: "start an agent that investigates a ticket paused for repair",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsFix(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

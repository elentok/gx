package cmd

import (
	"context"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newDesignsCmd is `gx designs`: the Designs, the sessions that run gx-design
// on a map epic.
func newDesignsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "designs", Short: "map epics' design sessions owned by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newDesignsOpenCmd())
	return cmd
}

// runDesignsOpen never starts the server: a dead socket is a refusal.
func runDesignsOpen(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.DesignOpenRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.OpenDesign(ctx, req)
	return printSessionResult(w, jsonOut, res, err)
}

func newDesignsOpenCmd() *cobra.Command {
	var jsonOut bool
	var req server.DesignOpenRequest
	cmd := &cobra.Command{
		Use:   "open <project:epic[/NN]> [--new]",
		Short: "print the Design's live session, starting one if there is none",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			req.Epic = args[0]
			return runDesignsOpen(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&req.New, "new", false, "stop the live session and start a fresh one")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

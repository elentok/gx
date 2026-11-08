package serverapi

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
)

func newServerCleanupAgentCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var project string
	c := &cobra.Command{
		Use:   "cleanup-agent",
		Short: "Start the cleanup agent on the branches and epics no button can decide",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := cl.Reads.StartCleanupAgent(ctx, wire.CleanupWriteRequest{Project: project})
			switch {
			case apiclient.IsNotRunning(err):
				res = wire.OneOffResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
			case err != nil:
				return fmt.Errorf("server write failed: %w", err)
			}
			w := c.OutOrStdout()
			if jsonOut {
				if err := cmdbase.EncodeProvenance(w, res, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd)); err != nil {
					return err
				}
			} else if !res.Refused {
				if _, err := fmt.Fprintf(w, "started %s\n", res.Address); err != nil {
					return err
				}
			}
			if res.Refused {
				return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	c.Flags().StringVar(&project, "project", "", "the project")
	_ = c.MarkFlagRequired("project")
	return c
}

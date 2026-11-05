package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// runProjectRetryMigration asks the server to run a project's pending migration
// steps again. A refusal is an error carrying the server's message.
func runProjectRetryMigration(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, name string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.Sessions.RetryMigration(ctx, wire.RetryMigrationRequest{Project: name})
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, res, viaServer, callerActor(os.Getwd))
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintf(w, "migration of project %s started\n", name)
	return err
}

func newProjectRetryMigrationCmd(d deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "retry-migration <name>",
		Short: "run a project's pending migration steps again in the server",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			return runProjectRetryMigration(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

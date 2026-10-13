package cmd

import (
	"context"
	"fmt"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/tickets"
	"github.com/spf13/cobra"
)

// newServerEpicsCmd holds the verbs that take an epic address; every
// `gx server tickets` verb takes a ticket address.
func newServerEpicsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "epics", Short: "act on a whole epic through the gx server"}
	var jsonOut bool
	merge := &cobra.Command{
		Use:   "merge <project:epic>",
		Short: "merge a complete epic now: land it directly, or fork a merge ticket when it is not a fast-forward",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, ok := tickets.ParseEpicAddress(args[0]); !ok {
				return fmt.Errorf("%s is not an epic address (want project:epic)", args[0])
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.EpicMerge(ctx, args[0])
			})
		},
	}
	merge.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	cmd.AddCommand(merge)
	return cmd
}

package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// newTicketsAskCmd is `gx tickets ask`: an agent's question, sent to the server
// as a "## Question" + "## Handoff" body. It never starts the server.
func newTicketsAskCmd(d deps) *cobra.Command {
	var body string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "ask <project:epic/NN>",
		Short: "record a question on a ticket and pause it as asking",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			text, err := readBodyArg(d, body)
			if err != nil {
				return err
			}
			return serverQueueWrite(d.dial(), c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				return cl.TicketAsk(ctx, args[0], text)
			})
		},
	}
	cmd.Flags().StringVar(&body, "body", "-", "## Question and ## Handoff sections, or - to read them from stdin")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

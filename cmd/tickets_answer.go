package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// newTicketsAnswerCmd is `gx tickets answer`: a person's reply to a paused
// agent's question. The server mirror also takes the state token of the
// question being answered. It never starts the server.
func newTicketsAnswerCmd(d deps, withState bool) *cobra.Command {
	var req server.QueueRequest
	var body string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "answer <project:epic/NN>",
		Short: "answer the question a paused ticket's live agent is waiting on",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req.Address, req.Source = args[0], "cli"
			if body != "" {
				text, err := readBodyArg(d, body)
				if err != nil {
					return err
				}
				req.Text = text
			}
			return serverQueueWrite(c, jsonOut, func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
				res, err := cl.TicketAnswer(ctx, req)
				if err == nil && res.Refused && res.Reason == server.ReasonStale && !jsonOut {
					res.Message = staleMessage(res)
				}
				return res, err
			})
		},
	}
	cmd.Flags().StringVar(&req.Choice, "choice", "", "an option id, or yes or no")
	cmd.Flags().StringVar(&req.Note, "note", "", "a note sent with the choice")
	cmd.Flags().StringVar(&body, "body", "", "the free-text answer, or - to read it from stdin")
	if withState {
		cmd.Flags().StringVar(&req.State, "state", "", "the state token of the question being answered")
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// staleMessage adds the current question to a stale refusal, so the caller can
// answer it.
func staleMessage(res server.QueueResult) string {
	if res.Question == nil {
		return res.Message
	}
	return fmt.Sprintf("%s\nquestion %d: %s\nstate: %s", res.Message, res.Question.Seq, res.Question.Summary, res.State)
}

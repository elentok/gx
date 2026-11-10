package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"

	"github.com/spf13/cobra"
)

func newAgentsAnswerCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "answer <session>",
		Short: "answer a session's Question card; the request body (source, card_id, answers, note, images) is JSON on stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var req wire.SessionAnswerRequest
			if err := json.NewDecoder(c.InOrStdin()).Decode(&req); err != nil {
				return fmt.Errorf("reading the answer from stdin: %w", err)
			}
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			req.ID = args[0]
			return runAgentsAnswer(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runAgentsAnswer(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req wire.SessionAnswerRequest) error {
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (wire.SessionResult, error) {
		return cl.Sessions.Answer(ctx, req)
	})
}

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func newAgentsKeysCmd(d deps) *cobra.Command {
	var jsonOut bool
	req := server.SessionKeysRequest{}
	cmd := &cobra.Command{
		Use:   "keys <session> (--key <name> | --text <text>) [--only-blocked]",
		Short: "send a named key (Esc, Enter, Tab, Up, Down, Left, Right, Ctrl+C) or literal text to a live session",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if (req.Key == "") == (req.Text == "") {
				return fmt.Errorf("give exactly one of --key or --text")
			}
			cl, err := d.dial()()
			if err != nil {
				return err
			}
			req.ID = args[0]
			return runAgentsKeys(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&req.Key, "key", "", "a named key: Esc, Enter, Tab, Shift+Tab, Backspace, Up, Down, Left, Right or Ctrl+C")
	cmd.Flags().StringVar(&req.Text, "text", "", "literal text, typed without Enter")
	cmd.Flags().BoolVar(&req.OnlyBlocked, "only-blocked", false, "send only while the session is Blocked, and wait for it to leave Blocked")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func runAgentsKeys(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.SessionKeysRequest) error {
	return runAgentsSessionCall(ctx, w, jsonOut, func(ctx context.Context) (server.SessionResult, error) {
		return cl.KeysSession(ctx, req)
	})
}

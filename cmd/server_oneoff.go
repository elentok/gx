package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

func newServerOneOffCmd() *cobra.Command {
	var req server.OneOffRequest
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   `one-off "<prompt>"`,
		Short: "create a top-level ticket from a prompt and queue it",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			req.Prompt = args[0]
			if req.Cwd, err = os.Getwd(); err != nil {
				return err
			}
			ctx := c.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runServerOneOff(ctx, cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "project to create the ticket in (default: the project owning the current directory, else scratch)")
	cmd.Flags().StringVar(&req.Name, "name", "", "epic name (default: the prompt's first words plus a short unique suffix)")
	cmd.Flags().StringVar(&req.Agent, "agent", "", `agent to run the ticket under: "claude" or "codex" (default claude)`)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of the address")
	return cmd
}

// runServerOneOff never starts the server: a dead socket is a refusal with a
// `gx server start` hint.
func runServerOneOff(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.OneOffRequest) error {
	res, err := cl.OneOff(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.OneOffResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, res.Address)
	return err
}

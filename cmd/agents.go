package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newAgentsCmd is `gx agents`: the server's interactive sessions.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agents", Short: "interactive agent sessions owned by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newAgentsCreateCmd(), newAgentsListCmd())
	return cmd
}

func newAgentsCreateCmd() *cobra.Command {
	var jsonOut bool
	var req server.CreateSessionRequest
	cmd := &cobra.Command{
		Use:   "create --project <name> [--dir <dir>]",
		Short: "start an interactive session (prints its id)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if req.Dir == "" {
				req.Dir = "."
			}
			dir, err := filepath.Abs(req.Dir)
			if err != nil {
				return err
			}
			req.Dir = dir
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsCreate(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().StringVar(&req.Project, "project", "", "the project the session belongs to")
	cmd.Flags().StringVar(&req.Dir, "dir", "", "the dir the agent runs in (default the current dir)")
	cmd.Flags().StringVar(&req.Name, "name", "", "the session's name")
	cmd.Flags().StringVar(&req.Ticket, "ticket", "", "the Session ticket's address")
	cmd.Flags().StringVar(&req.Harness, "harness", "", "the harness to run (default claude)")
	cmd.Flags().IntVar(&req.Cols, "cols", 0, "the terminal's width (default the loop's)")
	cmd.Flags().IntVar(&req.Rows, "rows", 0, "the terminal's height (default the loop's)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

// runAgentsCreate never starts the server: a dead socket is a refusal.
func runAgentsCreate(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.CreateSessionRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.CreateSession(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.SessionResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
	case err != nil:
		return fmt.Errorf("server write failed: %w", err)
	}
	if jsonOut {
		stamped, err := stampProvenance(res, viaServer, callerActor(os.Getwd))
		if err != nil {
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stamped)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s): %s", res.Reason, res.Message)
	}
	_, err = fmt.Fprintln(w, res.Session.ID)
	return err
}

func newAgentsListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list interactive sessions",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runAgentsList(c.Context(), cl, c.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit structured JSON instead of human-readable text")
	return cmd
}

func runAgentsList(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	list, err := cl.Sessions(ctx)
	if err != nil {
		return fmt.Errorf("server read failed (is `gx server` running?): %w", err)
	}
	if jsonOut {
		return encodeProvenance(w, list, viaServer, callerActor(os.Getwd))
	}
	for _, s := range list {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Status.State, s.Project, s.Name, s.Dir); err != nil {
			return err
		}
	}
	return nil
}

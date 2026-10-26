package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/spf13/cobra"
)

// newDesignsCmd is `gx designs`: the Designs, the sessions that run gx-design
// on a map epic.
func newDesignsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "designs", Short: "map epics' design sessions owned by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newDesignsOpenCmd(), newDesignsDeleteCmd(), newDesignsWorktreeCmd())
	return cmd
}

// runDesignsWorktree never starts the server: a dead socket is a refusal. The
// caller's own gx session, when set, is left out of the rebase's live check.
func runDesignsWorktree(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req server.DesignWorktreeRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.DesignWorktree(ctx, req)
	switch {
	case apiclient.IsNotRunning(err):
		res = server.DesignWorktreeResult{Refused: true, Reason: server.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}
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
	if _, err := fmt.Fprintln(w, res.Path); err != nil {
		return err
	}
	if res.Note == "" {
		return nil
	}
	_, err = fmt.Fprintln(w, res.Note)
	return err
}

func newDesignsWorktreeCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "worktree <project:epic>",
		Short: "print the map's design worktree, made and rebased onto trunk if needed (the trunk checkout after Branch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			req := server.DesignWorktreeRequest{Epic: args[0], ExcludeSession: os.Getenv(nativerunner.AgentSessionEnv)}
			return runDesignsWorktree(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runDesignsDelete never starts the server: a dead socket is a refusal.
func runDesignsDelete(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, epic string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := cl.DeleteDesign(ctx, server.DesignDeleteRequest{Epic: epic})
	return printSessionResult(w, jsonOut, res, err)
}

func newDesignsDeleteCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "delete <project:epic>",
		Short: "delete an uncharted design: a map with no Destination, no tickets and no live session",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runDesignsDelete(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
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

// designOpenRequest builds the request for `gx designs open`. --idea and
// --project start a New Design, where the argument is the slug the map will
// take; they go together and exclude --new.
func designOpenRequest(arg, idea, project string, fresh bool) (server.DesignOpenRequest, error) {
	req := server.DesignOpenRequest{Epic: arg, New: fresh}
	if idea == "" && project == "" {
		return req, nil
	}
	switch {
	case idea == "":
		return req, fmt.Errorf("--project needs --idea")
	case project == "":
		return req, fmt.Errorf("--idea needs --project")
	case fresh:
		return req, fmt.Errorf("--new cannot be combined with --idea and --project")
	}
	req.NewDesign, req.Idea, req.Project = true, idea, project
	return req, nil
}

func newDesignsOpenCmd() *cobra.Command {
	var jsonOut, fresh bool
	var idea, project string
	cmd := &cobra.Command{
		Use:   "open <project:epic[/NN]> [--new] | open <slug> --idea <text> --project <project>",
		Short: "print the Design's live session, starting one if there is none",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req, err := designOpenRequest(args[0], idea, project, fresh)
			if err != nil {
				return err
			}
			cl, err := serverClient()
			if err != nil {
				return err
			}
			return runDesignsOpen(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().BoolVar(&fresh, "new", false, "stop the live session and start a fresh one")
	cmd.Flags().StringVar(&idea, "idea", "", "start a New Design for this idea (the argument is the map's slug)")
	cmd.Flags().StringVar(&project, "project", "", "project the New Design's map will live in")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}
